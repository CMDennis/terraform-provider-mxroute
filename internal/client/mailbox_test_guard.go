package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"sync"

	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
)

var probeMailboxName = regexp.MustCompile(`^tfcheck[a-f0-9]{24}$`)

// NewMailboxTest permits ordinary reads plus exactly one create and three
// single-field updates of a single disposable mailbox. No deletion, routing,
// domain, shared-setting or reseller write can pass this transport. The caller
// must establish absence, save provenance and obtain human authorization first.
func NewMailboxTest(server, username, apiKey, version, domain, local string) (*Client, error) {
	c, err := New(server, username, apiKey, version)
	if err != nil {
		return nil, err
	}
	canonical, err := identity.Domain(domain)
	if err != nil || canonical != domain || !probeMailboxName.MatchString(local) || c.account.Impersonated {
		return nil, errors.New("invalid isolated mailbox test scope")
	}
	c.http.Transport = newMailboxTestTransport(c.baseURL, c.http.Transport, domain, local)
	return c, nil
}

type mailboxTestTransport struct {
	mu               sync.Mutex
	reads            readOnlyTransport
	collection, item string
	local            string
	step             int
	stopped          bool
}

func newMailboxTestTransport(origin string, next http.RoundTripper, domain, local string) *mailboxTestTransport {
	return &mailboxTestTransport{reads: readOnlyTransport{origin: origin, next: next},
		collection: identity.Path("domains", domain, "email-accounts"),
		item:       identity.Path("domains", domain, "email-accounts", local), local: local}
}

func (t *mailboxTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodGet {
		return t.reads.RoundTrip(r)
	}
	blocked := errors.New("operation blocked by isolated mailbox test policy")
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped || r.URL.Scheme+"://"+r.URL.Host != t.reads.origin || r.URL.User != nil || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Fragment != "" || r.Body == nil {
		return nil, blocked
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 2049))
	_ = r.Body.Close()
	if err != nil || len(data) > 2048 {
		return nil, blocked
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return nil, blocked
	}
	expectedStatus := http.StatusOK
	switch t.step {
	case 0:
		var input CreateEmailAccountRequest
		if r.Method != http.MethodPost || r.URL.EscapedPath() != t.collection || len(fields) != 4 || json.Unmarshal(data, &input) != nil ||
			input.Username != t.local || !validPassword(input.Password, 72) || input.Quota == nil || *input.Quota != 16 || input.Limit == nil || *input.Limit != 1 {
			return nil, blocked
		}
		expectedStatus = http.StatusCreated
	case 1, 2, 3:
		var input UpdateEmailAccountRequest
		if r.Method != http.MethodPatch || r.URL.EscapedPath() != t.item || len(fields) != 1 || json.Unmarshal(data, &input) != nil {
			return nil, blocked
		}
		valid := (t.step == 1 && input.Quota != nil && *input.Quota == 32) ||
			(t.step == 2 && input.Limit != nil && *input.Limit == 2) ||
			(t.step == 3 && input.Password != nil && validPassword(*input.Password, 72))
		if !valid {
			return nil, blocked
		}
	default:
		return nil, blocked
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	// Never replay even a rejected/ambiguous request. A failed write permanently
	// disables this run's mutation capability; reads remain available to inspect.
	t.stopped = true
	resp, err := t.reads.next.RoundTrip(r)
	if err == nil && resp.StatusCode == expectedStatus {
		t.step++
		t.stopped = false
	}
	return resp, err
}
