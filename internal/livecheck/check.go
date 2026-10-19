// Package livecheck implements bounded ordinary-account read-only discovery.
// Inventories and fingerprints live only in memory. Reports contain no object
// identities, response bodies, credentials, counts or inventory fingerprints.
package livecheck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
)

type Report struct {
	Checks            map[string]string `json:"checks"`
	Completed         bool              `json:"completed"`
	BaselineUnchanged bool              `json:"baseline_unchanged"`
}

// Run never imports objects, creates Terraform state, or calls a write method.
// Use client.NewReadOnly at the live entry point for transport enforcement.
func Run(ctx context.Context, c *client.Client) Report {
	r := Report{Checks: map[string]string{}}
	before, ok := snapshot(ctx, c, &r)
	if !ok {
		return r
	}
	after, ok := snapshot(ctx, c, &r)
	if !ok {
		return r
	}
	r.Completed = true
	r.BaselineUnchanged = len(before) == len(after)
	for key, value := range before {
		if after[key] != value {
			r.BaselineUnchanged = false
		}
	}
	return r
}

func capture[T any](r *Report, hashes map[string][32]byte, op, key string, read func() (T, error), stable func(T) any) (T, bool) {
	v, err := read()
	if err != nil {
		status := "failed"
		var apiErr *client.Error
		if errors.As(err, &apiErr) {
			status = fmt.Sprintf("failed:%s:HTTP%d", apiErr.Kind, apiErr.Status)
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = "failed:cancelled"
		}
		r.Checks[op] = status
		return v, false
	}
	data, err := json.Marshal(stable(v))
	if err != nil {
		r.Checks[op] = "failed:encoding"
		return v, false
	}
	// Arrays in these discovery responses are unordered collections. Sort their
	// canonical JSON encodings recursively; duplicates remain detectable.
	var tree any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&tree) != nil {
		r.Checks[op] = "failed:encoding"
		return v, false
	}
	data, _ = json.Marshal(canonical(tree))
	hashes[key] = sha256.Sum256(data)
	r.Checks[op] = "passed"
	return v, true
}

func canonical(v any) any {
	switch value := v.(type) {
	case map[string]any:
		for k, item := range value {
			value[k] = canonical(item)
		}
	case []any:
		for i, item := range value {
			value[i] = canonical(item)
		}
		sort.Slice(value, func(i, j int) bool {
			a, _ := json.Marshal(value[i])
			b, _ := json.Marshal(value[j])
			return string(a) < string(b)
		})
	}
	return v
}

func unchanged[T any](v T) any { return v }

func stableMailbox(v client.EmailAccount) client.EmailAccount {
	// Incoming mail and outgoing counters may legitimately change during reads.
	v.Usage, v.Sent = nil, nil
	return v
}

func snapshot(ctx context.Context, c *client.Client, r *Report) (map[string][32]byte, bool) {
	return snapshotExceptMailbox(ctx, c, r, "", "")
}

