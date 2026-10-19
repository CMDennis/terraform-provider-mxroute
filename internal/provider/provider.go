package provider

import (
	"context"
	"os"
	"strings"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type clientFactory func(server, username, apiKey, version string) (*client.Client, error)
type mxrouteProvider struct {
	version   string
	newClient clientFactory
}
type providerModel struct {
	Server   types.String `tfsdk:"server"`
	Username types.String `tfsdk:"username"`
	APIKey   types.String `tfsdk:"api_key"`
}

var _ provider.Provider = &mxrouteProvider{}
var _ provider.ProviderWithValidateConfig = &mxrouteProvider{}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &mxrouteProvider{version: version, newClient: client.New} }
}

func (p *mxrouteProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName, resp.Version = "mxroute", p.version
}

func (p *mxrouteProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "MXroute all-operation beta candidate. Requires Terraform >= 1.11.1, < 2.0. Domains, pointers, mailboxes, routing, shared spam controls, usage reporting, DNS discovery, reseller packages/users and impersonation aliases are mock-tested; live behavior remains unverified. Credentials are kept only in memory; use environment variables or ephemeral inputs to avoid saving them in configuration or plans.",
		Attributes: map[string]schema.Attribute{
			"server":   schema.StringAttribute{Optional: true, MarkdownDescription: "Mail server hostname, without scheme, port or path. Falls back to `MXROUTE_SERVER` only when omitted or null."},
			"username": schema.StringAttribute{Optional: true, MarkdownDescription: "DirectAdmin username or `reseller|user` impersonation identity. Each name contains 1–16 letters, digits or underscores. Falls back to `MXROUTE_USERNAME` only when omitted or null."},
			"api_key":  schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: "Complete API key, including any colons, used unchanged. Falls back to `MXROUTE_API_KEY` only when omitted or null. Sensitive masks display; supply via the environment or an ephemeral input to keep it out of saved plans."},
		},
	}
}

// resolve preserves explicit empty and unknown values rather than replacing
// them with inherited environment values. Diagnostics never contain inputs.
func resolve(v types.String, name, env string, configuring bool, diags *diag.Diagnostics) (string, bool) {
	if v.IsUnknown() {
		if configuring {
			diags.AddAttributeError(path.Root(name), "Unknown provider configuration", "Set this provider input to a value known before apply. Newly created account identities may require a separate apply.")
		}
		return "", false
	}
	value := v.ValueString()
	if v.IsNull() {
		value = os.Getenv(env)
	}
	if strings.TrimSpace(value) == "" {
		diags.AddAttributeError(path.Root(name), "Missing provider configuration", "Set a nonempty "+name+" or supply "+env+" when the argument is omitted.")
		return "", false
	}
	return value, true
}

func (p *mxrouteProvider) values(m providerModel, configuring bool, diags *diag.Diagnostics) (string, string, string) {
	server, ok := resolve(m.Server, "server", "MXROUTE_SERVER", configuring, diags)
	if ok {
		if err := identity.Server(server); err != nil {
			diags.AddAttributeError(path.Root("server"), "Invalid server", err.Error())
		}
	}
	username, ok := resolve(m.Username, "username", "MXROUTE_USERNAME", configuring, diags)
	if ok {
		if _, err := identity.ParseAccount(username); err != nil {
			diags.AddAttributeError(path.Root("username"), "Invalid username", err.Error())
		}
	}
	key, ok := resolve(m.APIKey, "api_key", "MXROUTE_API_KEY", configuring, diags)
	if ok && strings.ContainsAny(key, "\r\n") {
		diags.AddAttributeError(path.Root("api_key"), "Invalid API key", "API keys cannot contain line breaks.")
	}
	return server, username, key
}

func (p *mxrouteProvider) ValidateConfig(ctx context.Context, req provider.ValidateConfigRequest, resp *provider.ValidateConfigResponse) {
	var m providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p.values(m, false, &resp.Diagnostics)
}

func (p *mxrouteProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var m providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	server, username, key := p.values(m, true, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := p.newClient(server, username, key, p.version)
	if err != nil {
		resp.Diagnostics.AddError("Unable to configure MXroute client", "Check the provider inputs. Client initialization failed; input values are redacted.")
		return
	}
	resp.ResourceData, resp.DataSourceData = c, c
}

func (p *mxrouteProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{newDomainResource, newPointerResource, newMailboxResource, newForwarderResource, newCatchAllResource, newSpamResource("settings"), newSpamResource("whitelist_entry"), newSpamResource("blacklist_entry"), newResellerPackageResource, newResellerUserResource}
}
func (p *mxrouteProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newDomainDataSource("verification_key"), newDomainDataSource("domains"), newDomainDataSource("domain"),
		newDomainDataSource("domain_pointers"), newDomainDataSource("domain_pointer"), newDomainDataSource("dns_records"),
		newMailDataSource("email_account"), newMailDataSource("email_accounts"), newMailDataSource("forwarder"), newMailDataSource("forwarders"), newMailDataSource("catch_all"),
		newReportingDataSource("spam_settings"), newReportingDataSource("spam_whitelist"), newReportingDataSource("spam_blacklist"), newReportingDataSource("quota"), newReportingDataSource("email_usage"),
		newResellerDataSource("reseller_packages"), newResellerDataSource("reseller_package"), newResellerDataSource("reseller_users"), newResellerDataSource("reseller_user"),
	}
}
