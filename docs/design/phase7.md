# Phase 7 validation and production preparation

October 19, 2026. Phase 7's authorized scope is implemented: offline validation,
ordinary-account reads, an isolated new-mailbox API test, compatibility hardening,
field audit and user documentation. A follow-up explicitly authorized creating
and testing a new mailbox while preserving existing configuration. Only that mailbox was imported into separate protected Terraform state, and a
saved no-change plan passed. A later explicit request authorized its removal:
a reviewed single-object deletion plan passed and the original inventory was restored. No pre-existing object was written, deleted or imported.
Only the reviewed cleanup plan was applied. No blanket destroy or
reseller/impersonation operation was performed.
No credentials, account identifiers, inventories or raw responses are stored here.

The provider remains a beta candidate with limited live validation. The full
live-lifecycle/stable-release gate is **not satisfied**: C01/C03/C04 and other
mutation/deletion scenarios remain live-unverified. The narrower successful tests
are recorded below without claiming complete production coverage. Publication,
repository visibility changes and releases remain deferred.

## Live evidence

All fourteen ordinary-account GET operations passed through the normal strict
typed decoder. A dedicated transport rejected non-GET methods and unsupported
route families. Two inventory/settings passes reported a completed run and an
unchanged baseline. That original read-only run created no objects. The later
[isolated mailbox test](mailbox-live-testing.md) created one new mailbox, then
removed it under separate explicit authorization. Existing configuration remained unchanged. Runtime
usage counters were excluded from configuration comparisons. Details and precise
comparison limits are in the [read-only testing procedure](live-testing.md).

| Scenario | Status | Limit |
| --- | --- | --- |
| Domain inventory, verification key, domain detail, pointer list and DNS | live-passed | Reads only; list completeness is not independently established |
| Mailbox list and an existing mailbox detail sample per nonempty domain | live-passed | No password or lifecycle operation; no claim of exhaustive detail coverage |
| Forwarder list and catch-all read | live-passed | Delivery and routing were untouched |
| Spam score and both sender lists | live-passed | No score propagation, sender-entry mutation or cross-domain ownership experiment |
| Account quota and per-email usage | live-passed | Both raw and enveloped forms supported; hourly freshness not independently verified |
| Before/after observed settings and membership | live-passed | No differences observed; unexposed settings and passwords cannot be compared |
| New test-mailbox create, quota/limit changes and password PATCH | live-passed | Direct typed API calls for one proven-new mailbox only; password authentication not independently tested |
| Test-mailbox import, refresh and no-change Terraform plan | live-passed | Separate private state, no password inputs, no apply/destroy; only the new test object adopted |
| Owned test-mailbox deletion through the provider | live-passed | Explicit cleanup request, ownership/dependency checks, reviewed single-object saved plan, independent absence and baseline verification; no leftovers |
| Terraform-driven create/update, replacement and other resource mutations | live-unverified | No changes to existing objects; applicable scenarios remain covered by mocks |
| Reseller packages/users, suspension and impersonation | live-unverified | Explicitly excluded from live scope; offline coverage remains |
| Deletion cascades, reserved system mailbox behavior, DNS/verification prerequisites, shared score propagation, package/override precedence | live-unverified | Requires isolated sandbox and specific authorization |

## Compatibility correction

The first live run authenticated and read the domain inventory and verification
key, then stopped at a strict quota decode error. Read-only shape inspection
revealed a standard `success/data` envelope where OpenAPI describes a raw object.
The decoder now accepts both complete forms for the two raw quota endpoints.
An envelope marker cannot fall back to raw decoding; false/non-boolean success,
missing/null/partial data and malformed fields still fail and preserve state.

Synthetic S034/S035 contract fixtures reproduce structure only. Client regression
tests cover both endpoints and valid/invalid envelope variants. The Terraform
usage reporting test now exercises both raw and wrapped responses, nullable
grace-period clearing, large byte values, empty usage and no-op plans with no
writes. No resource schema changed.

## Offline scenario matrix

All entries below are offline-passed against explicit loopback mocks with generated
runtime credentials. Existing acceptance tests include second-apply no-op plans,
imports, drift/deletion and guarded failure recovery; local test teardown affects
only the mock. Each resource's full evidence remains in its earlier phase record.

