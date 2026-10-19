package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
)

func TestContractScenarios(t *testing.T) {
	data, err := os.ReadFile("../../testdata/contracts/scenarios.json")
	if err != nil {
		t.Fatal("missing contract cases")
	}
	var fixtures struct {
		Cases []struct {
			ID        string `json:"id"`
			Operation string `json:"operation_key"`
			Path      string `json:"path"`
			Request   struct {
				JSON          map[string]any `json:"json"`
				RuntimeFields map[string]any `json:"runtime_fields"`
			} `json:"request"`
			Response struct {
				Status   int               `json:"status"`
				Headers  map[string]string `json:"headers"`
				Body     any               `json:"body"`
				Encoding string            `json:"body_encoding"`
			} `json:"response"`
		} `json:"cases"`
	}
	if json.Unmarshal(data, &fixtures) != nil {
		t.Fatal("invalid contract cases")
	}
	b := loadBaseline(t)
	for _, fixture := range fixtures.Cases {
		t.Run(fixture.ID, func(t *testing.T) {
			c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
				for k, v := range fixture.Response.Headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(fixture.Response.Status)
				switch fixture.Response.Encoding {
				case "json":
					_ = json.NewEncoder(w).Encode(fixture.Response.Body)
				case "raw":
					_, _ = io.WriteString(w, fixture.Response.Body.(string))
				}
			}, "owner")
			c.operationTimeout = 25 * time.Millisecond
			var op baselineOperation
			for _, o := range b.Operations {
				if o.Key == fixture.Operation {
					op = o
					break
				}
			}
			args, _, _ := operationArgs(op, testutil.Secret(t))
			_, template, _ := strings.Cut(op.Key, " ")
			parts := strings.Split(strings.Trim(template, "/"), "/")
			values := strings.Split(strings.Trim(fixture.Path, "/"), "/")
			index := 1
			for i, p := range parts {
				if strings.HasPrefix(p, "{") {
					v, _ := url.PathUnescape(values[i])
					args[index] = reflect.ValueOf(v)
					index++
				}
			}
			if fixture.Request.JSON != nil {
				fields := fixture.Request.JSON
				for k := range fixture.Request.RuntimeFields {
					fields[k] = testutil.Secret(t)
				}
				encoded, _ := json.Marshal(fields)
				typ := args[len(args)-1].Type()
				switch typ.Kind() {
				case reflect.Struct:
					value := reflect.New(typ)
					if json.Unmarshal(encoded, value.Interface()) != nil {
						t.Fatal("invalid synthetic input")
					}
					args[len(args)-1] = value.Elem()
				case reflect.Int64:
					args[len(args)-1] = reflect.ValueOf(int64(fields["high_score"].(float64)))
				}
			}
			result := reflect.ValueOf(c).MethodByName(op.ClientMethod).Call(args)
			var actualErr error
			if !result[len(result)-1].IsNil() {
				actualErr = result[len(result)-1].Interface().(error)
			}
			expectedError := fixture.Response.Status >= 400 || strings.HasPrefix(fixture.ID, "S028") || strings.HasPrefix(fixture.ID, "S029") || strings.HasPrefix(fixture.ID, "S030") || strings.HasPrefix(fixture.ID, "S031")
			if (actualErr != nil) != expectedError {
				t.Fatal("contract response classification mismatch")
			}
			if actualErr != nil && IsNotFound(actualErr) {
				t.Fatal("error response established unconfirmed absence")
			}
			if strings.HasPrefix(fixture.ID, "S007") || strings.HasPrefix(fixture.ID, "S008") {
				account, err := c.FindEmailAccount(context.Background(), "example.com", "team")
				if strings.HasPrefix(fixture.ID, "S008") {
					var e *Error
					if !errors.As(err, &e) || e.Kind != Ambiguous {
						t.Fatal("ambiguous lookup was accepted")
					}
				} else if err != nil || *account.Username != "Team" {
					t.Fatal("canonical spelling lost")
				}
			}
		})
	}
}

