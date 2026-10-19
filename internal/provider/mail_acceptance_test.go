package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
)

type mailService struct {
	failReadAfterPatch                                                                  bool
	mu                                                                                  sync.Mutex
	key, password                                                                       string
	accounts                                                                            map[string]map[string]any
	forwarders                                                                          map[string][]string
	catchType                                                                           string
	catchAddress                                                                        *string
	patches                                                                             [][]string
	rotations, creates, forwardCreates, catchPatches, deletes                           int
	failForwardCreate, failDelete, failPatch, ambiguous, malformed, deny, detailMissing bool
}

func (s *mailService) serve(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.Header.Get("X-API-Key") != s.key || r.Header.Get("X-Server") != "mail.example.com" || r.Header.Get("X-Username") != "owner" {
			t.Error("unexpected mock credential context")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fail := func(status int, code string) {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]string{"code": code}})
		}
		reply := func(data any) { _ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data}) }
		if r.Method == "GET" && s.deny {
			fail(403, "FORBIDDEN")
			return
		}
		if r.Method == "GET" && s.malformed {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(p) < 3 || p[0] != "domains" || p[1] != "example.com" {
			fail(404, "NOT_FOUND")
			return
		}
		switch p[2] {
		case "email-accounts":
			if len(p) == 3 && r.Method == "GET" {
				values := []any{}
				for _, v := range s.accounts {
					values = append(values, v)
				}
				if s.ambiguous && len(values) > 0 {
					copy := map[string]any{}
					for k, v := range values[0].(map[string]any) {
						copy[k] = v
					}
					copy["username"] = strings.ToUpper(copy["username"].(string))
					values = append(values, copy)
				}
				reply(values)
				return
			}
			if len(p) == 3 && r.Method == "POST" {
				var v client.CreateEmailAccountRequest
				if json.NewDecoder(r.Body).Decode(&v) != nil {
					fail(400, "VALIDATION_ERROR")
					return
				}
				if v.Password != s.password {
					t.Error("mailbox password altered")
				}
				key := strings.ToLower(v.Username)
				if _, ok := s.accounts[key]; ok {
					fail(409, "CONFLICT")
					return
				}
				quota, limit := int64(1024), int64(9600)
				if v.Quota != nil {
					quota = *v.Quota
				}
				if v.Limit != nil {
					limit = *v.Limit
				}
				// Preserve server casing to prove subsequent calls use canonical identity.
				s.accounts[key] = map[string]any{"username": v.Username, "email": v.Username + "@example.com", "quota": quota, "limit": limit, "usage": 1.25, "sent": int64(2), "suspended": false}
				s.rotations++
				s.creates++
				w.WriteHeader(201)
				return
			}
			if len(p) == 4 {
				key := strings.ToLower(p[3])
				v, ok := s.accounts[key]
				if !ok {
					fail(404, "NOT_FOUND")
					return
				}
				if p[3] != v["username"] {
					t.Error("request did not use canonical mailbox name")
					fail(400, "VALIDATION_ERROR")
					return
				}
				switch r.Method {
				case "GET":
					if s.detailMissing {
						fail(404, "NOT_FOUND")
						return
					}
					reply(v)
					return
				case "PATCH":
					if s.failPatch {
						s.failPatch = false
						fail(422, "VALIDATION_ERROR")
						return
					}
					var body map[string]json.RawMessage
					if json.NewDecoder(r.Body).Decode(&body) != nil {
						fail(400, "VALIDATION_ERROR")
						return
					}
					fields := []string{}
					for k, raw := range body {
						fields = append(fields, k)
						if k == "password" {
							var password string
							_ = json.Unmarshal(raw, &password)
							if password != s.password {
								t.Error("rotated password altered")
							}
							s.rotations++
						} else if k == "quota" || k == "limit" {
							var n int64
							_ = json.Unmarshal(raw, &n)
							v[k] = n
						} else {
							t.Error("unexpected mailbox PATCH field")
						}
					}
					s.patches = append(s.patches, fields)
					if s.failReadAfterPatch {
						s.malformed = true
						s.failReadAfterPatch = false
					}
					w.WriteHeader(200)
					return
				case "DELETE":
					if s.failDelete {
						s.failDelete = false
						fail(422, "VALIDATION_ERROR")
						return
					}
					delete(s.accounts, key)
					s.deletes++
					w.WriteHeader(204)
					return
				}
			}
		case "forwarders":
			if len(p) == 3 && r.Method == "GET" {
				values := []any{}
				for alias, dest := range s.forwarders {
					values = append(values, map[string]any{"alias": alias, "email": alias + "@example.com", "destinations": dest})
				}
				reply(values)
				return
			}
			if len(p) == 3 && r.Method == "POST" {
				s.forwardCreates++
				if s.failForwardCreate {
					s.failForwardCreate = false
					fail(422, "VALIDATION_ERROR")
					return
				}
				var v client.CreateForwarderRequest
				_ = json.NewDecoder(r.Body).Decode(&v)
				if _, ok := s.forwarders[v.Alias]; ok {
					fail(409, "CONFLICT")
					return
				}
				s.forwarders[v.Alias] = v.Destinations
				w.WriteHeader(201)
				return
			}
			if len(p) == 4 && r.Method == "DELETE" {
				if s.failDelete {
					s.failDelete = false
					fail(422, "VALIDATION_ERROR")
					return
				}
				delete(s.forwarders, p[3])
				s.deletes++
				w.WriteHeader(204)
				return
			}
		case "catch-all":
			if r.Method == "GET" {
				reply(map[string]any{"type": s.catchType, "address": s.catchAddress})
				return
			}
			if r.Method == "PATCH" {
				var v client.SetCatchAllRequest
				_ = json.NewDecoder(r.Body).Decode(&v)
				if v.Type != "address" && v.Address != nil {
					t.Error("non-address catch-all sent stale destination")
				}
				s.catchType = v.Type
				if v.Type == "address" {
					s.catchAddress = v.Address
				}
				// Intentionally retain a stale API address in other modes.
				s.catchPatches++
				w.WriteHeader(200)
				return
			}
		}
		t.Error("unexpected mock operation")
		fail(400, "VALIDATION_ERROR")
	})
}
func mailFixture(t *testing.T) (*mailService, map[string]func() (tfprotov6.ProviderServer, error), secretChecks) {
	t.Helper()
	base, _, checks := acceptanceFixture(t)
	s := &mailService{key: base.expectedKey, password: base.expectedPassword, accounts: map[string]map[string]any{}, forwarders: map[string][]string{}, catchType: "fail"}
	server := httptest.NewServer(s.serve(t))
	t.Cleanup(server.Close)
	factories := map[string]func() (tfprotov6.ProviderServer, error){"mxroute": providerserver.NewProtocol6WithError(&mxrouteProvider{version: "test", newClient: func(host, user, key, version string) (*client.Client, error) {
		return client.NewMock(host, user, key, version, server.URL)
	}})}
	return s, factories, checks
}
func mailboxConfig(username string, quota, limit, version int, password bool) string {
	attrs := ""
	if quota >= 0 {
		attrs += fmt.Sprintf("quota = %d\n", quota)
	}
	if limit >= 0 {
		attrs += fmt.Sprintf("limit = %d\n", limit)
	}
	if version > 0 {
		attrs += fmt.Sprintf("password_wo_version = %d\n", version)
	}
	if password {
		attrs += "password_wo = var.fixture_password\n"
	}
	return fixtureHeader + fmt.Sprintf("resource \"mxroute_email_account\" \"test\" {\ndomain = \"example.com\"\nusername = %q\n%s}\n", username, attrs)
}
func mailStep(config string, checks secretChecks) resource.TestStep {
	return resource.TestStep{Config: config, ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{checks}}, ConfigStateChecks: []statecheck.StateCheck{checks}}
}
func emptyMailPlan(config string, checks secretChecks) resource.TestStep {
	return resource.TestStep{Config: config, PlanOnly: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan(), checks}}}
}
func TestOfflineMailboxLifecycle(t *testing.T) {
	s, factories, checks := mailFixture(t)
	rotatedPassword := testutil.Secret(t)
	checks.secrets = append(checks.secrets, rotatedPassword)
	initial := mailboxConfig("Support", 0, 120, 1, true)
	updated := mailboxConfig("Support", 512, 120, 1, false)
	rotated := mailboxConfig("Support", 512, 120, 2, true)
	data := `
data "mxroute_email_account" "test" {
 domain = mxroute_email_account.test.domain
 username = "SUPPORT"
}
data "mxroute_email_accounts" "test" {
 domain = mxroute_email_account.test.domain
 depends_on = [mxroute_email_account.test]
}
`
	first := mailStep(initial+data, checks)
	first.Check = resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr("mxroute_email_account.test", "quota", "0"), resource.TestCheckResourceAttr("mxroute_email_account.test", "canonical_username", "Support"), resource.TestCheckResourceAttr("data.mxroute_email_account.test", "usage", "1.25"), resource.TestCheckResourceAttr("data.mxroute_email_accounts.test", "accounts.#", "1"))
	rotateStep := mailStep(rotated, checks)
	rotateStep.PreConfig = func() {
		t.Setenv("TF_VAR_fixture_password", rotatedPassword)
		s.mu.Lock()
		s.password = rotatedPassword
		s.mu.Unlock()
	}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		first, emptyMailPlan(initial+data, checks),
		mailStep(updated, checks), rotateStep, emptyMailPlan(rotated, checks),
		mailStep(mailboxConfig("support", 512, 120, 2, false), checks),
		{Config: mailboxConfig("support", 512, 120, 0, false), ResourceName: "mxroute_email_account.test", ImportState: true, ImportStateId: "EXAMPLE.COM./SUPPORT", ImportStateVerify: true, ImportStateVerifyIgnore: []string{"password_wo_version"}},
		emptyMailPlan(mailboxConfig("support", 512, 120, 2, false), checks),
		mailStep(mailboxConfig("support", 512, 240, 2, false), checks),
	}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creates != 1 || s.rotations != 2 || len(s.patches) != 3 || len(s.accounts) != 0 {
		t.Fatal("mailbox lifecycle duplicated identity, rotated unexpectedly or failed cleanup")
	}
	for i, want := range []string{"quota", "password", "limit"} {
		if len(s.patches[i]) != 1 || s.patches[i][0] != want {
			t.Fatal("mailbox update overwrote unrelated fields")
		}
	}
}
func TestOfflineMailboxDefaultsAndDrift(t *testing.T) {
	s, factories, checks := mailFixture(t)
	cfg := mailboxConfig("support", -1, -1, 1, true)
	drift := emptyMailPlan(cfg, checks)
	drift.PreConfig = func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		v := s.accounts["support"]
		v["quota"] = int64(2048)
		v["usage"] = 9.5
		v["sent"] = int64(23)
		v["suspended"] = true
	}
	explicit := mailboxConfig("support", 0, 900, 1, false)
	restore := mailStep(explicit, checks)
	restore.PreConfig = func() { s.mu.Lock(); defer s.mu.Unlock(); s.accounts["support"]["quota"] = int64(40) }
	deleted := mailStep(explicit, checks)
	deleted.PreConfig = func() { s.mu.Lock(); defer s.mu.Unlock(); delete(s.accounts, "support") }
	// A new mailbox after external deletion requires a supplied password again.
	deleted.Config = mailboxConfig("support", 0, 900, 1, true)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		{Config: cfg, Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr("mxroute_email_account.test", "quota", "1024"), resource.TestCheckResourceAttr("mxroute_email_account.test", "limit", "9600"))},
		drift, mailStep(explicit, checks), restore, deleted, emptyMailPlan(deleted.Config, checks),
	}})
	if s.creates != 2 {
		t.Fatal("external deletion did not recreate mailbox")
	}
}
func routingConfig(destinations string, mode, address string) string {
	a := ""
	if address != "" {
		a = fmt.Sprintf("address = %q", address)
	}
	return domainHeader + fmt.Sprintf(`
resource "mxroute_forwarder" "test" {
 domain = "example.com"
 alias = "sales"
 destinations = %s
}
resource "mxroute_catch_all" "test" {
 domain = "example.com"
 type = %q
 %s
}
`, destinations, mode, a)
}
func TestOfflineRoutingLifecycle(t *testing.T) {
	s, factories, checks := mailFixture(t)
	cfg := routingConfig(`["one@example.net", "two@example.org"]`, "address", "catch@example.net")
	data := `
data "mxroute_forwarder" "test" {
 domain = mxroute_forwarder.test.domain
 alias = "SALES"
}
data "mxroute_forwarders" "test" {
 domain = mxroute_forwarder.test.domain
 depends_on = [mxroute_forwarder.test]
}
data "mxroute_catch_all" "test" { domain = mxroute_catch_all.test.domain }
`
	first := mailStep(cfg+data, checks)
	first.Check = resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr("data.mxroute_forwarder.test", "destinations.#", "2"), resource.TestCheckResourceAttr("data.mxroute_forwarders.test", "forwarders.#", "1"), resource.TestCheckResourceAttr("data.mxroute_catch_all.test", "type", "address"))
	fail := routingConfig(`["one@example.net", "two@example.org"]`, "fail", "") + data
	blackhole := routingConfig(`[":blackhole:"]`, "blackhole", "")
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		first, emptyMailPlan(strings.Replace(cfg+data, `["one@example.net", "two@example.org"]`, `["two@example.org", "one@example.net"]`, 1), checks),
		{Config: cfg, ResourceName: "mxroute_forwarder.test", ImportState: true, ImportStateId: "EXAMPLE.COM./SALES", ImportStateVerify: true},
		{Config: cfg, ResourceName: "mxroute_catch_all.test", ImportState: true, ImportStateId: "EXAMPLE.COM.", ImportStateVerify: true},
		emptyMailPlan(cfg, checks),
		{Config: fail, Check: resource.ComposeTestCheckFunc(resource.TestCheckNoResourceAttr("mxroute_catch_all.test", "address"), resource.TestCheckNoResourceAttr("data.mxroute_catch_all.test", "address"))}, emptyMailPlan(fail, checks),
		{Config: blackhole, PlanOnly: true, ExpectNonEmptyPlan: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectResourceAction("mxroute_forwarder.test", plancheck.ResourceActionDestroyBeforeCreate)}}},
		mailStep(blackhole, checks), emptyMailPlan(blackhole, checks),
		mailStep(routingConfig(`[":fail:"]`, "address", "new@example.org"), checks),
	}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catchType != "address" || s.catchAddress == nil || *s.catchAddress != "new@example.org" || s.catchPatches != 4 || len(s.forwarders) != 0 {
		t.Fatal("catch-all destroy reset routing or routing lifecycle failed")
	}
}
func TestOfflineForwarderReplacementFailure(t *testing.T) {
	s, factories, checks := mailFixture(t)
	initial := routingConfig(`["one@example.net"]`, "fail", "")
	replacement := routingConfig(`["two@example.org"]`, "fail", "")
	failed := resource.TestStep{Config: replacement, PreConfig: func() { s.mu.Lock(); s.failForwardCreate = true; s.mu.Unlock() }, ExpectError: regexp.MustCompile(`documented API\s+constraints`)}
	recovered := mailStep(replacement, checks)
	recovered.PreConfig = func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.forwarders) != 0 {
			t.Error("failed replacement did not leave absent alias")
		}
	}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{mailStep(initial, checks), failed, recovered, emptyMailPlan(replacement, checks)}})
	if s.forwardCreates != 3 {
		t.Fatal("failed forwarder replacement was replayed or not recovered")
	}
}
func TestOfflineMailReadFailuresPreserveState(t *testing.T) {
	for _, kind := range []string{"malformed", "forbidden", "ambiguous", "detail 404"} {
		t.Run(kind, func(t *testing.T) {
			s, factories, checks := mailFixture(t)
			cfg := mailboxConfig("Support", 0, 120, 1, true)
			broken := resource.TestStep{Config: cfg, PlanOnly: true, ExpectError: regexp.MustCompile("MXroute operation failed"), PreConfig: func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				switch kind {
				case "malformed":
					s.malformed = true
				case "forbidden":
					s.deny = true
				case "ambiguous":
					s.ambiguous = true
				case "detail 404":
					s.detailMissing = true
				}
			}}
			recovered := emptyMailPlan(cfg, checks)
			recovered.PreConfig = func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.malformed = false
				s.deny = false
				s.ambiguous = false
				s.detailMissing = false
			}
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{mailStep(cfg, checks), broken, recovered}})
			if s.creates != 1 || s.rotations != 1 {
				t.Fatal("failed refresh lost mailbox state")
			}
		})
	}
}

