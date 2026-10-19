package provider

// This file is the only resource registered by the Phase 1 acceptance provider.
// It is compiled only into tests, uses a synthetic loopback service, and cannot
// appear in a release binary or address any MXroute API operation.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
	"github.com/CMDennis/terraform-provider-mxroute/internal/schemautil"
	"github.com/CMDennis/terraform-provider-mxroute/internal/testutil"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
)

type fixtureProvider struct{ *mxrouteProvider }

func (p *fixtureProvider) Resources(context.Context) []func() fwresource.Resource {
	return []func() fwresource.Resource{func() fwresource.Resource { return &fixtureResource{} }}
}

// Terraform normally starts a separate provider process for each alias. The
// testing library reattaches all aliases to one server, so that behavior is
// verified using this test executable as an actual plugin in the alias test.
// This entry point does not exist in the production binary.
func TestMain(m *testing.M) {
	fixture, full := os.Getenv("MXROUTE_PHASE1_FIXTURE_ENDPOINT"), os.Getenv("MXROUTE_PHASE6_PROVIDER_ENDPOINT")
	if endpoint := fixture + full; endpoint != "" {
		if _, err := client.NewMock("mail.example.com", "ordinary", os.Getenv("MXROUTE_API_KEY"), "test", endpoint); err != nil || (fixture != "" && full != "") {
			os.Exit(2)
		}
		err := providerserver.Serve(context.Background(), func() fwprovider.Provider {
			p := &mxrouteProvider{version: "test", newClient: func(host, username, key, version string) (*client.Client, error) {
				return client.NewMock(host, username, key, version, endpoint)
			}}
			if fixture != "" {
				return &fixtureProvider{p}
			}
			return p
		}, providerserver.ServeOpts{Address: "registry.terraform.io/cmdennis/mxroute", ProtocolVersion: 6})
		if err != nil {
			os.Exit(2)
		}
		return
	}
	os.Exit(m.Run())
}

type fixtureModel struct {
	Name     types.String `tfsdk:"name"`
	Quota    types.Int64  `tfsdk:"quota"`
	Password types.String `tfsdk:"password_wo"`
	Version  types.Int64  `tfsdk:"password_wo_version"`
}
type fixtureResource struct{ c *client.Client }

var _ fwresource.ResourceWithConfigure = &fixtureResource{}
var _ fwresource.ResourceWithModifyPlan = &fixtureResource{}
var _ fwresource.ResourceWithImportState = &fixtureResource{}
var _ fwresource.ResourceWithUpgradeState = &fixtureResource{}

func (r *fixtureResource) Metadata(_ context.Context, req fwresource.MetadataRequest, resp *fwresource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_fixture"
}
func (r *fixtureResource) Schema(_ context.Context, _ fwresource.SchemaRequest, resp *fwresource.SchemaResponse) {
	attrs := schemautil.PasswordAttributes()
	attrs["name"] = schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attrs["quota"] = schema.Int64Attribute{Required: true}
	resp.Schema = schema.Schema{Version: schemautil.InitialVersion, Attributes: attrs}
}
func (r *fixtureResource) UpgradeState(context.Context) map[int64]fwresource.StateUpgrader {
	return schemautil.InitialStateUpgraders()
}
func (r *fixtureResource) Configure(_ context.Context, req fwresource.ConfigureRequest, resp *fwresource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected client", "Expected the MXroute client.")
		return
	}
	r.c = c
}
func (r *fixtureResource) ModifyPlan(ctx context.Context, req fwresource.ModifyPlanRequest, resp *fwresource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var m fixtureModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	prior := types.Int64Null()
	creating := req.State.Raw.IsNull()
	if !req.State.Raw.IsNull() {
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("password_wo_version"), &prior)...)
		var name types.String
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("name"), &name)...)
		creating = !m.Name.IsUnknown() && !m.Name.Equal(name)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	_, d := schemautil.PasswordChange(m.Password, m.Version, prior, creating, false, 8, 72)
	resp.Diagnostics.Append(d...)
}