func TestMalformedReadsPreserveState(t *testing.T) {
	cases := []struct{ name, body, contentType string }{
		{"empty", "", "application/json"},
		{"broken", `{"success":true,"data":[`, "application/json"},
		{"null-data", `{"success":true,"data":null}`, "application/json"},
		{"missing-data", `{"success":true}`, "application/json"},
		{"missing-success", `{"data":[]}`, "application/json"},
		{"null-item", `{"success":true,"data":[null]}`, "application/json"},
		{"empty-identity", `{"success":true,"data":[""]}`, "application/json"},
		{"wrong-shape", `{"success":true,"data":{}}`, "application/json"},
		{"wrong-content-type", `{"success":true,"data":[]}`, "text/html"},
		{"missing-content-type", `{"success":true,"data":[]}`, ""},
		{"trailing-json", `{"success":true,"data":[]} {}`, "application/json"},
		{"false-malformed-error", `{"success":false,"error":"not an object"}`, "application/json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header()["Content-Type"] = []string{tc.contentType}
				_, _ = io.WriteString(w, tc.body)
			}, "owner")
			list, err := c.ListDomains(context.Background())
			if err == nil || list != nil || IsNotFound(err) {
				t.Fatal("malformed read could erase state")
			}
		})
	}
}
func TestNullablePresenceAndAdditiveSchema(t *testing.T) {
	body := `{"success":true,"data":{"type":"fail","address":null,"new_field":true}}`
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}, "owner")
	v, err := c.GetCatchAll(context.Background(), "example.com")
	if err != nil || v.Address != nil {
		t.Fatal("explicit null or additive fields rejected")
	}
	body = `{"success":true,"data":{"type":"fail"}}`
	if v, err := c.GetCatchAll(context.Background(), "example.com"); err == nil || v != nil {
		t.Fatal("omitted nullable field incorrectly became known null")
	}
	body = `{"success":true,"data":{"username":"team","email":"team@example.com","quota":0,"usage":0,"limit":0,"sent":0}}`
	if v, err := c.GetEmailAccount(context.Background(), "example.com", "team"); err == nil || v != nil {
		t.Fatal("missing false field became known false")
	}
}
func TestConfirmedAbsenceOnlyFromCompleteLists(t *testing.T) {
	body := `{"success":true,"data":[]}`
	status := 200
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}, "owner")
	if _, err := c.FindDomain(context.Background(), "example.com"); !IsNotFound(err) {
		t.Fatal("complete empty list did not prove absence")
	}
	status = 404
	body = `{"success":false,"error":{"code":"NOT_FOUND"}}`
	if _, err := c.FindDomain(context.Background(), "example.com"); err == nil || IsNotFound(err) {
		t.Fatal("HTTP 404 proved absence")
	}
	status = 200
	body = `{"success":true}`
	if _, err := c.FindDomain(context.Background(), "example.com"); err == nil || IsNotFound(err) {
		t.Fatal("incomplete list proved absence")
	}
}
func TestTypedErrorsAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		kind   ErrorKind
	}{
		{401, "UNAUTHORIZED", Authentication}, {403, "FORBIDDEN", Authorization}, {404, "NOT_FOUND", NotFound}, {409, "CONFLICT", Conflict}, {422, "BUSINESS_ERROR", Business}, {429, "RATE_LIMITED", RateLimited}, {500, "SERVER_ERROR", Upstream}, {503, "SERVER_ERROR", Upstream}, {200, "BUSINESS_ERROR", Business}, {302, "", Protocol},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			secret := testutil.Secret(t)
			c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"code": tc.code, "message": secret, "field": secret}})
			}, "owner")
			if tc.status == 429 {
				c.operationTimeout = 5 * time.Millisecond
			}
			_, err := c.ListDomains(context.Background())
			if tc.status == 429 {
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("throttle did not honor deadline")
				}
				return
			}
			var e *Error
			if !errors.As(err, &e) || e.Kind != tc.kind || e.Status != tc.status || IsNotFound(err) {
				t.Fatal("incorrect typed error")
			}
			for _, s := range []string{fmt.Sprint(err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)} {
				if strings.Contains(s, secret) || strings.Contains(s, c.apiKey) || strings.Contains(s, "example.com") {
					t.Fatal("diagnostic leaked server data")
				}
			}
		})
	}
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, "<html>upstream diagnostic</html>")
	}, "owner")
	_, err := c.ListDomains(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Kind != Authentication {
		t.Fatal("non-JSON error lost HTTP meaning")
	}
}
func TestSparseWritesInspectFailureEnvelopes(t *testing.T) {
	for _, body := range []string{`{"success":false}`, `{"success":false,"error":{"code":17}}`, `{"success":false,"error":"opaque"}`} {
		t.Run(body, func(t *testing.T) {
			c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					w.WriteHeader(403)
					return
				}
				_, _ = io.WriteString(w, body)
			}, "owner")
			err := c.SetMailHosting(context.Background(), "example.com", false)
			if err == nil {
				t.Fatal("success:false accepted as a sparse success")
			}
		})
	}
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"undocumented":"payload"}`) }, "owner")
	if err := c.SetMailHosting(context.Background(), "example.com", false); err != nil {
		t.Fatal("sparse undocumented content was trusted as a schema")
	}
}
func TestReadRetriesAndAmbiguousWrites(t *testing.T) {
	var requests atomic.Int32
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) < 3 {
			w.WriteHeader(503)
			return
		}
		_, _ = io.WriteString(w, `{"success":true,"data":[]}`)
	}, "owner")
	if _, err := c.ListDomains(context.Background()); err != nil || requests.Load() != 3 {
		t.Fatal("safe read retry failed")
	}
	// A partial create can be visible on readback, but existence does not prove
	// ownership or that every requested attribute/password was applied.
	var writes, reads atomic.Int32
	c = newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			writes.Add(1)
			w.WriteHeader(500)
			return
		}
		reads.Add(1)
		_, _ = io.WriteString(w, `{"success":true,"data":["example.com"]}`)
	}, "owner")
	_, err := c.CreateDomain(context.Background(), CreateDomainRequest{Domain: "example.com"})
	var e *Error
	if !errors.As(err, &e) || !e.Uncertain || !e.ReadbackAvailable || writes.Load() != 1 || reads.Load() != 1 {
		t.Fatal("ambiguous create was replayed or silently accepted")
	}
	// Password mutations and deletes are subject to the same no-replay rule.
	for _, method := range []string{"PATCH", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			var calls atomic.Int32
			c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == method {
					calls.Add(1)
					w.WriteHeader(503)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"success":true,"data":[]}`)
			}, "owner")
			var err error
			if method == "PATCH" {
				password := testutil.Secret(t)
				err = c.UpdateEmailAccount(context.Background(), "example.com", "team", UpdateEmailAccountRequest{Password: &password})
			} else {
				err = c.DeleteEmailAccount(context.Background(), "example.com", "team")
			}
			var e *Error
			if !errors.As(err, &e) || !e.Uncertain || calls.Load() != 1 {
				t.Fatal("unsafe mutation retry")
			}
		})
	}
}
func TestThrottledWriteRetryAndHeaders(t *testing.T) {
	var calls atomic.Int32
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(204)
	}, "owner")
	// Use cancellation to verify rejected writes are not blindly retried
	// before the server-directed pause elapses.
	c.operationTimeout = 5 * time.Millisecond
	if err := c.DeleteDomain(context.Background(), "example.com"); !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 {
		t.Fatal("throttled write ignored reset/deadline")
	}
	now := time.Now().Truncate(time.Second)
	for _, header := range []http.Header{
		{"Retry-After": []string{"2"}},
		{"Retry-After": []string{now.Add(2 * time.Second).UTC().Format(http.TimeFormat)}},
		{"X-Ratelimit-Reset": []string{fmt.Sprint(now.Add(2 * time.Second).Unix())}},
	} {
		if !retryTime(header, now).Equal(now.Add(2 * time.Second)) {
			t.Fatal("server reset header not respected")
		}
	}
	// An explicit zero Retry-After allows a jittered retry; it must not be
	// confused with a missing header and the conservative fallback pause.
	var retryCalls atomic.Int32
	c = newMock(t, func(w http.ResponseWriter, r *http.Request) {
		if retryCalls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		w.WriteHeader(204)
	}, "owner")
	if err := c.DeleteDomain(context.Background(), "example.com"); err != nil || retryCalls.Load() != 2 {
		t.Fatal("clearly rejected write was not safely retried")
	}
}
func TestCancellationAndTimeout(t *testing.T) {
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}, "owner")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.ListDomains(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
	c.operationTimeout = 5 * time.Millisecond
	if _, err := c.ListDomains(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("operation timeout lost")
	}
	err := c.UpdateEmailAccount(context.Background(), "example.com", "team", UpdateEmailAccountRequest{Quota: ptr(int64(0))})
	var e *Error
	if !errors.As(err, &e) || !e.Uncertain || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("timed out write lost uncertainty")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestTransportFailuresAndPartialBodies(t *testing.T) {
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected real transport") }, "owner")
	var calls int
	secret := testutil.Secret(t)
	c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) { calls++; return nil, errors.New(secret) })
	_, err := c.ListDomains(context.Background())
	if err == nil || calls != 3 || strings.Contains(err.Error(), secret) {
		t.Fatal("transport retry/redaction failed")
	}
	calls = 0
	err = c.DeleteDomain(context.Background(), "example.com")
	var e *Error
	if !errors.As(err, &e) || !e.Uncertain || calls != 4 {
		t.Fatal("write transport failure was replayed (one write plus three reconciliation reads expected)")
	}
	c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: &brokenBody{}}, nil
	})
	if _, err := c.ListDomains(context.Background()); err == nil || IsNotFound(err) {
		t.Fatal("truncated response accepted")
	}
}

