package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSpamVerifiedSave(t *testing.T) {
	for _, tc := range []struct {
		name           string
		status         int
		apply, badRead bool
		writes         int
		ok             bool
	}{
		{"success", 200, true, false, 1, true},
		{"unconfirmed applied", 500, true, false, 1, true},
		{"busy then success", 503, false, false, 2, true},
		{"failure no replay", 500, false, false, 1, false},
		{"acknowledged but unchanged", 200, false, false, 1, false},
		{"readback failed", 200, true, true, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			score, writes := int64(15), 0
			c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "GET" {
					if tc.badRead && writes > 0 {
						_, _ = w.Write([]byte(`{"success":true}`))
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"high_score": score}})
					return
				}
				var body map[string]int64
				if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["high_score"] != 20 {
					t.Error("spam write contains unintended fields")
				}
				writes++
				if tc.apply || writes > 1 {
					score = 20
				}
				if writes == 1 {
					w.WriteHeader(tc.status)
				}
			}, "owner")
			c.retryBase = time.Millisecond
			err := c.ApplySpamScore(context.Background(), "example.com", 20)
			if (err == nil) != tc.ok || writes != tc.writes {
				t.Fatalf("unexpected reconciliation: success=%v writes=%d", err == nil, writes)
			}
		})
	}
}
func TestSpamBusyBoundedAndCancelled(t *testing.T) {
	writes := 0
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"high_score":15}}`))
			return
		}
		writes++
		w.WriteHeader(503)
	}, "owner")
	c.retryBase = time.Millisecond
	if err := c.ApplySpamScore(context.Background(), "example.com", 20); err == nil || writes != 3 {
		t.Fatal("busy saves were not bounded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.ApplySpamScore(ctx, "example.com", 20); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
}
func TestSpamCompleteListsAndUnavailableSettings(t *testing.T) {
	for _, body := range []string{`{"success":true}`, `{"success":true,"data":null}`, `{"success":true,"data":["x@example.net","X@example.net"]}`, `{"success":true,"data":[""]}`} {
		c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}, "owner")
		if _, err := c.FindSpamEntry(context.Background(), "example.com", "x@example.net", false); err == nil || IsNotFound(err) {
			t.Fatal("invalid list established absence")
		}
	}
	for _, status := range []int{403, 404, 500} {
		c := newMock(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }, "owner")
		c.retryBase = time.Millisecond
		if _, err := c.FindSpamEntry(context.Background(), "example.com", "x@example.net", false); err == nil || IsNotFound(err) {
			t.Fatal("unavailable domain established absence")
		}
	}
	for _, body := range []string{`{"success":true,"data":{}}`, `{"success":true,"data":{"high_score":null}}`, `{"success":true,"data":{"high_score":0}}`, `{"success":true,"data":{"high_score":51}}`} {
		c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}, "owner")
		if err := c.ApplySpamScore(context.Background(), "example.com", 20); err == nil {
			t.Fatal("unavailable score silently defaulted")
		}
	}
}
func TestSpamConcurrentSharedEntries(t *testing.T) {
	var mu sync.Mutex
	lists := map[string][]string{"owner": {"unmanaged@example.net"}, "other": {}}
	mutations := 0
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		user := r.Header.Get("X-Username")
		if user == "reseller|owner" {
			user = "owner"
		}
		values := lists[user]
		if r.Method == "GET" {
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": values})
			return
		}
		var body struct {
			Entry string `json:"entry"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid entry body")
		}
		lists[user] = append(values, body.Entry)
		mutations++
		w.WriteHeader(201)
	}, "owner")
	alias, err := NewMock(c.server, "reseller|owner", c.apiKey, "test", c.baseURL)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewMock(c.server, "other", c.apiKey, "test", c.baseURL)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, x := range []struct {
		c             *Client
		domain, entry string
	}{{c, "example.com", "NEW@example.net"}, {alias, "example.org", "new@example.net"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- x.c.ChangeSpamEntry(context.Background(), x.domain, x.entry, false, true)
		}()
	}
	wg.Wait()
	close(errs)
	successes, conflicts := 0, 0
	for err := range errs {
		if err == nil {
			successes++
		} else {
			var e *Error
			if errors.As(err, &e) && e.Kind == Conflict {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
	}
	if successes != 1 || conflicts != 1 || mutations != 1 || len(lists["owner"]) != 2 {
		t.Fatal("duplicate account entry or unrelated entry lost")
	}

	// Different managed entries must also coexist under concurrent applies.
	distinctErrors := make(chan error, 2)
	for _, entry := range []string{"one@example.org", "two@example.org"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			distinctErrors <- alias.ChangeSpamEntry(context.Background(), "example.org", entry, false, true)
		}()
	}
	wg.Wait()
	close(distinctErrors)
	for err := range distinctErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(lists["owner"]) != 4 || mutations != 3 {
		t.Fatal("concurrent entries overwrote account state")
	}
	if err := other.ChangeSpamEntry(context.Background(), "example.com", "new@example.net", false, true); err != nil {
		t.Fatal("independent account was blocked")
	}
	if len(lists["other"]) != 1 {
		t.Fatal("effective account isolation failed")
	}
}
func TestSpamCanonicalDeleteAndNoBlindAdoption(t *testing.T) {
	deleted := false
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			values := []string{}
			if !deleted {
				values = []string{"*+Tag=Test@Example.NET"}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": values})
			return
		}
		if r.Method != "DELETE" || !strings.HasSuffix(r.URL.EscapedPath(), "/%2A+Tag=Test@Example.NET") {
			t.Error("delete lost canonical spelling or wildcard escaping")
		}
		deleted = true
		w.WriteHeader(204)
	}, "owner")
	if err := c.ChangeSpamEntry(context.Background(), "example.com", "*+tag=test@example.net", true, true); err == nil {
		t.Fatal("existing entry silently adopted")
	}
	if err := c.ChangeSpamEntry(context.Background(), "example.org", "*+tag=test@example.net", true, false); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("entry not deleted")
	}
	if err := c.ChangeSpamEntry(context.Background(), "example.com", "*+tag=test@example.net", true, false); err != nil {
		t.Fatal("confirmed absent delete should be idempotent")
	}
}

func TestSpamRequestBodyLimit(t *testing.T) {
	calls := 0
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(200) }, "owner")
	err := c.execute(context.Background(), operation{name: "spam limit", method: "PATCH", path: "/domains/example.com/spam/settings", status: 200, mode: sparse}, map[string]string{"high_score": strings.Repeat("0", 2048)}, nil)
	if err == nil || calls != 0 {
		t.Fatal("oversized spam body reached transport")
	}
}
