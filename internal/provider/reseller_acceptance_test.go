package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// The synthetic reseller service models only documented routes. Its package
// reassignment behavior is a switchable assumption because precedence between
// package quota and user overrides is live-unverified (C04).
type mockPackage struct {
	quotaGB                                 *float64
	domains, accounts, forwarders, pointers *int64
}
type mockUser struct {
	email, domain, pkg string
	suspended          bool
	limit              *int64
	used               float64
}
type resellerService struct {
	mu                                                                                                    sync.Mutex
	key, password, url                                                                                    string
	packages                                                                                              map[string]*mockPackage
	users                                                                                                 map[string]*mockUser
	userPatches, packagePatches                                                                           [][]string
	creates, packageCreates, rotations, packageChanges, suspends, unsuspends, userDeletes, packageDeletes int
	failedWrites, impersonatedReads                                                                       int
	preserveOverride, failUserPatch, malformed, deny, detailMissing, caseVariant                          bool
}

func ptr[T any](v T) *T { return &v }
func (p *mockPackage) quotaMB() *int64 {
	if p.quotaGB == nil {
		return nil
	}
	return ptr(int64(*p.quotaGB * 1024))
}
func mockLimit(value string, fallback *int64) (*int64, bool) {
	if value == "" {
		return fallback, true
	}
	if value == unlimited {
		return nil, true
	}
	n, err := strconv.ParseInt(value, 10, 64)
	return &n, err == nil && n >= 0
}
func (s *resellerService) serve(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.Header.Get("X-API-Key") != s.key || r.Header.Get("X-Server") != "mail.example.com" {
			t.Error("unexpected mock credential context")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fail := func(status int, code string) {
			if r.Method != "GET" {
				s.failedWrites++
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]string{"code": code}})
		}
		reply := func(data any) { _ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data}) }
		user := r.Header.Get("X-Username")
		p := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if p[0] != "reseller" {
			// Account routes act on the plain reseller or an owned impersonation target.
			effective := user
			if owner, target, ok := strings.Cut(user, "|"); ok {
				if _, owned := s.users[target]; owner != "reseller" || !owned {
					fail(403, "FORBIDDEN")
					return
				}
				effective = target
				s.impersonatedReads++
			} else if user != "reseller" {
				fail(403, "FORBIDDEN")
				return
			}
			domain := "reseller.example.net"
			if u, ok := s.users[effective]; ok {
				domain = u.domain
			}
			switch {
			case r.Method == "GET" && r.URL.Path == "/quota":
				_ = json.NewEncoder(w).Encode(map[string]any{"username": effective, "total_used": int64(1024), "total_limit": int64(0), "percent_used": 0.0, "breakdown": map[string]int64{"email": 1024, "web": 0, "databases": 0, "backups": 0, "other": 0}, "grace_period": nil, "updated_at": "2026-10-19T08:00:00-04:00"})
			case r.Method == "GET" && r.URL.Path == "/domains":
				reply([]string{domain})
			default:
				t.Error("unexpected account operation")
				fail(400, "VALIDATION_ERROR")
			}
			return
		}
		if user != "reseller" {
			if strings.Contains(user, "|") {
				t.Error("impersonated reseller request reached the API")
			}
			fail(403, "FORBIDDEN")
			return
		}
		if r.Method == "GET" && s.deny {
			fail(403, "FORBIDDEN")
			return
		}
		if r.Method == "GET" && s.malformed {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		names := func(keys []string) []string {
			slices.Sort(keys)
			if s.caseVariant {
				for _, k := range slices.Clone(keys) {
					keys = append(keys, strings.ToUpper(k))
				}
			}
			return keys
		}
		packageJSON := func(name string, v *mockPackage) map[string]any {
			return map[string]any{"name": name, "settings": map[string]any{"quota_gb": v.quotaGB, "quota_unlimited": v.quotaGB == nil, "domains": v.domains, "email_accounts": v.accounts, "email_forwarders": v.forwarders, "domain_pointers": v.pointers}}
		}
		userJSON := func(name string, u *mockUser) map[string]any {
			return map[string]any{"username": name, "email": u.email, "domain": u.domain, "package": u.pkg, "suspended": u.suspended, "quota": map[string]any{"limit": u.limit, "used": u.used, "unlimited": u.limit == nil}}
		}
		switch {
		case len(p) == 2 && p[1] == "packages" && r.Method == "GET":
			keys := []string{}
			for k := range s.packages {
				keys = append(keys, k)
			}
			reply(names(keys))
		case len(p) == 2 && p[1] == "packages" && r.Method == "POST":
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				fail(400, "VALIDATION_ERROR")
				return
			}
			if _, ok := s.packages[body["name"]]; ok {
				fail(409, "CONFLICT")
				return
			}
			v := &mockPackage{quotaGB: ptr(1.0)}
			if q := body["quota"]; q == unlimited {
				v.quotaGB = nil
			} else if q != "" {
				gb, err := strconv.ParseFloat(q, 64)
				if err != nil {
					fail(422, "VALIDATION_ERROR")
					return
				}
				v.quotaGB = &gb
			}
			ok := make([]bool, 4)
			v.domains, ok[0] = mockLimit(body["domains"], ptr(int64(1)))
			v.accounts, ok[1] = mockLimit(body["email_accounts"], ptr(int64(100)))
			v.forwarders, ok[2] = mockLimit(body["email_forwarders"], ptr(int64(100)))
			v.pointers, ok[3] = mockLimit(body["domain_pointers"], ptr(int64(10)))
			if slices.Contains(ok, false) {
				fail(422, "VALIDATION_ERROR")
				return
			}
			s.packages[body["name"]] = v
			s.packageCreates++
			w.WriteHeader(201)
		case len(p) == 3 && p[1] == "packages":
			v, ok := s.packages[p[2]]
			if !ok {
				fail(404, "NOT_FOUND")
				return
			}
			switch r.Method {
			case "GET":
				if s.detailMissing {
					fail(404, "NOT_FOUND")
					return
				}
				reply(packageJSON(p[2], v))
			case "PATCH":
				var body map[string]string
				_ = json.NewDecoder(r.Body).Decode(&body)
				fields := []string{}
				for k, value := range body {
					fields = append(fields, k)
					target := map[string]**int64{"domains": &v.domains, "email_accounts": &v.accounts, "email_forwarders": &v.forwarders, "domain_pointers": &v.pointers}[k]
					if k == "quota" {
						if value == unlimited {
							v.quotaGB = nil
						} else {
							gb, _ := strconv.ParseFloat(value, 64)
							v.quotaGB = &gb
						}
					} else if target != nil {
						*target, _ = mockLimit(value, nil)
					} else {
						t.Error("unexpected package PATCH field")
					}
				}
				slices.Sort(fields)
				s.packagePatches = append(s.packagePatches, fields)
				w.WriteHeader(200)
			case "DELETE":
				for _, u := range s.users {
					if u.pkg == p[2] {
						fail(422, "BUSINESS_ERROR")
						return
					}
				}
				delete(s.packages, p[2])
				s.packageDeletes++
				w.WriteHeader(204)
			}
		case len(p) == 2 && p[1] == "users" && r.Method == "GET":
			keys := []string{}
			for k := range s.users {
				keys = append(keys, k)
			}
			reply(names(keys))
		case len(p) == 2 && p[1] == "users" && r.Method == "POST":
			var v client.CreateResellerUserRequest
			if json.NewDecoder(r.Body).Decode(&v) != nil {
				fail(400, "VALIDATION_ERROR")
				return
			}
			if v.Password != s.password {
				t.Error("reseller user password altered")
			}
			pkg, ok := s.packages[v.Package]
			if !ok {
				fail(422, "VALIDATION_ERROR")
				return
			}
			for k := range s.users {
				if strings.EqualFold(k, v.Username) {
					fail(409, "CONFLICT")
					return
				}
			}
			s.users[v.Username] = &mockUser{email: v.Email, domain: v.Username + ".example.net", pkg: v.Package, limit: pkg.quotaMB(), used: 12.5}
			s.creates++
			s.rotations++
			w.WriteHeader(201)
		case len(p) >= 3 && p[1] == "users":
			u, ok := s.users[p[2]]
			if !ok {
				fail(404, "NOT_FOUND")
				return
			}
			action := r.Method
			if len(p) == 4 {
				action += " " + p[3]
			}
			switch action {
			case "GET":
				if s.detailMissing {
					fail(404, "NOT_FOUND")
					return
				}
				reply(userJSON(p[2], u))
			case "PATCH":
				if s.failUserPatch {
					s.failUserPatch = false
					fail(422, "VALIDATION_ERROR")
					return
				}
				var body map[string]string
				_ = json.NewDecoder(r.Body).Decode(&body)
				fields := []string{}
				for k, value := range body {
					fields = append(fields, k)
					switch k {
					case "password":
						if value != s.password {
							t.Error("rotated reseller user password altered")
						}
						s.rotations++
					case "quota":
						u.limit, _ = mockLimit(value, nil)
					default:
						t.Error("unexpected user PATCH field")
					}
				}
				slices.Sort(fields)
				s.userPatches = append(s.userPatches, fields)
				w.WriteHeader(200)
			case "DELETE":
				delete(s.users, p[2])
				s.userDeletes++
				w.WriteHeader(204)
			case "POST suspend":
				u.suspended = true
				s.suspends++
				w.WriteHeader(200)
			case "POST unsuspend":
				u.suspended = false
				s.unsuspends++
				w.WriteHeader(200)
			case "PATCH package":
				var body map[string]string
				_ = json.NewDecoder(r.Body).Decode(&body)
				pkg, ok := s.packages[body["package"]]
				if !ok {
					fail(422, "VALIDATION_ERROR")
					return
				}
				u.pkg = body["package"]
				if !s.preserveOverride {
					u.limit = pkg.quotaMB()
				}
				s.packageChanges++
				reply(userJSON(p[2], u))
			default:
				t.Error("unexpected user operation")
				fail(400, "VALIDATION_ERROR")
			}
		default:
			t.Error("unexpected reseller operation")
			fail(400, "VALIDATION_ERROR")
		}
	})
}
func resellerFixture(t *testing.T) (*resellerService, map[string]func() (tfprotov6.ProviderServer, error), secretChecks) {
	t.Helper()
	base, _, checks := acceptanceFixture(t)
	s := &resellerService{key: base.expectedKey, password: base.expectedPassword, packages: map[string]*mockPackage{}, users: map[string]*mockUser{}}
	server := httptest.NewServer(s.serve(t))
	t.Cleanup(server.Close)
	s.url = server.URL
	factories := map[string]func() (tfprotov6.ProviderServer, error){"mxroute": providerserver.NewProtocol6WithError(&mxrouteProvider{version: "test", newClient: func(host, user, key, version string) (*client.Client, error) {
		return client.NewMock(host, user, key, version, server.URL)
	}})}
	return s, factories, checks
}