type brokenBody struct{}

func (*brokenBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (*brokenBody) Close() error             { return nil }
func TestResponseSizeBound(t *testing.T) {
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.Repeat(" ", maxResponseBytes+1))
	}, "owner")
	if _, err := c.ListDomains(context.Background()); err == nil {
		t.Fatal("oversized response accepted")
	}
}
func TestImpersonationAndCreationValidation(t *testing.T) {
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid operation reached HTTP") }, "owner|ordinary")
	b := loadBaseline(t)
	for _, op := range b.Operations {
		if strings.Contains(op.Key, "/reseller/") {
			args, _, _ := operationArgs(op, testutil.Secret(t))
			result := reflect.ValueOf(c).MethodByName(op.ClientMethod).Call(args)
			if result[len(result)-1].IsNil() {
				t.Fatal("impersonated reseller operation accepted")
			}
		}
	}
	c.account.Impersonated = false
	if err := c.CreateResellerUser(context.Background(), CreateResellerUserRequest{Username: "MixedCaseLong", Password: testutil.Secret(t)}); err == nil {
		t.Fatal("creation used auth username rules")
	}
	if err := c.UpdateEmailAccount(context.Background(), "example.com", "team", UpdateEmailAccountRequest{Password: ptr("")}); err == nil {
		t.Fatal("explicit empty password silently preserved remote password")
	}
	if err := c.UpdateSpamSettings(context.Background(), "example.com", 0); err == nil {
		t.Fatal("invalid score accepted")
	}
	if err := c.SetCatchAll(context.Background(), "example.com", SetCatchAllRequest{Type: "address"}); err == nil {
		t.Fatal("missing forwarding address accepted")
	}
}
func TestRequestOmissions(t *testing.T) {
	for _, value := range []any{CreateDomainPointerRequest{Pointer: "alias.example.net"}, UpdateEmailAccountRequest{Quota: ptr(int64(0))}, CreateResellerPackageRequest{Name: "starter"}, UpdateResellerUserRequest{}, PackageLimits{}} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal("request encoding failed")
		}
		var fields map[string]any
		_ = json.Unmarshal(data, &fields)
		for _, key := range []string{"password", "limit", "alias", "domains", "email_accounts", "email_forwarders", "domain_pointers"} {
			if _, ok := fields[key]; ok {
				t.Fatal("omitted field serialized")
			}
		}
	}
	password := testutil.Secret(t)
	for _, value := range []any{CreateEmailAccountRequest{Password: password}, UpdateEmailAccountRequest{Password: &password}, CreateResellerUserRequest{Password: password}, UpdateResellerUserRequest{Password: &password}} {
		for _, s := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value)} {
			if strings.Contains(s, password) {
				t.Fatal("request formatting leaked password")
			}
		}
	}
}
func TestSharedCoordination(t *testing.T) {
	first, settings := coordination("mock", "mail.example.com", "owner", "ordinary")
	second, sameSettings := coordination("mock", "MAIL.EXAMPLE.COM", "OWNER", "ORDINARY")
	third, otherSettings := coordination("mock", "mail.example.com", "owner", "another")
	distinct, _ := coordination("mock", "other.example.com", "owner", "ordinary")
	if first != second || first != third || first == distinct || settings != sameSettings || settings == otherSettings {
		t.Fatal("coordination account boundaries incorrect")
	}
	if err := settings.acquire(context.Background()); err != nil {
		t.Fatal("cannot acquire settings lock")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := sameSettings.acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("settings lock ignored cancellation")
	}
	settings.release()
	// Each budget is independent; the exhausted read budget must not block writes.
	l := &accountLimits{}
	for range readBudget {
		if err := l.reserve(context.Background(), true); err != nil {
			t.Fatal("cannot reserve budget")
		}
	}
	if err := l.reserve(ctx, true); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("read budget not enforced")
	}
	if err := l.reserve(context.Background(), false); err != nil {
		t.Fatal("read limit blocked a write")
	}
	for range writeBudget - 1 {
		_ = l.reserve(context.Background(), false)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel2()
	if err := l.reserve(ctx2, false); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("write budget not enforced")
	}
	// Old reservations expire; server-directed blocks override available budget.
	l.mu.Lock()
	l.reads.sent = []time.Time{time.Now().Add(-time.Minute - time.Second)}
	l.mu.Unlock()
	if err := l.reserve(context.Background(), true); err != nil {
		t.Fatal("rolling budget failed to replenish")
	}
	l.block(true, time.Now().Add(time.Minute))
	if err := l.reserve(ctx2, true); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("server block not coordinated")
	}
	for attempt := range 10 {
		d := backoff(time.Second, attempt)
		if d < 0 || d > 5*time.Second {
			t.Fatal("unbounded retry jitter")
		}
	}
}
func TestSharedSettingsMutationsSerialize(t *testing.T) {
	var active, peak atomic.Int32
	handler := func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		for {
			p := peak.Load()
			if p >= n || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		active.Add(-1)
		w.WriteHeader(200)
	}
	c := newMock(t, handler, "owner|ordinary")
	alias, err := NewMock("mail.example.com", "ordinary", testutil.Secret(t), "test", c.baseURL)
	if err != nil {
		t.Fatal("alias initialization failed")
	}
	var group sync.WaitGroup
	failures := make(chan error, 8)
	for i := range 8 {
		group.Go(func() {
			selected := c
			if i%2 == 0 {
				selected = alias
			}
			failures <- selected.UpdateSpamSettings(context.Background(), "example.com", 5)
		})
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal("serialized mutation failed")
		}
	}
	if peak.Load() != 1 {
		t.Fatal("account-shared settings writes overlapped")
	}
}