// The synthetic HTTP payload is local test data, never an MXroute API model.
type fixtureObject struct {
	Name     string `json:"name"`
	Quota    int64  `json:"quota"`
	Password string `json:"password,omitempty"`
}

func (r *fixtureResource) write(ctx context.Context, cfg tfsdk.Config, plan tfsdk.Plan, state *tfsdk.State, creating bool, d *diag.Diagnostics) {
	var m, config fixtureModel
	d.Append(plan.Get(ctx, &m)...)
	d.Append(cfg.Get(ctx, &config)...)
	prior := types.Int64Null()
	if !creating {
		d.Append(state.GetAttribute(ctx, path.Root("password_wo_version"), &prior)...)
	}
	if d.HasError() {
		return
	}
	rotate, pd := schemautil.PasswordChange(config.Password, m.Version, prior, creating, true, 8, 72)
	d.Append(pd...)
	if d.HasError() {
		return
	}
	o := fixtureObject{Name: m.Name.ValueString(), Quota: m.Quota.ValueInt64()}
	if rotate {
		o.Password = config.Password.ValueString()
	}
	data, err := json.Marshal(o)
	if err != nil {
		d.AddError("Fixture encoding failed", "Unable to encode synthetic payload.")
		return
	}
	resp, err := r.c.Request(ctx, http.MethodPut, identity.Path("fixture", o.Name), bytes.NewReader(data))
	if err != nil {
		d.AddError("Fixture write failed", "Loopback request failed.")
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		d.AddError("Fixture write failed", "Unexpected loopback status.")
		return
	}
	m.Password = types.StringNull()
	d.Append(state.Set(ctx, &m)...)
}
func (r *fixtureResource) Create(ctx context.Context, req fwresource.CreateRequest, resp *fwresource.CreateResponse) {
	r.write(ctx, req.Config, req.Plan, &resp.State, true, &resp.Diagnostics)
}
func (r *fixtureResource) Update(ctx context.Context, req fwresource.UpdateRequest, resp *fwresource.UpdateResponse) {
	// Framework initializes response State from prior state; explicitly retain
	// it here so the fixture's prior rotation version is available to write.
	resp.State = req.State
	r.write(ctx, req.Config, req.Plan, &resp.State, false, &resp.Diagnostics)
}
func (r *fixtureResource) Read(ctx context.Context, req fwresource.ReadRequest, resp *fwresource.ReadResponse) {
	var m fixtureModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	res, err := r.c.Request(ctx, http.MethodGet, identity.Path("fixture", m.Name.ValueString()), nil)
	if err != nil {
		resp.Diagnostics.AddError("Fixture read failed", "Loopback request failed.")
		return
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	var o fixtureObject
	if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&o) != nil {
		resp.Diagnostics.AddError("Fixture read failed", "Invalid loopback response.")
		return
	}
	m.Name, m.Quota, m.Password = types.StringValue(o.Name), types.Int64Value(o.Quota), types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *fixtureResource) Delete(ctx context.Context, req fwresource.DeleteRequest, resp *fwresource.DeleteResponse) {
	var m fixtureModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	res, err := r.c.Request(ctx, http.MethodDelete, identity.Path("fixture", m.Name.ValueString()), nil)
	if err != nil {
		resp.Diagnostics.AddError("Fixture delete failed", "Loopback request failed.")
		return
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		resp.Diagnostics.AddError("Fixture delete failed", "Unexpected loopback status.")
	}
}
func (r *fixtureResource) ImportState(ctx context.Context, req fwresource.ImportStateRequest, resp *fwresource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

type fixtureService struct {
	mu                            sync.Mutex
	objects                       map[string]fixtureObject
	rotations, writes             int
	expectedPassword, expectedKey string
}

func (s *fixtureService) serve(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.Header.Get("X-API-Key") != s.expectedKey {
			t.Error("fixture did not receive disposable credentials")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		key := r.Header.Get("X-Server") + "/" + r.Header.Get("X-Username") + r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPut:
			var o fixtureObject
			if json.NewDecoder(r.Body).Decode(&o) != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if o.Password != "" {
				if o.Password != s.expectedPassword {
					t.Error("password was altered before transport")
				}
				s.rotations++
			}
			o.Password = "" // The mock does not retain passwords either.
			s.objects[key] = o
			s.writes++
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			o, ok := s.objects[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if err := json.NewEncoder(w).Encode(o); err != nil {
				t.Error("fixture response failed")
			}
		case http.MethodDelete:
			delete(s.objects, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

type secretChecks struct {
	secrets []string
	dir     string
}

func (c secretChecks) inspect(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return errors.New("cannot inspect Terraform artifact")
	}
	for _, secret := range c.secrets {
		if bytes.Contains(data, []byte(secret)) {
			return errors.New("secret persisted in Terraform JSON artifact")
		}
	}
	return c.inspectDisk()
}
func (c secretChecks) inspectDisk() error {
	return filepath.WalkDir(c.dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.Contains(name, "tfstate") && name != "tfplan" && !strings.HasSuffix(name, ".tfplan") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, secret := range c.secrets {
			if bytes.Contains(data, []byte(secret)) {
				return errors.New("secret persisted in saved Terraform artifact")
			}
		}
		if name == "tfplan" || strings.HasSuffix(name, ".tfplan") {
			archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				return errors.New("cannot inspect saved Terraform plan archive")
			}
			for _, file := range archive.File {
				reader, err := file.Open()
				if err != nil {
					return err
				}
				content, readErr := io.ReadAll(reader)
				_ = reader.Close()
				if readErr != nil {
					return readErr
				}
				for _, secret := range c.secrets {
					if bytes.Contains(content, []byte(secret)) {
						return errors.New("secret persisted inside saved Terraform plan")
					}
				}
			}
		}
		return nil
	})
}
func (c secretChecks) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	resp.Error = c.inspect(req.Plan)
	if resp.Error == nil && req.Plan.PlannedValues != nil {
		resp.Error = nullPasswords(req.Plan.PlannedValues.RootModule)
	}
}
func (c secretChecks) CheckState(_ context.Context, req statecheck.CheckStateRequest, resp *statecheck.CheckStateResponse) {
	resp.Error = c.inspect(req.State)
	if resp.Error == nil && req.State.Values != nil {
		resp.Error = nullPasswords(req.State.Values.RootModule)
	}
}

func nullPasswords(module *tfjson.StateModule) error {
	if module == nil {
		return nil
	}
	for _, res := range module.Resources {
		if res.AttributeValues["password_wo"] != nil {
			return errors.New("write-only password attribute was not null")
		}
	}
	for _, child := range module.ChildModules {
		if err := nullPasswords(child); err != nil {
			return err
		}
	}
	return nil
}

func acceptanceFixture(t *testing.T) (*fixtureService, map[string]func() (tfprotov6.ProviderServer, error), secretChecks) {
	t.Helper()
	testutil.ClearCredentials(t)
	cli := os.Getenv("TF_ACC_TERRAFORM_PATH")
	if cli == "" {
		var err error
		cli, err = exec.LookPath("terraform")
		if err != nil {
			t.Fatal("install Terraform >= 1.11.1 or set TF_ACC_TERRAFORM_PATH; tests never auto-download it")
		}
	}
	t.Setenv("TF_ACC_TERRAFORM_PATH", cli)
	t.Setenv("TF_ACC_TERRAFORM_VERSION", "")
	t.Setenv("TF_ACC_PERSIST_WORKING_DIR", "")
	t.Setenv("TF_ACC_TEMP_DIR", t.TempDir())
	t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "cmdennis")
	t.Setenv("TF_ACC_PROVIDER_HOST", "registry.terraform.io")
	t.Setenv("TF_CLI_CONFIG_FILE", filepath.Join(t.TempDir(), "empty.tfrc"))
	if err := os.WriteFile(os.Getenv("TF_CLI_CONFIG_FILE"), nil, 0600); err != nil {
		t.Fatal("cannot create isolated CLI configuration")
	}
	key, password := testutil.Secret(t), testutil.Secret(t)
	t.Setenv("MXROUTE_API_KEY", key)
	t.Setenv("TF_VAR_fixture_password", password)
	s := &fixtureService{objects: map[string]fixtureObject{}, expectedPassword: password, expectedKey: key}
	server := httptest.NewServer(s.serve(t))
	t.Cleanup(server.Close)
	factories := map[string]func() (tfprotov6.ProviderServer, error){"mxroute": providerserver.NewProtocol6WithError(&fixtureProvider{&mxrouteProvider{
		version: "test", newClient: func(host, username, apiKey, version string) (*client.Client, error) {
			return client.NewMock(host, username, apiKey, version, server.URL)
		},
	}})}
	return s, factories, secretChecks{secrets: []string{key, password}, dir: t.TempDir()}
}

