package livecheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
)

func TestMailboxProbePreservesExistingConfigurationAndRetainsOwnership(t *testing.T) {
	for _, scenario := range []string{"success", "mailbox-collision", "forwarder-collision", "baseline-drift", "create-rejected", "patch-rejected"} {
		t.Run(scenario, func(t *testing.T) {
			key := testutil.Secret(t)
			local := ""
			creates, patches, score := 0, 0, 10
			cleanupDependency := false
			passwords := []string{}
			account := func(user string, quota, limit int64) map[string]any {
				return map[string]any{"username": user, "email": user + "@example.com", "quota": quota, "limit": limit, "usage": 0, "sent": 0, "suspended": false}
			}
			accounts := map[string]map[string]any{"original": account("original", 128, 100)}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-API-Key") != key {
					t.Error("unexpected synthetic credential")
				}
				w.Header().Set("Content-Type", "application/json")
				reply := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": v}) }
				if r.Method != "GET" {
					if local == "" || (r.Method != "POST" && r.Method != "PATCH") {
						t.Error("unapproved mutation")
						w.WriteHeader(403)
						return
					}
					var input map[string]any
					if json.NewDecoder(r.Body).Decode(&input) != nil {
						t.Error("malformed scoped test request")
						w.WriteHeader(400)
						return
					}
					if password, ok := input["password"].(string); ok {
						passwords = append(passwords, password)
						delete(input, "password")
					}
					if r.Method == "POST" {
						creates++
						if r.URL.Path != "/domains/example.com/email-accounts" || input["username"] != local || accounts[local] != nil {
							t.Error("create targeted an existing mailbox or different scope")
						}
						if scenario == "create-rejected" {
							w.WriteHeader(400)
							return
						}
						accounts[local] = account(local, int64(input["quota"].(float64)), int64(input["limit"].(float64)))
						if scenario == "baseline-drift" {
							score++
						}
						w.WriteHeader(201)
						return
					}
					patches++
					if r.URL.Path != "/domains/example.com/email-accounts/"+local || accounts[local] == nil {
						t.Error("patch targeted a pre-existing mailbox")
					}
					if scenario == "patch-rejected" {
						w.WriteHeader(400)
						return
					}
					for field, value := range input {
						accounts[local][field] = value
					}
					return
				}
				switch r.URL.Path {
				case "/domains":
					reply([]string{"example.com"})
				case "/domains/example.com":
					reply(map[string]any{"domain": "example.com", "mail_hosting": true, "ssl_enabled": true, "pointers": []string{}})
				case "/verification-key":
					reply(map[string]any{"key": "synthetic-verification", "record": map[string]any{"type": "TXT", "name": "verify.example.com", "value": "domain-verified"}})
				case "/quota":
					_, _ = w.Write([]byte(`{"username":"owner","total_used":0,"total_limit":0,"percent_used":0,"breakdown":{"email":0,"web":0,"databases":0,"backups":0,"other":0},"grace_period":null,"updated_at":"2026-10-19"}`))
				case "/quota/email":
					_, _ = w.Write([]byte(`{"username":"owner","accounts":[]}`))
				case "/domains/example.com/email-accounts":
					items := []any{}
					for _, value := range accounts {
						items = append(items, value)
					}
					reply(items)
				case "/domains/example.com/forwarders":
					if cleanupDependency {
						reply([]any{map[string]any{"alias": "relay", "email": "relay@example.com", "destinations": []string{local + "@example.com"}}})
					} else if scenario == "forwarder-collision" && local != "" {
						reply([]any{map[string]any{"alias": local, "email": local + "@example.com", "destinations": []string{"target@example.net"}}})
					} else {
						reply([]any{})
					}
				case "/domains/example.com/catch-all":
					reply(map[string]any{"type": "fail", "address": nil})
				case "/domains/example.com/dns":
					reply(map[string]any{"mx_records": []any{}, "spf": map[string]any{"type": "TXT", "name": "example.com", "value": "v=spf1 -all"}, "dkim": nil, "verification": nil})
				case "/domains/example.com/spam/settings":
					reply(map[string]any{"high_score": score})
				default:
					if strings.HasPrefix(r.URL.Path, "/domains/example.com/email-accounts/") {
						value := accounts[strings.TrimPrefix(r.URL.Path, "/domains/example.com/email-accounts/")]
						if value == nil {
							w.WriteHeader(404)
							return
						}
						reply(value)
					} else {
						reply([]any{})
					}
				}
			}))
			defer server.Close()
			makeClient := func() (*client.Client, error) {
				return client.NewMock("mail.example.com", "owner", key, "test", server.URL)
			}
			reader, err := makeClient()
			if err != nil {
				t.Fatal("cannot create isolated client")
			}
			dir := t.TempDir()
			r := RunMailbox(context.Background(), reader, func(domain, user string) (*client.Client, error) {
				local = user
				if domain != "example.com" {
					t.Error("unexpected domain selection")
				}
				if scenario == "mailbox-collision" {
					accounts[local] = account(local, 128, 100)
				}
				return makeClient()
			}, dir, nil)
			if r.Completed != (scenario == "success") || r.Created != (scenario == "success" || scenario == "baseline-drift" || scenario == "patch-rejected") || r.Retained != r.Created {
				t.Fatal("mailbox test completion or retained ownership is incorrect")
			}
			wantCreates, wantPatches := 1, 0
			if strings.HasSuffix(scenario, "collision") {
				wantCreates = 0
			} else if scenario == "success" {
				wantPatches = 3
			} else if scenario == "patch-rejected" {
				wantPatches = 1
			}
			if creates != wantCreates || patches != wantPatches || accounts["original"]["quota"] != int64(128) || accounts["original"]["limit"] != int64(100) {
				t.Fatal("test changed existing configuration or continued after a failure")
			}
			manifest, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
			if err != nil {
				t.Fatal("private provenance was not retained")
			}
			info, _ := os.Stat(filepath.Join(dir, "manifest.json"))
			if info.Mode().Perm() != 0600 {
				t.Fatal("private provenance permissions are not restrictive")
			}
			public, _ := json.Marshal(r)
			for _, value := range append(passwords, key) {
				if strings.Contains(string(manifest), value) || strings.Contains(string(public), value) {
					t.Fatal("credential persisted in test record or report")
				}
			}
			for _, value := range []string{local, "example.com", "owner"} {
				if strings.Contains(string(public), value) {
					t.Fatal("account identity was printed in a public report")
				}
			}
			if scenario == "success" && (!r.BaselineUnchanged || len(passwords) != 2 || passwords[0] == passwords[1]) {
				t.Fatal("successful mailbox test did not confirm baseline and distinct rotation")
			}
			if scenario == "success" {
				path := filepath.Join(dir, "manifest.json")
				accounts[local]["usage"] = 0.01 // Storage counters do not establish ownership or emptiness.
				before := VerifyMailboxCleanup(context.Background(), reader, path, true)
				if !before.Completed || !before.BaselineUnchanged {
					t.Fatal("owned mailbox did not pass the explicitly authorized cleanup check")
				}
				cleanupDependency = true
				if VerifyMailboxCleanup(context.Background(), reader, path, true).Completed {
					t.Fatal("cleanup check accepted an unexpected forwarding dependency")
				}
				cleanupDependency = false
				if VerifyMailboxCleanup(context.Background(), reader, path, false).Completed {
					t.Fatal("cleanup check reported removal while the mailbox still existed")
				}
				delete(accounts, local) // External deletion of this synthetic mailbox only.
				var cleanupRecord map[string]any
				_ = json.Unmarshal(manifest, &cleanupRecord)
				cleanupRecord["cleanup"] = map[string]any{"state": "applied_pending_independent_API_verification", "delete_actions": 1}
				journal, _ := json.Marshal(cleanupRecord)
				if os.WriteFile(path, journal, 0600) != nil {
					t.Fatal("cannot prepare synthetic authorized cleanup journal")
				}
				after := VerifyMailboxCleanup(context.Background(), reader, path, false)
				if !after.Completed || !after.BaselineUnchanged || creates != 1 || patches != 3 {
					t.Fatal("cleanup checks changed configuration or failed to confirm removal")
				}
				cleanupRecord["status"] = "removed_confirmed"
				journal, _ = json.Marshal(cleanupRecord)
				if os.WriteFile(path, journal, 0600) != nil || !VerifyMailboxCleanup(context.Background(), reader, path, false).Completed {
					t.Fatal("finalized cleanup record could not be checked again using reads only")
				}
				var proof map[string]any
				_ = json.Unmarshal(manifest, &proof)
				proof["created_by_this_run"] = false
				invalid, _ := json.Marshal(proof)
				if os.WriteFile(path, invalid, 0600) != nil {
					t.Fatal("cannot prepare synthetic invalid ownership proof")
				}
				if VerifyMailboxCleanup(context.Background(), reader, path, false).Completed {
					t.Fatal("cleanup check trusted a name prefix without creation provenance")
				}
			}
		})
	}
}