func TestServerThrottleBlocksAliasAndKeepsBudgetsSeparate(t *testing.T) {
	var calls atomic.Int32
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(time.Minute).Unix()))
			_, _ = io.WriteString(w, `{"success":true,"data":[]}`)
			return
		}
		w.WriteHeader(204)
	}, "owner")
	alias, err := NewMock("mail.example.com", "owner|ordinary", testutil.Secret(t), "test", c.baseURL)
	if err != nil {
		t.Fatal("alias initialization failed")
	}
	if _, err := c.ListDomains(context.Background()); err != nil {
		t.Fatal("successful limited read rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if _, err := alias.ListDomains(ctx); !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 {
		t.Fatal("server block did not reach impersonated alias")
	}
	if err := alias.DeleteDomain(context.Background(), "example.com"); err != nil || calls.Load() != 2 {
		t.Fatal("read throttle blocked independent write budget")
	}
}
func TestEscapedSegmentsAndRejectedPasswordReplay(t *testing.T) {
	var got string
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) { got = r.RequestURI; w.WriteHeader(204) }, "owner")
	if err := c.DeleteEmailAccount(context.Background(), "example.com", "ops/tag%?#@example.com"); err != nil || got != "/domains/example.com/email-accounts/ops%2Ftag%25%3F%23@example.com" {
		t.Fatal("mailbox segment was not escaped exactly once")
	}
	if err := c.DeleteSpamWhitelistEntry(context.Background(), "example.com", "*@trusted.example"); err != nil || got != "/domains/example.com/spam/whitelist/%2A@trusted.example" {
		t.Fatal("wildcard address path encoding changed")
	}
	password := testutil.Secret(t)
	var calls int
	var first []byte
	c = newMock(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if calls == 1 {
			first = body
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		if string(body) != string(first) || !strings.Contains(string(body), password) {
			t.Error("rejected request body changed before retry")
		}
		w.WriteHeader(201)
	}, "owner")
	if err := c.CreateEmailAccount(context.Background(), "example.com", CreateEmailAccountRequest{Username: "team", Password: password}); err != nil || calls != 2 {
		t.Fatal("explicit rejection was not retried safely")
	}
}
func TestWriteCancellationBeforeDispatchHasNoUncertainOutcome(t *testing.T) {
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) { t.Error("canceled operation reached HTTP") }, "owner")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := c.DeleteDomain(ctx, "example.com")
	var e *Error
	if !errors.Is(err, context.Canceled) || (errors.As(err, &e) && e.Uncertain) {
		t.Fatal("pre-dispatch cancellation reported an uncertain write")
	}
}
func TestInventoryHasAClientAndNamedRequestResponseTestForEveryOperation(t *testing.T) {
	b := loadBaseline(t)
	source, err := os.ReadFile("operations_test.go")
	if err != nil {
		t.Fatal("operation tests unavailable")
	}
	if len(b.Operations) != 43 {
		t.Fatal("baseline changed; review client coverage")
	}
	for _, op := range b.Operations {
		if _, ok := reflect.TypeFor[*Client]().MethodByName(op.ClientMethod); !ok {
			t.Fatal("missing client method for baseline operation")
		}
		if !strings.Contains(string(source), "func TestClient_"+op.ClientMethod+"(") {
			t.Fatal("missing named request/response test for baseline operation")
		}
	}
}

