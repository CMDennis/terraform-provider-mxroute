package provider

import (
	"context"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
	"github.com/CMDennis/terraform-provider-mxroute/internal/schemautil"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type pointerModel struct {
	ID               types.String `tfsdk:"id"`
	Domain           types.String `tfsdk:"domain"`
	Pointer          types.String `tfsdk:"pointer"`
	Type             types.String `tfsdk:"type"`
	Target           types.String `tfsdk:"target"`
	VerificationWait types.Int64  `tfsdk:"verification_wait_seconds"`
	VisibilityWait   types.Int64  `tfsdk:"visibility_wait_seconds"`
}
type pointerResource struct{ c *client.Client }

func newPointerResource() resource.Resource { return &pointerResource{} }
func (*pointerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain_pointer"
}
func (*pointerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{Version: schemautil.InitialVersion, MarkdownDescription: "Manage a domain alias or redirect. Reference the parent domain resource to order creation and cleanup. Pointer verification prerequisites are live-unverified and are checked independently of the parent when the optional DNS preflight is enabled. Identity and mode changes require replacement; do not assume create-before-destroy works for a unique pointer.", Attributes: map[string]schema.Attribute{
		"id":                        schema.StringAttribute{Computed: true, MarkdownDescription: "Account-relative domain/pointer identity."},
		"domain":                    schema.StringAttribute{Required: true, Validators: []validator.String{domainValidator{}}, PlanModifiers: replace, MarkdownDescription: "Owned parent domain; changing it replaces the pointer."},
		"pointer":                   schema.StringAttribute{Required: true, Validators: []validator.String{domainValidator{}}, PlanModifiers: replace, MarkdownDescription: "Canonical pointer DNS name; changing it replaces the pointer."},
		"type":                      schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("alias"), Validators: []validator.String{modeValidator{}}, PlanModifiers: replace, MarkdownDescription: "alias (default) or redirect. Changing mode replaces the pointer."},
		"target":                    schema.StringAttribute{Computed: true, MarkdownDescription: "Server-reported target domain."},
		"verification_wait_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), Validators: []validator.Int64{waitValidator{}}, MarkdownDescription: "Opt-in DNS TXT preflight for the pointer name itself, independent of its parent (0 disables, maximum 1800). No API validation errors are retried."},
		"visibility_wait_seconds":   schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(30), Validators: []validator.Int64{waitValidator{}}, MarkdownDescription: "Bound post-create list visibility polling in seconds (0 checks once; maximum 1800)."},
	}}
}
func (*pointerResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return schemautil.InitialStateUpgraders()
}
func (r *pointerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	configuredClient(req.ProviderData, &r.c, &resp.Diagnostics)
}
func (r *pointerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m pointerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.Domain.Equal(m.Pointer) {
		resp.Diagnostics.AddError("Invalid pointer", "The pointer must differ from its parent domain.")
		return
	}
	if _, err := readDomain(ctx, r.c, m.Domain.ValueString()); err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	if err := waitVerification(ctx, r.c, m.Pointer.ValueString(), m.VerificationWait.ValueInt64()); err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	alias := m.Type.ValueString() == "alias"
	if err := r.c.CreateDomainPointer(ctx, m.Domain.ValueString(), client.CreateDomainPointerRequest{Pointer: m.Pointer.ValueString(), Alias: &alias}); err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	m.ID = types.StringValue(m.Domain.ValueString() + "/" + m.Pointer.ValueString())
	m.Target = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	v, err := waitPointer(ctx, r.c, m.Domain.ValueString(), m.Pointer.ValueString(), m.VisibilityWait.ValueInt64())
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	m.Type = types.StringValue(*v.Type)
	m.Target = types.StringValue(*v.Target)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *pointerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m pointerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, err := readPointer(ctx, r.c, m.Domain.ValueString(), m.Pointer.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	m.ID = types.StringValue(m.Domain.ValueString() + "/" + m.Pointer.ValueString())
	m.Type = types.StringValue(*v.Type)
	m.Target = types.StringValue(*v.Target)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *pointerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Only local wait controls can update; identity and mode require replacement.
	var m pointerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, err := readPointer(ctx, r.c, m.Domain.ValueString(), m.Pointer.ValueString())
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	m.ID = types.StringValue(m.Domain.ValueString() + "/" + m.Pointer.ValueString())
	m.Type = types.StringValue(*v.Type)
	m.Target = types.StringValue(*v.Target)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *pointerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m pointerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := readPointer(ctx, r.c, m.Domain.ValueString(), m.Pointer.ValueString())
	if client.IsNotFound(err) {
		return
	}
	if err == nil {
		err = r.c.DeleteDomainPointer(ctx, m.Domain.ValueString(), m.Pointer.ValueString())
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
	}
}
func (*pointerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	domain, pointer, err := identity.Scoped(req.ID, true)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import identifier", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), domain)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pointer"), pointer)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), domain+"/"+pointer)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("verification_wait_seconds"), int64(0))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("visibility_wait_seconds"), int64(30))...)
}
