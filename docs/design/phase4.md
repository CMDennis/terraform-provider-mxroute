# Phase 4 mailbox and routing management

Implemented October 19, 2026. All validation uses explicit loopback mocks and
runtime-generated credentials. No authenticated MXroute request or live mutation
was made. Runtime API compatibility, system-mailbox restrictions, list completeness,
server password rules and forwarding side effects remain live-unverified under
C02/C03/C05/C06/C08/C09/C10/C11/C12/C13. Publication remains deferred.

## Terraform surfaces and lifecycle decisions

Three resources at schema version 0, each with its initial state upgrade hook:

- `mxroute_email_account`: create/read/partial update/delete, case-insensitive
  canonical lookup, domain/local-part import and write-only password rotation.
  Optional/computed quota (MB, explicit zero unlimited) and daily limit adopt server
  defaults when omitted; only changed, explicitly configured fields enter PATCH.
  Computed email, canonical username, usage (MB), sent count and suspension have no
  independent write behavior. The 9600 send ceiling is enforced; undocumented
  numeric bounds remain server validated.
- `mxroute_forwarder`: create/list-backed read/delete and import; destination sets
  and identity changes replace. Destination replacement uses destroy-before-create,
  interrupts routing and can leave the alias absent if creation fails. No update
  operation or create-before-destroy guarantee is invented. Special targets must
  stand alone; combinations remain unsupported pending a verified contract.
- `mxroute_catch_all`: apply/read/import of the existing domain singleton. Destroy
  only relinquishes management, with no remote API call or default reset. Address
  is required only for address mode; fail/blackhole omit it on writes and return
  null in state even if the API retains a stale address. One owner per domain.

Five data sources add single/list mailbox and forwarder reads plus catch-all.
Unordered collections and destination collections are sets. Mailbox single reads
resolve the canonical local part from a complete valid list before detail GET;
ambiguous matches block reads and mutations. Only a successful complete list
proving absence removes mailbox/forwarder state. Parent-context 404, detail 404,
auth failures, malformed responses and ambiguous lists preserve state. A missing
catch-all access domain never authorizes state removal.

Configured local-part casing is retained in `username`/`alias`, with separate
computed canonical fields. Casing-only edits cause no remote writes or
replacement. IDs and imports normalize to domain/lowercase-local-part. Creates
preflight complete lists, require import of existing case variants and never
silently adopt an existing mailbox. Deletes use the canonical server identity
and remove only the requested object, never unmanaged siblings or a parent.

Passwords are checked at plan/apply for 8–72 Unicode characters, sourced from
configuration only, and accompanied by a positive monotonic rotation version.
Creation/replacement requires a password; imports and unchanged versions do not.
Confirmed PATCH completion saves its version before readback; read failure cannot
replay that confirmed rotation. Uncertain writes still fail without automatic
adoption/replay. Confirmed creates retain their IDs before readback, but Terraform
may taint a create whose readback fails; review before replacement. Safe fixed
categories surface password-field character/strength rejections without retaining
raw server messages. Category detection uses message keywords; exact live wording
and password acceptance remain unverified.

The API's automatic Expert Spam Filtering activation for certain destinations is
prominently documented as a side effect, without an unsupported control field.
No system mailbox identity or management restriction is guessed.

## Offline evidence

| Scenario | Regression evidence |
| --- | --- |
| Create, explicit zero quota, quota-only/password-only/limit-only PATCH, canonical writes, casing edit, volatile reads, imports, stable plans and deletion | `TestOfflineMailboxLifecycle` |
| Omitted server defaults, external quota/usage/sent/suspension changes, explicit quota drift repair and external deletion | `TestOfflineMailboxDefaultsAndDrift` |
| Declarative import of a pre-existing canonical mailbox with no known password or version, empty next plan and first rotation | `TestOfflineMailboxImportAdoptionAndRotation` |
| Validation failure preserves mailbox state and unrelated settings | `TestOfflineMailboxFailedUpdatePreservesState` |
| Confirmed rotation then failed readback preserves completed version and avoids another PATCH | `TestOfflineMailboxConfirmedRotationReadbackFailure` |
| Existing case variant never creates a duplicate or adopts state | `TestOfflineMailboxExistingCaseVariantRejected` |
| Malformed, forbidden, ambiguous or detail-404 reads preserve mailbox state | `TestOfflineMailReadFailuresPreserveState`, `TestMailAbsenceRequiresCompleteList` |
| All routing data sources, destination set order, imports, no-op plans, special targets and every catch-all mode including stale address clearing | `TestOfflineRoutingLifecycle` |
| Replacement create failure leaves alias absent and recovers; failed delete preserves old routing | `TestOfflineForwarderReplacementFailure`, `TestOfflineForwarderFailedDeletePreservesRouting` |
| Forwarder/catch-all drift, external forwarder deletion, unrelated reads fail safely, singleton destroy preserves last routing value | `TestOfflineRoutingDriftAndDeletion`, `TestOfflineRoutingReadFailuresPreserveState`, `TestOfflineRoutingLifecycle` |
| Password length edges, daily ceiling, local identities, special combinations and conditional catch-all address | `TestMailValidators` |
| Server password category advice and redaction on both create and update | `TestMailboxPasswordRejectionsAreSafe` |
| Empty mailbox/forwarder collections and nullable catch-all read without mutations | `TestOfflineMailEmptyLists` |
| Combined domain → mailbox → forwarding workflow, stable plan and dependency-ordered deletion | `TestOfflineMailWorkflowDependencies` |
| Write-only fields null and generated secrets absent in saved plans/state and compressed plan members | `secretChecks` attached to mailbox create/rotation/import plans and states |

The [mail workflow](../../examples/mail/README.md) documents creation, import,
rotation, units, omission, canonical names, singleton removal, routing interruption,
side effects and recovery. Generated references and reserved-domain examples cover
all eight Phase 4 surfaces. API coverage mappings distinguish these implemented
surfaces from later planned spam, usage and reseller work.

`make check build` passed with Go 1.26.9 and Terraform 1.14.6, including formatting,
static analysis, every package under the race detector, all offline Terraform
regressions, eight inventory-tool tests, the 43-operation/33-contract-scenario
baseline, reproducible generated docs and the local provider build. The complete
Phase 4 provider regressions also passed on Go 1.26.9/Terraform 1.11.1 and
Go 1.27.1/Terraform 1.16.6. The final runs include a different generated password
at rotation, empty collections and dependency-ordered ordinary-mail cleanup.

Phase 4 source, example and reference-page review found no literal password or
private-key assignments. No Terraform state or saved plan artifacts are present
in the checkout. Runtime secret checks cover the generated passwords and API keys
in plan/state JSON and the saved artifacts, including compressed plan contents.

The Phase 4 offline exit gate is satisfied. Mock tests cannot prove live
compatibility and do not authorize authenticated tests, cleanup of real account
data, publication or repository visibility changes.