type canceledBody struct{ cancel context.CancelFunc }

func (b *canceledBody) Read([]byte) (int, error) { b.cancel(); return 0, context.Canceled }
func (*canceledBody) Close() error               { return nil }
func TestCancellationDuringResponseBody(t *testing.T) {
	for _, write := range []bool{false, true} {
		c := newMock(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected transport") }, "owner")
		ctx, cancel := context.WithCancel(context.Background())
		c.http.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
			status := 200
			if write {
				status = 201
			}
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: &canceledBody{cancel: cancel}}, nil
		})
		var err error
		if write {
			_, err = c.CreateDomain(ctx, CreateDomainRequest{Domain: "example.com"})
		} else {
			_, err = c.ListDomains(ctx)
		}
		cancel()
		var e *Error
		if !errors.Is(err, context.Canceled) || !errors.As(err, &e) || e.Uncertain != write {
			t.Fatal("streaming cancellation lost context or write uncertainty")
		}
	}
}

func TestPasswordLengthsCountCharacters(t *testing.T) {
	var calls int
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(201) }, "owner")
	for _, length := range []int{4, 7, 8, 72, 73} {
		// Generate multibyte inputs in memory; never save a dummy password literal.
		password := strings.Repeat(string(rune(0x10000+int(testutil.Secret(t)[0]))), length)
		before := calls
		err := c.CreateEmailAccount(context.Background(), "example.com", CreateEmailAccountRequest{Username: "team", Password: password})
		valid := length >= 8 && length <= 72
		if (err == nil) != valid || (calls > before) != valid {
			t.Fatal("mailbox password bounds counted bytes instead of characters")
		}
		if validPassword(password, 0) != (length >= 8) {
			t.Fatal("reseller password minimum counted bytes")
		}
	}
	password := string([]byte{0xff}) + testutil.Secret(t)
	if validPassword(password, 72) {
		t.Fatal("invalid UTF-8 password was accepted and would change during JSON encoding")
	}
}