const fixtureHeader = `
terraform {
  required_version = ">= 1.11.1, < 2.0.0"
  required_providers { mxroute = { source = "registry.terraform.io/cmdennis/mxroute" } }
}
variable "fixture_password" {
  type = string
  sensitive = true
  ephemeral = true
}
provider "mxroute" {
  server = "mail.example.com"
  username = "owner"
}
`

func fixtureConfig(quota int, version int, password bool) string {
	attrs := ""
	if version > 0 {
		attrs += fmt.Sprintf("password_wo_version = %d\n", version)
	}
	if password {
		attrs += "password_wo = var.fixture_password\n"
	}
	return fixtureHeader + fmt.Sprintf("resource \"mxroute_fixture\" \"test\" {\nname = \"fixture\"\nquota = %d\n%s}\n", quota, attrs)
}

func TestOfflinePasswordLifecycle(t *testing.T) {
	s, factories, checks := acceptanceFixture(t)
	step := func(config string) resource.TestStep {
		return resource.TestStep{Config: config,
			ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{checks}}, ConfigStateChecks: []statecheck.StateCheck{checks}}
	}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		step(fixtureConfig(0, 1, true)),
		{Config: fixtureConfig(0, 1, true), PlanOnly: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan(), checks}}},
		step(fixtureConfig(10, 1, false)),
		step(fixtureConfig(10, 2, true)),
		{Config: strings.ReplaceAll(fixtureConfig(10, 2, true), `name = "fixture"`, `name = "replacement"`), PlanOnly: true, ExpectNonEmptyPlan: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectResourceAction("mxroute_fixture.test", plancheck.ResourceActionDestroyBeforeCreate), checks}}},
		{Config: fixtureConfig(10, 0, false), ResourceName: "mxroute_fixture.test", ImportState: true, ImportStateId: "fixture", ImportStateVerify: true, ImportStateVerifyIdentifierAttribute: "name", ImportStateVerifyIgnore: []string{"password_wo_version"}},
		{Config: fixtureConfig(10, 2, false), PlanOnly: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan(), checks}}},
	}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rotations != 2 || s.writes != 3 || len(s.objects) != 0 {
		t.Fatal("password lifecycle, update preservation or loopback cleanup failed")
	}
}

