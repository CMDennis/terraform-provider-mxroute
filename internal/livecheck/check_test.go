package livecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
)

func TestDiscoveryBaselineAndFailureIsolation(t *testing.T) {
	for _, scenario := range []string{"stable", "changed", "malformed", "budget", "duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			pass, requests := 0, 0
			key := testutil.Secret(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != "GET" || r.Header.Get("X-API-Key") != key || strings.HasPrefix(r.URL.Path, "/reseller/") {
					t.Error("unapproved discovery operation or credential")
					w.WriteHeader(403)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				data := `[]`
				switch r.URL.Path {
				case "/domains":
					pass++
					data = `["example.com"]`
					if scenario == "budget" {
						domains := make([]string, 21)
						for i := range domains {
							domains[i] = fmt.Sprintf("d%d.example.com", i)
						}
						b, _ := json.Marshal(domains)
						data = string(b)
					} else if scenario == "duplicate" {
						data = `["example.com","example.com"]`
					}
				case "/verification-key":
					data = `{"key":"synthetic-verification","record":{"type":"TXT","name":"verify.example.com","value":"domain-verified"}}`
				case "/quota":
					_, _ = fmt.Fprintf(w, `{"username":"owner","total_used":%d,"total_limit":0,"percent_used":0,"breakdown":{"email":1,"web":0,"databases":0,"backups":0,"other":0},"grace_period":null,"updated_at":"2026-10-19"}`, pass)
					return
				case "/quota/email":
					_, _ = fmt.Fprintf(w, `{"username":"owner","accounts":[{"email_address":"support@example.com","size_bytes":%d,"updated_at":"2026-10-19"}]}`, pass)
					return
				case "/domains/example.com":
					data = `{"domain":"example.com","mail_hosting":true,"ssl_enabled":true,"pointers":[]}`
				case "/domains/example.com/email-accounts", "/domains/example.com/email-accounts/support":
					data = fmt.Sprintf(`{"username":"support","email":"support@example.com","quota":0,"usage":%d,"limit":9600,"sent":%d,"suspended":false}`, pass, pass)
					if strings.HasSuffix(r.URL.Path, "/email-accounts") {
						data = "[" + data + "]"
					}
				case "/domains/example.com/catch-all":
					data = `{"type":"fail","address":null}`
				case "/domains/example.com/dns":
					data = `{"mx_records":[],"spf":{"type":"TXT","name":"example.com","value":"v=spf1 -all"},"dkim":null,"verification":null}`
				case "/domains/example.com/spam/settings":
					data = `{"high_score":10}`
					if scenario == "changed" && pass == 2 {
						data = `{"high_score":11}`
					} else if scenario == "malformed" {
						data = `{}`
					}
				case "/domains/example.com/spam/whitelist":
					data = `["first@example.org","second@example.net"]`
					if pass == 2 {
						data = `["second@example.net","first@example.org"]`
					}
				}
				_, _ = fmt.Fprintf(w, `{"success":true,"data":%s}`, data)
			}))
			defer server.Close()
			c, err := client.NewMock("mail.example.com", "owner", key, "test", server.URL)
			if err != nil {
				t.Fatal("cannot construct isolated mock discovery client")
			}
			r := Run(context.Background(), c)
			if r.BaselineUnchanged != (scenario == "stable") || r.Completed != (scenario == "stable" || scenario == "changed") {
				t.Fatal("discovery did not distinguish stable, changed and incomplete baselines")
			}
			if scenario == "stable" && (requests != 28 || len(r.Checks) != 14) {
				t.Fatal("discovery exceeded its bounded read budget or missed a route")
			}
			if (scenario == "budget" || scenario == "duplicate") && requests != 1 {
				t.Fatal("invalid inventory did not stop discovery immediately")
			}
			b, _ := json.Marshal(r)
			for _, sensitive := range []string{key, "example.com", "owner", "support", "synthetic-verification", "first@example.org"} {
				if strings.Contains(string(b), sensitive) {
					t.Fatal("discovery report contains account data")
				}
			}
		})
	}
}

func TestBaselinePreservesLargeIntegerChanges(t *testing.T) {
	r := Report{Checks: map[string]string{}}
	a, b := map[string][32]byte{}, map[string][32]byte{}
	for i, h := range []map[string][32]byte{a, b} {
		_, ok := capture(&r, h, "synthetic", "limit", func() (int64, error) { return 1<<53 + int64(i), nil }, unchanged[int64])
		if !ok {
			t.Fatal("cannot fingerprint a valid integer limit")
		}
	}
	if a["limit"] == b["limit"] {
		t.Fatal("large integer rounding hid a baseline change")
	}
}
