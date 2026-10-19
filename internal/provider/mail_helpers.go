package provider

import (
	"context"
	"net/mail"
	"strings"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Keep configured casing separate from the server's canonical local part. A
// casing-only edit is an update with no remote write, never a replacement.
func localReplace() []planmodifier.String {
	return []planmodifier.String{stringplanmodifier.RequiresReplaceIf(func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
		resp.RequiresReplace = !strings.EqualFold(req.PlanValue.ValueString(), req.StateValue.ValueString())
	}, "Replace when the local identity changes, ignoring case.", "Replace when the local identity changes, ignoring case.")}
}
func validLocal(s string) bool {
	_, _, err := identity.Scoped("example.com/"+s, false)
	return err == nil
}

type localValidator struct{}

func (localValidator) Description(context.Context) string {
	return "A nonempty mailbox or alias local part, without a domain or separator."
}
func (v localValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (localValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() && !validLocal(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid local part", "Supply a nonempty local part without @, whitespace, or path separators.")
	}
}

type sendLimitValidator struct{}

func (sendLimitValidator) Description(context.Context) string {
	return "Daily send limit at most 9600; other bounds remain server validated."
}
func (v sendLimitValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (sendLimitValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() && req.ConfigValue.ValueInt64() > 9600 {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid daily send limit", "The documented daily send ceiling is 9600.")
	}
}
func validAddress(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && strings.Count(s, "@") == 1 && !strings.ContainsAny(s, "\r\n\t ,;<>")
}

type destinationsValidator struct{}

func (destinationsValidator) Description(context.Context) string {
	return "One or more email addresses, or a single :blackhole: or :fail: target."
}
func (v destinationsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (destinationsValidator) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	values := req.ConfigValue.Elements()
	if len(values) == 0 {
		resp.Diagnostics.AddAttributeError(req.Path, "Missing destinations", "Supply at least one destination.")
		return
	}
	for _, a := range values {
		s := a.(types.String)
		if s.IsUnknown() {
			continue
		}
		special := s.ValueString() == ":blackhole:" || s.ValueString() == ":fail:"
		if s.IsNull() || (special && len(values) != 1) || (!special && !validAddress(s.ValueString())) {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid forwarding destinations", "Use email addresses, or exactly one special destination (:blackhole: or :fail:) by itself. Special-target combinations are unverified and unsupported.")
			return
		}
	}
}
func readMailbox(ctx context.Context, c *client.Client, domain, user string) (*client.EmailAccount, error) {
	a, err := c.FindEmailAccount(ctx, domain, user)
	if err != nil {
		return nil, err
	}
	// A detail 404 after a successful lookup is an error, not proof of absence.
	v, err := c.GetEmailAccount(ctx, domain, *a.Username)
	if err != nil {
		return nil, err
	}
	if !validLocal(*v.Username) || !strings.EqualFold(*v.Username, *a.Username) || !strings.EqualFold(*v.Email, *v.Username+"@"+domain) {
		return nil, protocolError()
	}
	return v, nil
}
func readForwarder(ctx context.Context, c *client.Client, domain, alias string) (*client.Forwarder, error) {
	v, err := c.FindForwarder(ctx, domain, alias)
	if err != nil {
		return nil, err
	}
	if !validLocal(*v.Alias) || !strings.EqualFold(*v.Email, *v.Alias+"@"+domain) {
		return nil, protocolError()
	}
	return v, nil
}
func mailboxValues(m *mailboxModel, v *client.EmailAccount) {
	m.ID = types.StringValue(m.Domain.ValueString() + "/" + strings.ToLower(*v.Username))
	m.CanonicalUsername, m.Email = types.StringValue(*v.Username), types.StringValue(*v.Email)
	m.Quota, m.Limit = types.Int64Value(*v.Quota), types.Int64Value(*v.Limit)
	m.Usage, m.Sent, m.Suspended = types.Float64Value(*v.Usage), types.Int64Value(*v.Sent), types.BoolValue(*v.Suspended)
	m.Password = types.StringNull()
}
func forwarderValues(ctx context.Context, m *forwarderModel, v *client.Forwarder, d *diag.Diagnostics) {
	m.ID = types.StringValue(m.Domain.ValueString() + "/" + strings.ToLower(*v.Alias))
	m.CanonicalAlias, m.Email = types.StringValue(*v.Alias), types.StringValue(*v.Email)
	var ds diag.Diagnostics
	m.Destinations, ds = types.SetValueFrom(ctx, types.StringType, v.Destinations)
	d.Append(ds...)
}
func readCatchAll(ctx context.Context, c *client.Client, domain string) (*client.CatchAll, error) {
	v, err := c.GetCatchAll(ctx, domain)
	if err != nil {
		return nil, err
	}
	if *v.Type != "fail" && *v.Type != "blackhole" && *v.Type != "address" {
		return nil, protocolError()
	}
	if *v.Type == "address" && (v.Address == nil || !validAddress(*v.Address)) {
		return nil, protocolError()
	}
	return v, nil
}
func catchAllValues(m *catchAllModel, v *client.CatchAll) {
	m.ID, m.Type = m.Domain, types.StringValue(*v.Type)
	m.Address = types.StringNull()
	if *v.Type == "address" {
		m.Address = types.StringValue(*v.Address)
	}
	m.Description = types.StringNull()
	if v.Description != nil {
		m.Description = types.StringValue(*v.Description)
	}
}
