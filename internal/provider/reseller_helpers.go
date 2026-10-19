package provider

import (
	"context"
	"regexp"
	"strconv"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Reseller limits use the API's own write form: a canonical decimal string or
// "unlimited". Package quota is GB; user quota is MB. Response nulls and the
// unlimited flags map back to "unlimited"; zero always stays a distinct limit.
const unlimited = "unlimited"

var wholeNumber = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})$`)
var decimalNumber = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})(\.[0-9]{0,5}[1-9])?$`)

type limitValidator struct{ decimal bool }

func (v limitValidator) Description(context.Context) string {
	if v.decimal {
		return "unlimited, or a canonical non-negative decimal without leading or trailing zeros."
	}
	return "unlimited, or a canonical non-negative integer without leading zeros."
}
func (v limitValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }
func (v limitValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	s := req.ConfigValue.ValueString()
	// GB values round-trip through a JSON number, so they keep fewer digits.
	if s == unlimited || (!v.decimal && wholeNumber.MatchString(s)) || (v.decimal && decimalNumber.MatchString(s)) {
		return
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Invalid limit", "Use \"unlimited\" or "+v.Description(context.Background()))
}

type packageNameValidator struct{}

func (packageNameValidator) Description(context.Context) string {
	return "Package name of letters, digits, underscores or hyphens; case is preserved."
}
func (v packageNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (packageNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() {
		if _, err := identity.Package(req.ConfigValue.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid package name", err.Error())
		}
	}
}

type existingUserValidator struct{}

func (existingUserValidator) Description(context.Context) string {
	return "Existing username of 1–16 letters, digits or underscores. New users require 1–10 lowercase letters, digits or underscores."
}
func (v existingUserValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (existingUserValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if !req.ConfigValue.IsNull() && !req.ConfigValue.IsUnknown() {
		if err := identity.ExistingUser(req.ConfigValue.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid username", err.Error())
		}
	}
}

// requirePlainReseller gives a clear diagnostic before any request. The client
// independently refuses reseller operations through an impersonated alias.
func requirePlainReseller(c *client.Client, d *diag.Diagnostics) bool {
	if c != nil && c.Account().Impersonated {
		d.AddError("Reseller provider required", "Reseller users and packages require a plain reseller provider configuration, not a reseller|user impersonation alias. Use a separate provider alias with the plain reseller username.")
		return false
	}
	return true
}

func countSetting(v *int64) (string, bool) {
	if v == nil {
		return unlimited, true
	}
	return strconv.FormatInt(*v, 10), *v >= 0
}

// Unlimited is authoritative only with a null GB value; a contradictory or
// negative response is incomplete readback, never a guessed value.
func packageQuota(gb *float64, isUnlimited bool) (string, bool) {
	if isUnlimited {
		return unlimited, gb == nil
	}
	if gb == nil || *gb < 0 {
		return "", false
	}
	return strconv.FormatFloat(*gb, 'f', -1, 64), true
}
func userQuota(q *client.UserQuota) (string, bool) {
	if *q.Unlimited {
		return unlimited, q.Limit == nil
	}
	if q.Limit == nil || *q.Limit < 0 {
		return "", false
	}
	return strconv.FormatInt(*q.Limit, 10), true
}

type packageSettings struct{ QuotaGB, Domains, EmailAccounts, EmailForwarders, DomainPointers string }

func packageValues(v *client.ResellerPackage) (packageSettings, error) {
	s := v.Settings
	var p packageSettings
	ok := make([]bool, 5)
	p.QuotaGB, ok[0] = packageQuota(s.QuotaGB, *s.QuotaUnlimited)
	p.Domains, ok[1] = countSetting(s.Domains)
	p.EmailAccounts, ok[2] = countSetting(s.EmailAccounts)
	p.EmailForwarders, ok[3] = countSetting(s.EmailForwarders)
	p.DomainPointers, ok[4] = countSetting(s.DomainPointers)
	for _, valid := range ok {
		if !valid {
			return p, protocolError()
		}
	}
	return p, nil
}

// readResellerPackage proves presence with a complete list before the detail
// read. A detail 404 after a successful list is an error, not absence.
func readResellerPackage(ctx context.Context, c *client.Client, name string) (*client.ResellerPackage, packageSettings, error) {
	if err := c.FindResellerPackage(ctx, name); err != nil {
		return nil, packageSettings{}, err
	}
	v, err := c.GetResellerPackage(ctx, name)
	if err != nil {
		return nil, packageSettings{}, err
	}
	if *v.Name != name {
		return nil, packageSettings{}, protocolError()
	}
	p, err := packageValues(v)
	return v, p, err
}

func readResellerUser(ctx context.Context, c *client.Client, username string) (*client.ResellerUser, string, error) {
	if err := c.FindResellerUser(ctx, username); err != nil {
		return nil, "", err
	}
	v, err := c.GetResellerUser(ctx, username)
	if err != nil {
		return nil, "", err
	}
	if *v.Username != username {
		return nil, "", protocolError()
	}
	if _, err := identity.Package(*v.Package); err != nil {
		return nil, "", protocolError()
	}
	quota, ok := userQuota(v.Quota)
	if !ok || *v.Quota.Used < 0 {
		return nil, "", protocolError()
	}
	return v, quota, nil
}

// nameSet rejects invalid or duplicate list members rather than collapsing them.
func nameSet(ctx context.Context, names []string, valid func(string) error) (types.Set, error) {
	seen := map[string]bool{}
	for _, name := range names {
		if valid(name) != nil || seen[name] {
			return types.SetNull(types.StringType), protocolError()
		}
		seen[name] = true
	}
	v, d := types.SetValueFrom(ctx, types.StringType, names)
	if d.HasError() {
		return v, protocolError()
	}
	return v, nil
}
func validPackageName(s string) error { _, err := identity.Package(s); return err }

// optionalLimit returns a pointer only for an explicitly configured value.
func optionalLimit(cfg, plan, prior types.String) *string {
	if cfg.IsNull() || plan.IsUnknown() || (!prior.IsNull() && !prior.IsUnknown() && plan.Equal(prior)) {
		return nil
	}
	v := plan.ValueString()
	return &v
}
