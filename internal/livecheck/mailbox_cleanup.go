package livecheck

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
	"github.com/CMDennis/terraform-provider-mxroute/internal/identity"
)

// VerifyMailboxCleanup performs only reads. Actual cleanup must use a separately
// authorized, reviewed single-object plan. A name prefix alone is never proof
// of ownership, and neither this check nor the manifest authorizes deletion.
func VerifyMailboxCleanup(ctx context.Context, c *client.Client, manifestPath string, before bool) Report {
	r := Report{Checks: map[string]string{}}
	fail := func(check string) Report {
		r.Checks[check] = "failed:review_required"
		return r
	}
	info, err := os.Stat(manifestPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > 1<<20 {
		return fail("private_ownership_record")
	}
	data, err := os.ReadFile(manifestPath)
	var m mailboxManifest
	if err != nil || json.Unmarshal(data, &m) != nil || !m.Created || !m.AbsenceConfirmed || len(m.Baseline) == 0 ||
		(m.Status != "complete_mailbox_retained" && (before || m.Status != "removed_confirmed")) {
		return fail("creation_provenance")
	}
	domain, err := identity.Domain(m.Domain)
	if err != nil || domain != m.Domain || len(m.LocalPart) != 31 || !strings.HasPrefix(m.LocalPart, "tfcheck") || m.Email != m.LocalPart+"@"+m.Domain {
		return fail("exact_test_identity")
	}
	if _, err := hex.DecodeString(m.LocalPart[7:]); err != nil {
		return fail("exact_test_identity")
	}
	if _, exists := m.Baseline[m.Domain+"/domain"]; !exists {
		return fail("original_domain_ownership")
	}
	listed, err := c.FindEmailAccount(ctx, m.Domain, m.LocalPart)
	if before {
		if err != nil || *listed.Username != m.LocalPart {
			return fail("current_mailbox_identity")
		}
		v, err := c.GetEmailAccount(ctx, m.Domain, m.LocalPart)
		if err != nil || *v.Username != m.LocalPart || !strings.EqualFold(*v.Email, m.Email) || *v.Quota != 32 || *v.Limit != 2 || *v.Suspended || *v.Usage < 0 || *v.Sent != 0 {
			return fail("unchanged_test_mailbox_settings")
		}
		// The user explicitly authorized removal of this exact test mailbox.
		// Storage use is not ownership evidence and does not imply that a different
		// object was adopted. Never claim the mailbox is empty from usage counters.
		if *v.Usage > 0 {
			r.Checks["test_mailbox_storage"] = "nonzero:explicit_removal_authorized"
		}
		forwarders, err := c.ListForwarders(ctx, m.Domain)
		if err != nil {
			return fail("forwarding_dependencies")
		}
		for _, f := range forwarders {
			if strings.EqualFold(*f.Alias, m.LocalPart) {
				return fail("forwarding_dependencies")
			}
			for _, destination := range f.Destinations {
				if strings.EqualFold(destination, m.Email) || strings.EqualFold(destination, m.LocalPart) {
					return fail("forwarding_dependencies")
				}
			}
		}
		catchAll, err := c.GetCatchAll(ctx, m.Domain)
		if err != nil || (catchAll.Address != nil && (strings.EqualFold(*catchAll.Address, m.Email) || strings.EqualFold(*catchAll.Address, m.LocalPart))) {
			return fail("catch_all_dependencies")
		}
		r.Checks["owned_test_mailbox_and_dependencies"] = "passed"
	} else {
		if !client.IsNotFound(err) {
			return fail("complete_list_confirms_removal")
		}
		r.Checks["complete_list_confirms_removal"] = "passed"
	}
	var current map[string][32]byte
	var valid bool
	if before {
		current, valid = snapshotExceptMailbox(ctx, c, &r, m.Domain, m.LocalPart)
	} else {
		current, valid = snapshot(ctx, c, &r)
	}
	if !valid || !sameSnapshot(m.Baseline, current) {
		return fail("original_configuration_baseline")
	}
	r.Checks["original_configuration_baseline"] = "passed"
	r.Completed, r.BaselineUnchanged = true, true
	return r
}