var resellerHeader = strings.Replace(fixtureHeader, `username = "owner"`, `username = "reseller"`, 1)

func packageBlock(label, name, attrs string) string {
	return fmt.Sprintf("resource \"mxroute_reseller_package\" %q {\nname = %q\n%s\n}\n", label, name, attrs)
}

const premiumLimits = `quota_gb = "unlimited"
domains = "unlimited"
email_accounts = "0"
email_forwarders = "25"
domain_pointers = "0"`

func resellerPackages(starter string) string {
	return packageBlock("starter", "starter", starter) + packageBlock("premium", "Premium-2", premiumLimits)
}

type userOpts struct {
	name, email, pkg, quota, suspended string
	version                            int
	password                           bool
}

func (o userOpts) hcl() string {
	attrs := ""
	if o.quota != "" {
		attrs += fmt.Sprintf("quota_mb = %q\n", o.quota)
	}
	if o.suspended != "" {
		attrs += "suspended = " + o.suspended + "\n"
	}
	if o.version > 0 {
		attrs += fmt.Sprintf("password_wo_version = %d\n", o.version)
	}
	if o.password {
		attrs += "password_wo = var.fixture_password\n"
	}
	pkg := strconv.Quote(o.pkg)
	if o.pkg == "starter" || o.pkg == "premium" {
		pkg = "mxroute_reseller_package." + o.pkg + ".name"
	}
	return fmt.Sprintf("resource \"mxroute_reseller_user\" \"customer\" {\nusername = %q\nemail = %q\npackage = %s\n%s}\n", o.name, o.email, pkg, attrs)
}
func customer() userOpts {
	return userOpts{name: "customer1", email: "owner@example.net", pkg: "starter", version: 1, password: true}
}
func (o userOpts) with(change func(*userOpts)) userOpts { change(&o); return o }

