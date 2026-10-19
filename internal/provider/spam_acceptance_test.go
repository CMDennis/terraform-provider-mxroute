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
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

type spamService struct {
	mu                                                              sync.Mutex
	key                                                             string
	lists                                                           map[string][]string
	score                                                           int64
	creates, deletes, patches                                       int
	deny, malformed, missingDomain, grace, emptyUsage, wrappedUsage bool
}

func (s *spamService) serve(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.Header.Get("X-API-Key") != s.key || r.Header.Get("X-Server") != "mail.example.com" || r.Header.Get("X-Username") != "owner" {
			t.Error("unexpected mock credential context")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if s.deny {
			w.WriteHeader(403)
			return
		}
		if s.malformed {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		reply := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": v}) }
		usageReply := func(v any) {
			if s.wrappedUsage {
				reply(v)
			} else {
				_ = json.NewEncoder(w).Encode(v)
			}
		}
		if r.URL.Path == "/quota" {
			var grace any
			if s.grace {
				grace = map[string]any{"days_remaining": 3, "deadline": "2026-10-22"}
			}
			usageReply(map[string]any{"username": "owner", "total_used": int64(5368709121), "total_limit": int64(0), "percent_used": 0.125, "breakdown": map[string]int64{"email": 4294967296, "web": 1, "databases": 0, "backups": 1073741824, "other": 0}, "grace_period": grace, "updated_at": "2026-10-19T08:00:00-04:00"})
			return
		}
		if r.URL.Path == "/quota/email" {
			accounts := []any{}
			if !s.emptyUsage {
				accounts = append(accounts, map[string]any{"email_address": "large@example.com", "size_bytes": int64(4294967296), "updated_at": "2026-10-19T08:00:00-04:00"}, map[string]any{"email_address": "small@example.org", "size_bytes": int64(1), "updated_at": "2026-10-19T08:00:00-04:00"})
			}
			usageReply(map[string]any{"username": "owner", "accounts": accounts})
			return
		}
		p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(p) < 4 || p[0] != "domains" || (p[1] != "example.com" && p[1] != "example.org") || p[2] != "spam" || s.missingDomain {
			w.WriteHeader(404)
			return
		}
		kind := p[3]
		if kind == "settings" {
			if r.Method == "GET" {
				reply(map[string]int64{"high_score": s.score})
				return
			}
			var body map[string]int64
			if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["high_score"] < 1 || body["high_score"] > 50 {
				t.Error("unintended spam settings body")
				w.WriteHeader(400)
				return
			}
			s.score = body["high_score"]
			s.patches++
			return
		}
		values, ok := s.lists[kind]
		if !ok {
			w.WriteHeader(404)
			return
		}
		switch r.Method {
		case "GET":
			reply(values)
		case "POST":
			var body struct {
				Entry string `json:"entry"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				w.WriteHeader(400)
				return
			}
			for _, v := range values {
				if strings.EqualFold(v, body.Entry) {
					w.WriteHeader(409)
					return
				}
			}
			s.lists[kind] = append(values, body.Entry)
			s.creates++
			w.WriteHeader(201)
		case "DELETE":
			if len(p) != 5 {
				t.Error("malformed entry path")
				w.WriteHeader(400)
				return
			}
			for i, v := range values {
				if v == p[4] {
					s.lists[kind] = append(values[:i], values[i+1:]...)
					s.deletes++
					w.WriteHeader(204)
					return
				}
			}
			w.WriteHeader(404)
		default:
			w.WriteHeader(405)
		}
	})
}
func spamFixture(t *testing.T) (*spamService, map[string]func() (tfprotov6.ProviderServer, error), secretChecks) {
	t.Helper()
	base, _, checks := acceptanceFixture(t)
	s := &spamService{key: base.expectedKey, score: 15, lists: map[string][]string{"whitelist": {"unmanaged@example.net"}, "blacklist": {"unmanaged@example.org"}}}
	server := httptest.NewServer(s.serve(t))
	t.Cleanup(server.Close)
	factories := map[string]func() (tfprotov6.ProviderServer, error){"mxroute": providerserver.NewProtocol6WithError(&mxrouteProvider{version: "test", newClient: func(host, user, key, version string) (*client.Client, error) {
		return client.NewMock(host, user, key, version, server.URL)
	}})}
	return s, factories, checks
}
func spamConfig(domain, entry string, score int) string {
	return domainHeader + fmt.Sprintf(`
resource "mxroute_spam_settings" "test" {
 access_domain = %q
 high_score = %d
}
resource "mxroute_spam_whitelist_entry" "test" {
 access_domain = %q
 entry = %q
}
resource "mxroute_spam_blacklist_entry" "test" {
 access_domain = "example.org"
 entry = "blocked+tag=test@example.net"
}
data "mxroute_spam_settings" "test" {
 access_domain = "example.org"
 depends_on = [mxroute_spam_settings.test]
}
data "mxroute_spam_whitelist" "test" {
 access_domain = "example.org"
 depends_on = [mxroute_spam_whitelist_entry.test]
}
data "mxroute_spam_blacklist" "test" {
 access_domain = "example.com"
 depends_on = [mxroute_spam_blacklist_entry.test]
}
`, domain, score, domain, entry)
}
func TestOfflineSpamLifecycle(t *testing.T) {
	s, factories, checks := spamFixture(t)
	cfg := spamConfig("example.com", "*@Example.NET", 20)
	changed := spamConfig("example.org", "*@example.net", 50)
	first := mailStep(cfg, checks)
	first.Check = resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr("mxroute_spam_settings.test", "id", "spam-settings"), resource.TestCheckResourceAttr("mxroute_spam_whitelist_entry.test", "id", "*@example.net"), resource.TestCheckResourceAttr("data.mxroute_spam_settings.test", "high_score", "20"), resource.TestCheckResourceAttr("data.mxroute_spam_whitelist.test", "entries.#", "2"), resource.TestCheckResourceAttr("data.mxroute_spam_blacklist.test", "entries.#", "2"))
	update := mailStep(changed, checks)
	update.ConfigPlanChecks.PreApply = append(update.ConfigPlanChecks.PreApply, plancheck.ExpectResourceAction("mxroute_spam_whitelist_entry.test", plancheck.ResourceActionUpdate))
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		first, emptyMailPlan(cfg, checks),
		{Config: cfg, ResourceName: "mxroute_spam_settings.test", ImportState: true, ImportStateId: "EXAMPLE.COM.", ImportStateVerify: true},
		{Config: cfg, ResourceName: "mxroute_spam_whitelist_entry.test", ImportState: true, ImportStateId: "EXAMPLE.COM./*@Example.NET", ImportStateVerify: true},
		{Config: cfg, ResourceName: "mxroute_spam_blacklist_entry.test", ImportState: true, ImportStateId: "example.org/blocked+tag=test@example.net", ImportStateVerify: true},
		update, emptyMailPlan(changed, checks),
	}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creates != 2 || s.deletes != 2 || s.patches != 2 || s.score != 50 || len(s.lists["whitelist"]) != 1 || len(s.lists["blacklist"]) != 1 {
		t.Fatal("shared ownership, unrelated entries or relinquish semantics failed")
	}
}
func TestOfflineSpamDriftAndDeletion(t *testing.T) {
	s, factories, checks := spamFixture(t)
	cfg := spamConfig("example.com", "*@example.net", 20)
	drift := mailStep(cfg, checks)
	drift.PreConfig = func() { s.mu.Lock(); s.score = 10; s.mu.Unlock() }
	recreate := mailStep(cfg, checks)
	recreate.PreConfig = func() { s.mu.Lock(); s.lists["whitelist"] = []string{"unmanaged@example.net"}; s.mu.Unlock() }
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{mailStep(cfg, checks), drift, recreate, emptyMailPlan(cfg, checks)}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creates != 3 || s.patches != 2 {
		t.Fatal("drift or external entry deletion was not reconciled")
	}
}
func TestOfflineSpamExistingEntryRequiresImport(t *testing.T) {
	_, factories, checks := spamFixture(t)
	cfg := domainHeader + `resource "mxroute_spam_whitelist_entry" "test" {
 access_domain = "example.com"
 entry = "UNMANAGED@example.net"
}`
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{{Config: cfg, ExpectError: regexp.MustCompile("import to adopt")}}})
}
func TestOfflineSpamReadErrorsPreserveState(t *testing.T) {
	for _, kind := range []string{"forbidden", "malformed", "missing domain"} {
		t.Run(kind, func(t *testing.T) {
			s, factories, checks := spamFixture(t)
			cfg := spamConfig("example.com", "*@example.net", 20)
			fail := resource.TestStep{Config: cfg, PlanOnly: true, ExpectError: regexp.MustCompile("MXroute operation failed"), PreConfig: func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.deny = kind == "forbidden"
				s.malformed = kind == "malformed"
				s.missingDomain = kind == "missing domain"
			}}
			recover := emptyMailPlan(cfg, checks)
			recover.PreConfig = func() { s.mu.Lock(); s.deny = false; s.malformed = false; s.missingDomain = false; s.mu.Unlock() }
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{mailStep(cfg, checks), fail, recover}})
		})
	}
}
func TestOfflineUsageReporting(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		t.Run(fmt.Sprintf("wrapped=%t", wrapped), func(t *testing.T) {
			testOfflineUsageReporting(t, wrapped)
		})
	}
}
func testOfflineUsageReporting(t *testing.T, wrapped bool) {
	s, factories, checks := spamFixture(t)
	s.grace = true
	s.wrappedUsage = wrapped
	cfg := domainHeader + `data "mxroute_quota" "test" {}
data "mxroute_email_usage" "test" {}
`
	first := mailStep(cfg, checks)
	first.Check = resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr("data.mxroute_quota.test", "total_used", "5368709121"), resource.TestCheckResourceAttr("data.mxroute_quota.test", "total_limit", "0"), resource.TestCheckResourceAttr("data.mxroute_quota.test", "percent_used", "0.125"), resource.TestCheckResourceAttr("data.mxroute_quota.test", "breakdown.email", "4294967296"), resource.TestCheckResourceAttr("data.mxroute_quota.test", "breakdown.web", "1"), resource.TestCheckResourceAttr("data.mxroute_quota.test", "breakdown.databases", "0"), resource.TestCheckResourceAttr("data.mxroute_quota.test", "breakdown.backups", "1073741824"), resource.TestCheckResourceAttr("data.mxroute_quota.test", "breakdown.other", "0"), resource.TestCheckResourceAttr("data.mxroute_quota.test", "grace_period.days_remaining", "3"), resource.TestCheckResourceAttr("data.mxroute_quota.test", "grace_period.deadline", "2026-10-22"), resource.TestCheckResourceAttr("data.mxroute_quota.test", "updated_at", "2026-10-19T08:00:00-04:00"), resource.TestCheckResourceAttr("data.mxroute_email_usage.test", "accounts.0.size_bytes", "4294967296"), resource.TestCheckResourceAttr("data.mxroute_email_usage.test", "accounts.1.email_address", "small@example.org"))
	cleared := mailStep(cfg, checks)
	cleared.PreConfig = func() { s.mu.Lock(); s.grace = false; s.emptyUsage = true; s.mu.Unlock() }
	cleared.Check = resource.ComposeTestCheckFunc(resource.TestCheckNoResourceAttr("data.mxroute_quota.test", "grace_period.days_remaining"), resource.TestCheckResourceAttr("data.mxroute_email_usage.test", "accounts.#", "0"))
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{first, emptyMailPlan(cfg, checks), cleared, emptyMailPlan(cfg, checks)}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creates+s.deletes+s.patches != 0 {
		t.Fatal("hourly usage generated writes")
	}
}

func TestOfflineSpamEntryReplacement(t *testing.T) {
	s, factories, checks := spamFixture(t)
	cfg := spamConfig("example.com", "first@example.net", 1)
	changed := spamConfig("example.org", "second@example.net", 1)
	replace := mailStep(changed, checks)
	replace.ConfigPlanChecks.PreApply = append(replace.ConfigPlanChecks.PreApply, plancheck.ExpectResourceAction("mxroute_spam_whitelist_entry.test", plancheck.ResourceActionDestroyBeforeCreate))
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{mailStep(cfg, checks), replace, emptyMailPlan(changed, checks)}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creates != 3 || s.deletes != 3 || s.score != 1 {
		t.Fatal("entry replacement or score boundary failed")
	}
}
func TestOfflineUsageReadFailures(t *testing.T) {
	for _, kind := range []string{"malformed", "forbidden"} {
		t.Run(kind, func(t *testing.T) {
			s, factories, checks := spamFixture(t)
			cfg := domainHeader + `data "mxroute_quota" "test" {}
data "mxroute_email_usage" "test" {}`
			fail := resource.TestStep{Config: cfg, PlanOnly: true, ExpectError: regexp.MustCompile("MXroute operation failed"), PreConfig: func() { s.mu.Lock(); s.malformed = kind == "malformed"; s.deny = kind == "forbidden"; s.mu.Unlock() }}
			recover := emptyMailPlan(cfg, checks)
			recover.PreConfig = func() { s.mu.Lock(); s.malformed = false; s.deny = false; s.mu.Unlock() }
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{mailStep(cfg, checks), fail, recover}})
		})
	}
}
