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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type mailboxModel struct {
	ID                types.String  `tfsdk:"id"`
	Domain            types.String  `tfsdk:"domain"`
	Username          types.String  `tfsdk:"username"`
	CanonicalUsername types.String  `tfsdk:"canonical_username"`
	Email             types.String  `tfsdk:"email"`
	Password          types.String  `tfsdk:"password_wo"`
	Version           types.Int64   `tfsdk:"password_wo_version"`
	Quota             types.Int64   `tfsdk:"quota"`
	Limit             types.Int64   `tfsdk:"limit"`
	Usage             types.Float64 `tfsdk:"usage"`
	Sent              types.Int64   `tfsdk:"sent"`
	Suspended         types.Bool    `tfsdk:"suspended"`
}
type mailboxResource struct{ c *client.Client }

var _ resource.ResourceWithModifyPlan = &mailboxResource{}

func newMailboxResource() resource.Resource { return &mailboxResource{} }
func (*mailboxResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_email_account"
}
func (*mailboxResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := schemautil.PasswordAttributes()
	attrs["id"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Account-relative domain/lowercase-local-part identity."}
	attrs["domain"] = schema.StringAttribute{Required: true, Validators: []validator.String{domainValidator{}}, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Owned canonical domain. Reference its resource to order cleanup."}
	attrs["username"] = schema.StringAttribute{Required: true, Validators: []validator.String{localValidator{}}, PlanModifiers: localReplace(), MarkdownDescription: "Mailbox local part. Case-insensitive lookup; casing-only edits do not replace or duplicate the mailbox."}
	attrs["canonical_username"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Canonical server name used for reads, updates and deletion."}
	attrs["email"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Server-reported email address."}
	attrs["quota"] = schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "Storage quota in MB; 0 means unlimited. Omission adopts the server value (documented creation default 1024 MB), and updates preserve it. Other bounds are server validated."}
	attrs["limit"] = schema.Int64Attribute{Optional: true, Computed: true, Validators: []validator.Int64{sendLimitValidator{}}, MarkdownDescription: "Daily send limit, at most 9600. Omission adopts the server value (documented creation default 9600) and preserves it on update; other bounds are server validated."}
	attrs["usage"] = schema.Float64Attribute{Computed: true, MarkdownDescription: "Volatile mailbox storage usage in MB."}
	attrs["sent"] = schema.Int64Attribute{Computed: true, MarkdownDescription: "Volatile daily sent count."}
	attrs["suspended"] = schema.BoolAttribute{Computed: true, MarkdownDescription: "Server suspension status; the API has no mailbox suspension write operation."}
	resp.Schema = schema.Schema{Version: schemautil.InitialVersion, MarkdownDescription: "Manage a mailbox. Creation and explicit rotation require a write-only password of 8–72 characters and a positive rotation version. Server-specific strength/character requirements still apply. Import requires no password. Delete permanently removes the mailbox and may destroy its mail; system-mailbox restrictions remain live-unverified. No system identity is inferred or automatically managed.", Attributes: attrs}
}
func (*mailboxResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return schemautil.InitialStateUpgraders()
}
func (r *mailboxResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	configuredClient(req.ProviderData, &r.c, &resp.Diagnostics)
}
func (*mailboxResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var cfg mailboxModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	prior := types.Int64Null()
	creating := req.State.Raw.IsNull()
	if !creating {
		var old mailboxModel
		resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
		prior = old.Version
		if !cfg.Domain.IsUnknown() && !cfg.Username.IsUnknown() {
			creating = !cfg.Domain.Equal(old.Domain) || !strings.EqualFold(cfg.Username.ValueString(), old.Username.ValueString())
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}
	_, d := schemautil.PasswordChange(cfg.Password, cfg.Version, prior, creating, false, 8, 72)
	resp.Diagnostics.Append(d...)
}
func (r *mailboxResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m, cfg mailboxModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	_, d := schemautil.PasswordChange(cfg.Password, m.Version, types.Int64Null(), true, true, 8, 72)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Do not adopt an existing mailbox or create a case variant of it.
	_, err := r.c.FindEmailAccount(ctx, m.Domain.ValueString(), m.Username.ValueString())
	if err == nil {
		resp.Diagnostics.AddError("Mailbox already exists", "Import the existing mailbox before managing it.")
		return
	}
	if !client.IsNotFound(err) {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	input := client.CreateEmailAccountRequest{Username: m.Username.ValueString(), Password: cfg.Password.ValueString()}
	if !m.Quota.IsUnknown() && !m.Quota.IsNull() {
		q := m.Quota.ValueInt64()
		input.Quota = &q
	}
	if !m.Limit.IsUnknown() && !m.Limit.IsNull() {
		l := m.Limit.ValueInt64()
		input.Limit = &l
	}
	if err := r.c.CreateEmailAccount(ctx, m.Domain.ValueString(), input); err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	// Retain identity if readback fails. Terraform may taint this confirmed create;
	// operators must review before replacement rather than replaying the POST.
	m.ID = types.StringValue(m.Domain.ValueString() + "/" + strings.ToLower(m.Username.ValueString()))
	m.Password = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	v, err := readMailbox(ctx, r.c, m.Domain.ValueString(), m.Username.ValueString())
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	mailboxValues(&m, v)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *mailboxResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m mailboxModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, err := readMailbox(ctx, r.c, m.Domain.ValueString(), m.Username.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	mailboxValues(&m, v)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *mailboxResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, old, cfg mailboxModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rotate, d := schemautil.PasswordChange(cfg.Password, m.Version, old.Version, false, true, 8, 72)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, err := readMailbox(ctx, r.c, m.Domain.ValueString(), m.Username.ValueString())
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	input := client.UpdateEmailAccountRequest{}
	if rotate {
		p := cfg.Password.ValueString()
		input.Password = &p
	}
	// Only explicit changed desired fields are written. Optional/computed fields
	// can be unknown after another edit; omission must never become zero.
	if !cfg.Quota.IsNull() && !m.Quota.IsUnknown() && !m.Quota.Equal(old.Quota) {
		q := m.Quota.ValueInt64()
		input.Quota = &q
	}
	if !cfg.Limit.IsNull() && !m.Limit.IsUnknown() && !m.Limit.Equal(old.Limit) {
		l := m.Limit.ValueInt64()
		input.Limit = &l
	}
	if input.Password != nil || input.Quota != nil || input.Limit != nil {
		if err := r.c.UpdateEmailAccount(ctx, m.Domain.ValueString(), *v.Username, input); err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		// The PATCH succeeded. Persist its rotation version before readback so
		// an unrelated read failure cannot replay a confirmed password change.
		completed := m
		mailboxValues(&completed, v)
		if input.Quota != nil {
			completed.Quota = types.Int64Value(*input.Quota)
		}
		if input.Limit != nil {
			completed.Limit = types.Int64Value(*input.Limit)
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &completed)...)
		if resp.Diagnostics.HasError() {
			return
		}
		v, err = readMailbox(ctx, r.c, m.Domain.ValueString(), *v.Username)
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
	}
	mailboxValues(&m, v)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *mailboxResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m mailboxModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, err := readMailbox(ctx, r.c, m.Domain.ValueString(), m.Username.ValueString())
	if client.IsNotFound(err) {
		return
	}
	if err == nil {
		err = r.c.DeleteEmailAccount(ctx, m.Domain.ValueString(), *v.Username)
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
	}
}
func (*mailboxResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	domain, user, err := identity.Scoped(req.ID, false)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import identifier", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), domain)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("username"), user)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), domain+"/"+user)...)
}
