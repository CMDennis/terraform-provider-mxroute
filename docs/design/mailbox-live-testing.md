# Isolated live mailbox validation

October 19, 2026 follow-up: the user explicitly authorized creating a new test
mailbox and testing it with the supplied account API key, while preserving all
existing configuration. This extends the earlier read-only scope only for that
new mailbox. No deletion or wider lifecycle testing was authorized.

## Exact scope and safeguards

The opt-in `cmd/mxroute-mailboxtest` runner generated one 96-bit random local part
on an existing mail-enabled owned domain. It first captured configuration across
all discovered domains, checked available account capacity, and confirmed both
mailbox and forwarder-alias absence using complete successful lists. Passwords
were generated in memory and were never written to artifacts or output.

The authorized operations were:

1. Create the unused mailbox with a 16 MB quota and daily sending limit of 1.
2. Read its list and detail representations and verify identity and settings.
3. PATCH only its quota to 32 MB, then verify the send limit was preserved.
4. PATCH only its daily sending limit to 2, then verify quota was preserved.
5. PATCH only its password once, then verify the readable settings remained intact.

A dedicated HTTP transport permits exactly this one POST and three single-field
PATCHes, in order, to that exact identity. Other mailboxes, domains, routing,
spam/shared settings, reseller operations, all DELETE requests and additional
mutations are blocked before transport. A failed write disables later writes for
the run; it cannot be replayed. The normal typed client's response validation,
rate limits, timeouts and cancellation still apply. The ordinary read-only runner
was not given write capability.

The runner rechecks target identity and expected state before each update and
compares existing configuration against its baseline after every confirmed step.
Only the newly created mailbox is excluded from that comparison. Duplicate
case-insensitive matches fail closed. Volatile usage/sent counters and hourly
timestamps are excluded, with the same comparison limits as the read-only checks.
Unexpected differences stop the run; there is no automatic repair or teardown.

The addition intentionally creates one new recipient address on the selected
domain and uses one mailbox slot. No mail was sent or mailbox contents changed.
No domain, forwarding, catch-all or shared spam configuration was written.

## Results

The create and all three PATCH requests succeeded. Mailbox list/detail readback
confirmed quota and limit values and preserved unrelated readable settings. All
four post-operation comparisons reported the pre-existing configuration unchanged.
The password PATCH was accepted; password values are unreadable, so this result
does not claim an independently verified IMAP/SMTP login or delivery test.

The locally built provider then imported only this newly created mailbox into a
separate protected Terraform state. With Terraform 1.16.6, an ordinary saved plan
returned exit code 0 and exactly one `no-op` resource action. Both imported
password fields were null. The runtime API key was absent from the local state,
configuration and saved plan, including compressed plan members. No apply or
destroy was run. This validates live import, refresh and no-change adoption for
the test mailbox; Terraform-driven create/update remains mock-tested.

At completion of the creation/update run, the mailbox was retained with quota
32 MB and daily send limit 2. It was subsequently removed under the explicit
cleanup authorization below and is now confirmed absent. Its
generated password was not saved. The exact identity, approved operation scope,
absence/creation provenance, configuration fingerprints and adoption outcome are
in a mode-0600 manifest under a private mode-0700 temporary directory outside Git.
Terraform configuration, empty current state, prior state backups and saved plans
remain in that same private run directory for review. Public reports and repository records contain no account
identifiers or credentials. Do not rerun the creation tool to resume this run:
that would create a different mailbox. Cleanup was authorized separately and recorded below; the original creation
request alone did not authorize deletion.

## Offline regressions

- `TestMailboxTestGuardExactScopeAndSequence`: existing-target, route, field,
  sequence and deletion restrictions; only the four approved requests reach the
  mock service.
- `TestMailboxTestGuardNeverReplaysFailedWrites`: rejected, throttled and upstream
  failures cannot be dispatched twice.
- `TestMailboxProbePreservesExistingConfigurationAndRetainsOwnership`: successful
  isolated workflow, mailbox/alias collisions, unrelated drift, rejected create
  and PATCH, stopping without deletion, distinct runtime passwords, retained
  private provenance and anonymized reporting.

Default tests and CI continue to use explicit local mocks and generated runtime
credentials. The live tool requires the `--authorized-new-mailbox-retain` flag
and runtime `MXROUTE_*` variables; neither the flag nor possessing credentials
substitutes for human authorization.

After this addition, `make check build` passed with Go 1.26.9 / Terraform 1.11.1,
including the full offline lifecycle suite, race checks, static analysis,
43-operation/35-case inventory, eight inventory-tool tests, generated docs check
and native build. The client and live-check packages also passed under the race
detector with Go 1.27.1. The live import/no-change-plan check used Terraform 1.16.6.
Privacy review found no supplied credentials, actual account/test identifiers or
live state/plans in repository files or history; private-record permissions and
ownership evidence passed inspection. No files were staged or committed.

## Explicitly authorized cleanup

The user subsequently requested removal of this exact test mailbox. Its private
creation/absence provenance and current identity, quota, limit and suspension
were rechecked. Forwarding and catch-all dependency checks passed, and the
pre-existing configuration matched the original saved baseline before deletion.
A positive storage counter was observed; it was not treated as evidence of
emptiness or ownership. The user's explicit mailbox-removal request covered its
contents. No mailbox contents were inspected or sent elsewhere.

The separate Terraform state contained only the owned test mailbox. Its local
`prevent_destroy` setting was disabled solely for this approved cleanup. A saved
plan was inspected and required to contain exactly one delete action for that
identity, no other resource changes and no deferred actions. Its checksum and
exact identity were rechecked immediately before applying that saved plan.
There was no blanket destroy, sweeper or deletion retry.

The provider's deletion completed successfully and the isolated state became
empty. An independent complete API list then proved mailbox absence. The entire
original configuration baseline matched again after cleanup. The private
manifest is marked `removed_confirmed`; no test mailbox remains. No pre-existing
object, domain, routing or shared setting was changed. Passwords and API keys
remain absent from repository artifacts.
The private test configuration was renamed out of Terraform's active `.tf`
configuration after cleanup, preserving its audit history without leaving an
active configuration for recreating the mailbox.

`VerifyMailboxCleanup` performs GET requests only; it cannot delete anything.
Regressions cover missing creation proof, an unexpected forwarding dependency,
nonzero storage on an explicitly authorized removal, absence confirmation, the
original baseline and compatibility with both the initial retention-policy text
and the later structured private cleanup journal.

## Remaining limits

Mailbox replacement, external deletion/drift repair, server defaults and
zero/unlimited writes were not tested live. Domain/pointer lifecycles, forwarding,
shared spam changes, reseller packages/users and impersonation remain
live-unverified. The test does not establish a fully live-validated release gate.
