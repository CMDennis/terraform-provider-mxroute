package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

type domainService struct {
	mu                                                 sync.Mutex
	key                                                string
	domains                                            map[string]bool
	pointers                                           map[string]map[string]string
	events                                             []string
	failPatch, malformed, deny, rejectCreate, nullable bool
	invisibleDomainReads, invisiblePointerReads        int
	creates, patches, pointerCreates                   int
}

func (s *domainService) serve(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.Header.Get("X-API-Key") != s.key || r.Header.Get("X-Server") != "mail.example.com" || r.Header.Get("X-Username") != "owner" {
			t.Error("mock received unexpected credential context")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fail := func(status int, code string) {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"code": code}})
		}
		reply := func(status int, data any) {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
		}
		if s.deny {
			fail(403, "FORBIDDEN")
			return
		}
		if r.Method == "GET" && s.malformed {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.Method == "GET" && r.URL.Path == "/verification-key" {
			reply(200, map[string]any{"key": "_da-verify-synthetic", "record": map[string]string{"name": "_da-verify-synthetic", "type": "TXT", "value": "domain-verified"}})
			return
		}
		if len(parts) == 1 && parts[0] == "domains" {
			if r.Method == "GET" {
				names := make([]string, 0, len(s.domains))
				for name := range s.domains {
					if s.invisibleDomainReads == 0 {
						names = append(names, name)
					}
				}
				if s.invisibleDomainReads > 0 {
					s.invisibleDomainReads--
				}
				sort.Strings(names)
				reply(200, names)
				return
			}
			if r.Method == "POST" {
				s.creates++
				if s.rejectCreate {
					fail(422, "VALIDATION_ERROR")
					return
				}
				var body client.CreateDomainRequest
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					fail(400, "VALIDATION_ERROR")
					return
				}
				if _, ok := s.domains[body.Domain]; ok {
					fail(409, "CONFLICT")
					return
				}
				s.domains[body.Domain] = true
				s.pointers[body.Domain] = map[string]string{}
				s.events = append(s.events, "create-domain")
				reply(201, map[string]any{"domain": body.Domain, "ssl_enabled": false})
				return
			}
		}
		if len(parts) < 2 || parts[0] != "domains" {
			fail(404, "NOT_FOUND")
			return
		}
		domain := parts[1]
		enabled, ok := s.domains[domain]
		if !ok {
			// The mock explicitly confirms an empty child list after deletion; a real
			// parent-context 404 is never interpreted as proof that a pointer is gone.
			if len(parts) == 3 && parts[2] == "pointers" && r.Method == "GET" {
				reply(200, []any{})
				return
			}
			fail(404, "NOT_FOUND")
			return
		}
		if len(parts) == 2 {
			if r.Method == "GET" {
				if s.invisibleDomainReads > 0 {
					fail(404, "NOT_FOUND")
					return
				}
				names := make([]string, 0, len(s.pointers[domain]))
				for name := range s.pointers[domain] {
					names = append(names, name)
				}
				reply(200, map[string]any{"domain": domain, "mail_hosting": enabled, "ssl_enabled": false, "pointers": names})
				return
			}
			if r.Method == "DELETE" {
				if len(s.pointers[domain]) != 0 {
					t.Error("parent cleanup preceded child cleanup")
					fail(409, "CONFLICT")
					return
				}
				delete(s.domains, domain)
				delete(s.pointers, domain)
				s.events = append(s.events, "delete-domain")
				w.WriteHeader(204)
				return
			}
		}
		if len(parts) == 3 && parts[2] == "mail-status" && r.Method == "PATCH" {
			s.patches++
			if s.failPatch {
				s.failPatch = false
				fail(422, "VALIDATION_ERROR")
				return
			}
			var body struct {
				Enabled bool `json:"enabled"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.domains[domain] = body.Enabled
			w.WriteHeader(200)
			return
		}
		if len(parts) == 3 && parts[2] == "dns" && r.Method == "GET" {
			var dkim, verification any
			if !s.nullable {
				dkim = map[string]string{"type": "TXT", "name": "s._domainkey", "value": "v=DKIM1; k=rsa; p=SYNTHETIC + / ="}
				verification = map[string]string{"type": "TXT", "name": "_da-verify-synthetic", "value": "domain-verified"}
			}
			reply(200, map[string]any{"mx_records": []any{map[string]any{"priority": 10, "hostname": "mx.example.net."}}, "spf": map[string]string{"type": "TXT", "name": "@", "value": "v=spf1 include:example.net ~all"}, "dkim": dkim, "verification": verification})
			return
		}
		if len(parts) >= 3 && parts[2] == "pointers" {
			if len(parts) == 3 && r.Method == "GET" {
				values := []any{}
				if s.invisiblePointerReads > 0 {
					s.invisiblePointerReads--
					reply(200, values)
					return
				}
				for name, mode := range s.pointers[domain] {
					values = append(values, map[string]string{"pointer": name, "type": mode, "target": domain})
				}
				reply(200, values)
				return
			}
			if len(parts) == 3 && r.Method == "POST" {
				s.pointerCreates++
				var body client.CreateDomainPointerRequest
				_ = json.NewDecoder(r.Body).Decode(&body)
				if _, ok := s.pointers[domain][body.Pointer]; ok {
					fail(409, "CONFLICT")
					return
				}
				mode := "alias"
				if body.Alias != nil && !*body.Alias {
					mode = "redirect"
				}
				s.pointers[domain][body.Pointer] = mode
				s.events = append(s.events, "create-pointer")
				w.WriteHeader(201)
				return
			}
			if len(parts) == 4 && r.Method == "DELETE" {
				delete(s.pointers[domain], parts[3])
				s.events = append(s.events, "delete-pointer")
				w.WriteHeader(204)
				return
			}
		}
		fail(404, "NOT_FOUND")
	})
}
func domainFixture(t *testing.T) (*domainService, map[string]func() (tfprotov6.ProviderServer, error), secretChecks) {
	t.Helper()
	base, _, checks := acceptanceFixture(t)
	s := &domainService{key: base.expectedKey, domains: map[string]bool{}, pointers: map[string]map[string]string{}}
	server := httptest.NewServer(s.serve(t))
	t.Cleanup(server.Close)
	factories := map[string]func() (tfprotov6.ProviderServer, error){"mxroute": providerserver.NewProtocol6WithError(&mxrouteProvider{version: "test", newClient: func(host, user, key, version string) (*client.Client, error) {
		return client.NewMock(host, user, key, version, server.URL)
	}})}
	return s, factories, checks
}

const domainHeader = `
terraform {
 required_version = ">= 1.11.1, < 2.0.0"
 required_providers { mxroute = { source = "registry.terraform.io/cmdennis/mxroute" } }
}
provider "mxroute" {
 server = "mail.example.com"
 username = "owner"
}
`

func domainConfig(hosting bool, pointer bool) string {
	config := domainHeader + fmt.Sprintf(`
resource "mxroute_domain" "test" {
 domain = "example.com"
 mail_hosting = %t
}
`, hosting)
	if pointer {
		config += `
resource "mxroute_domain_pointer" "test" {
 domain = mxroute_domain.test.domain
 pointer = "alias.example.net"
 type = "alias"
}
`
	}
	return config
}
func TestOfflineDomainWorkflow(t *testing.T) {
	s, factories, checks := domainFixture(t)
	cfg := domainConfig(false, true) + `
data "mxroute_verification_key" "test" {}
data "mxroute_domains" "test" { depends_on = [mxroute_domain.test] }
data "mxroute_domain" "test" { domain = mxroute_domain.test.domain }
data "mxroute_domain_pointers" "test" {
 domain = mxroute_domain.test.domain
 depends_on = [mxroute_domain_pointer.test]
}
data "mxroute_domain_pointer" "test" {
 domain = mxroute_domain_pointer.test.domain
 pointer = mxroute_domain_pointer.test.pointer
}
data "mxroute_dns_records" "test" { domain = mxroute_domain.test.domain }
`
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		{Config: cfg, Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr("mxroute_domain.test", "mail_hosting", "false"),
			resource.TestCheckResourceAttr("mxroute_domain_pointer.test", "target", "example.com"),
			resource.TestCheckResourceAttr("data.mxroute_domains.test", "domains.#", "1"),
			resource.TestCheckResourceAttr("data.mxroute_domain_pointers.test", "pointers.#", "1"),
			resource.TestCheckResourceAttr("data.mxroute_domain.test", "mail_hosting", "false"),
			resource.TestCheckResourceAttr("data.mxroute_domain_pointer.test", "type", "alias"),
			resource.TestCheckResourceAttr("data.mxroute_verification_key.test", "record.value", "domain-verified"),
			resource.TestCheckResourceAttr("data.mxroute_dns_records.test", "dkim.value", "v=DKIM1; k=rsa; p=SYNTHETIC + / ="),
		)},
		{Config: cfg, PlanOnly: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan(), checks}}},
		{RefreshState: true},
		{Config: cfg, ResourceName: "mxroute_domain.test", ImportState: true, ImportStateId: "EXAMPLE.COM.", ImportStateVerify: true},
		{Config: cfg, ResourceName: "mxroute_domain_pointer.test", ImportState: true, ImportStateId: "EXAMPLE.COM./ALIAS.EXAMPLE.NET.", ImportStateVerify: true},
		{Config: cfg, PlanOnly: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
		{Config: strings.Replace(cfg, "mail_hosting = false", "mail_hosting = true", 1)},
		{Config: strings.Replace(cfg, `type = "alias"`, `type = "redirect"`, 1), PlanOnly: true, ExpectNonEmptyPlan: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectResourceAction("mxroute_domain_pointer.test", plancheck.ResourceActionDestroyBeforeCreate)}}},
	}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creates != 1 || s.patches != 2 || s.pointerCreates != 1 || len(s.domains) != 0 {
		t.Fatal("workflow did not converge or clean up")
	}
	n := len(s.events)
	if n < 2 || s.events[n-2] != "delete-pointer" || s.events[n-1] != "delete-domain" {
		t.Fatal("dependency cleanup order incorrect")
	}
}
func TestOfflineDomainPartialCreateRecovery(t *testing.T) {
	s, factories, checks := domainFixture(t)
	s.failPatch = true
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		{Config: domainConfig(false, false), ExpectNonEmptyPlan: true, Check: resource.TestCheckResourceAttr("mxroute_domain.test", "mail_hosting_pending", "true")},
		{Config: domainConfig(false, false), Check: resource.TestCheckResourceAttr("mxroute_domain.test", "mail_hosting", "false")},
		{Config: domainConfig(false, false), PlanOnly: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
	}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creates != 1 || s.patches != 2 {
		t.Fatal("partial create repeated creation instead of recovering")
	}
}
func TestOfflineDomainDriftAndDeletion(t *testing.T) {
	s, factories, checks := domainFixture(t)
	cfg := domainConfig(false, true)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		{Config: cfg},
		{PreConfig: func() { s.mu.Lock(); s.domains["example.com"] = true; s.mu.Unlock() }, Config: cfg, PlanOnly: true, ExpectNonEmptyPlan: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectResourceAction("mxroute_domain.test", plancheck.ResourceActionUpdate)}}},
		{Config: cfg},
		{PreConfig: func() { s.mu.Lock(); s.pointers["example.com"]["alias.example.net"] = "redirect"; s.mu.Unlock() }, Config: cfg, PlanOnly: true, ExpectNonEmptyPlan: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectResourceAction("mxroute_domain_pointer.test", plancheck.ResourceActionDestroyBeforeCreate)}}},
		{Config: cfg},
		{PreConfig: func() { s.mu.Lock(); delete(s.pointers["example.com"], "alias.example.net"); s.mu.Unlock() }, Config: cfg},
		{PreConfig: func() {
			s.mu.Lock()
			delete(s.domains, "example.com")
			delete(s.pointers, "example.com")
			s.mu.Unlock()
		}, Config: cfg},
	}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creates != 2 || s.pointerCreates != 4 {
		t.Fatal("external drift/deletion was not repaired")
	}
}
func TestOfflineDomainReadFailuresPreserveState(t *testing.T) {
	s, factories, checks := domainFixture(t)
	cfg := domainConfig(false, true)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		{Config: cfg},
		{PreConfig: func() { s.mu.Lock(); s.malformed = true; s.mu.Unlock() }, Config: cfg, PlanOnly: true, ExpectError: regexp.MustCompile("incomplete or invalid API response")},
		{PreConfig: func() { s.mu.Lock(); s.malformed = false; s.deny = true; s.mu.Unlock() }, Config: cfg, PlanOnly: true, ExpectError: regexp.MustCompile("credential role")},
		{PreConfig: func() { s.mu.Lock(); s.deny = false; s.mu.Unlock() }, Config: cfg, PlanOnly: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
	}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creates != 1 || s.pointerCreates != 1 {
		t.Fatal("read failures caused state loss")
	}
}
func TestOfflineDNSNullRefresh(t *testing.T) {
	s, factories, checks := domainFixture(t)
	cfg := domainConfig(true, false) + `data "mxroute_dns_records" "test" { domain = mxroute_domain.test.domain }`
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		{Config: cfg, Check: resource.TestCheckResourceAttrSet("data.mxroute_dns_records.test", "dkim.value")},
		{PreConfig: func() { s.mu.Lock(); s.nullable = true; s.mu.Unlock() }, Config: cfg, Check: resource.ComposeTestCheckFunc(resource.TestCheckNoResourceAttr("data.mxroute_dns_records.test", "dkim.value"), resource.TestCheckNoResourceAttr("data.mxroute_dns_records.test", "verification.value"))},
	}})
}
func TestVerificationWait(t *testing.T) {
	s, _, _ := domainFixture(t)
	server := httptest.NewServer(s.serve(t))
	defer server.Close()
	c, err := client.NewMock("mail.example.com", "owner", s.key, "test", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	previous := lookupTXT
	t.Cleanup(func() { lookupTXT = previous })
	calls := 0
	lookupTXT = func(_ context.Context, name string) ([]string, error) {
		calls++
		if name != "_da-verify-synthetic.alias.example.net" {
			t.Error("pointer did not verify its own name")
		}
		if calls == 1 {
			return nil, &net.DNSError{IsNotFound: true}
		}
		return []string{"domain-verified"}, nil
	}
	if err := waitVerification(context.Background(), c, "alias.example.net", 2); err != nil || calls != 2 {
		t.Fatal("recognized DNS wait failed")
	}
	lookupTXT = func(context.Context, string) ([]string, error) { return []string{"wrong-value"}, nil }
	if err := waitVerification(context.Background(), c, "alias.example.net", 1); err == nil {
		t.Fatal("missing TXT did not time out")
	}
	lookupTXT = func(context.Context, string) ([]string, error) {
		t.Fatal("disabled DNS preflight performed lookup")
		return nil, nil
	}
	if err := waitVerification(context.Background(), c, "example.com", 0); err != nil {
		t.Fatal(err)
	}
}

func TestOfflineOnboardingDNSAndVisibilityWait(t *testing.T) {
	s, factories, checks := domainFixture(t)
	s.invisibleDomainReads = 1
	s.invisiblePointerReads = 1
	previous := lookupTXT
	t.Cleanup(func() { lookupTXT = previous })
	seen := map[string]bool{}
	lookupTXT = func(_ context.Context, name string) ([]string, error) {
		seen[name] = true
		return []string{"domain-verified"}, nil
	}
	cfg := domainConfig(true, true) + `
data "mxroute_verification_key" "onboarding" {}
# Terraform's built-in object represents the external DNS provider in this
# offline workflow. The injected TXT resolver stands in for DNS propagation.
resource "terraform_data" "verification" {
 input = data.mxroute_verification_key.onboarding.record
}
resource "terraform_data" "pointer_verification" {
 input = {
  name = "${data.mxroute_verification_key.onboarding.record.name}.alias.example.net"
  value = data.mxroute_verification_key.onboarding.record.value
 }
}
`
	cfg = strings.Replace(cfg, `domain = "example.com"`, "domain = \"example.com\"\ndepends_on = [terraform_data.verification]", 1)
	cfg = strings.Replace(cfg, `pointer = "alias.example.net"`, "pointer = \"alias.example.net\"\ndepends_on = [terraform_data.pointer_verification]", 1)
	cfg = strings.Replace(cfg, `mail_hosting = true`, "mail_hosting = true\nverification_wait_seconds = 2\nvisibility_wait_seconds = 2", 1)
	cfg = strings.Replace(cfg, `type = "alias"`, "type = \"alias\"\nverification_wait_seconds = 2\nvisibility_wait_seconds = 2", 1)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{{Config: cfg}}})
	if s.invisibleDomainReads != 0 || s.invisiblePointerReads != 0 {
		t.Fatal("visibility polling was not exercised")
	}
	if !seen["_da-verify-synthetic.example.com"] || !seen["_da-verify-synthetic.alias.example.net"] {
		t.Fatal("domain and pointer verification were not independent")
	}
}
func TestOfflineFailedDNSPreventsCreation(t *testing.T) {
	s, factories, checks := domainFixture(t)
	previous := lookupTXT
	t.Cleanup(func() { lookupTXT = previous })
	lookupTXT = func(context.Context, string) ([]string, error) { return nil, &net.DNSError{IsNotFound: true} }
	cfg := strings.Replace(domainConfig(false, false), `mail_hosting = false`, "mail_hosting = false\nverification_wait_seconds = 1", 1)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{{Config: cfg, ExpectError: regexp.MustCompile("readiness wait expired")}}})
	if s.creates != 0 {
		t.Fatal("domain created without successful requested DNS preflight")
	}
}
func TestOfflineDomainValidationErrorNotRetried(t *testing.T) {
	s, factories, checks := domainFixture(t)
	s.rejectCreate = true
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{{Config: domainConfig(false, false), ExpectError: regexp.MustCompile("documented API constraints")}}})
	if s.creates != 1 {
		t.Fatal("unrelated validation error was retried")
	}
}
func TestOfflineDomainServerDefault(t *testing.T) {
	s, factories, checks := domainFixture(t)
	cfg := strings.Replace(domainConfig(true, false), "mail_hosting = true", "", 1)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		{Config: cfg, Check: resource.TestCheckResourceAttr("mxroute_domain.test", "mail_hosting", "true")},
		{Config: cfg, PlanOnly: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
	}})
	if s.patches != 0 {
		t.Fatal("omitted hosting sent an invented default")
	}
}