func TestOfflineMailboxImportAdoptionAndRotation(t *testing.T) {
	s, factories, checks := mailFixture(t)
	s.accounts["support"] = map[string]any{"username": "Support", "email": "Support@example.com", "quota": int64(0), "limit": int64(120), "usage": 0.0, "sent": int64(0), "suspended": false}
	imported := mailboxConfig("support", 0, 120, 0, false)
	adoption := imported + `
import {
 to = mxroute_email_account.test
 id = "EXAMPLE.COM./SUPPORT"
}
`
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		mailStep(adoption, checks), emptyMailPlan(imported, checks),
		mailStep(mailboxConfig("support", 0, 120, 1, true), checks),
		emptyMailPlan(mailboxConfig("support", 0, 120, 1, false), checks),
	}})
	if s.creates != 0 || s.rotations != 1 {
		t.Fatal("import required a password, replaced mailbox, or failed explicit first rotation")
	}
}

func TestOfflineMailboxFailedUpdatePreservesState(t *testing.T) {
	s, factories, checks := mailFixture(t)
	initial := mailboxConfig("Support", 0, 120, 1, true)
	updated := mailboxConfig("Support", 512, 120, 1, false)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		mailStep(initial, checks),
		{Config: updated, PreConfig: func() { s.mu.Lock(); s.failPatch = true; s.mu.Unlock() }, ExpectError: regexp.MustCompile(`documented API\s+constraints`)},
		mailStep(updated, checks), emptyMailPlan(updated, checks),
	}})
	if s.creates != 1 || s.rotations != 1 || len(s.patches) != 1 {
		t.Fatal("failed update lost state or replayed password")
	}
}
func TestOfflineRoutingDriftAndDeletion(t *testing.T) {
	s, factories, checks := mailFixture(t)
	cfg := routingConfig(`["one@example.net"]`, "address", "catch@example.net")
	drift := mailStep(cfg, checks)
	drift.PreConfig = func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.forwarders["sales"] = []string{"drift@example.org"}
		s.catchType = "blackhole"
	}
	deleted := mailStep(cfg, checks)
	deleted.PreConfig = func() { s.mu.Lock(); defer s.mu.Unlock(); delete(s.forwarders, "sales") }
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		mailStep(cfg, checks), drift, deleted, emptyMailPlan(cfg, checks),
	}})
	if s.forwardCreates != 3 || s.catchType != "address" {
		t.Fatal("routing drift/deletion not reconciled")
	}
}
func TestOfflineForwarderFailedDeletePreservesRouting(t *testing.T) {
	s, factories, checks := mailFixture(t)
	initial := routingConfig(`["one@example.net"]`, "fail", "")
	changed := routingConfig(`["two@example.org"]`, "fail", "")
	recovered := mailStep(changed, checks)
	recovered.PreConfig = func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.forwarders) != 1 || s.forwardCreates != 1 {
			t.Error("failed delete lost routing or created duplicate alias")
		}
	}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		mailStep(initial, checks),
		{Config: changed, PreConfig: func() { s.mu.Lock(); s.failDelete = true; s.mu.Unlock() }, ExpectError: regexp.MustCompile(`documented API\s+constraints`)},
		recovered, emptyMailPlan(changed, checks),
	}})
}

