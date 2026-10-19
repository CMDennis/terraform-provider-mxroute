package client

import (
	"context"
	"errors"
	"strings"

	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
)

// Hold the effective-account lock across preflight, write and readback, including
// aliases. Nested typed operations use this private marker to avoid re-locking.
type sharedContextKey struct{}

func (c *Client) sharedChange(ctx context.Context, change func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()
	if err := c.settings.acquire(ctx); err != nil {
		return err
	}
	defer c.settings.release()
	return change(context.WithValue(ctx, sharedContextKey{}, c.settings))
}

// SpamEntries validates the entire complete list before callers infer absence.
// Do not collapse case-insensitive duplicates and accidentally adopt ambiguity.
func (c *Client) SpamEntries(ctx context.Context, domain string, blacklist bool) ([]string, error) {
	var values []string
	var err error
	if blacklist {
		values, err = c.GetSpamBlacklist(ctx, domain)
	} else {
		values, err = c.GetSpamWhitelist(ctx, domain)
	}
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, v := range values {
		key, e := identity.Entry(v)
		if e != nil || len(v) > 254 {
			return nil, &Error{Operation: "spam list", Kind: Protocol}
		}
		if seen[key] {
			return nil, &Error{Operation: "spam list", Kind: Ambiguous}
		}
		seen[key] = true
	}
	return values, nil
}
func (c *Client) FindSpamEntry(ctx context.Context, domain, entry string, blacklist bool) (*string, error) {
	values, err := c.SpamEntries(ctx, domain, blacklist)
	if err != nil {
		return nil, err
	}
	return unique(values, entry, func(s string) string { return s })
}
func (c *Client) ReadSpamSettings(ctx context.Context, domain string) (*SpamSettings, error) {
	v, err := c.GetSpamSettings(ctx, domain)
	if err != nil {
		return nil, err
	}
	if *v.HighScore < 1 || *v.HighScore > 50 {
		return nil, &Error{Operation: "spam settings", Kind: Protocol}
	}
	return v, nil
}

// A failed save may have applied. Read before any retry, and only replay a
// documented busy (503) response after a successful mismatching read. Other
// uncertain outcomes are returned, never blindly retried.
func (c *Client) verifiedSpamChange(ctx context.Context, write func(context.Context) error, matches func(context.Context) (bool, error)) error {
	for attempt := 0; attempt < 3; attempt++ {
		err := write(ctx)
		var apiErr *Error
		uncertain := errors.As(err, &apiErr) && apiErr.Uncertain
		if err != nil && !uncertain {
			return err
		}
		matched, readErr := matches(ctx)
		if readErr != nil {
			// Retain identity after an acknowledged or uncertain mutation so the next
			// refresh can recover; a failed read never proves the object absent.
			return &Error{Operation: "spam readback", Kind: Protocol, Uncertain: true, cause: readErr}
		}
		if matched {
			return nil
		}
		if err == nil {
			return &Error{Operation: "spam readback", Kind: Protocol, Uncertain: true, ReadbackAvailable: true}
		}
		if apiErr.Status != 503 || attempt == 2 {
			return err
		}
		if err := pause(ctx, backoff(c.retryBase, attempt)); err != nil {
			return &Error{Operation: "spam retry", Kind: Upstream, Uncertain: true, cause: err}
		}
	}
	panic("bounded spam loop exhausted")
}

func (c *Client) ApplySpamScore(ctx context.Context, domain string, score int64) error {
	if score < 1 || score > 50 {
		return &Error{Operation: "spam settings", Kind: Validation}
	}
	return c.sharedChange(ctx, func(ctx context.Context) error {
		v, err := c.ReadSpamSettings(ctx, domain)
		if err != nil {
			return err
		}
		if *v.HighScore == score {
			return nil
		}
		return c.verifiedSpamChange(ctx, func(ctx context.Context) error { return c.UpdateSpamSettings(ctx, domain, score) }, func(ctx context.Context) (bool, error) {
			v, err := c.ReadSpamSettings(ctx, domain)
			return err == nil && *v.HighScore == score, err
		})
	})
}
func (c *Client) ChangeSpamEntry(ctx context.Context, domain, entry string, blacklist, present bool) error {
	if _, err := identity.Entry(entry); err != nil || len(entry) > 254 {
		return &Error{Operation: "spam entry", Kind: Validation}
	}
	return c.sharedChange(ctx, func(ctx context.Context) error {
		existing, err := c.FindSpamEntry(ctx, domain, entry, blacklist)
		if err != nil && !IsNotFound(err) {
			return err
		}
		if present && err == nil {
			return &Error{Operation: "spam entry already exists; import to adopt", Kind: Conflict}
		}
		if !present && IsNotFound(err) {
			return nil
		}
		wire := entry
		if existing != nil {
			wire = *existing
		}
		return c.verifiedSpamChange(ctx, func(ctx context.Context) error {
			if blacklist {
				if present {
					return c.AddSpamBlacklistEntry(ctx, domain, wire)
				}
				return c.DeleteSpamBlacklistEntry(ctx, domain, wire)
			}
			if present {
				return c.AddSpamWhitelistEntry(ctx, domain, wire)
			}
			return c.DeleteSpamWhitelistEntry(ctx, domain, wire)
		}, func(ctx context.Context) (bool, error) {
			found, err := c.FindSpamEntry(ctx, domain, entry, blacklist)
			if IsNotFound(err) {
				return !present, nil
			}
			return err == nil && present && strings.EqualFold(*found, entry), err
		})
	})
}
