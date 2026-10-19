package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
)

const guardedTestLocal = "tfcheck0123456789abcdef01234567"

func TestMailboxTestGuardExactScopeAndSequence(t *testing.T) {
	calls := 0
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method == "POST" {
			w.WriteHeader(201)
		} else {
			w.WriteHeader(200)
		}
	}, "owner")
	c.http.Transport = newMailboxTestTransport(c.baseURL, c.http.Transport, "example.com", guardedTestLocal)
	request := func(method, path string, body any, allowed bool) {
		t.Helper()
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal("cannot encode synthetic request")
		}
		resp, err := c.Request(context.Background(), method, path, strings.NewReader(string(b)))
		if resp != nil {
			_ = resp.Body.Close()
		}
		if (err == nil) != allowed {
			t.Fatal("isolated mailbox guard accepted the wrong scope or sequence")
		}
	}
	collection := "/domains/example.com/email-accounts"
	item := collection + "/" + guardedTestLocal
	p := testutil.Secret(t)
	create := map[string]any{"username": guardedTestLocal, "password": p, "quota": 16, "limit": 1}
	for _, method := range []string{"DELETE", "PUT", "PATCH"} {
		request(method, item, map[string]any{"quota": 32}, false)
	}
	request("POST", "/domains/other.example.net/email-accounts", create, false)
	request("POST", "/reseller/users", create, false)
	request("PATCH", "/domains/example.com/spam/settings", map[string]any{"high_score": 15}, false)
	request("POST", collection, map[string]any{"username": "existing", "password": p, "quota": 16, "limit": 1}, false)
	request("POST", collection, create, true)
	request("POST", collection, create, false)
	request("DELETE", item, nil, false)
	request("PATCH", collection+"/existing", map[string]any{"quota": 32}, false)
	request("PATCH", item, map[string]any{"quota": 32, "limit": 2}, false)
	request("PATCH", item, map[string]any{"limit": 2}, false)
	request("PATCH", item, map[string]any{"quota": 64}, false)
	request("PATCH", item, map[string]any{"quota": 32}, true)
	request("PATCH", item, map[string]any{"limit": 2}, true)
	request("PATCH", item, map[string]any{"password": p}, true)
	request("PATCH", item, map[string]any{"password": p}, false)
	request("DELETE", item, nil, false)
	if calls != 4 {
		t.Fatal("blocked mutation reached the service")
	}
}

func TestMailboxTestGuardNeverReplaysFailedWrites(t *testing.T) {
	for _, status := range []int{400, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
			}, "owner")
			c.http.Transport = newMailboxTestTransport(c.baseURL, c.http.Transport, "example.com", guardedTestLocal)
			payload, _ := json.Marshal(map[string]any{"username": guardedTestLocal, "password": testutil.Secret(t), "quota": 16, "limit": 1})
			for i := 0; i < 2; i++ {
				resp, err := c.Request(context.Background(), "POST", "/domains/example.com/email-accounts", strings.NewReader(string(payload)))
				if resp != nil {
					_ = resp.Body.Close()
				}
				if (err != nil) != (i == 1) {
					t.Fatal("failed mutation was replayed")
				}
			}
			if calls != 1 {
				t.Fatal("failed mutation reached service more than once")
			}
		})
	}
}
