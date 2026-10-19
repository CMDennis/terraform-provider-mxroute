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

type mailDataSource struct {
	kind string
	c    *client.Client
}

func newMailDataSource(kind string) func() datasource.DataSource {
	return func() datasource.DataSource { return &mailDataSource{kind: kind} }
}
func (d *mailDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.kind
}
func mailboxAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"username":  schema.StringAttribute{Computed: true, MarkdownDescription: "Server canonical mailbox local part."},
		"email":     schema.StringAttribute{Computed: true, MarkdownDescription: "Mailbox email address."},
		"quota":     schema.Int64Attribute{Computed: true, MarkdownDescription: "Storage quota in MB; zero means unlimited."},
		"limit":     schema.Int64Attribute{Computed: true, MarkdownDescription: "Daily send limit."},
		"usage":     schema.Float64Attribute{Computed: true, MarkdownDescription: "Volatile storage usage in MB."},
		"sent":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Volatile daily sent count."},
		"suspended": schema.BoolAttribute{Computed: true, MarkdownDescription: "Server-reported suspension status."},
	}
}
func forwarderAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"alias": schema.StringAttribute{Computed: true}, "email": schema.StringAttribute{Computed: true}, "destinations": schema.SetAttribute{Computed: true, ElementType: types.StringType},
	}
}
func (d *mailDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := map[string]schema.Attribute{"domain": schema.StringAttribute{Required: true, Validators: []validator.String{domainValidator{}}, MarkdownDescription: "Owned canonical domain."}}
	descriptions := map[string]string{
		"email_account":  "Look up a mailbox case-insensitively using a complete list then canonical detail read. Ambiguous matches fail. No password can be read.",
		"email_accounts": "List mailboxes and volatile usage in an owned domain. No passwords are returned. Collection completeness remains live-unverified.",
		"forwarder":      "Look up one forwarder from a complete successful list; ambiguous matches fail.",
		"forwarders":     "List forwarders and unordered destination sets for an owned domain. Collection completeness remains live-unverified.",
		"catch_all":      "Read the existing catch-all routing setting. Non-address modes expose a null address, clearing stale values. Unavailable settings are errors.",
	}
	switch d.kind {
	case "email_account":
		for k, v := range mailboxAttributes() {
			if k != "username" {
				attrs[k] = v
			}
		}
		attrs["username"] = schema.StringAttribute{Required: true, Validators: []validator.String{localValidator{}}, MarkdownDescription: "Mailbox local part; case-insensitive lookup."}
		attrs["canonical_username"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Server canonical mailbox local part."}
	case "email_accounts":
		attrs["accounts"] = schema.SetNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: mailboxAttributes()}}
	case "forwarder":
		attrs["alias"] = schema.StringAttribute{Required: true, Validators: []validator.String{localValidator{}}}
		attrs["canonical_alias"] = schema.StringAttribute{Computed: true}
		attrs["email"] = schema.StringAttribute{Computed: true}
		attrs["destinations"] = schema.SetAttribute{Computed: true, ElementType: types.StringType}
	case "forwarders":
		attrs["forwarders"] = schema.SetNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: forwarderAttributes()}}
	case "catch_all":
		attrs["type"] = schema.StringAttribute{Computed: true, MarkdownDescription: "fail, blackhole, or address."}
		attrs["address"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Forwarding destination, null outside address mode."}
		attrs["description"] = schema.StringAttribute{Computed: true}
	}
	resp.Schema = schema.Schema{MarkdownDescription: descriptions[d.kind], Attributes: attrs}
}
func (d *mailDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	configuredClient(req.ProviderData, &d.c, &resp.Diagnostics)
}
func mailboxObject(v client.EmailAccount) types.Object {
	values := map[string]attr.Value{"username": types.StringValue(*v.Username), "email": types.StringValue(*v.Email), "quota": types.Int64Value(*v.Quota), "limit": types.Int64Value(*v.Limit), "usage": types.Float64Value(*v.Usage), "sent": types.Int64Value(*v.Sent), "suspended": types.BoolValue(*v.Suspended)}
	return object(values)
}
func object(values map[string]attr.Value) types.Object {
	ts := map[string]attr.Type{}
	for k, v := range values {
		ts[k] = v.Type(context.Background())
	}
	return types.ObjectValueMust(ts, values)
}
func (d *mailDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var domain types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("domain"), &domain)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.State.Raw = req.Config.Raw
	set := func(k string, v any) { resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(k), v)...) }
	switch d.kind {
	case "email_account":
		var user types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("username"), &user)...)
		if resp.Diagnostics.HasError() {
			return
		}
		v, err := readMailbox(ctx, d.c, domain.ValueString(), user.ValueString())
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		set("canonical_username", *v.Username)
		for k, val := range mailboxObject(*v).Attributes() {
			if k != "username" {
				set(k, val)
			}
		}
	case "email_accounts":
		vs, err := d.c.ListEmailAccounts(ctx, domain.ValueString())
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		values := make([]attr.Value, 0, len(vs))
		// Use schema types even for an empty collection.
		ts := map[string]attr.Type{"username": types.StringType, "email": types.StringType, "quota": types.Int64Type, "limit": types.Int64Type, "usage": types.Float64Type, "sent": types.Int64Type, "suspended": types.BoolType}
		for _, v := range vs {
			values = append(values, mailboxObject(v))
		}
		set("accounts", types.SetValueMust(types.ObjectType{AttrTypes: ts}, values))
	case "forwarder":
		var alias types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("alias"), &alias)...)
		if resp.Diagnostics.HasError() {
			return
		}
		v, err := readForwarder(ctx, d.c, domain.ValueString(), alias.ValueString())
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		set("canonical_alias", *v.Alias)
		set("email", *v.Email)
		values, ds := types.SetValueFrom(ctx, types.StringType, v.Destinations)
		resp.Diagnostics.Append(ds...)
		set("destinations", values)
	case "forwarders":
		vs, err := d.c.ListForwarders(ctx, domain.ValueString())
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		values := make([]attr.Value, 0, len(vs))
		ts := map[string]attr.Type{"alias": types.StringType, "email": types.StringType, "destinations": types.SetType{ElemType: types.StringType}}
		for _, v := range vs {
			destinations, ds := types.SetValueFrom(ctx, types.StringType, v.Destinations)
			resp.Diagnostics.Append(ds...)
			values = append(values, types.ObjectValueMust(ts, map[string]attr.Value{"alias": types.StringValue(*v.Alias), "email": types.StringValue(*v.Email), "destinations": destinations}))
		}
		set("forwarders", types.SetValueMust(types.ObjectType{AttrTypes: ts}, values))
	case "catch_all":
		v, err := readCatchAll(ctx, d.c, domain.ValueString())
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		m := catchAllModel{Domain: domain}
		catchAllValues(&m, v)
		set("type", m.Type)
		set("address", m.Address)
		set("description", m.Description)
	}
}