func TestOfflineMailboxExistingCaseVariantRejected(t *testing.T) {
	s, factories, checks := mailFixture(t)
	s.accounts["support"] = map[string]any{"username": "Support", "email": "Support@example.com", "quota": int64(0), "limit": int64(120), "usage": 0.0, "sent": int64(0), "suspended": false}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		{Config: mailboxConfig("SUPPORT", 0, 120, 1, true), ExpectError: regexp.MustCompile("Mailbox already exists")},
	}})
	if s.creates != 0 || len(s.accounts) != 1 {
		t.Fatal("pre-existing mailbox was duplicated or adopted")
	}
}
func TestOfflineRoutingReadFailuresPreserveState(t *testing.T) {
	s, factories, checks := mailFixture(t)
	cfg := routingConfig(`["one@example.net"]`, "fail", "")
	bad := resource.TestStep{Config: cfg, PlanOnly: true, PreConfig: func() { s.mu.Lock(); s.malformed = true; s.mu.Unlock() }, ExpectError: regexp.MustCompile("incomplete or invalid API response")}
	denied := bad
	denied.PreConfig = func() { s.mu.Lock(); s.malformed = false; s.deny = true; s.mu.Unlock() }
	denied.ExpectError = regexp.MustCompile("credential role")
	recovered := emptyMailPlan(cfg, checks)
	recovered.PreConfig = func() { s.mu.Lock(); s.deny = false; s.mu.Unlock() }
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{mailStep(cfg, checks), bad, denied, recovered}})
	if s.forwardCreates != 1 || s.catchPatches != 1 {
		t.Fatal("failed routing reads lost state")
	}
}

