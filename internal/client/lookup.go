package client

import (
	"context"
	"strings"
)

// unique never guesses between case-folded matches, even when one has the
// desired case. Call only with a successfully decoded complete collection.
func unique[T any](items []T, name string, key func(T) string) (*T, error) {
	var found *T
	for _, item := range items {
		if strings.EqualFold(key(item), name) {
			if found != nil {
				return nil, &Error{Operation: "lookup", Kind: Ambiguous}
			}
			copy := item
			found = &copy
		}
	}
	if found == nil {
		return nil, &Error{Operation: "lookup", Kind: NotFound, ConfirmedAbsent: true}
	}
	return found, nil
}

// exactName is for reseller packages and users, whose lookup is not documented
// as case-insensitive. Only an exact match without case variants proves
// presence; a variant is neither adopted nor treated as absence.
func exactName(items []string, name string) error {
	exact, variants := 0, 0
	for _, item := range items {
		if item == name {
			exact++
		} else if strings.EqualFold(item, name) {
			variants++
		}
	}
	switch {
	case exact == 1 && variants == 0:
		return nil
	case exact == 0 && variants == 0:
		return &Error{Operation: "lookup", Kind: NotFound, ConfirmedAbsent: true}
	}
	return &Error{Operation: "lookup", Kind: Ambiguous}
}
func (c *Client) FindResellerPackage(ctx context.Context, name string) error {
	items, err := c.ListResellerPackages(ctx)
	if err != nil {
		return err
	}
	return exactName(items, name)
}
func (c *Client) FindResellerUser(ctx context.Context, username string) error {
	items, err := c.ListResellerUsers(ctx)
	if err != nil {
		return err
	}
	return exactName(items, username)
}
func (c *Client) FindDomain(ctx context.Context, name string) (*string, error) {
	items, err := c.ListDomains(ctx)
	if err != nil {
		return nil, err
	}
	return unique(items, name, func(s string) string { return s })
}
func (c *Client) FindDomainPointer(ctx context.Context, domain, pointer string) (*DomainPointer, error) {
	items, err := c.ListDomainPointers(ctx, domain)
	if err != nil {
		return nil, err
	}
	return unique(items, pointer, func(p DomainPointer) string { return *p.Pointer })
}
func (c *Client) FindEmailAccount(ctx context.Context, domain, user string) (*EmailAccount, error) {
	items, err := c.ListEmailAccounts(ctx, domain)
	if err != nil {
		return nil, err
	}
	return unique(items, user, func(a EmailAccount) string { return *a.Username })
}
func (c *Client) FindForwarder(ctx context.Context, domain, alias string) (*Forwarder, error) {
	items, err := c.ListForwarders(ctx, domain)
	if err != nil {
		return nil, err
	}
	return unique(items, alias, func(f Forwarder) string { return *f.Alias })
}