func snapshotExceptMailbox(ctx context.Context, c *client.Client, r *Report, excludeDomain, excludeUser string) (map[string][32]byte, bool) {
	h := map[string][32]byte{}
	domains, ok := capture(r, h, "ListDomains", "domains", func() ([]string, error) { return c.ListDomains(ctx) }, unchanged[[]string])
	if !ok {
		return h, false
	}
	// At most 204 logical reads per snapshot, never an unbounded per-item
	// fanout. Typed client rate limits and retries apply to each HTTP attempt.
	if len(domains) > 20 {
		r.Checks["inventory_budget"] = "failed:scope_exceeds_budget"
		return h, false
	}
	seen := map[string]bool{}
	for _, domain := range domains {
		normal, err := identity.Domain(domain)
		if err != nil || normal != domain || seen[normal] {
			r.Checks["inventory_identity"] = "failed:invalid_or_duplicate"
			return h, false
		}
		seen[normal] = true
	}
	if _, ok = capture(r, h, "GetVerificationKey", "verification", func() (*client.VerificationKey, error) { return c.GetVerificationKey(ctx) }, unchanged[*client.VerificationKey]); !ok {
		return h, false
	}
	if _, ok = capture(r, h, "GetQuota", "quota", func() (*client.Quota, error) { return c.GetQuota(ctx) }, func(v *client.Quota) any { return v.TotalLimit }); !ok {
		return h, false
	}
	if _, ok = capture(r, h, "GetEmailUsage", "usage", func() (*client.EmailUsage, error) { return c.GetEmailUsage(ctx) }, func(v *client.EmailUsage) any {
		// Account usage may refresh hourly. Collection membership is checked by
		// ListEmailAccounts; this report exercises usage decoding only.
		return "decoded"
	}); !ok {
		return h, false
	}
	sort.Strings(domains)
	for _, domain := range domains {
		key := domain + "/"
		if _, ok = capture(r, h, "GetDomain", key+"domain", func() (*client.Domain, error) { return c.GetDomain(ctx, domain) }, unchanged[*client.Domain]); !ok {
			return h, false
		}
		if _, ok = capture(r, h, "ListDomainPointers", key+"pointers", func() ([]client.DomainPointer, error) { return c.ListDomainPointers(ctx, domain) }, unchanged[[]client.DomainPointer]); !ok {
			return h, false
		}
		mailboxes, valid := capture(r, h, "ListEmailAccounts", key+"mailboxes", func() ([]client.EmailAccount, error) {
			items, err := c.ListEmailAccounts(ctx, domain)
			if err != nil || domain != excludeDomain {
				return items, err
			}
			filtered := make([]client.EmailAccount, 0, len(items))
			matches := 0
			for _, account := range items {
				if strings.EqualFold(*account.Username, excludeUser) {
					matches++
					continue
				}
				filtered = append(filtered, account)
			}
			if matches > 1 {
				return nil, errors.New("ambiguous test mailbox inventory")
			}
			return filtered, nil
		}, func(v []client.EmailAccount) any {
			stable := make([]client.EmailAccount, len(v))
			for i, account := range v {
				stable[i] = stableMailbox(account)
			}
			return stable
		})
		if !valid {
			return h, false
		}
		if len(mailboxes) > 0 {
			sort.Slice(mailboxes, func(i, j int) bool { return *mailboxes[i].Username < *mailboxes[j].Username })
			user := *mailboxes[0].Username
			if user == "." || user == ".." || strings.ContainsAny(user, "/\\\r\n") {
				r.Checks["inventory_identity"] = "failed:invalid_mailbox"
				return h, false
			}
			if _, ok = capture(r, h, "GetEmailAccount", key+"mailbox-sample", func() (*client.EmailAccount, error) { return c.GetEmailAccount(ctx, domain, user) }, func(v *client.EmailAccount) any { return stableMailbox(*v) }); !ok {
				return h, false
			}
		}
		if _, ok = capture(r, h, "ListForwarders", key+"forwarders", func() ([]client.Forwarder, error) { return c.ListForwarders(ctx, domain) }, unchanged[[]client.Forwarder]); !ok {
			return h, false
		}
		if _, ok = capture(r, h, "GetCatchAll", key+"catch-all", func() (*client.CatchAll, error) { return c.GetCatchAll(ctx, domain) }, unchanged[*client.CatchAll]); !ok {
			return h, false
		}
		if _, ok = capture(r, h, "GetDNSRecords", key+"dns", func() (*client.DNSRecords, error) { return c.GetDNSRecords(ctx, domain) }, unchanged[*client.DNSRecords]); !ok {
			return h, false
		}
		if _, ok = capture(r, h, "GetSpamSettings", key+"score", func() (*client.SpamSettings, error) { return c.GetSpamSettings(ctx, domain) }, unchanged[*client.SpamSettings]); !ok {
			return h, false
		}
		if _, ok = capture(r, h, "GetSpamWhitelist", key+"whitelist", func() ([]string, error) { return c.GetSpamWhitelist(ctx, domain) }, unchanged[[]string]); !ok {
			return h, false
		}
		if _, ok = capture(r, h, "GetSpamBlacklist", key+"blacklist", func() ([]string, error) { return c.GetSpamBlacklist(ctx, domain) }, unchanged[[]string]); !ok {
			return h, false
		}
	}
	return h, true
}