func TestOfflineResellerLifecycle(t *testing.T) {
	s, factories, checks := resellerFixture(t)
	rotatedPassword := testutil.Secret(t)
	checks.secrets = append(checks.secrets, rotatedPassword)
	data := `
data "mxroute_reseller_packages" "all" { depends_on = [mxroute_reseller_package.starter, mxroute_reseller_package.premium] }
data "mxroute_reseller_package" "premium" { name = mxroute_reseller_package.premium.name }
data "mxroute_reseller_users" "all" { depends_on = [mxroute_reseller_user.customer] }
data "mxroute_reseller_user" "customer" { username = mxroute_reseller_user.customer.username }
`
	initial := resellerHeader + resellerPackages("") + customer().hcl() + data
	first := mailStep(initial, checks)
	first.Check = resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttr("mxroute_reseller_package.starter", "quota_gb", "1"),
		resource.TestCheckResourceAttr("mxroute_reseller_package.starter", "domains", "1"),
		resource.TestCheckResourceAttr("mxroute_reseller_package.starter", "email_accounts", "100"),
		resource.TestCheckResourceAttr("mxroute_reseller_package.starter", "email_forwarders", "100"),
		resource.TestCheckResourceAttr("mxroute_reseller_package.starter", "domain_pointers", "10"),
		resource.TestCheckResourceAttr("data.mxroute_reseller_package.premium", "quota_gb", "unlimited"),
		resource.TestCheckResourceAttr("data.mxroute_reseller_package.premium", "email_accounts", "0"),
		resource.TestCheckResourceAttr("data.mxroute_reseller_package.premium", "domains", "unlimited"),
		resource.TestCheckResourceAttr("mxroute_reseller_user.customer", "quota_mb", "1024"),
		resource.TestCheckResourceAttr("mxroute_reseller_user.customer", "suspended", "false"),
		resource.TestCheckResourceAttr("mxroute_reseller_user.customer", "current_email", "owner@example.net"),
		resource.TestCheckResourceAttr("mxroute_reseller_user.customer", "domain", "customer1.example.net"),
		resource.TestCheckResourceAttr("data.mxroute_reseller_packages.all", "names.#", "2"),
		resource.TestCheckTypeSetElemAttr("data.mxroute_reseller_packages.all", "names.*", "Premium-2"),
		resource.TestCheckResourceAttr("data.mxroute_reseller_users.all", "usernames.#", "1"),
		resource.TestCheckResourceAttr("data.mxroute_reseller_user.customer", "package", "starter"),
		resource.TestCheckResourceAttr("data.mxroute_reseller_user.customer", "quota_mb", "1024"),
		resource.TestCheckResourceAttr("data.mxroute_reseller_user.customer", "quota_used_mb", "12.5"),
	)
	limits := resellerHeader + resellerPackages(`domains = "5"`)
	quota := customer().with(func(o *userOpts) { o.quota, o.password = "2048", false })
	rotated := quota.with(func(o *userOpts) { o.version, o.password = 2, true })
	rotateStep := mailStep(limits+rotated.hcl(), checks)
	rotateStep.PreConfig = func() {
		t.Setenv("TF_VAR_fixture_password", rotatedPassword)
		s.mu.Lock()
		s.password = rotatedPassword
		s.mu.Unlock()
	}
	switched := quota.with(func(o *userOpts) { o.version, o.pkg = 2, "premium" })
	suspended := switched.with(func(o *userOpts) { o.suspended = "true" })
	active := switched.with(func(o *userOpts) { o.suspended = "false" })
	final := limits + active.hcl()
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		first, emptyMailPlan(initial, checks),
		mailStep(limits+quota.hcl(), checks), rotateStep,
		mailStep(limits+switched.hcl(), checks),
		mailStep(limits+suspended.hcl(), checks), emptyMailPlan(limits+suspended.hcl(), checks),
		mailStep(final, checks),
		{Config: final, ResourceName: "mxroute_reseller_user.customer", ImportState: true, ImportStateId: "customer1", ImportStateVerify: true, ImportStateVerifyIdentifierAttribute: "username", ImportStateVerifyIgnore: []string{"password_wo_version"}},
		{Config: final, ResourceName: "mxroute_reseller_package.premium", ImportState: true, ImportStateId: "Premium-2", ImportStateVerify: true, ImportStateVerifyIdentifierAttribute: "name"},
		emptyMailPlan(final, checks),
	}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creates != 1 || s.packageCreates != 2 || s.rotations != 2 || s.packageChanges != 1 || s.suspends != 1 || s.unsuspends != 1 {
		t.Fatal("reseller lifecycle duplicated identities, replayed writes or missed transitions")
	}
	if fmt.Sprint(s.userPatches) != "[[quota] [password] [quota]]" || fmt.Sprint(s.packagePatches) != "[[domains]]" {
		t.Fatalf("updates sent unrelated fields: users %v packages %v", s.userPatches, s.packagePatches)
	}
	if len(s.users) != 0 || len(s.packages) != 0 || s.failedWrites != 0 {
		t.Fatal("cleanup did not remove users before their packages")
	}
}