| Resource / area | Regression evidence and behavior |
| --- | --- |
| Domain and pointer | `TestOfflineDomainWorkflow`, `TestOfflineDomainDriftAndDeletion`: create/read/update-or-replace/delete, imports, no-op, pointer drift/replacement and external deletion; partial setup and DNS readiness recovery tested separately |
| Mailbox | `TestOfflineMailboxLifecycle`, `TestOfflineMailboxDefaultsAndDrift`, `TestOfflineMailboxImportAdoptionAndRotation`: every writable field, casing, defaults, zero/unlimited, adoption without secrets, drift/deletion, no-op and explicit rotations |
| Forwarder and catch-all | `TestOfflineRoutingLifecycle`, `TestOfflineRoutingDriftAndDeletion`: destination replacement, imports, all catch-all modes, no-op, drift/deletion and singleton retention; failed replacement/delete paths preserve known state |
| Whitelist/blacklist entries and score | `TestOfflineSpamLifecycle`, `TestOfflineSpamDriftAndDeletion`, `TestOfflineSpamEntryReplacement`: shared ownership, entry imports/replacement/deletion, score updates and drift, unrelated preservation, no-op, singleton retention |
| Reseller package and user | `TestOfflineResellerLifecycle`, `TestOfflineResellerDriftAndReplacement`: limits, every user transition, imports, no-op, drift/deletion; sequencing, partial recovery, in-use packages and impersonation boundaries have dedicated tests |
| Unknown/null/default normalization | Password/schema/validator tests, DNS null refresh, nullable grace period, resource defaults, normalized sets, case-only edits and imported version tests |
| Failure and concurrency | Existing client hardening/spam tests: cancellation before dispatch/during response, deadlines, rate-limit headers, bounded retries, malformed/partial/oversized responses, ambiguous writes, serialized shared saves and account-isolated aliases |
| Large inventories | `TestLargeInventoryUsesOneListAndNeverFansOut`: 10,000 domains and mailboxes; a lookup uses one list request, late duplicates fail as ambiguous, no per-item detail fanout |
| Read-only guard | `TestReadOnlyTransportBlocksAllMutationsAndUnknownRoutes`, `TestReadOnlyTransportRejectsOriginBodyAndQuery`: no blocked request reaches the mock service |
| Baseline integrity/privacy | `TestDiscoveryBaselineAndFailureIsolation`: stable/reordered collections and volatile counters accepted; setting drift, malformed replies, duplicate identities and oversized scope detected; reports contain no account values. `TestBaselinePreservesLargeIntegerChanges` guards 64-bit fingerprint precision |
| Isolated mailbox test guard / runner | `TestMailboxTestGuardExactScopeAndSequence`, `TestMailboxTestGuardNeverReplaysFailedWrites`, `TestMailboxProbePreservesExistingConfigurationAndRetainsOwnership`: exact target/field/sequence allowlist, no deletes/replay, collision rejection, baseline drift detection and private provenance |
| State compatibility | `TestResourceSchemaCompatibilityBaseline`: all ten resource schemas remain version 0, with pinned types/persistence flags in a synthetic fixture. No earlier release state or migration exists; future versions require actual upgrade tests |
| Secret persistence | Shared `secretChecks` inspect plan/state JSON, saved files and compressed plan members; write-only fields remain null and runtime-generated secrets never persist |

## Documentation, upstream and release limits

Generated reference docs and examples cover ten resources and twenty data sources.
The [migration guide](migration.md) lists all import identifiers, matching desired
configuration, no-change adoption, units, shared owners and unreadable passwords.
The [troubleshooting/support guide](troubleshooting.md) documents failure recovery,
request budgets, compatibility and state-upgrade policy. Existing mail, spam,
reseller, alias and onboarding examples cover authentication, rotation, side
effects and deletion semantics.

The current public OpenAPI document was downloaded again and is byte-identical to
the pinned SHA-256. No operation or field additions/breaking changes were found.
The [field audit](field-coverage.md) enumerates all request properties and
success-response fields, mapping each to Terraform behavior or an explicit
limitation. C01/C03/C04 are still disclosed instead of inferred from successful
reads. The inventory now contains 43 operations and 35 synthetic contract cases.

CI contains no live job or secret references. Default unit, acceptance and docs
workflows remain mock-only. The opt-in read checker is separate from the Terraform
test harness and never supplies a configurable production endpoint to mocks.
Linux/macOS CI now covers both pinned Go and Terraform versions; an added CI
configuration is not a claim that a remote run has completed. Windows/OpenTofu
runtime support and stable release readiness remain unclaimed.

## Validation commands

Use the Python dependency from `requirements-dev.txt` (an isolated environment is
recommended), preinstalled Terraform and Go toolchains, and no live credentials:

```sh
TF_ACC_TERRAFORM_PATH=/path/to/terraform-1.11.1 make check build
GOTOOLCHAIN=go1.27.1 TF_ACC_TERRAFORM_PATH=/path/to/terraform-1.16.6 make check
make vuln
```

The Phase 7 record distinguishes offline success, authorized live read success
and every live-unverified lifecycle. Publication must preserve that distinction.

Recorded local results:

- Go 1.26.9 / Terraform 1.11.1: `make check build` passed, including all offline
  lifecycle tests under the race detector, eight inventory-tool tests,
  43 operations / 35 contract cases, generated documentation check and native build.
- Go 1.27.1 / Terraform 1.16.6: `make check build` passed with the same complete
  offline suite, inventory checks, generated documentation check and native build.
- `make vuln`: no vulnerabilities found.
- Provider, aliases, mail, spam and reseller standalone examples passed local
  `terraform validate` with generated dummy runtime credentials. The onboarding
  example additionally depends on the AWS provider; its DNS workflow is covered
  by the injected-resolver offline acceptance tests, not a live AWS run.
- Cross-compilation passed for macOS, Linux and Windows on both amd64 and arm64
  with Go 1.26.9 and CGO disabled. Initial sandboxed attempts could not resolve
  target packages; the same builds passed with the sandbox restriction lifted.
  Cross-build success is not OS runtime validation: remote Linux/macOS CI results
  remain unobserved and Windows runtime support is not claimed.
- No source files were staged or committed; publication and repository settings
  were untouched. Live response data, Terraform state/plans and account identities
  were not saved into the checkout.
- Privacy review found none of the supplied credentials or account identifiers in
  repository files or Git history, and no state, saved plans or crash logs in the
  checkout. Coverage test/documentation targets and new documentation links resolve;
  `git diff --check` passes.
