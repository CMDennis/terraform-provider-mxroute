package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestLargeInventoryUsesOneListAndNeverFansOut(t *testing.T) {
	const size = 10000
	domains := make([]string, size)
	accounts := make([]map[string]any, size)
	for i := range domains {
		domains[i] = fmt.Sprintf("d%d.example.com", i)
		user := fmt.Sprintf("user%d", i)
		accounts[i] = map[string]any{"username": user, "email": user + "@example.com", "quota": 0, "usage": 0, "limit": 9600, "sent": 0, "suspended": false}
	}
	requests := 0
	duplicate := false
	c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		var data any
		switch r.URL.Path {
		case "/domains":
			data = domains
		case "/domains/example.com/email-accounts":
			items := accounts
			if duplicate {
				items = append(items, accounts[len(accounts)-1])
			}
			data = items
		default:
			t.Error("large inventory triggered a per-item detail request")
			w.WriteHeader(500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
	}, "owner")
	if _, err := c.FindDomain(context.Background(), domains[size-1]); err != nil || requests != 1 {
		t.Fatal("large domain lookup exceeded one list request")
	}
	if _, err := c.FindEmailAccount(context.Background(), "example.com", "user9999"); err != nil || requests != 2 {
		t.Fatal("large mailbox lookup exceeded one list request")
	}
	duplicate = true
	_, err := c.FindEmailAccount(context.Background(), "example.com", "user9999")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Kind != Ambiguous || requests != 3 {
		t.Fatal("late duplicate in a large inventory was silently adopted")
	}
}