func TestOfflineMailboxConfirmedRotationReadbackFailure(t *testing.T) {
	s, factories, checks := mailFixture(t)
	initial := mailboxConfig("Support", 0, 120, 1, true)
	rotated := mailboxConfig("Support", 0, 120, 2, true)
	recovered := emptyMailPlan(mailboxConfig("Support", 0, 120, 2, false), checks)
	recovered.PreConfig = func() { s.mu.Lock(); s.malformed = false; s.mu.Unlock() }
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		mailStep(initial, checks),
		{Config: rotated, PreConfig: func() { s.mu.Lock(); s.failReadAfterPatch = true; s.mu.Unlock() }, ExpectError: regexp.MustCompile("incomplete or invalid API response")},
		recovered,
	}})
	if s.rotations != 2 || len(s.patches) != 1 {
		t.Fatal("failed readback replayed a confirmed rotation")
	}
}

func TestOfflineMailEmptyLists(t *testing.T) {
	s, factories, checks := mailFixture(t)
	cfg := domainHeader + `
data "mxroute_email_accounts" "test" { domain = "example.com" }
data "mxroute_forwarders" "test" { domain = "example.com" }
data "mxroute_catch_all" "test" { domain = "example.com" }
`
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		{Config: cfg, Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr("data.mxroute_email_accounts.test", "accounts.#", "0"), resource.TestCheckResourceAttr("data.mxroute_forwarders.test", "forwarders.#", "0"), resource.TestCheckNoResourceAttr("data.mxroute_catch_all.test", "address"))},
		emptyMailPlan(cfg, checks),
	}})
	if s.creates != 0 || s.forwardCreates != 0 || s.catchPatches != 0 {
		t.Fatal("data source mutated remote configuration")
	}
}

