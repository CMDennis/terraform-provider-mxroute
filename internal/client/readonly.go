package client

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// NewReadOnly is reserved for explicitly authorized ordinary-account discovery.
// The transport rejects writes and routes outside the documented read surface,
// even if a future caller accidentally invokes a mutation method.
func NewReadOnly(server, username, apiKey, version string) (*Client, error) {
	c, err := New(server, username, apiKey, version)
	if err != nil {
		return nil, err
	}
	if c.account.Impersonated {
		return nil, errors.New("live discovery requires a plain ordinary account")
	}
	c.http.Transport = &readOnlyTransport{origin: c.baseURL, next: c.http.Transport}
	return c, nil
}

type readOnlyTransport struct {
	origin string
	next   http.RoundTripper
}

func (t *readOnlyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet || r.Body != nil || r.URL.Scheme+"://"+r.URL.Host != t.origin ||
		r.URL.User != nil || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Fragment != "" || !ordinaryReadPath(r.URL.EscapedPath()) {
		return nil, errors.New("operation blocked by read-only discovery policy")
	}
	return t.next.RoundTrip(r)
}

func ordinaryReadPath(path string) bool {
	switch path {
	case "/domains", "/verification-key", "/quota", "/quota/email":
		return true
	}
	p := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(p) < 2 || p[0] != "domains" {
		return false
	}
	for _, segment := range p[1:] {
		s, err := url.PathUnescape(segment)
		if err != nil || s == "" || s == "." || s == ".." || strings.ContainsAny(s, "/\\\r\n") {
			return false
		}
	}
	if len(p) == 2 {
		return true
	}
	if len(p) == 3 {
		switch p[2] {
		case "pointers", "email-accounts", "forwarders", "dns", "catch-all":
			return true
		}
	}
	if len(p) == 4 {
		return p[2] == "email-accounts" || (p[2] == "spam" && (p[3] == "settings" || p[3] == "whitelist" || p[3] == "blacklist"))
	}
	return false
}