func TestOfflineProviderAliases(t *testing.T) {
	s, _, checks := acceptanceFixture(t)
	config := fixtureConfig(0, 1, true) + `
provider "mxroute" {
  alias = "impersonated"
  server = "mail.example.com"
  username = "owner|ordinary"
}
provider "mxroute" {
  alias = "other"
  server = "other.example.net"
  username = "otherowner"
}
resource "mxroute_fixture" "impersonated" {
  provider = mxroute.impersonated
  name = "fixture"
  quota = 10
  password_wo = var.fixture_password
  password_wo_version = 1
}
resource "mxroute_fixture" "other" {
  provider = mxroute.other
  name = "fixture"
  quota = 20
  password_wo = var.fixture_password
  password_wo_version = 1
}
`
	server := httptest.NewServer(s.serve(t))
	defer server.Close()
	if err := os.WriteFile(filepath.Join(checks.dir, "main.tf"), []byte(config), 0600); err != nil {
		t.Fatal("cannot write synthetic configuration")
	}
	terraform := localPlugin(t, checks, "MXROUTE_PHASE1_FIXTURE_ENDPOINT", server.URL)
	run := func(args ...string) []byte {
		t.Helper()
		out, err := terraform(args...)
		if err != nil {
			t.Fatalf("local Terraform command failed: %s", out)
		}
		return out
	}
	run("plan", "-input=false", "-out=tfplan")
	if err := checks.inspect(json.RawMessage(run("show", "-json", "tfplan"))); err != nil {
		t.Fatal(err)
	}
	run("apply", "-input=false", "-auto-approve", "tfplan")
	if err := checks.inspect(json.RawMessage(run("show", "-json"))); err != nil {
		t.Fatal(err)
	}
	run("plan", "-input=false", "-detailed-exitcode")
	run("destroy", "-input=false", "-auto-approve")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writes != 3 || s.rotations != 3 || len(s.objects) != 0 {
		t.Fatal("provider aliases shared state or failed loopback cleanup")
	}
}