func TestOfflineResellerQuotaOverrideSequencing(t *testing.T) {
	for _, preserved := range []bool{false, true} {
		t.Run(fmt.Sprintf("override preserved by reassignment=%v", preserved), func(t *testing.T) {
			s, factories, checks := resellerFixture(t)
			s.preserveOverride = preserved
			user := customer().with(func(o *userOpts) { o.quota = "2048" })
			switched := user.with(func(o *userOpts) { o.pkg, o.password = "premium", false })
			config := resellerHeader + resellerPackages("")
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
				mailStep(config+user.hcl(), checks), mailStep(config+switched.hcl(), checks), emptyMailPlan(config+switched.hcl(), checks),
			}})
			// Creation applies the override after the package default. Reassignment
			// re-sends it only when readback shows the API replaced it.
			want := 2
			if preserved {
				want = 1
			}
			if s.packageChanges != 1 || len(s.userPatches) != want {
				t.Fatalf("package then override sequencing failed: %d changes, patches %v", s.packageChanges, s.userPatches)
			}
		})
	}
}

func TestOfflineResellerPartialUpdateRecovery(t *testing.T) {
	s, factories, checks := resellerFixture(t)
	config := resellerHeader + resellerPackages("")
	user := customer().with(func(o *userOpts) { o.quota = "2048" })
	switched := config + user.with(func(o *userOpts) { o.pkg, o.password = "premium", false }).hcl()
	recovered := mailStep(switched, checks)
	recovered.PreConfig = func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if u := s.users["customer1"]; u.pkg != "Premium-2" || u.limit != nil {
			t.Error("mock did not record the confirmed reassignment")
		}
	}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		mailStep(config+user.hcl(), checks),
		{Config: switched, PreConfig: func() { s.mu.Lock(); s.failUserPatch = true; s.mu.Unlock() }, ExpectError: regexp.MustCompile(`documented API\s+constraints`)},
		recovered, emptyMailPlan(switched, checks),
	}})
	// The recovery apply re-sends only the failed quota override: no second
	// reassignment, recreation or password write.
	if s.creates != 1 || s.packageChanges != 1 || s.rotations != 1 || fmt.Sprint(s.userPatches) != "[[quota] [quota]]" || s.failedWrites != 1 {
		t.Fatalf("partial update was not recovered in place: changes %d patches %v", s.packageChanges, s.userPatches)
	}
}

