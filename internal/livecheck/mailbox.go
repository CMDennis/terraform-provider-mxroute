package livecheck

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CMDennis/terraform-provider-mxroute/internal/client"
)

type MailboxReport struct {
	Report
	CreationAttempted bool `json:"creation_attempted"`
	Created           bool `json:"mailbox_created_confirmed"`
	Retained          bool `json:"mailbox_retained"`
}

type mailboxManifest struct {
	StartedAt        string              `json:"started_at"`
	Domain           string              `json:"domain"`
	LocalPart        string              `json:"local_part"`
	Email            string              `json:"email"`
	Scope            []string            `json:"approved_scope"`
	AbsenceConfirmed bool                `json:"absence_confirmed_before_create"`
	Created          bool                `json:"created_by_this_run"`
	Status           string              `json:"status"`
	Cleanup          any                 `json:"cleanup"` // Initial retention policy or later authorized cleanup journal.
	Baseline         map[string][32]byte `json:"baseline_configuration_fingerprints"`
}

func randomHex(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("cannot generate disposable mailbox input")
	}
	return hex.EncodeToString(b), nil
}

func saveMailboxManifest(dir string, manifest mailboxManifest) bool {
	f, err := os.CreateTemp(dir, ".manifest-*")
	if err != nil {
		return false
	}
	name := f.Name()
	defer os.Remove(name)
	err = json.NewEncoder(f).Encode(manifest)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err == nil && os.Rename(name, filepath.Join(dir, "manifest.json")) == nil
}

func sameSnapshot(a, b map[string][32]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for key, v := range a {
		if other, exists := b[key]; !exists || other != v {
			return false
		}
	}
	return true
}

