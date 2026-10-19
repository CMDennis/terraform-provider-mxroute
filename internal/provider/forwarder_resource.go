package provider

import (
	"context"
	"strings"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
	"github.com/CMDennis/terraform-provider-mxroute/internal/schemautil"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type forwarderModel struct {
	ID             types.String `tfsdk:"id"`
	Domain         types.String `tfsdk:"domain"`
	Alias          types.String `tfsdk:"alias"`
	CanonicalAlias types.String `tfsdk:"canonical_alias"`
	Email          types.String `tfsdk:"email"`
	Destinations   types.Set    `tfsdk:"destinations"`
}
type forwarderResource struct{ c *client.Client }

func newForwarderResource() resource.Resource { return &forwarderResource{} }
func (*forwarderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_forwarder"
}
func (*forwarderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Version: schemautil.InitialVersion, MarkdownDescription: "Manage a forwarder using a destination set. Destination changes require destroy-before-create replacement and interrupt routing; create-before-destroy is not supported for a unique alias. A failed replacement create leaves the alias absent; correct the error and apply again. Forwarding to Yahoo, AOL, Gmail and similar providers automatically enables Expert Spam Filtering for the domain. This side effect is not independently managed by this resource.", Attributes: map[string]schema.Attribute{
		"id":              schema.StringAttribute{Computed: true, MarkdownDescription: "Account-relative domain/lowercase-alias identity."},
		"domain":          schema.StringAttribute{Required: true, Validators: []validator.String{domainValidator{}}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Owned canonical domain. Reference its resource to order cleanup."},
		"alias":           schema.StringAttribute{Required: true, Validators: []validator.String{localValidator{}}, PlanModifiers: localReplace(), MarkdownDescription: "Forwarder local part; case-insensitive lookup. Identity changes replace."},
		"canonical_alias": schema.StringAttribute{Computed: true, MarkdownDescription: "Canonical server alias."},
		"email":           schema.StringAttribute{Computed: true, MarkdownDescription: "Server-reported forwarding address."},
		"destinations":    schema.SetAttribute{Required: true, ElementType: types.StringType, Validators: []validator.Set{destinationsValidator{}}, PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()}, MarkdownDescription: "Unordered email destinations, or a single :blackhole: (discard) or :fail: (reject) target. Special targets cannot be combined with other targets. Changes replace and interrupt routing."},
	}}
}
func (*forwarderResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return schemautil.InitialStateUpgraders()
}
func (r *forwarderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	configuredClient(req.ProviderData, &r.c, &resp.Diagnostics)
}
func (r *forwarderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m forwarderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	var destinations []string
	resp.Diagnostics.Append(m.Destinations.ElementsAs(ctx, &destinations, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.c.FindForwarder(ctx, m.Domain.ValueString(), m.Alias.ValueString())
	if err == nil {
		resp.Diagnostics.AddError("Forwarder already exists", "Import the existing forwarder before managing it.")
		return
	}
	if !client.IsNotFound(err) {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	if err := r.c.CreateForwarder(ctx, m.Domain.ValueString(), client.CreateForwarderRequest{Alias: m.Alias.ValueString(), Destinations: destinations}); err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	m.ID = types.StringValue(m.Domain.ValueString() + "/" + strings.ToLower(m.Alias.ValueString()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	v, err := readForwarder(ctx, r.c, m.Domain.ValueString(), m.Alias.ValueString())
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	forwarderValues(ctx, &m, v, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *forwarderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m forwarderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, err := readForwarder(ctx, r.c, m.Domain.ValueString(), m.Alias.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	forwarderValues(ctx, &m, v, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *forwarderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m forwarderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, err := readForwarder(ctx, r.c, m.Domain.ValueString(), m.Alias.ValueString())
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	forwarderValues(ctx, &m, v, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *forwarderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m forwarderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, err := readForwarder(ctx, r.c, m.Domain.ValueString(), m.Alias.ValueString())
	if client.IsNotFound(err) {
		return
	}
	if err == nil {
		err = r.c.DeleteForwarder(ctx, m.Domain.ValueString(), *v.Alias)
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
	}
}
func (*forwarderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	domain, alias, err := identity.Scoped(req.ID, false)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import identifier", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), domain)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("alias"), alias)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), domain+"/"+alias)...)
}
