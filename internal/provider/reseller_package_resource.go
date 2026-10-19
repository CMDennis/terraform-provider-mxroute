package provider

import (
	"context"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
	"github.com/CMDennis/terraform-provider-mxroute/internal/schemautil"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type resellerPackageModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	QuotaGB         types.String `tfsdk:"quota_gb"`
	Domains         types.String `tfsdk:"domains"`
	EmailAccounts   types.String `tfsdk:"email_accounts"`
	EmailForwarders types.String `tfsdk:"email_forwarders"`
	DomainPointers  types.String `tfsdk:"domain_pointers"`
}
type resellerPackageResource struct{ c *client.Client }

var _ resource.ResourceWithModifyPlan = &resellerPackageResource{}

func newResellerPackageResource() resource.Resource { return &resellerPackageResource{} }
func (*resellerPackageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_reseller_package"
}

type packageLimit struct {
	decimal              bool
	description, initial string
}

func packageLimitAttributes() map[string]packageLimit {
	return map[string]packageLimit{
		"quota_gb":         {true, "Disk quota in **GB**, or `unlimited`.", "1"},
		"domains":          {false, "Domain limit, or `unlimited`.", "1"},
		"email_accounts":   {false, "Usable customer mailbox limit, or `unlimited`. `0` permits no customer mailboxes and is distinct from `unlimited`; the system mailbox does not consume this allowance.", "100"},
		"email_forwarders": {false, "Forwarder limit, or `unlimited`.", "100"},
		"domain_pointers":  {false, "Domain pointer limit, or `unlimited`.", "10"},
	}
}
func (*resellerPackageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id":   schema.StringAttribute{Computed: true, MarkdownDescription: "Package name; account-relative to the plain reseller provider."},
		"name": schema.StringAttribute{Required: true, Validators: []validator.String{packageNameValidator{}}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Package name of letters, digits, underscores or hyphens. Case is preserved and compared exactly; a case-only variant of an existing package is rejected. Changing it replaces the package, which fails while users are assigned."},
	}
	for name, a := range packageLimitAttributes() {
		attrs[name] = schema.StringAttribute{Optional: true, Computed: true, Validators: []validator.String{limitValidator{decimal: a.decimal}},
			MarkdownDescription: a.description + " Documented creation default `" + a.initial + "`. Omission adopts the server value and later updates preserve it."}
	}
	resp.Schema = schema.Schema{Version: schemautil.InitialVersion, Attributes: attrs, MarkdownDescription: "Manage a reseller hosting package through a **plain reseller** provider (not an impersonation alias). Limits use strings so `unlimited` and `0` stay distinct; package quota is in GB, unlike user quota in MB. Only configured, changed limits are sent on update. Whether package updates change existing users' effective quota is live-unverified. Delete fails while users are assigned (the error is surfaced and no user is removed); reference the package from `mxroute_reseller_user.package` so Terraform removes users first. Import with the exact package name. Runtime behavior is live-unverified."}
}
func (*resellerPackageResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return schemautil.InitialStateUpgraders()
}
func (r *resellerPackageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	configuredClient(req.ProviderData, &r.c, &resp.Diagnostics)
}
func (r *resellerPackageResource) ModifyPlan(_ context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if !req.Plan.Raw.IsNull() {
		requirePlainReseller(r.c, &resp.Diagnostics)
	}
}
func packageModelValues(m *resellerPackageModel, p packageSettings) {
	m.ID = m.Name
	m.QuotaGB, m.Domains, m.EmailAccounts = types.StringValue(p.QuotaGB), types.StringValue(p.Domains), types.StringValue(p.EmailAccounts)
	m.EmailForwarders, m.DomainPointers = types.StringValue(p.EmailForwarders), types.StringValue(p.DomainPointers)
}
func packageLimits(cfg, plan, prior resellerPackageModel) client.PackageLimits {
	return client.PackageLimits{
		Quota: optionalLimit(cfg.QuotaGB, plan.QuotaGB, prior.QuotaGB), Domains: optionalLimit(cfg.Domains, plan.Domains, prior.Domains),
		EmailAccounts: optionalLimit(cfg.EmailAccounts, plan.EmailAccounts, prior.EmailAccounts), EmailForwarders: optionalLimit(cfg.EmailForwarders, plan.EmailForwarders, prior.EmailForwarders),
		DomainPointers: optionalLimit(cfg.DomainPointers, plan.DomainPointers, prior.DomainPointers),
	}
}

