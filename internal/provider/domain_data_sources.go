package provider

import (
	"context"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type domainDataSource struct {
	kind string
	c    *client.Client
}

func newDomainDataSource(kind string) func() datasource.DataSource {
	return func() datasource.DataSource { return &domainDataSource{kind: kind} }
}
func (d *domainDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.kind
}
func recordAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"type":        schema.StringAttribute{Computed: true, MarkdownDescription: "DNS record type."},
		"name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Record name as returned by the API; preserve content when passing to a DNS provider."},
		"value":       schema.StringAttribute{Computed: true, MarkdownDescription: "Exact DNS content, without added quoting or normalization."},
		"description": schema.StringAttribute{Computed: true, MarkdownDescription: "Optional API description; null if absent."},
	}
}
func recordSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{Computed: true, Attributes: recordAttributes()}
}
func pointerAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"pointer": schema.StringAttribute{Computed: true}, "type": schema.StringAttribute{Computed: true}, "target": schema.StringAttribute{Computed: true},
	}
}
func (d *domainDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := map[string]schema.Attribute{}
	if d.kind != "verification_key" && d.kind != "domains" {
		attrs["domain"] = schema.StringAttribute{Required: true, Validators: []validator.String{domainValidator{}}, MarkdownDescription: "Canonical owned domain name."}
	}
	descriptions := map[string]string{
		"verification_key": "Obtain the account verification key before adding a domain. Create the returned TXT record under the domain using a DNS provider, wait for propagation, then create the MXroute domain. This value is account information and will be stored in Terraform state.",
		"domains":          "List all domain names in the effective account. Returned collections are unordered; live completeness remains unverified.",
		"domain":           "Look up a domain and its mail hosting, SSL status and pointer summary.",
		"domain_pointers":  "List pointers for an owned domain. An unavailable parent produces an error, not an empty list.",
		"domain_pointer":   "Look up one pointer using a complete successful list; ambiguous matches are errors.",
		"dns_records":      "Discover MX and SPF records and nullable DKIM and verification records. DNS content is preserved exactly. This data source does not write records at a DNS provider.",
	}
	switch d.kind {
	case "verification_key":
		attrs["key"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Account verification key."}
		attrs["record"] = recordSchema()
		attrs["description"] = schema.StringAttribute{Computed: true}
	case "domains":
		attrs["domains"] = schema.SetAttribute{Computed: true, ElementType: types.StringType}
	case "domain":
		attrs["mail_hosting"] = schema.BoolAttribute{Computed: true}
		attrs["ssl_enabled"] = schema.BoolAttribute{Computed: true}
		attrs["pointers"] = schema.SetAttribute{Computed: true, ElementType: types.StringType}
	case "domain_pointers":
		attrs["pointers"] = schema.SetNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: pointerAttributes()}}
	case "domain_pointer":
		attrs["pointer"] = schema.StringAttribute{Required: true, Validators: []validator.String{domainValidator{}}}
		attrs["type"] = schema.StringAttribute{Computed: true}
		attrs["target"] = schema.StringAttribute{Computed: true}
	case "dns_records":
		attrs["mx_records"] = schema.SetNestedAttribute{Computed: true, MarkdownDescription: "Unordered MX records; priority and hostname can be passed to a DNS provider.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{"priority": schema.Int64Attribute{Computed: true}, "hostname": schema.StringAttribute{Computed: true}, "description": schema.StringAttribute{Computed: true}}}}
		attrs["spf"] = recordSchema()
		attrs["dkim"] = recordSchema()
		attrs["verification"] = recordSchema()
	}
	resp.Schema = schema.Schema{MarkdownDescription: descriptions[d.kind] + " Mock-tested alpha; runtime API behavior remains live-unverified.", Attributes: attrs}
}
func (d *domainDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	configuredClient(req.ProviderData, &d.c, &resp.Diagnostics)
}

