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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type domainModel struct {
	ID                 types.String `tfsdk:"id"`
	Domain             types.String `tfsdk:"domain"`
	MailHosting        types.Bool   `tfsdk:"mail_hosting"`
	MailHostingPending types.Bool   `tfsdk:"mail_hosting_pending"`
	SSLEnabled         types.Bool   `tfsdk:"ssl_enabled"`
	Pointers           types.Set    `tfsdk:"pointers"`
	VerificationWait   types.Int64  `tfsdk:"verification_wait_seconds"`
	VisibilityWait     types.Int64  `tfsdk:"visibility_wait_seconds"`
}
type domainResource struct{ c *client.Client }

func newDomainResource() resource.Resource { return &domainResource{} }
func (*domainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain"
}
func (*domainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{Version: schemautil.InitialVersion, MarkdownDescription: "Manage a verified mail domain. Create the verification TXT record and allow DNS propagation first. Deleting a domain can affect mail and dependent objects; upstream cascade/recovery behavior is live-unverified. Use `prevent_destroy` for important domains. The provider never purges unmanaged children.", Attributes: map[string]schema.Attribute{
		"id":                        schema.StringAttribute{Computed: true, MarkdownDescription: "Account-relative domain identity."},
		"domain":                    schema.StringAttribute{Required: true, Validators: []validator.String{domainValidator{}}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Canonical DNS name. Changing it replaces the domain."},
		"mail_hosting":              schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Enable mail hosting. Omission adopts the server value; only an explicit change sends a mail-status update."},
		"mail_hosting_pending":      schema.BoolAttribute{Computed: true, MarkdownDescription: "True when creation succeeded but requested mail hosting is not confirmed. A warning explains the failed step. Run a normal refreshed plan/apply to finish setup without replacing the domain. During this pending state mail_hosting retains the requested value until refresh."},
		"ssl_enabled":               schema.BoolAttribute{Computed: true, MarkdownDescription: "Server-reported SSL status."},
		"pointers":                  schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Read-only pointer name summary. Manage pointers with separate resources."},
		"verification_wait_seconds": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), Validators: []validator.Int64{waitValidator{}}, MarkdownDescription: "Before creation, poll DNS for the exact account verification TXT record for up to this many seconds (0 disables preflight; maximum 1800). Does not retry rejected API creates."},
		"visibility_wait_seconds":   schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(30), Validators: []validator.Int64{waitValidator{}}, MarkdownDescription: "Bound post-create visibility polling in seconds (0 checks once; maximum 1800). Only complete successful lists proving absence are retried."},
	}}
}
func (*domainResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return schemautil.InitialStateUpgraders()
}
func (r *domainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	configuredClient(req.ProviderData, &r.c, &resp.Diagnostics)
}
func (r *domainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m domainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := waitVerification(ctx, r.c, m.Domain.ValueString(), m.VerificationWait.ValueInt64()); err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	created, err := r.c.CreateDomain(ctx, client.CreateDomainRequest{Domain: m.Domain.ValueString()})
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	desired := m.MailHosting
	m.MailHostingPending = types.BoolValue(true)
	// Save the stable requested identity immediately after confirmed creation,
	// before visibility polling or the independent mail-status mutation.
	m.ID = m.Domain
	m.MailHosting = types.BoolNull()
	m.SSLEnabled = types.BoolValue(*created.SSLEnabled)
	m.Pointers = types.SetNull(types.StringType)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	canonical, e := identity.Domain(*created.Domain)
	if e != nil || canonical != m.Domain.ValueString() {
		r.pending(ctx, &m, desired, protocolError(), resp)
		return
	}
	v, err := waitDomain(ctx, r.c, m.Domain.ValueString(), m.VisibilityWait.ValueInt64())
	if err != nil {
		r.pending(ctx, &m, desired, err, resp)
		return
	}
	domainValues(ctx, &m, v, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	if !desired.IsUnknown() && !desired.IsNull() && desired.ValueBool() != *v.MailHosting {
		if err := r.c.SetMailHosting(ctx, m.Domain.ValueString(), desired.ValueBool()); err != nil {
			r.pending(ctx, &m, desired, err, resp)
			return
		}
		v, err = readDomain(ctx, r.c, m.Domain.ValueString())
		if err != nil {
			r.pending(ctx, &m, desired, err, resp)
			return
		}
		domainValues(ctx, &m, v, &resp.Diagnostics)
	}
	if !desired.IsUnknown() && !desired.IsNull() && !m.MailHosting.Equal(desired) {
		r.pending(ctx, &m, desired, protocolError(), resp)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// Terraform taints a newly created resource if Create returns an error. After
// confirmed creation, retain requested intent with an explicit pending status
// and warning instead. The next Read refreshes mail_hosting to the actual remote
// value, so the next apply repairs only that field. No write is retried here.
func (r *domainResource) pending(ctx context.Context, m *domainModel, desired types.Bool, err error, resp *resource.CreateResponse) {
	if !desired.IsUnknown() {
		m.MailHosting = desired
	}
	m.MailHostingPending = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
	resp.Diagnostics.AddWarning("Domain created; configuration pending", "The domain identity has been saved. Mail hosting or visibility could not be confirmed: "+err.Error()+". Review the remote object, then run a normal refreshed plan/apply to complete setup. The domain will not be recreated solely because this follow-up step failed. While mail_hosting_pending is true, mail_hosting records requested intent until refresh.")
}
func (r *domainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m domainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, err := readDomain(ctx, r.c, m.Domain.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	m.ID = m.Domain
	domainValues(ctx, &m, v, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *domainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, prior domainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.MailHosting.IsUnknown() && !m.MailHosting.IsNull() && !m.MailHosting.Equal(prior.MailHosting) {
		if err := r.c.SetMailHosting(ctx, m.Domain.ValueString(), m.MailHosting.ValueBool()); err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
	}
	v, err := readDomain(ctx, r.c, m.Domain.ValueString())
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	m.ID = m.Domain
	domainValues(ctx, &m, v, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *domainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m domainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := readDomain(ctx, r.c, m.Domain.ValueString())
	if client.IsNotFound(err) {
		return
	}
	if err == nil {
		err = r.c.DeleteDomain(ctx, m.Domain.ValueString())
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
	}
}
func (*domainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	name, err := identity.Domain(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import identifier", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("verification_wait_seconds"), int64(0))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("visibility_wait_seconds"), int64(30))...)
}
