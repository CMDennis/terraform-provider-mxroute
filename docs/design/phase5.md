# Phase 5 shared spam controls and reporting

Implemented October 19, 2026 using explicit numeric-loopback mocks and generated
runtime credentials. No authenticated MXroute request or live mutation was made.
The provider now registers eight resources and sixteen data sources. Reseller
Terraform surfaces remain Phase 6. Publication remains deferred.

## Score ownership decision and remaining uncertainty

The frozen OpenAPI explicitly establishes account-wide sender lists, but does not
fully establish high-score scope. Additional public evidence read October 19,
2026, [MXroute Spam Filters and Sender Lists](https://docs.mxroute.com/docs/panel/spam-filters.html),
describes score saves as also updating shared scoring settings. Phase 5 therefore
uses conservative **account ownership**, with one score resource per effective
account and a separate mutable `access_domain`. This resolves the safe ownership
design for a credential-free preview; it is an inference from shared effects,
not verification that every domain returns an identical score. Exact shared versus
selected-domain propagation, server readback and production compatibility remain
live-unverified under C01 and must be resolved before stabilizing the live contract.
No production experiment was used to settle this question.

## Resources and identity

All three resources start at schema version 0 with initial upgrade hooks:

- `mxroute_spam_settings` reads and applies scores from 1 through 50, sends only
  `high_score`, and has constant account-relative ID `spam-settings`. Import takes
  an owned domain as the required read context. Create applies desired settings;
  destroy relinquishes management and preserves remote filtering without an API
  call. Access-domain changes update in place. Existing delivery policy, which
  cannot be managed through this endpoint, determines how scored mail is handled.
- `mxroute_spam_whitelist_entry` and `mxroute_spam_blacklist_entry` use lowercase
  entry IDs independent of access domain. Import takes `access-domain/raw-entry`
  so the first read has a context. Creates reject pre-existing case variants and
  instruct import. Casing-only edits and access-domain edits read/retain the entry
  without remote writes; other entry changes replace. Deletes resolve original
  wire spelling and remove only that entry, verifying absence from a successful
  complete list. Unrelated entries remain untouched.

There is no authoritative whole-list resource. One Terraform owner is required
per effective account/list/entry and one score owner per account, including across
provider aliases. Independent states/processes cannot detect competing ownership.
Changing provider accounts requires explicit state migration/import; access-domain
changes are only meaningful within the same effective account.

Preflight, write and readback are serialized under the effective-account lock,
including impersonated aliases. The private context marker permits nested typed
operations to use the same lock without deadlocking. Separate processes retain
the documented coordination limitation. Typed request payloads and the explicit
2048-byte spam request guard prevent unintended or oversized fields.

Only complete, schema-valid, unambiguous lists prove entry absence. Missing access
domains, unverifiable account context, missing/null fields, malformed values and
unavailable score settings preserve state and return errors. Values outside the
score range and invalid/duplicate sender-list entries also fail closed.

After acknowledged or uncertain saves, exact readback verifies the desired score
or entry presence/absence. A 503 busy/locking response may be retried only after a
successful mismatching read, with bounded jitter/backoff and at most three saves.
Other uncertain saves are never replayed. Failed readback blocks retry. Acknowledged
or uncertain creates retain their stable identity on error so recovery is possible;
Terraform may taint failed creates, so inspect/import/reconcile before replacement.
Cancellation and operation deadlines bound the lock, reads and retries.

## Data sources and units

`mxroute_spam_settings`, `mxroute_spam_whitelist` and `mxroute_spam_blacklist` use
`access_domain`. Sender entries are unordered sets with original spelling retained;
invalid or case-ambiguous lists are errors rather than silently collapsed data.

`mxroute_quota` exposes every documented field: username; int64 `total_used` and
`total_limit` in bytes (zero limit means unlimited); float64 API `percent_used`;
all five byte breakdown categories; nullable typed `grace_period` with remaining
days and exact deadline; and the exact upstream `updated_at` date-time. A null
grace period clears prior values without fabricating a default.

`mxroute_email_usage` exposes username and an ordered list of `email_address`,
int64 `size_bytes`, and exact `updated_at`. The API's largest-first order and empty
lists are preserved. There is no MB/GB conversion. These computed-only hourly
snapshots can be stale; refresh does not force a recount or cause configuration
writes. Missing values fail strict decoding instead of becoming zero.

## Offline evidence

| Scenario | Regression evidence |
| --- | --- |
| Shared reads through two domains, score changes, entry casing/access-domain changes without recreation, all imports, no-op plans, deletion and singleton relinquish | `TestOfflineSpamLifecycle` |
| Score drift repair and complete-list-confirmed external entry deletion/recreation | `TestOfflineSpamDriftAndDeletion` |
| Existing case variant requires import; unrelated sender entries preserved | `TestOfflineSpamExistingEntryRequiresImport`, `TestOfflineSpamLifecycle` |
| Entry changes replace with a stable next plan; score boundary 1 | `TestOfflineSpamEntryReplacement` |
| Forbidden/malformed/missing access-domain reads preserve state and recover to an empty plan | `TestOfflineSpamReadErrorsPreserveState` |
| Successful and uncertain applied writes, rejected busy retry, no replay for other failures, unchanged readback, failed readback | `TestSpamVerifiedSave` |
| Busy retry ceiling, cancellation and oversized request rejected before transport | `TestSpamBusyBoundedAndCancelled`, `TestSpamRequestBodyLimit` |
| Concurrent duplicate detection across ordinary/impersonated aliases, distinct simultaneous entries, unrelated-entry preservation, separate account isolation | `TestSpamConcurrentSharedEntries` |
| Canonical delete spelling, wildcard/plus/equal escaping, pre-existing entry rejection, idempotent absent delete | `TestSpamCanonicalDeleteAndNoBlindAdoption` |
| Invalid/incomplete/null/duplicate lists, unavailable domain or account, unavailable/out-of-range score | `TestSpamCompleteListsAndUnavailableSettings` |
| Score and entry bounds, unknown/null deferred values | `TestSpamValidators` |
| All three resource create identities survive acknowledged save followed by failed readback | `TestSpamCreateRetainsIdentityAfterFailedReadback` |
| Exact bytes above 32-bit range, unlimited zero, percentage, every category, nullable grace transition, timestamps, ordered/empty usage, no-op plans and no writes | `TestOfflineUsageReporting` |
| Failed reporting reads return errors and recover without fabricated values | `TestOfflineUsageReadFailures` |
| Generated credentials absent from Terraform plans/state and saved compressed plan contents | Shared `secretChecks` attached to phase 5 lifecycle plans and states |

The [spam and reporting guide](../../examples/spam/README.md), reserved-domain
examples and generated reference pages cover all eight new surfaces. The pinned
43-operation inventory now marks Phase 5 Terraform surfaces implemented and
mock-tested, with Phase 6 surfaces still planned. The baseline and synthetic
contract cases remain unchanged.

Final validation passed:

- `make check build` with Go 1.26.9 and Terraform 1.14.6: formatting, static
  analysis, all packages under the race detector, the complete offline Terraform
  suite, all eight inventory-tool tests, the 43-operation/33-case pinned inventory,
  reproducible generated documentation and the local provider build.
- All Phase 5 provider lifecycle, reporting, validation and recovery tests with
  Go 1.26.9 and the minimum supported Terraform 1.11.1.
- All Phase 5 client/provider regressions with Go 1.27.1 and Terraform 1.16.6.
- Source/example/documentation review found no literal credential assignments or
  private-key material in Phase 5 artifacts. No saved Terraform state or plans
  exist in the checkout. All Phase 5 inventory test/documentation references
  resolve, and `git diff --check` passes. Nothing was staged or committed.

The Phase 5 credential-free preview implementation and offline gate are complete,
subject to the explicit C01 propagation/stabilization limit above. Mock success
cannot establish production API compatibility. Offline success never authorizes
live tests, real-account cleanup, repository visibility changes or publication.