func TestOfflineResellerDriftAndReplacement(t *testing.T) {
	s, factories, checks := resellerFixture(t)
	config := resellerHeader + resellerPackages("") + customer().with(func(o *userOpts) { o.quota, o.suspended = "2048", "false" }).hcl()
	drift := mailStep(config, checks)
	drift.PreConfig = func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		u := s.users["customer1"]
		u.suspended, u.pkg, u.limit, u.email = true, "Premium-2", ptr(int64(10)), "changed@example.net"
		s.packages["Premium-2"].forwarders = ptr(int64(30))
		s.packages["starter"].domains = ptr(int64(7))
	}
	drift.Check = resource.ComposeTestCheckFunc(
		resource.TestCheckResourceAttr("mxroute_reseller_user.customer", "email", "owner@example.net"),
		resource.TestCheckResourceAttr("mxroute_reseller_user.customer", "current_email", "changed@example.net"),
		resource.TestCheckResourceAttr("mxroute_reseller_package.starter", "domains", "7"),
	)
	emailChange := strings.Replace(config, "owner@example.net", "new@example.net", 1)
	deleted := mailStep(config, checks)
	deleted.PreConfig = func() { s.mu.Lock(); defer s.mu.Unlock(); delete(s.users, "customer1") }
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		mailStep(config, checks), drift, emptyMailPlan(config, checks),
		{Config: emailChange, PlanOnly: true, ExpectNonEmptyPlan: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectResourceAction("mxroute_reseller_user.customer", plancheck.ResourceActionDestroyBeforeCreate)}}},
		deleted, emptyMailPlan(config, checks),
	}})
	if s.creates != 2 || s.unsuspends != 1 || s.packageChanges != 1 || fmt.Sprint(s.packagePatches) != "[[email_forwarders]]" {
		t.Fatalf("drift was not repaired in place or deletion not recreated: patches %v", s.packagePatches)
	}
}