// RunMailbox never deletes anything. The entry point supplies NewReadOnly for
// discovery and NewMailboxTest for mutations. Private provenance is retained
// outside Git; credentials and generated passwords are never saved or reported.
func RunMailbox(ctx context.Context, reader *client.Client, scopedClient func(domain, local string) (*client.Client, error), dir string, progress func(string)) MailboxReport {
	r := MailboxReport{Report: Report{Checks: map[string]string{}}}
	check := func(name string, err error) bool {
		_, ok := capture(&r.Report, map[string][32]byte{}, name, name, func() (bool, error) { return true, err }, unchanged[bool])
		if ok && progress != nil {
			progress(name + ":passed")
		}
		return ok
	}
	before, ok := snapshot(ctx, reader, &r.Report)
	if !ok {
		return r
	}
	domains, err := reader.ListDomains(ctx)
	if !check("domain_selection", err) {
		return r
	}
	sort.Strings(domains)
	selected := ""
	for _, domain := range domains {
		if _, known := before[domain+"/domain"]; !known {
			check("domain_selection", errors.New("domain inventory changed during selection"))
			return r
		}
		value, err := reader.GetDomain(ctx, domain)
		if !check("domain_readiness", err) {
			return r
		}
		if *value.MailHosting {
			selected = domain
			break
		}
	}
	if selected == "" {
		check("domain_selection", errors.New("no existing mail-enabled domain"))
		return r
	}
	quota, err := reader.GetQuota(ctx)
	if !check("capacity_read", err) {
		return r
	}
	if *quota.TotalUsed < 0 || *quota.TotalLimit < 0 || (*quota.TotalLimit > 0 && *quota.TotalLimit-*quota.TotalUsed < 32<<20) {
		check("capacity_check", errors.New("insufficient account capacity"))
		return r
	}
	name, err := randomHex(12)
	if !check("identity_generation", err) {
		return r
	}
	local := "tfcheck" + name
	c, err := scopedClient(selected, local)
	if !check("scope_guard", err) {
		return r
	}
	manifest := mailboxManifest{StartedAt: time.Now().UTC().Format(time.RFC3339), Domain: selected, LocalPart: local, Email: local + "@" + selected,
		Scope:  []string{"create:quota_mb=16,daily_limit=1", "update:quota_mb=32", "update:daily_limit=2", "rotate:password_once"},
		Status: "planned", Cleanup: "No deletion authorized or implemented; retain test mailbox for review.", Baseline: before}
	if !saveMailboxManifest(dir, manifest) {
		check("ownership_record", errors.New("cannot save private ownership record"))
		return r
	}
	_, err = c.FindEmailAccount(ctx, selected, local)
	if !client.IsNotFound(err) {
		check("mailbox_absence", errors.New("mailbox absence was not confirmed"))
		return r
	}
	_, err = c.FindForwarder(ctx, selected, local)
	if !client.IsNotFound(err) {
		check("routing_alias_absence", errors.New("forwarder alias absence was not confirmed"))
		return r
	}
	manifest.AbsenceConfirmed = true
	manifest.Status = "create_pending"
	if !saveMailboxManifest(dir, manifest) {
		check("ownership_record", errors.New("cannot save pre-create evidence"))
		return r
	}
	password, err := randomHex(24)
	if !check("password_generation", err) {
		return r
	}
	// Upper/lowercase, numbers and punctuation are guaranteed without a saved
	// literal password. The random component is fresh for every invocation.
	password = "Aa9!" + password
	q, limit := int64(16), int64(1)
	r.CreationAttempted = true
	err = c.CreateEmailAccount(ctx, selected, client.CreateEmailAccountRequest{Username: local, Password: password, Quota: &q, Limit: &limit})
	password = ""
	if !check("mailbox_create", err) {
		manifest.Status = "create_failed_or_uncertain_review_before_any_retry"
		saveMailboxManifest(dir, manifest)
		return r
	}
	r.Created, r.Retained = true, true
	manifest.Created, manifest.Status = true, "created_confirmed"
	if !saveMailboxManifest(dir, manifest) {
		check("ownership_record", errors.New("cannot retain creation evidence"))
		return r
	}
	verify := func(label string, expectedQuota, expectedLimit int64) bool {
		listed, err := c.FindEmailAccount(ctx, selected, local)
		if err == nil && *listed.Username != local {
			err = errors.New("unexpected canonical mailbox identity")
		}
		if !check(label+"_list", err) {
			return false
		}
		value, err := c.GetEmailAccount(ctx, selected, local)
		if err == nil && (*value.Username != local || !strings.EqualFold(*value.Email, manifest.Email) || *value.Quota != expectedQuota || *value.Limit != expectedLimit || *value.Suspended) {
			err = errors.New("mailbox readback does not confirm the scoped operation")
		}
		return check(label+"_readback", err)
	}
	baseline := func() bool {
		after, valid := snapshotExceptMailbox(ctx, reader, &r.Report, selected, local)
		r.BaselineUnchanged = valid && sameSnapshot(before, after)
		if !r.BaselineUnchanged {
			check("existing_configuration", errors.New("baseline comparison failed; no automatic repair"))
			manifest.Status = "baseline_changed_or_unavailable_stopped"
			saveMailboxManifest(dir, manifest)
			return false
		}
		return check("existing_configuration", nil)
	}
	if !verify("create", 16, 1) || !baseline() {
		return r
	}
	for _, step := range []string{"quota", "send_limit", "password_rotation"} {
		if !verify("before_"+step, q, limit) {
			return r
		}
		manifest.Status = step + "_pending"
		if !saveMailboxManifest(dir, manifest) {
			check("ownership_record", errors.New("cannot save operation intent"))
			return r
		}
		input := client.UpdateEmailAccountRequest{}
		switch step {
		case "quota":
			q = 32
			input.Quota = &q
		case "send_limit":
			limit = 2
			input.Limit = &limit
		case "password_rotation":
			p, err := randomHex(24)
			if !check("rotation_generation", err) {
				return r
			}
			p = "Aa9!" + p
			input.Password = &p
		}
		err = c.UpdateEmailAccount(ctx, selected, local, input)
		input.Password = nil
		if !check(step, err) {
			manifest.Status = step + "_failed_or_uncertain_stopped"
			saveMailboxManifest(dir, manifest)
			return r
		}
		manifest.Status = step + "_confirmed"
		if !saveMailboxManifest(dir, manifest) || !verify(step, q, limit) || !baseline() {
			return r
		}
	}
	manifest.Status = "complete_mailbox_retained"
	if !saveMailboxManifest(dir, manifest) {
		check("ownership_record", errors.New("cannot save completion record"))
		return r
	}
	r.Completed = true
	return r
}
