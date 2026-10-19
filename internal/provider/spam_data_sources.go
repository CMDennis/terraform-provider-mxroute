package provider

import (
	"context"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type reportingDataSource struct {
	kind string
	c    *client.Client
}

func newReportingDataSource(kind string) func() datasource.DataSource {
	return func() datasource.DataSource { return &reportingDataSource{kind: kind} }
}
func (d *reportingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.kind
}
func byteAttribute(description string) schema.Int64Attribute {
	return schema.Int64Attribute{Computed: true, MarkdownDescription: description + " Unit: bytes, with no MB/GB conversion."}
}
func (d *reportingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := map[string]schema.Attribute{}
	description := "Read account reporting. Values refresh hourly upstream, may be stale, and are computed only; they never cause configuration writes. All results are scoped to the provider's effective account. Unavailable/malformed values are errors, never invented zero values."
	switch d.kind {
	case "spam_settings", "spam_whitelist", "spam_blacklist":
		attrs["access_domain"] = schema.StringAttribute{Required: true, Validators: []validator.String{domainValidator{}}, MarkdownDescription: "Owned canonical domain used as the API access context; a missing domain is an error."}
		if d.kind == "spam_settings" {
			attrs["high_score"] = schema.Int64Attribute{Computed: true, MarkdownDescription: "Current score, 1–50. Saves affect shared account scoring as well as the selected domain. Exact cross-domain propagation remains live-unverified."}
		} else {
			attrs["entries"] = schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Complete shared sender list, with original wire spelling preserved. Case-insensitive duplicate/invalid entries are errors."}
		}
		description = "Read spam controls through an owned access domain. Sender lists are account-wide; access_domain does not own a separate copy. Complete successful list reads are required. Settings unavailable, unverifiable account context and ambiguous lists are errors. Collection completeness and runtime behavior remain live-unverified."
	case "quota":
		attrs["username"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Server-reported effective account username."}
		attrs["total_used"] = byteAttribute("Total storage consumed.")
		attrs["total_limit"] = byteAttribute("Account storage limit; zero means unlimited.")
		attrs["percent_used"] = schema.Float64Attribute{Computed: true, MarkdownDescription: "API-reported percentage; preserved without recomputing unlimited semantics."}
		breakdown := map[string]schema.Attribute{}
		for _, k := range []string{"email", "web", "databases", "backups", "other"} {
			breakdown[k] = byteAttribute("Category storage consumed.")
		}
		attrs["breakdown"] = schema.SingleNestedAttribute{Computed: true, Attributes: breakdown, MarkdownDescription: "Storage category breakdown in bytes."}
		attrs["grace_period"] = schema.SingleNestedAttribute{Computed: true, MarkdownDescription: "Nullable grace period for an over-quota account; null is preserved and clears previous state.", Attributes: map[string]schema.Attribute{
			"days_remaining": schema.Int64Attribute{Computed: true, MarkdownDescription: "Server-reported remaining days."},
			"deadline":       schema.StringAttribute{Computed: true, MarkdownDescription: "Server-provided deadline string, preserved exactly."},
		}}
		attrs["updated_at"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Upstream date-time of the hourly snapshot, preserved exactly including timezone."}
	case "email_usage":
		attrs["username"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Server-reported effective account username."}
		attrs["accounts"] = schema.ListNestedAttribute{Computed: true, MarkdownDescription: "Per-email storage in the server's largest-first order; empty lists remain empty.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"email_address": schema.StringAttribute{Computed: true, MarkdownDescription: "Mailbox email address."},
			"size_bytes":    byteAttribute("Mailbox storage consumed."),
			"updated_at":    schema.StringAttribute{Computed: true, MarkdownDescription: "Upstream hourly snapshot date-time, preserved exactly."},
		}}}
	}
	resp.Schema = schema.Schema{MarkdownDescription: description, Attributes: attrs}
}
func (d *reportingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	configuredClient(req.ProviderData, &d.c, &resp.Diagnostics)
}
func (d *reportingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	resp.State.Raw = req.Config.Raw
	set := func(k string, v any) { resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(k), v)...) }
	switch d.kind {
	case "spam_settings", "spam_whitelist", "spam_blacklist":
		var domain types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("access_domain"), &domain)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if d.kind == "spam_settings" {
			v, err := d.c.ReadSpamSettings(ctx, domain.ValueString())
			if err != nil {
				apiDiagnostic(&resp.Diagnostics, err)
				return
			}
			set("high_score", *v.HighScore)
			return
		}
		v, err := d.c.SpamEntries(ctx, domain.ValueString(), d.kind == "spam_blacklist")
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		values, ds := types.SetValueFrom(ctx, types.StringType, v)
		resp.Diagnostics.Append(ds...)
		set("entries", values)
	case "quota":
		v, err := d.c.GetQuota(ctx)
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		set("username", *v.Username)
		set("total_used", *v.TotalUsed)
		set("total_limit", *v.TotalLimit)
		set("percent_used", *v.PercentUsed)
		set("updated_at", *v.UpdatedAt)
		b := v.Breakdown
		set("breakdown", object(map[string]attr.Value{"email": types.Int64Value(*b.Email), "web": types.Int64Value(*b.Web), "databases": types.Int64Value(*b.Databases), "backups": types.Int64Value(*b.Backups), "other": types.Int64Value(*b.Other)}))
		graceTypes := map[string]attr.Type{"days_remaining": types.Int64Type, "deadline": types.StringType}
		grace := types.ObjectNull(graceTypes)
		if v.GracePeriod != nil {
			grace = object(map[string]attr.Value{"days_remaining": types.Int64Value(*v.GracePeriod.DaysRemaining), "deadline": types.StringValue(*v.GracePeriod.Deadline)})
		}
		set("grace_period", grace)
	case "email_usage":
		v, err := d.c.GetEmailUsage(ctx)
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		set("username", *v.Username)
		ts := map[string]attr.Type{"email_address": types.StringType, "size_bytes": types.Int64Type, "updated_at": types.StringType}
		values := make([]attr.Value, 0, len(v.Accounts))
		for _, v := range v.Accounts {
			values = append(values, object(map[string]attr.Value{"email_address": types.StringValue(*v.EmailAddress), "size_bytes": types.Int64Value(*v.SizeBytes), "updated_at": types.StringValue(*v.UpdatedAt)}))
		}
		set("accounts", types.ListValueMust(types.ObjectType{AttrTypes: ts}, values))
	}
}
