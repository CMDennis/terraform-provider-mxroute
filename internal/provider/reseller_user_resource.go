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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type resellerUserModel struct {
	ID           types.String  `tfsdk:"id"`
	Username     types.String  `tfsdk:"username"`
	Email        types.String  `tfsdk:"email"`
	CurrentEmail types.String  `tfsdk:"current_email"`
	Package      types.String  `tfsdk:"package"`
	Password     types.String  `tfsdk:"password_wo"`
	Version      types.Int64   `tfsdk:"password_wo_version"`
	QuotaMB      types.String  `tfsdk:"quota_mb"`
	QuotaUsedMB  types.Float64 `tfsdk:"quota_used_mb"`
	Suspended    types.Bool    `tfsdk:"suspended"`
	Domain       types.String  `tfsdk:"domain"`
}
type resellerUserResource struct{ c *client.Client }

var _ resource.ResourceWithModifyPlan = &resellerUserResource{}

func newResellerUserResource() resource.Resource { return &resellerUserResource{} }
func (*resellerUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_reseller_user"
}
func (*resellerUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := schemautil.PasswordAttributes()
	attrs["id"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Username; account-relative to the plain reseller provider."}
	attrs["username"] = schema.StringAttribute{Required: true, Validators: []validator.String{existingUserValidator{}}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Username. New users require 1–10 lowercase letters, digits or underscores; imported users may use the 1–16 character authentication syntax. Compared exactly. Changing it REPLACES the user."}
	attrs["email"] = schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Contact email used at creation. No update route is documented, so changing it plans a REPLACEMENT that deletes the user and its data; protect important users with `prevent_destroy`. Remote changes appear in `current_email` and never trigger replacement. Import adopts the remote value."}
	attrs["current_email"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Server-reported contact email."}
	attrs["package"] = schema.StringAttribute{Required: true, Validators: []validator.String{packageNameValidator{}}, MarkdownDescription: "Assigned package. Changes are applied in place through the package-assignment endpoint. Reference `mxroute_reseller_package.<name>.name` so users are removed before their packages."}
	attrs["quota_mb"] = schema.StringAttribute{Optional: true, Computed: true, Validators: []validator.String{limitValidator{}}, MarkdownDescription: "Effective user disk quota in **MB**, or `unlimited` (package quota is GB). When set, it is an explicit override applied after any package reassignment and re-applied if reassignment changed it. Omission adopts the server value and never sends a quota. Package/override precedence is live-unverified."}
	attrs["quota_used_mb"] = schema.Float64Attribute{Computed: true, MarkdownDescription: "Volatile disk usage in MB."}
	attrs["suspended"] = schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Suspension state. Changes dispatch suspend or unsuspend only when the remote state differs. Omission adopts the server value."}
	attrs["domain"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Server-reported primary domain."}
	resp.Schema = schema.Schema{Version: schemautil.InitialVersion, Attributes: attrs, MarkdownDescription: "Manage a reseller-owned user through a **plain reseller** provider (not an impersonation alias). Creation and explicit rotation require a write-only password of at least 8 characters and a positive rotation version; server strength rules still apply and import requires no password. Updates run in a fixed order — unsuspend, package reassignment, password/quota PATCH, suspend — and record each confirmed step so a failed later step is retried alone. Delete permanently removes the user account and everything it hosts. Runtime behavior is live-unverified."}
}
func (*resellerUserResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return schemautil.InitialStateUpgraders()
}
func (r *resellerUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	configuredClient(req.ProviderData, &r.c, &resp.Diagnostics)
}
func (r *resellerUserResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || !requirePlainReseller(r.c, &resp.Diagnostics) {
		return
	}
	var cfg resellerUserModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	prior := types.Int64Null()
	creating := req.State.Raw.IsNull()
	if !creating {
		var old resellerUserModel
		resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
		prior = old.Version
		creating = (!cfg.Username.IsUnknown() && !cfg.Username.Equal(old.Username)) || (!cfg.Email.IsUnknown() && !cfg.Email.Equal(old.Email))
	}
	if resp.Diagnostics.HasError() {
		return
	}
	// Existing users keep authentication syntax; only creation is stricter.
	if creating && !cfg.Username.IsUnknown() {
		if err := identity.ValidateNewUser(cfg.Username.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("username"), "Invalid new username", err.Error())
		}
	}
	_, d := schemautil.PasswordChange(cfg.Password, cfg.Version, prior, creating, false, 8, 0)
	resp.Diagnostics.Append(d...)
}
func userModelValues(m *resellerUserModel, v *client.ResellerUser, quota string) {
	m.ID = types.StringValue(*v.Username)
	if m.Email.IsNull() || m.Email.IsUnknown() {
		m.Email = types.StringValue(*v.Email)
	}
	m.CurrentEmail, m.Package, m.Domain = types.StringValue(*v.Email), types.StringValue(*v.Package), types.StringValue(*v.Domain)
	m.QuotaMB, m.QuotaUsedMB, m.Suspended = types.StringValue(quota), types.Float64Value(*v.Quota.Used), types.BoolValue(*v.Suspended)
	m.Password = types.StringNull()
}

// converge applies the remaining desired user changes in a deterministic
// order, persisting each confirmed step before the next one. done must
// describe confirmed remote state; plan/cfg are the desired values.
func (r *resellerUserResource) converge(ctx context.Context, plan, cfg, done resellerUserModel, rotate bool, state *tfsdk.State, d *diag.Diagnostics) {
	name := plan.Username.ValueString()
	var v *client.ResellerUser
	var quota string
	refresh := func() bool {
		var err error
		if v, quota, err = readResellerUser(ctx, r.c, name); err != nil {
			apiDiagnostic(d, err)
			return false
		}
		userModelValues(&done, v, quota)
		d.Append(state.Set(ctx, &done)...)
		return !d.HasError()
	}
	step := func(write func() error) bool {
		if err := write(); err != nil {
			apiDiagnostic(d, err)
			return false
		}
		return refresh()
	}
	if !refresh() {
		return
	}
	manageSuspension := !cfg.Suspended.IsNull() && !plan.Suspended.IsUnknown()
	if manageSuspension && !plan.Suspended.ValueBool() && *v.Suspended && !step(func() error { return r.c.UnsuspendResellerUser(ctx, name) }) {
		return
	}
	// Reassign first, then re-read: the explicit quota comparison below uses
	// the post-reassignment value whether or not the API reset an override.
	if plan.Package.ValueString() != *v.Package && !step(func() error { return r.c.SetResellerUserPackage(ctx, name, plan.Package.ValueString()) }) {
		return
	}
	input := client.UpdateResellerUserRequest{}
	if rotate {
		p := cfg.Password.ValueString()
		input.Password = &p
	}
	if !cfg.QuotaMB.IsNull() && !plan.QuotaMB.IsUnknown() && plan.QuotaMB.ValueString() != quota {
		q := plan.QuotaMB.ValueString()
		input.Quota = &q
	}
	if input.Password != nil || input.Quota != nil {
		if err := r.c.UpdateResellerUser(ctx, name, input); err != nil {
			apiDiagnostic(d, err)
			return
		}
		// Persist a confirmed rotation before readback so a later failure
		// cannot replay it. Quota is verified by the readback that follows.
		done.Version = plan.Version
		d.Append(state.Set(ctx, &done)...)
		if !refresh() {
			return
		}
	}
	if manageSuspension && plan.Suspended.ValueBool() && !*v.Suspended && !step(func() error { return r.c.SuspendResellerUser(ctx, name) }) {
		return
	}
	done.Version = plan.Version
	d.Append(state.Set(ctx, &done)...)
	if plan.Package.ValueString() != *v.Package || (!cfg.QuotaMB.IsNull() && plan.QuotaMB.ValueString() != quota) || (manageSuspension && plan.Suspended.ValueBool() != *v.Suspended) {
		d.AddError("User readback did not confirm requested settings", "The API reported a different package, quota or suspension state after successful writes. State records the reported values; check package/override precedence before retrying.")
	}
}
func (r *resellerUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m, cfg resellerUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || !requirePlainReseller(r.c, &resp.Diagnostics) {
		return
	}
	_, d := schemautil.PasswordChange(cfg.Password, m.Version, types.Int64Null(), true, true, 8, 0)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Never adopt an existing user or create a case variant of one.
	if err := r.c.FindResellerUser(ctx, m.Username.ValueString()); err == nil {
		resp.Diagnostics.AddError("Reseller user already exists", "Import the existing user before managing it.")
		return
	} else if !client.IsNotFound(err) {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	input := client.CreateResellerUserRequest{Username: m.Username.ValueString(), Email: m.Email.ValueString(), Password: cfg.Password.ValueString(), Package: m.Package.ValueString()}
	if err := r.c.CreateResellerUser(ctx, input); err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	// Retain the confirmed identity and rotation version before any follow-up
	// read or write. Terraform may taint a failed create: review before replacing.
	m.ID, m.Password = m.Username, types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.converge(ctx, m, cfg, m, false, &resp.State, &resp.Diagnostics)
}
func (r *resellerUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m resellerUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || !requirePlainReseller(r.c, &resp.Diagnostics) {
		return
	}
	v, quota, err := readResellerUser(ctx, r.c, m.Username.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	userModelValues(&m, v, quota)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *resellerUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, old, cfg resellerUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || !requirePlainReseller(r.c, &resp.Diagnostics) {
		return
	}
	rotate, d := schemautil.PasswordChange(cfg.Password, m.Version, old.Version, false, true, 8, 0)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.converge(ctx, m, cfg, old, rotate, &resp.State, &resp.Diagnostics)
}
func (r *resellerUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m resellerUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || !requirePlainReseller(r.c, &resp.Diagnostics) {
		return
	}
	err := r.c.FindResellerUser(ctx, m.Username.ValueString())
	if client.IsNotFound(err) {
		return
	}
	if err == nil {
		err = r.c.DeleteResellerUser(ctx, m.Username.ValueString())
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
	}
}
func (*resellerUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if identity.ExistingUser(req.ID) != nil {
		resp.Diagnostics.AddError("Invalid import identifier", "Use the exact existing username without an impersonation prefix.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("username"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
