package provider

import (
	"context"
	"errors"
	"strings"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
	"github.com/CMDennis/terraform-provider-mxroute/internal/schemautil"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type scoreValidator struct{}

func (scoreValidator) Description(context.Context) string               { return "Integer from 1 through 50." }
func (v scoreValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (scoreValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() && (req.ConfigValue.ValueInt64() < 1 || req.ConfigValue.ValueInt64() > 50) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid spam score", "Use an integer from 1 through 50.")
	}
}

type entryValidator struct{}

func (entryValidator) Description(context.Context) string {
	return "Spam entry of 1–254 ASCII characters from the documented character set."
}
func (v entryValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (entryValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	v := req.ConfigValue.ValueString()
	if _, err := identity.Entry(v); err != nil || len(v) > 254 {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid spam entry", "Use 1–254 letters, digits, @, dot, hyphen, underscore, *, + or =, without spaces.")
	}
}

type spamModel struct {
	ID     types.String `tfsdk:"id"`
	Domain types.String `tfsdk:"access_domain"`
	Entry  types.String `tfsdk:"entry"`
}
type spamScoreModel struct {
	ID     types.String `tfsdk:"id"`
	Domain types.String `tfsdk:"access_domain"`
	Score  types.Int64  `tfsdk:"high_score"`
}
type spamResource struct {
	c    *client.Client
	kind string
}

func newSpamResource(kind string) func() resource.Resource {
	return func() resource.Resource { return &spamResource{kind: kind} }
}
func (r *spamResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_spam_" + r.kind
}
func (r *spamResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id":            schema.StringAttribute{Computed: true, MarkdownDescription: "Stable account-relative identity, independent of access_domain."},
		"access_domain": schema.StringAttribute{Required: true, Validators: []validator.String{domainValidator{}}, MarkdownDescription: "Owned domain used to access shared settings. May change in place within the same effective account. A missing domain is an error, never proof that shared state disappeared."},
	}
	description := "Manage one shared sender-list entry. One Terraform owner per effective account/list/case-insensitive entry, including across domains, aliases and independent states. Existing entries must be imported. Unrelated entries are preserved. Entry changes replace; case-only edits and access_domain changes do not. Destroy removes only this entry. Import with access-domain/entry; the stored ID is the lowercase entry alone. Cross-state duplicate ownership cannot be detected. Wildcards are encoded once; use a complete domain after @. Runtime behavior is live-unverified."
	if r.kind == "settings" {
		attrs["high_score"] = schema.Int64Attribute{Required: true, Validators: []validator.Int64{scoreValidator{}}, MarkdownDescription: "High-score threshold, 1–50; lower means stricter filtering. Only this documented field is sent. Existing delivery policy determines rejection/discard behavior."}
		description = "Manage spam scoring with one Terraform owner per effective account. Saving affects shared account scoring and the selected domain; do not configure separate copies for different domains. The precise propagation across domains remains live-unverified. Create applies desired settings; import adopts them using an owned access domain as import ID. The stored ID is spam-settings. Destroy only relinquishes management and PRESERVES remote filtering; no reset/delete API exists. Changing access_domain updates in place. Unavailable settings are errors."
	} else {
		attrs["entry"] = schema.StringAttribute{Required: true, Validators: []validator.String{entryValidator{}}, PlanModifiers: localReplace(), MarkdownDescription: "Sender address or pattern; compared case-insensitively. 1–254 documented ASCII characters. Use *@example.net for a whole sender domain; * after @ is not supported by SpamAssassin."}
	}
	resp.Schema = schema.Schema{Version: schemautil.InitialVersion, MarkdownDescription: description, Attributes: attrs}
}
func (*spamResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return schemautil.InitialStateUpgraders()
}
func (r *spamResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	configuredClient(req.ProviderData, &r.c, &resp.Diagnostics)
}
func uncertainSpam(err error) bool { var e *client.Error; return errors.As(err, &e) && e.Uncertain }
func (r *spamResource) write(ctx context.Context, plan tfsdk.Plan, state *tfsdk.State, d *diag.Diagnostics) {
	if r.kind == "settings" {
		var m spamScoreModel
		d.Append(plan.Get(ctx, &m)...)
		if d.HasError() {
			return
		}
		err := r.c.ApplySpamScore(ctx, m.Domain.ValueString(), m.Score.ValueInt64())
		if err == nil || uncertainSpam(err) {
			m.ID = types.StringValue("spam-settings")
			d.Append(state.Set(ctx, &m)...)
		}
		if err != nil {
			apiDiagnostic(d, err)
		}
		return
	}
	var m spamModel
	d.Append(plan.Get(ctx, &m)...)
	if d.HasError() {
		return
	}
	err := r.c.ChangeSpamEntry(ctx, m.Domain.ValueString(), m.Entry.ValueString(), r.kind == "blacklist_entry", true)
	if err == nil || uncertainSpam(err) {
		m.ID = types.StringValue(strings.ToLower(m.Entry.ValueString()))
		d.Append(state.Set(ctx, &m)...)
	}
	if err != nil {
		apiDiagnostic(d, err)
	}
}
func (r *spamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	r.write(ctx, req.Plan, &resp.State, &resp.Diagnostics)
}
func (r *spamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.kind == "settings" {
		r.write(ctx, req.Plan, &resp.State, &resp.Diagnostics)
		return
	}
	var m spamModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// An access-context or case-only edit never mutates the shared list.
	if _, err := r.c.FindSpamEntry(ctx, m.Domain.ValueString(), m.Entry.ValueString(), r.kind == "blacklist_entry"); err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	m.ID = types.StringValue(strings.ToLower(m.Entry.ValueString()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *spamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.kind == "settings" {
		var m spamScoreModel
		resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
		if resp.Diagnostics.HasError() {
			return
		}
		v, err := r.c.ReadSpamSettings(ctx, m.Domain.ValueString())
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		m.Score = types.Int64Value(*v.HighScore)
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		return
	}
	var m spamModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.c.FindSpamEntry(ctx, m.Domain.ValueString(), m.Entry.ValueString(), r.kind == "blacklist_entry")
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *spamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.kind == "settings" {
		return
	}
	var m spamModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.ChangeSpamEntry(ctx, m.Domain.ValueString(), m.Entry.ValueString(), r.kind == "blacklist_entry", false); err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
	}
}
func (r *spamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if r.kind == "settings" {
		domain, err := identity.Domain(req.ID)
		if err != nil {
			resp.Diagnostics.AddError("Invalid import identifier", err.Error())
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("access_domain"), domain)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), "spam-settings")...)
		return
	}
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import identifier", "Use access-domain/entry, with the raw unencoded entry.")
		return
	}
	domain, err := identity.Domain(parts[0])
	if err != nil {
		resp.Diagnostics.AddError("Invalid import identifier", err.Error())
		return
	}
	entry, err := identity.Entry(parts[1])
	if err != nil || len(parts[1]) > 254 {
		resp.Diagnostics.AddError("Invalid import identifier", "Supply an entry of 1–254 documented ASCII characters.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("access_domain"), domain)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("entry"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), entry)...)
}