// localPlugin installs this test executable as a dev-override plugin so each
// provider alias runs in its own process, as in production Terraform. The
// testing library's in-process factories reattach all aliases to one server.
// Output is redacted; the endpoint must be a loopback mock.
func localPlugin(t *testing.T, checks secretChecks, env, endpoint string) func(args ...string) ([]byte, error) {
	t.Helper()
	t.Setenv(env, endpoint)
	// Use an isolated dev override so there are no Registry calls or downloads.
	pluginDir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("cannot locate test plugin executable")
	}
	if err := os.Symlink(executable, filepath.Join(pluginDir, "terraform-provider-mxroute")); err != nil {
		t.Fatal("cannot install local test plugin")
	}
	cliConfig := fmt.Sprintf("provider_installation {\n dev_overrides {\n \"registry.terraform.io/cmdennis/mxroute\" = %q\n }\n}\n", pluginDir)
	if err := os.WriteFile(os.Getenv("TF_CLI_CONFIG_FILE"), []byte(cliConfig), 0600); err != nil {
		t.Fatal("cannot configure local test plugin")
	}
	return func(args ...string) ([]byte, error) {
		cmd := exec.Command(os.Getenv("TF_ACC_TERRAFORM_PATH"), args...)
		cmd.Dir = checks.dir
		out, err := cmd.CombinedOutput()
		for _, secret := range checks.secrets {
			out = bytes.ReplaceAll(out, []byte(secret), []byte("[redacted]"))
		}
		return out, err
	}
}

func TestOfflineCreationRequiresPassword(t *testing.T) {
	_, factories, checks := acceptanceFixture(t)
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		{Config: fixtureConfig(0, 0, false), ExpectError: regexp.MustCompile("Missing password")},
	}})
}

func TestOfflineImportedPasswordLifecycle(t *testing.T) {
	s, factories, checks := acceptanceFixture(t)
	// Only this loopback object exists before import; it has no known password
	// or rotation version, just like an adopted real object would.
	s.objects["mail.example.com/owner/fixture/fixture"] = fixtureObject{Name: "fixture", Quota: 10}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, WorkingDir: checks.dir, Steps: []resource.TestStep{
		{Config: fixtureConfig(10, 0, false), ResourceName: "mxroute_fixture.test", ImportState: true, ImportStateId: "fixture", ImportStatePersist: true},
		{Config: fixtureConfig(10, 0, false), PlanOnly: true, ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan(), checks}}},
		{Config: fixtureConfig(10, 1, true), ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{checks}}, ConfigStateChecks: []statecheck.StateCheck{checks}},
	}})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rotations != 1 || s.writes != 1 || len(s.objects) != 0 {
		t.Fatal("import forced a password change or explicit rotation failed")
	}
}