func TestOfflineMailWorkflowDependencies(t *testing.T) {
	s, _, checks := mailFixture(t)
	domains := &domainService{key: s.key, domains: map[string]bool{}, pointers: map[string]map[string]string{}}
	domainHandler, mailHandler := domains.serve(t), s.serve(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.Method == "DELETE" && len(parts) == 2 {
			s.mu.Lock()
			children := len(s.accounts) + len(s.forwarders)
			s.mu.Unlock()
			if children != 0 {
				t.Error("domain deleted before mailbox/forwarder dependencies")
				w.WriteHeader(409)
				return
			}
		}
		if len(parts) >= 3 && (parts[2] == "email-accounts" || parts[2] == "forwarders" || parts[2] == "catch-all") {
			domains.mu.Lock()
			_, owned := domains.domains[parts[1]]
			domains.mu.Unlock()
			if !owned {
				t.Error("mail child created before domain")
				w.WriteHeader(404)
				return
			}
			mailHandler.ServeHTTP(w, r)
			return
		}
		domainHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	factories := map[string]func() (tfprotov6.ProviderServer, error){"mxroute": providerserver.NewProtocol6WithError(&mxrouteProvider{version: "test", newClient: func(host, user, key, version string) (*client.Client, error) {
		return client.NewMock(host, user, key, version, server.URL)
	}})}
	cfg := fixtureHeader + `
resource "mxroute_domain" "mail" {
 domain = "example.com"
 mail_hosting = true
}
resource "mxroute_email_account" "support" {
 domain = mxroute_domain.mail.domain
 username = "Support"
 password_wo = var.fixture_password
 password_wo_version = 1
 quota = 1024
 limit = 120
}
resource "mxroute_forwarder" "sales" {
 domain = mxroute_domain.mail.domain
 alias = "sales"
 destinations = [mxroute_email_account.support.email, "archive@example.net"]
}
resource "mxroute_catch_all" "routing" {
 domain = mxroute_domain.mail.domain
 type = "fail"
}
`
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{mailStep(cfg, checks), emptyMailPlan(cfg, checks)}})
	if domains.creates != 1 || s.creates != 1 || s.forwardCreates != 1 || len(domains.domains) != 0 || len(s.accounts) != 0 || len(s.forwarders) != 0 {
		t.Fatal("ordinary mail workflow or dependency cleanup failed")
	}
}