func TestOfflineResellerPackageInUse(t *testing.T) {
	s, factories, checks := resellerFixture(t)
	config := resellerHeader + packageBlock("starter", "starter", "")
	blocked := resource.TestStep{Config: resellerHeader, ExpectError: regexp.MustCompile(`account limits and operation\s+prerequisites`), PreConfig: func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.users["external"] = &mockUser{email: "other@example.org", domain: "external.example.org", pkg: "starter", limit: ptr(int64(1024))}
	}}
	released := resource.TestStep{Config: resellerHeader, PreConfig: func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.users["external"]; !ok || s.packages["starter"] == nil {
			t.Error("in-use failure removed a user or the package")
		}
		delete(s.users, "external")
	}}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{mailStep(config, checks), blocked, released}})
	if s.packageDeletes != 1 || s.userDeletes != 0 || s.failedWrites != 1 {
		t.Fatal("package-in-use deletion cascaded or was not surfaced")
	}
}

func TestOfflineResellerCreationRules(t *testing.T) {
	cases := map[string]struct {
		config string
		seed   func(*resellerService)
		want   string
	}{
		"uppercase new username": {customer().with(func(o *userOpts) { o.name, o.pkg = "Customer1", "seeded" }).hcl(), nil, "Invalid new username"},
		"long new username":      {customer().with(func(o *userOpts) { o.name, o.pkg = "customer_abc", "seeded" }).hcl(), nil, "Invalid new username"},
		"missing password":       {customer().with(func(o *userOpts) { o.version, o.password, o.pkg = 0, false, "seeded" }).hcl(), nil, "Missing password"},
		"existing user": {customer().with(func(o *userOpts) { o.pkg = "seeded" }).hcl(), func(s *resellerService) {
			s.users["customer1"] = &mockUser{email: "owner@example.net", domain: "customer1.example.net", pkg: "seeded"}
		}, "Reseller user already exists"},
		"existing user case variant": {customer().with(func(o *userOpts) { o.pkg = "seeded" }).hcl(), func(s *resellerService) {
			s.users["CUSTOMER1"] = &mockUser{email: "owner@example.net", domain: "customer1.example.net", pkg: "seeded"}
		}, "multiple matching objects"},
		"existing package":              {packageBlock("starter", "seeded", ""), nil, "Reseller package already exists"},
		"existing package case variant": {packageBlock("starter", "SEEDED", ""), nil, "multiple matching objects"},
		"invalid limit":                 {packageBlock("starter", "fresh", `email_accounts = "01"`), nil, "Invalid limit"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s, factories, checks := resellerFixture(t)
			s.packages["seeded"] = &mockPackage{quotaGB: ptr(1.0), domains: ptr(int64(1)), accounts: ptr(int64(1)), forwarders: ptr(int64(1)), pointers: ptr(int64(1))}
			if c.seed != nil {
				c.seed(s)
			}
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
				{Config: resellerHeader + c.config, ExpectError: regexp.MustCompile(c.want)},
			}})
			if s.creates != 0 || s.packageCreates != 0 || s.failedWrites != 0 {
				t.Fatal("rejected configuration reached a create operation")
			}
		})
	}
}

