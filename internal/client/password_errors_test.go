package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
)

func TestMailboxPasswordRejectionsAreSafe(t *testing.T) {
	for _, tc := range []struct{ message, field, want string }{
		{"unsupported characters", "password", "password characters"},
		{"password strength requirements", "password", "password strength"},
		{"password too weak", "password", "password strength"},
		{"opaque rejection", "password", "character and strength requirements"},
		{"unsupported characters", "unknown", "documented API constraints"},
	} {
		t.Run(tc.message+tc.field, func(t *testing.T) {
			password := testutil.Secret(t)
			c := newMock(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]string{"code": "VALIDATION_ERROR", "field": tc.field, "message": tc.message + " " + password}})
			}, "owner")
			for _, err := range []error{c.UpdateEmailAccount(context.Background(), "example.com", "support", UpdateEmailAccountRequest{Password: &password}), c.CreateEmailAccount(context.Background(), "example.com", CreateEmailAccountRequest{Username: "support", Password: password})} {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatal("password rejection category not surfaced")
				}
				for _, rendered := range []string{fmt.Sprint(err), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)} {
					if strings.Contains(rendered, password) || strings.Contains(rendered, tc.message+" ") {
						t.Fatal("server text leaked")
					}
				}
			}
		})
	}
}
