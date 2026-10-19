// Typed methods implement the pinned 43-operation baseline.
package client

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
)

func validPassword(value string, maximum int) bool {
	n := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && n >= 8 && (maximum == 0 || n <= maximum)
}

func read[T any](ctx context.Context, c *Client, op operation) (*T, error) {
	var value T
	if err := c.execute(ctx, op, nil, &value); err != nil {
		return nil, err
	}
	return &value, nil
}
func readList[T any](ctx context.Context, c *Client, op operation) ([]T, error) {
	value, err := read[[]T](ctx, c, op)
	if err != nil {
		return nil, err
	}
	return *value, nil
}

// Ambiguous writes are never replayed. Attempt a bounded read to report whether
// remote state is available for reconciliation; do not claim success based on
// existence alone or infer whether a password/side effect was applied.
func (c *Client) write(ctx context.Context, op operation, body, out any, observe func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()
	err := c.execute(ctx, op, body, out)
	var apiError *Error
	if errors.As(err, &apiError) && apiError.Uncertain && ctx.Err() == nil && observe != nil {
		apiError.ReadbackAvailable = observe(ctx) == nil
	}
	return err
}

func (c *Client) DeleteDomain(ctx context.Context, domain string) error {
	return c.write(ctx, operation{name: "DeleteDomain", method: "DELETE", path: identity.Path("domains", domain), status: 204, mode: sparse, reseller: false, shared: false}, nil, nil, func(ctx context.Context) error { _, err := c.ListDomains(ctx); return err })
}
func (c *Client) DeleteEmailAccount(ctx context.Context, domain string, user string) error {
	return c.write(ctx, operation{name: "DeleteEmailAccount", method: "DELETE", path: identity.Path("domains", domain, "email-accounts", user), status: 204, mode: sparse, reseller: false, shared: false}, nil, nil, func(ctx context.Context) error { _, err := c.ListEmailAccounts(ctx, domain); return err })
}
func (c *Client) DeleteForwarder(ctx context.Context, domain string, alias string) error {
	return c.write(ctx, operation{name: "DeleteForwarder", method: "DELETE", path: identity.Path("domains", domain, "forwarders", alias), status: 204, mode: sparse, reseller: false, shared: false}, nil, nil, func(ctx context.Context) error { _, err := c.ListForwarders(ctx, domain); return err })
}
func (c *Client) DeleteDomainPointer(ctx context.Context, domain string, pointer string) error {
	return c.write(ctx, operation{name: "DeleteDomainPointer", method: "DELETE", path: identity.Path("domains", domain, "pointers", pointer), status: 204, mode: sparse, reseller: false, shared: false}, nil, nil, func(ctx context.Context) error { _, err := c.ListDomainPointers(ctx, domain); return err })
}
func (c *Client) DeleteSpamBlacklistEntry(ctx context.Context, domain string, entry string) error {
	return c.write(ctx, operation{name: "DeleteSpamBlacklistEntry", method: "DELETE", path: identity.Path("domains", domain, "spam", "blacklist", entry), status: 204, mode: sparse, reseller: false, shared: true}, nil, nil, func(ctx context.Context) error { _, err := c.GetSpamBlacklist(ctx, domain); return err })
}
func (c *Client) DeleteSpamWhitelistEntry(ctx context.Context, domain string, entry string) error {
	return c.write(ctx, operation{name: "DeleteSpamWhitelistEntry", method: "DELETE", path: identity.Path("domains", domain, "spam", "whitelist", entry), status: 204, mode: sparse, reseller: false, shared: true}, nil, nil, func(ctx context.Context) error { _, err := c.GetSpamWhitelist(ctx, domain); return err })
}
func (c *Client) DeleteResellerPackage(ctx context.Context, name string) error {
	return c.write(ctx, operation{name: "DeleteResellerPackage", method: "DELETE", path: identity.Path("reseller", "packages", name), status: 204, mode: sparse, reseller: true, shared: false}, nil, nil, func(ctx context.Context) error { _, err := c.ListResellerPackages(ctx); return err })
}
func (c *Client) DeleteResellerUser(ctx context.Context, username string) error {
	return c.write(ctx, operation{name: "DeleteResellerUser", method: "DELETE", path: identity.Path("reseller", "users", username), status: 204, mode: sparse, reseller: true, shared: false}, nil, nil, func(ctx context.Context) error { _, err := c.ListResellerUsers(ctx); return err })
}
func (c *Client) ListDomains(ctx context.Context) ([]string, error) {
	return readList[string](ctx, c, operation{name: "ListDomains", method: "GET", path: identity.Path("domains"), status: 200, mode: wrapped, reseller: false, shared: false})
}
func (c *Client) GetDomain(ctx context.Context, domain string) (*Domain, error) {
	return read[Domain](ctx, c, operation{name: "GetDomain", method: "GET", path: identity.Path("domains", domain), status: 200, mode: wrapped, reseller: false, shared: false})
}
func (c *Client) GetCatchAll(ctx context.Context, domain string) (*CatchAll, error) {
	return read[CatchAll](ctx, c, operation{name: "GetCatchAll", method: "GET", path: identity.Path("domains", domain, "catch-all"), status: 200, mode: wrapped, reseller: false, shared: false})
}
func (c *Client) GetDNSRecords(ctx context.Context, domain string) (*DNSRecords, error) {
	return read[DNSRecords](ctx, c, operation{name: "GetDNSRecords", method: "GET", path: identity.Path("domains", domain, "dns"), status: 200, mode: wrapped, reseller: false, shared: false})
}
func (c *Client) ListEmailAccounts(ctx context.Context, domain string) ([]EmailAccount, error) {
	return readList[EmailAccount](ctx, c, operation{name: "ListEmailAccounts", method: "GET", path: identity.Path("domains", domain, "email-accounts"), status: 200, mode: wrapped, reseller: false, shared: false})
}
func (c *Client) GetEmailAccount(ctx context.Context, domain string, user string) (*EmailAccount, error) {
	return read[EmailAccount](ctx, c, operation{name: "GetEmailAccount", method: "GET", path: identity.Path("domains", domain, "email-accounts", user), status: 200, mode: wrapped, reseller: false, shared: false})
}
func (c *Client) ListForwarders(ctx context.Context, domain string) ([]Forwarder, error) {
	return readList[Forwarder](ctx, c, operation{name: "ListForwarders", method: "GET", path: identity.Path("domains", domain, "forwarders"), status: 200, mode: wrapped, reseller: false, shared: false})
}
func (c *Client) ListDomainPointers(ctx context.Context, domain string) ([]DomainPointer, error) {
	return readList[DomainPointer](ctx, c, operation{name: "ListDomainPointers", method: "GET", path: identity.Path("domains", domain, "pointers"), status: 200, mode: wrapped, reseller: false, shared: false})
}
func (c *Client) GetSpamBlacklist(ctx context.Context, domain string) ([]string, error) {
	return readList[string](ctx, c, operation{name: "GetSpamBlacklist", method: "GET", path: identity.Path("domains", domain, "spam", "blacklist"), status: 200, mode: wrapped, reseller: false, shared: false})
}
func (c *Client) GetSpamSettings(ctx context.Context, domain string) (*SpamSettings, error) {
	return read[SpamSettings](ctx, c, operation{name: "GetSpamSettings", method: "GET", path: identity.Path("domains", domain, "spam", "settings"), status: 200, mode: wrapped, reseller: false, shared: false})
}
func (c *Client) GetSpamWhitelist(ctx context.Context, domain string) ([]string, error) {
	return readList[string](ctx, c, operation{name: "GetSpamWhitelist", method: "GET", path: identity.Path("domains", domain, "spam", "whitelist"), status: 200, mode: wrapped, reseller: false, shared: false})
}
func (c *Client) GetQuota(ctx context.Context) (*Quota, error) {
	return read[Quota](ctx, c, operation{name: "GetQuota", method: "GET", path: identity.Path("quota"), status: 200, mode: raw, reseller: false, shared: false})
}
func (c *Client) GetEmailUsage(ctx context.Context) (*EmailUsage, error) {
	return read[EmailUsage](ctx, c, operation{name: "GetEmailUsage", method: "GET", path: identity.Path("quota", "email"), status: 200, mode: raw, reseller: false, shared: false})
}
func (c *Client) ListResellerPackages(ctx context.Context) ([]string, error) {
	return readList[string](ctx, c, operation{name: "ListResellerPackages", method: "GET", path: identity.Path("reseller", "packages"), status: 200, mode: wrapped, reseller: true, shared: false})
}
func (c *Client) GetResellerPackage(ctx context.Context, name string) (*ResellerPackage, error) {
	return read[ResellerPackage](ctx, c, operation{name: "GetResellerPackage", method: "GET", path: identity.Path("reseller", "packages", name), status: 200, mode: wrapped, reseller: true, shared: false})
}
func (c *Client) ListResellerUsers(ctx context.Context) ([]string, error) {
	return readList[string](ctx, c, operation{name: "ListResellerUsers", method: "GET", path: identity.Path("reseller", "users"), status: 200, mode: wrapped, reseller: true, shared: false})
}
func (c *Client) GetResellerUser(ctx context.Context, username string) (*ResellerUser, error) {
	return read[ResellerUser](ctx, c, operation{name: "GetResellerUser", method: "GET", path: identity.Path("reseller", "users", username), status: 200, mode: wrapped, reseller: true, shared: false})
}
func (c *Client) GetVerificationKey(ctx context.Context) (*VerificationKey, error) {
	return read[VerificationKey](ctx, c, operation{name: "GetVerificationKey", method: "GET", path: identity.Path("verification-key"), status: 200, mode: wrapped, reseller: false, shared: false})
}
func (c *Client) SetCatchAll(ctx context.Context, domain string, input SetCatchAllRequest) error {
	if (input.Type != "fail" && input.Type != "blackhole" && input.Type != "address") || (input.Type == "address" && (input.Address == nil || *input.Address == "")) {
		return &Error{Operation: "SetCatchAll", Kind: Validation}
	}
	return c.write(ctx, operation{name: "SetCatchAll", method: "PATCH", path: identity.Path("domains", domain, "catch-all"), status: 200, mode: sparse, reseller: false, shared: false}, input, nil, func(ctx context.Context) error { _, err := c.GetCatchAll(ctx, domain); return err })
}
func (c *Client) UpdateEmailAccount(ctx context.Context, domain string, user string, input UpdateEmailAccountRequest) error {
	if input.Password != nil && !validPassword(*input.Password, 72) {
		return &Error{Operation: "UpdateEmailAccount", Kind: Validation}
	}
	return c.write(ctx, operation{name: "UpdateEmailAccount", method: "PATCH", path: identity.Path("domains", domain, "email-accounts", user), status: 200, mode: sparse, reseller: false, shared: false}, input, nil, func(ctx context.Context) error { _, err := c.ListEmailAccounts(ctx, domain); return err })
}
func (c *Client) SetMailHosting(ctx context.Context, domain string, enabled bool) error {
	return c.write(ctx, operation{name: "SetMailHosting", method: "PATCH", path: identity.Path("domains", domain, "mail-status"), status: 200, mode: sparse, reseller: false, shared: false}, struct {
		Value bool `json:"enabled"`
	}{enabled}, nil, func(ctx context.Context) error { _, err := c.GetDomain(ctx, domain); return err })
}
func (c *Client) UpdateSpamSettings(ctx context.Context, domain string, highScore int64) error {
	if highScore < 1 || highScore > 50 {
		return &Error{Operation: "UpdateSpamSettings", Kind: Validation}
	}
	return c.write(ctx, operation{name: "UpdateSpamSettings", method: "PATCH", path: identity.Path("domains", domain, "spam", "settings"), status: 200, mode: sparse, reseller: false, shared: true}, struct {
		Value int64 `json:"high_score"`
	}{highScore}, nil, func(ctx context.Context) error { _, err := c.GetSpamSettings(ctx, domain); return err })
}
func (c *Client) UpdateResellerPackage(ctx context.Context, name string, input PackageLimits) error {
	return c.write(ctx, operation{name: "UpdateResellerPackage", method: "PATCH", path: identity.Path("reseller", "packages", name), status: 200, mode: sparse, reseller: true, shared: false}, input, nil, func(ctx context.Context) error { _, err := c.ListResellerPackages(ctx); return err })
}
func (c *Client) UpdateResellerUser(ctx context.Context, username string, input UpdateResellerUserRequest) error {
	if input.Password != nil && !validPassword(*input.Password, 0) {
		return &Error{Operation: "UpdateResellerUser", Kind: Validation}
	}
	return c.write(ctx, operation{name: "UpdateResellerUser", method: "PATCH", path: identity.Path("reseller", "users", username), status: 200, mode: sparse, reseller: true, shared: false}, input, nil, func(ctx context.Context) error { _, err := c.ListResellerUsers(ctx); return err })
}
func (c *Client) SetResellerUserPackage(ctx context.Context, username string, packageName string) error {
	return c.write(ctx, operation{name: "SetResellerUserPackage", method: "PATCH", path: identity.Path("reseller", "users", username, "package"), status: 200, mode: sparse, reseller: true, shared: false}, struct {
		Value string `json:"package"`
	}{packageName}, nil, func(ctx context.Context) error { _, err := c.ListResellerUsers(ctx); return err })
}
func (c *Client) CreateDomain(ctx context.Context, input CreateDomainRequest) (*DomainCreated, error) {
	var result DomainCreated
	if err := c.write(ctx, operation{name: "CreateDomain", method: "POST", path: identity.Path("domains"), status: 201, mode: wrapped, reseller: false, shared: false}, input, &result, func(ctx context.Context) error { _, err := c.ListDomains(ctx); return err }); err != nil {
		return nil, err
	}
	return &result, nil
}
func (c *Client) CreateEmailAccount(ctx context.Context, domain string, input CreateEmailAccountRequest) error {
	if !validPassword(input.Password, 72) {
		return &Error{Operation: "CreateEmailAccount", Kind: Validation}
	}
	return c.write(ctx, operation{name: "CreateEmailAccount", method: "POST", path: identity.Path("domains", domain, "email-accounts"), status: 201, mode: sparse, reseller: false, shared: false}, input, nil, func(ctx context.Context) error { _, err := c.ListEmailAccounts(ctx, domain); return err })
}
func (c *Client) CreateForwarder(ctx context.Context, domain string, input CreateForwarderRequest) error {
	return c.write(ctx, operation{name: "CreateForwarder", method: "POST", path: identity.Path("domains", domain, "forwarders"), status: 201, mode: sparse, reseller: false, shared: false}, input, nil, func(ctx context.Context) error { _, err := c.ListForwarders(ctx, domain); return err })
}
func (c *Client) CreateDomainPointer(ctx context.Context, domain string, input CreateDomainPointerRequest) error {
	return c.write(ctx, operation{name: "CreateDomainPointer", method: "POST", path: identity.Path("domains", domain, "pointers"), status: 201, mode: sparse, reseller: false, shared: false}, input, nil, func(ctx context.Context) error { _, err := c.ListDomainPointers(ctx, domain); return err })
}
func (c *Client) AddSpamBlacklistEntry(ctx context.Context, domain string, entry string) error {
	if _, err := identity.Entry(entry); err != nil || len(entry) > 254 {
		return &Error{Operation: "AddSpamBlacklistEntry", Kind: Validation}
	}
	return c.write(ctx, operation{name: "AddSpamBlacklistEntry", method: "POST", path: identity.Path("domains", domain, "spam", "blacklist"), status: 201, mode: sparse, reseller: false, shared: true}, struct {
		Value string `json:"entry"`
	}{entry}, nil, func(ctx context.Context) error { _, err := c.GetSpamBlacklist(ctx, domain); return err })
}
func (c *Client) AddSpamWhitelistEntry(ctx context.Context, domain string, entry string) error {
	if _, err := identity.Entry(entry); err != nil || len(entry) > 254 {
		return &Error{Operation: "AddSpamWhitelistEntry", Kind: Validation}
	}
	return c.write(ctx, operation{name: "AddSpamWhitelistEntry", method: "POST", path: identity.Path("domains", domain, "spam", "whitelist"), status: 201, mode: sparse, reseller: false, shared: true}, struct {
		Value string `json:"entry"`
	}{entry}, nil, func(ctx context.Context) error { _, err := c.GetSpamWhitelist(ctx, domain); return err })
}
func (c *Client) CreateResellerPackage(ctx context.Context, input CreateResellerPackageRequest) error {
	return c.write(ctx, operation{name: "CreateResellerPackage", method: "POST", path: identity.Path("reseller", "packages"), status: 201, mode: sparse, reseller: true, shared: false}, input, nil, func(ctx context.Context) error { _, err := c.ListResellerPackages(ctx); return err })
}
func (c *Client) CreateResellerUser(ctx context.Context, input CreateResellerUserRequest) error {
	if identity.ValidateNewUser(input.Username) != nil {
		return &Error{Operation: "CreateResellerUser", Kind: Validation}
	}
	if !validPassword(input.Password, 0) {
		return &Error{Operation: "CreateResellerUser", Kind: Validation}
	}
	return c.write(ctx, operation{name: "CreateResellerUser", method: "POST", path: identity.Path("reseller", "users"), status: 201, mode: sparse, reseller: true, shared: false}, input, nil, func(ctx context.Context) error { _, err := c.ListResellerUsers(ctx); return err })
}
func (c *Client) SuspendResellerUser(ctx context.Context, username string) error {
	return c.write(ctx, operation{name: "SuspendResellerUser", method: "POST", path: identity.Path("reseller", "users", username, "suspend"), status: 200, mode: sparse, reseller: true, shared: false}, nil, nil, func(ctx context.Context) error { _, err := c.ListResellerUsers(ctx); return err })
}
func (c *Client) UnsuspendResellerUser(ctx context.Context, username string) error {
	return c.write(ctx, operation{name: "UnsuspendResellerUser", method: "POST", path: identity.Path("reseller", "users", username, "unsuspend"), status: 200, mode: sparse, reseller: true, shared: false}, nil, nil, func(ctx context.Context) error { _, err := c.ListResellerUsers(ctx); return err })
}