func TestOfflineResellerReadFailuresPreserveState(t *testing.T) {
	for _, kind := range []string{"malformed", "forbidden", "detail 404", "case variant"} {
		t.Run(kind, func(t *testing.T) {
			s, factories, checks := resellerFixture(t)
			config := resellerHeader + resellerPackages("") + customer().hcl()
			set := func(value bool) {
				s.mu.Lock()
				defer s.mu.Unlock()
				switch kind {
				case "malformed":
					s.malformed = value
				case "forbidden":
					s.deny = value
				case "detail 404":
					s.detailMissing = value
				case "case variant":
					s.caseVariant = value
				}
			}
			recovered := emptyMailPlan(config, checks)
			recovered.PreConfig = func() { set(false) }
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
				mailStep(config, checks),
				{Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("MXroute operation failed"), PreConfig: func() { set(true) }},
				recovered,
			}})
			if s.creates != 1 || s.packageCreates != 2 || s.rotations != 1 {
				t.Fatal("failed reseller refresh lost state")
			}
		})
	}
}

func TestOfflineResellerImpersonationWorkflow(t *testing.T) {
	s, _, checks := resellerFixture(t)
	// Aliases need separate provider processes, so this test drives the
	// Terraform CLI with the test executable installed as a local plugin.
	terraform := localPlugin(t, checks, "MXROUTE_PHASE6_PROVIDER_ENDPOINT", s.url)
	write := func(config string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(checks.dir, "main.tf"), []byte(config), 0600); err != nil {
			t.Fatal("cannot write synthetic configuration")
		}
	}
	// Flags precede positional arguments; only state-changing commands take -input.
	command := func(args []string) []string {
		flags := []string{args[0], "-no-color"}
		if args[0] != "show" && args[0] != "output" {
			flags = append(flags, "-input=false")
		}
		return append(flags, args[1:]...)
	}
	run := func(args ...string) []byte {
		t.Helper()
		out, err := terraform(command(args)...)
		if err != nil {
			t.Fatalf("local Terraform command failed: %s", out)
		}
		return out
	}
	fails := func(want string, args ...string) {
		t.Helper()
		out, err := terraform(command(args)...)
		if err == nil || !regexp.MustCompile(want).Match(out) {
			t.Fatalf("expected failure matching %q: %s", want, out)
		}
	}
	// Stage 1 creates the identity with the plain reseller provider. The alias
	// uses a literal name and is only used after that identity exists.
	stage1 := resellerHeader + packageBlock("starter", "starter", "") + customer().hcl() + `
provider "mxroute" {
 alias = "customer"
 server = "mail.example.com"
 username = "reseller|customer1"
}
`
	stage2 := stage1 + `
data "mxroute_quota" "customer" { provider = mxroute.customer }
data "mxroute_domains" "customer" { provider = mxroute.customer }
output "quota_user" { value = data.mxroute_quota.customer.username }
output "domains" { value = data.mxroute_domains.customer.domains }
`
	write(stage1)
	run("plan", "-out=tfplan")
	if err := checks.inspect(json.RawMessage(run("show", "-json", "tfplan"))); err != nil {
		t.Fatal(err)
	}
	run("apply", "-auto-approve", "tfplan")
	write(stage2)
	run("apply", "-auto-approve")
	if err := checks.inspect(json.RawMessage(run("show", "-json"))); err != nil {
		t.Fatal(err)
	}
	var outputs struct {
		QuotaUser struct{ Value string } `json:"quota_user"`
		Domains   struct{ Value []string }
	}
	if json.Unmarshal(run("output", "-json"), &outputs) != nil || outputs.QuotaUser.Value != "customer1" || fmt.Sprint(outputs.Domains.Value) != "[customer1.example.net]" {
		t.Fatal("impersonated alias did not read the owned user's account")
	}
	run("plan", "-detailed-exitcode")
	write(stage2 + `data "mxroute_reseller_users" "wrong" { provider = mxroute.customer }`)
	fails(`Reseller provider required`, "plan")
	write(stage2 + `
provider "mxroute" {
 alias = "stranger"
 server = "mail.example.com"
 username = "reseller|stranger"
}
data "mxroute_quota" "stranger" { provider = mxroute.stranger }
`)
	fails(`credential\s+role`, "plan")
	write(stage2 + `
provider "mxroute" {
 alias = "ordinary"
 server = "mail.example.com"
 username = "ordinary"
}
data "mxroute_reseller_users" "ordinary" { provider = mxroute.ordinary }
`)
	fails(`credential\s+role`, "plan")
	write(stage2)
	run("destroy", "-auto-approve")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.impersonatedReads < 2 || s.creates != 1 || s.rotations != 1 || len(s.users) != 0 || len(s.packages) != 0 {
		t.Fatal("impersonated workflow did not target the owned user or failed cleanup")
	}
}