var recordTypes = map[string]attr.Type{"type": types.StringType, "name": types.StringType, "value": types.StringType, "description": types.StringType}
var pointerTypes = map[string]attr.Type{"pointer": types.StringType, "type": types.StringType, "target": types.StringType}
var mxTypes = map[string]attr.Type{"priority": types.Int64Type, "hostname": types.StringType, "description": types.StringType}

func optionalString(v *string) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(*v)
}
func recordValue(v *client.Record) (types.Object, diag.Diagnostics) {
	if v == nil {
		return types.ObjectNull(recordTypes), nil
	}
	return types.ObjectValue(recordTypes, map[string]attr.Value{"type": types.StringValue(*v.Type), "name": types.StringValue(*v.Name), "value": types.StringValue(*v.Value), "description": optionalString(v.Description)})
}
func (d *domainDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	domain, pointer := types.StringNull(), types.StringNull()
	if d.kind != "verification_key" && d.kind != "domains" {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("domain"), &domain)...)
	}
	if d.kind == "domain_pointer" {
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("pointer"), &pointer)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	// Initialize state from config so required selectors retain their exact value.
	resp.State = tfsdk.State{Raw: req.Config.Raw, Schema: req.Config.Schema}
	set := func(name string, value any) {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(name), value)...)
	}
	var err error
	switch d.kind {
	case "verification_key":
		var v *client.VerificationKey
		v, err = d.c.GetVerificationKey(ctx)
		if err == nil {
			set("key", *v.Key)
			record, ds := recordValue(v.Record)
			resp.Diagnostics.Append(ds...)
			set("record", record)
			set("description", optionalString(v.Description))
		}
	case "domains":
		var v []string
		v, err = d.c.ListDomains(ctx)
		if err == nil {
			set("domains", v)
		}
	case "domain":
		var v *client.Domain
		v, err = readDomain(ctx, d.c, domain.ValueString())
		if err == nil {
			set("mail_hosting", *v.MailHosting)
			set("ssl_enabled", *v.SSLEnabled)
			set("pointers", v.Pointers)
		}
	case "domain_pointer":
		var v *client.DomainPointer
		v, err = readPointer(ctx, d.c, domain.ValueString(), pointer.ValueString())
		if err == nil {
			set("type", *v.Type)
			set("target", *v.Target)
		}
	case "domain_pointers":
		var v []client.DomainPointer
		_, err = readDomain(ctx, d.c, domain.ValueString())
		if err == nil {
			v, err = d.c.ListDomainPointers(ctx, domain.ValueString())
		}
		if err == nil {
			values := make([]attr.Value, 0, len(v))
			for _, p := range v {
				o, ds := types.ObjectValue(pointerTypes, map[string]attr.Value{"pointer": types.StringValue(*p.Pointer), "type": types.StringValue(*p.Type), "target": types.StringValue(*p.Target)})
				resp.Diagnostics.Append(ds...)
				values = append(values, o)
			}
			collection, ds := types.SetValue(types.ObjectType{AttrTypes: pointerTypes}, values)
			resp.Diagnostics.Append(ds...)
			set("pointers", collection)
		}
	case "dns_records":
		var v *client.DNSRecords
		v, err = d.c.GetDNSRecords(ctx, domain.ValueString())
		if err == nil {
			for name, p := range map[string]*client.Record{"spf": v.SPF, "dkim": v.DKIM, "verification": v.Verification} {
				record, ds := recordValue(p)
				resp.Diagnostics.Append(ds...)
				set(name, record)
			}
			values := make([]attr.Value, 0, len(v.MXRecords))
			for _, p := range v.MXRecords {
				o, ds := types.ObjectValue(mxTypes, map[string]attr.Value{"priority": types.Int64Value(*p.Priority), "hostname": types.StringValue(*p.Hostname), "description": optionalString(p.Description)})
				resp.Diagnostics.Append(ds...)
				values = append(values, o)
			}
			collection, ds := types.SetValue(types.ObjectType{AttrTypes: mxTypes}, values)
			resp.Diagnostics.Append(ds...)
			set("mx_records", collection)
		}
	}
	if err != nil {
		apiDiagnostic(&resp.Diagnostics, err)
	}
}