// verifyPackage reports configured values that readback did not confirm. The
// caller still stores the remote values so the next plan shows the difference.
func verifyPackage(cfg, plan resellerPackageModel, p packageSettings, d *diag.Diagnostics) {
	for _, pair := range [][3]any{{cfg.QuotaGB, plan.QuotaGB, p.QuotaGB}, {cfg.Domains, plan.Domains, p.Domains}, {cfg.EmailAccounts, plan.EmailAccounts, p.EmailAccounts}, {cfg.EmailForwarders, plan.EmailForwarders, p.EmailForwarders}, {cfg.DomainPointers, plan.DomainPointers, p.DomainPointers}} {
		c, want, got := pair[0].(types.String), pair[1].(types.String), pair[2].(string)
		if !c.IsNull() && !want.IsUnknown() && want.ValueString() != got {
			d.AddError("Package readback did not confirm requested limits", "The API reported different package limits after a successful write. State records the reported values; review server normalization before retrying.")
			return
		}
	}
}
func (r *resellerPackageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m, cfg resellerPackageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || !requirePlainReseller(r.c, &resp.Diagnostics) {
		return
	}
	// Never adopt an existing package or create a case variant of one.
	if err := r.c.FindResellerPackage(ctx, m.Name.ValueString()); err == nil {
		resp.Diagnostics.AddError("Reseller package already exists", "Import the existing package before managing it.")
		return
	} else if !client.IsNotFound(err) {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	input := client.CreateResellerPackageRequest{Name: m.Name.ValueString(), PackageLimits: packageLimits(cfg, m, resellerPackageModel{})}
	if err := r.c.CreateResellerPackage(ctx, input); err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	// Retain identity if readback fails; unknown defaults become null on error.
	m.ID = m.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	_, p, err := readResellerPackage(ctx, r.c, m.Name.ValueString())
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	verified := m
	packageModelValues(&verified, p)
	verifyPackage(cfg, m, p, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &verified)...)
}
func (r *resellerPackageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m resellerPackageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || !requirePlainReseller(r.c, &resp.Diagnostics) {
		return
	}
	_, p, err := readResellerPackage(ctx, r.c, m.Name.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	packageModelValues(&m, p)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *resellerPackageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, old, cfg resellerPackageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || !requirePlainReseller(r.c, &resp.Diagnostics) {
		return
	}
	_, p, err := readResellerPackage(ctx, r.c, m.Name.ValueString())
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	// Compare with the fresh read so a retry after a partial failure sends
	// only limits that still differ; omission always preserves a limit.
	current := old
	packageModelValues(&current, p)
	input := packageLimits(cfg, m, current)
	if input != (client.PackageLimits{}) {
		if err := r.c.UpdateResellerPackage(ctx, m.Name.ValueString(), input); err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		if _, p, err = readResellerPackage(ctx, r.c, m.Name.ValueString()); err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
	}
	verified := m
	packageModelValues(&verified, p)
	verifyPackage(cfg, m, p, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &verified)...)
}
func (r *resellerPackageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m resellerPackageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || !requirePlainReseller(r.c, &resp.Diagnostics) {
		return
	}
	err := r.c.FindResellerPackage(ctx, m.Name.ValueString())
	if client.IsNotFound(err) {
		return
	}
	if err == nil {
		// An in-use package is a business error; users are never removed here.
		err = r.c.DeleteResellerPackage(ctx, m.Name.ValueString())
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
	}
}
func (*resellerPackageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := identity.Package(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid import identifier", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
