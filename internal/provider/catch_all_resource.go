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

type catchAllModel struct {
	ID          types.String `tfsdk:"id"`
	Domain      types.String `tfsdk:"domain"`
	Type        types.String `tfsdk:"type"`
	Address     types.String `tfsdk:"address"`
	Description types.String `tfsdk:"description"`
}
type catchAllResource struct{ c *client.Client }

var _ resource.ResourceWithValidateConfig = &catchAllResource{}

func newCatchAllResource() resource.Resource { return &catchAllResource{} }
func (*catchAllResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catch_all"
}
func (*catchAllResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Version: schemautil.InitialVersion, MarkdownDescription: "Manage the existing domain catch-all singleton. Create applies the desired setting; import adopts it. Destroy only relinquishes Terraform management and PRESERVES the remote routing setting, including any forwarding address. No reset or delete API exists. Use one owner per domain; switching modes must omit address for fail/blackhole.", Attributes: map[string]schema.Attribute{
		"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "Account-relative domain identity."},
		"domain":      schema.StringAttribute{Required: true, Validators: []validator.String{domainValidator{}}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Owned canonical domain. Changing it relinquishes the old setting and manages the new domain's setting."},
		"type":        schema.StringAttribute{Required: true, MarkdownDescription: "fail (reject), blackhole (discard), or address (forward)."},
		"address":     schema.StringAttribute{Optional: true, MarkdownDescription: "Email destination, required only for type address; omit for fail/blackhole. Non-address modes clear stale address from state."},
		"description": schema.StringAttribute{Computed: true, MarkdownDescription: "Optional server description."},
	}}
}
func (*catchAllResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return schemautil.InitialStateUpgraders()
}
func (r *catchAllResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	configuredClient(req.ProviderData, &r.c, &resp.Diagnostics)
}
func validateCatchAll(m catchAllModel, d *diag.Diagnostics) {
	if m.Type.IsUnknown() || m.Type.IsNull() {
		return
	}
	switch m.Type.ValueString() {
	case "address":
		if !m.Address.IsUnknown() && (m.Address.IsNull() || !validAddress(m.Address.ValueString())) {
			d.AddAttributeError(path.Root("address"), "Invalid catch-all address", "Supply an email address when type is address.")
		}
	case "fail", "blackhole":
		if !m.Address.IsUnknown() && !m.Address.IsNull() {
			d.AddAttributeError(path.Root("address"), "Unexpected catch-all address", "Omit address for fail or blackhole so mode changes clear stale values.")
		}
	default:
		d.AddAttributeError(path.Root("type"), "Invalid catch-all type", "Use fail, blackhole, or address.")
	}
}
func (*catchAllResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m catchAllModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if !resp.Diagnostics.HasError() {
		validateCatchAll(m, &resp.Diagnostics)
	}
}
func (r *catchAllResource) write(ctx context.Context, plan tfsdk.Plan, state *tfsdk.State, d *diag.Diagnostics) {
	var m catchAllModel
	d.Append(plan.Get(ctx, &m)...)
	if d.HasError() {
		return
	}
	validateCatchAll(m, d)
	if d.HasError() {
		return
	}
	input := client.SetCatchAllRequest{Type: m.Type.ValueString()}
	if input.Type == "address" {
		a := m.Address.ValueString()
		input.Address = &a
	}
	if err := r.c.SetCatchAll(ctx, m.Domain.ValueString(), input); err != nil {
		apiDiagnostic(d, err)
		return
	}
	m.ID = m.Domain
	d.Append(state.Set(ctx, &m)...)
	v, err := readCatchAll(ctx, r.c, m.Domain.ValueString())
	if err != nil {
		apiDiagnostic(d, err)
		return
	}
	catchAllValues(&m, v)
	d.Append(state.Set(ctx, &m)...)
}
func (r *catchAllResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.write(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *catchAllResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	r.write(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *catchAllResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m catchAllModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, err := readCatchAll(ctx, r.c, m.Domain.ValueString())
	// An unavailable parent is not proof that a non-deletable setting vanished.
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	catchAllValues(&m, v)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (*catchAllResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) { /* Relinquish management; never reset remote routing. */
}
func (*catchAllResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	domain, err := identity.Domain(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import identifier", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), domain)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), domain)...)
}
