package provider

import (
	"context"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type resellerDataSource struct {
	kind string
	c    *client.Client
}

func newResellerDataSource(kind string) func() datasource.DataSource {
	return func() datasource.DataSource { return &resellerDataSource{kind: kind} }
}
func (d *resellerDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.kind
}
func (d *resellerDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := map[string]schema.Attribute{}
	plain := " Requires a plain reseller provider; impersonation aliases are rejected before any request."
	description := ""
	switch d.kind {
	case "reseller_packages":
		attrs["names"] = schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Exact package names."}
		description = "List reseller package names from one complete successful response. No per-package detail requests are made." + plain
	case "reseller_package":
		attrs["name"] = schema.StringAttribute{Required: true, Validators: []validator.String{packageNameValidator{}}, MarkdownDescription: "Exact package name."}
		for name, a := range packageLimitAttributes() {
			attrs[name] = schema.StringAttribute{Computed: true, MarkdownDescription: a.description}
		}
		description = "Read one reseller package's limits. Null API limits and the unlimited quota flag become `unlimited`; `0` stays a distinct limit. Package quota is GB." + plain
	case "reseller_users":
		attrs["usernames"] = schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Exact usernames owned by the reseller."}
		description = "List reseller-owned usernames from one complete successful response. No per-user detail requests are made." + plain
	case "reseller_user":
		attrs["username"] = schema.StringAttribute{Required: true, Validators: []validator.String{existingUserValidator{}}, MarkdownDescription: "Exact existing username."}
		attrs["email"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Contact email."}
		attrs["domain"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Primary domain."}
		attrs["package"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Assigned package."}
		attrs["suspended"] = schema.BoolAttribute{Computed: true, MarkdownDescription: "Suspension state."}
		attrs["quota_mb"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Effective disk quota in MB, or `unlimited`."}
		attrs["quota_used_mb"] = schema.Float64Attribute{Computed: true, MarkdownDescription: "Volatile disk usage in MB."}
		description = "Read one reseller-owned user. Passwords are never returned. The effective quota is reported as returned, never derived from package settings." + plain
	}
	resp.Schema = schema.Schema{MarkdownDescription: description, Attributes: attrs}
}

func (d *resellerDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	configuredClient(req.ProviderData, &d.c, &resp.Diagnostics)
}
func (d *resellerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if !requirePlainReseller(d.c, &resp.Diagnostics) {
		return
	}
	resp.State.Raw = req.Config.Raw
	set := func(k string, v any) { resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(k), v)...) }
	key := func(k string) string {
		var v types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(k), &v)...)
		return v.ValueString()
	}
	switch d.kind {
	case "reseller_packages", "reseller_users":
		list, valid, attr := d.c.ListResellerPackages, validPackageName, "names"
		if d.kind == "reseller_users" {
			list, valid, attr = d.c.ListResellerUsers, identity.ExistingUser, "usernames"
		}
		names, err := list(ctx)
		if err == nil {
			var v types.Set
			if v, err = nameSet(ctx, names, valid); err == nil {
				set(attr, v)
			}
		}
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
		}
	case "reseller_package":
		name := key("name")
		if resp.Diagnostics.HasError() {
			return
		}
		_, p, err := readResellerPackage(ctx, d.c, name)
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		for k, v := range map[string]string{"quota_gb": p.QuotaGB, "domains": p.Domains, "email_accounts": p.EmailAccounts, "email_forwarders": p.EmailForwarders, "domain_pointers": p.DomainPointers} {
			set(k, v)
		}
	case "reseller_user":
		name := key("username")
		if resp.Diagnostics.HasError() {
			return
		}
		v, quota, err := readResellerUser(ctx, d.c, name)
		if err != nil {
			apiDiagnostic(&resp.Diagnostics, err)
			return
		}
		set("email", *v.Email)
		set("domain", *v.Domain)
		set("package", *v.Package)
		set("suspended", *v.Suspended)
		set("quota_mb", quota)
		set("quota_used_mb", *v.Quota.Used)
	}
}