func TestOfflineResellerImpersonatedResourcesRejected(t *testing.T) {
	s, factories, checks := resellerFixture(t)
	impersonated := strings.Replace(fixtureHeader, `username = "owner"`, `username = "reseller|customer1"`, 1)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		{Config: impersonated + packageBlock("starter", "starter", ""), ExpectError: regexp.MustCompile("Reseller provider required")},
		{Config: impersonated + customer().with(func(o *userOpts) { o.pkg = "seeded" }).hcl(), ExpectError: regexp.MustCompile("Reseller provider required")},
	}})
	if s.packageCreates != 0 || s.creates != 0 || s.failedWrites != 0 {
		t.Fatal("impersonated reseller resource reached the API")
	}
}

func TestResellerConversions(t *testing.T) {
	check := func(got string, ok bool, want string, wantOK bool) {
		t.Helper()
		if ok != wantOK || (ok && got != want) {
			t.Fatalf("conversion: got %q/%v, want %q/%v", got, ok, want, wantOK)
		}
	}
	got, ok := countSetting(nil)
	check(got, ok, unlimited, true)
	got, ok = countSetting(ptr(int64(0)))
	check(got, ok, "0", true)
	_, ok = countSetting(ptr(int64(-1)))
	check("", ok, "", false)
	got, ok = packageQuota(nil, true)
	check(got, ok, unlimited, true)
	got, ok = packageQuota(ptr(1.5), false)
	check(got, ok, "1.5", true)
	got, ok = packageQuota(ptr(0.0), false)
	check(got, ok, "0", true)
	_, ok = packageQuota(nil, false)
	check("", ok, "", false)
	_, ok = packageQuota(ptr(2.0), true)
	check("", ok, "", false)
	got, ok = userQuota(&client.UserQuota{Limit: nil, Unlimited: ptr(true)})
	check(got, ok, unlimited, true)
	got, ok = userQuota(&client.UserQuota{Limit: ptr(int64(0)), Unlimited: ptr(false)})
	check(got, ok, "0", true)
	_, ok = userQuota(&client.UserQuota{Limit: ptr(int64(5)), Unlimited: ptr(true)})
	check("", ok, "", false)
	_, ok = userQuota(&client.UserQuota{Limit: nil, Unlimited: ptr(false)})
	check("", ok, "", false)

	for _, c := range []struct {
		value   string
		decimal bool
		valid   bool
	}{
		{"0", false, true}, {"100", false, true}, {unlimited, false, true}, {"01", false, false}, {"-1", false, false},
		{"1.5", false, false}, {"Unlimited", false, false}, {" 1", false, false}, {"", false, false}, {"1234567890123456789", false, false},
		{"0.5", true, true}, {"2", true, true}, {unlimited, true, true}, {"1.50", true, false}, {"1.", true, false},
		{".5", true, false}, {"01.5", true, false}, {"1e3", true, false}, {"1234567890", true, false},
	} {
		resp := &validator.StringResponse{}
		limitValidator{decimal: c.decimal}.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringValue(c.value)}, resp)
		if resp.Diagnostics.HasError() == c.valid {
			t.Fatalf("limit %q (decimal=%v) validity mismatch", c.value, c.decimal)
		}
	}
	if _, err := nameSet(context.Background(), []string{"a", "a"}, validPackageName); err == nil {
		t.Fatal("duplicate list member accepted")
	}
	if _, err := nameSet(context.Background(), []string{"bad name"}, validPackageName); err == nil {
		t.Fatal("invalid list member accepted")
	}
}
