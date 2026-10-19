# Phase 6 reseller lifecycle and impersonation workflows

Implemented October 19, 2026 using explicit numeric-loopback mocks and generated
runtime credentials. No authenticated MXroute request, reseller account, owned
user or live mutation was used. The provider now registers ten resources and
twenty data sources, and every one of the 43 pinned operations has a Terraform
workflow with offline tests. This is the credential-free implementation
milestone: beta evaluation may begin with the live-validation gaps below
disclosed, but no production compatibility is claimed. Publication remains
deferred.

## Resources and identity

Both resources start at schema version 0 with initial upgrade hooks, and both
require a **plain reseller** provider. `ModifyPlan` and every CRUD method reject
an impersonation alias with a clear diagnostic before any request; the client's
existing `RequireReseller` guard remains as a second barrier.

- `mxroute_reseller_package`: create/read/update/delete and import by exact
  name. Limits are strings in the API write form — `quota_gb` (GB, canonical
  decimal) and `domains`, `email_accounts`, `email_forwarders`, `domain_pointers`
  (canonical integers) — each also accepting `unlimited`. Readback maps null
  limits and `quota_unlimited` to `unlimited`; `0` stays a distinct limit,
  including zero customer mailboxes. A contradictory unlimited flag/value pair
  or negative value is incomplete readback, not a guess. Omitted limits adopt
  the server value (documented creation defaults 1/1/100/100/10) and are never
  sent; updates PATCH only configured limits that differ from a fresh read.
  Delete calls only the documented route; an in-use package returns a business
  error, state is preserved and no user is touched.
- `mxroute_reseller_user`: create/read/update/delete and import by exact
  username. New usernames must satisfy the 1–10 lowercase creation rule at plan
  time, while existing/imported users keep the 1–16 authentication syntax.
  Passwords use the shared write-only/version convention with an 8-character
  minimum and no invented maximum. `package` and `suspended` update in place;
  `quota_mb` (MB or `unlimited`) is an optional/computed explicit override.
  Computed `current_email`, `domain` and volatile `quota_used_mb` are read-only.

Package names and usernames are compared exactly because the API does not
document case-insensitive lookup. Presence requires an exact match in a complete
successful list with no case variants, followed by a detail read; a case variant
is an ambiguity error that blocks reads, creates and deletes. Creation never
adopts an existing object: an exact match asks for import. Only a complete list
proving absence removes state. Detail 404s, authorization failures, malformed
lists and ambiguous names preserve state.

## Update ordering, recovery and unsupported edits

User updates follow one deterministic order, persisting state after each
confirmed step:

1. unsuspend when `suspended = false` is requested and the user is suspended;
2. package reassignment when the remote package differs, then a fresh read;
3. one PATCH with a due password rotation and/or the configured quota override
   when it still differs from the fresh read;
4. suspend when `suspended = true` is requested and the user is active.

Comparing the override after re-reading makes the outcome independent of the
unverified C04 precedence: if reassignment resets the override it is re-sent,
and if it preserves it nothing is resent. A confirmed rotation version is stored
before readback so a later read failure cannot replay it. If a later step fails,
the next apply repeats only what still differs, without recreating the user or
repeating the reassignment. When readback after successful writes does not
confirm a requested package, quota or suspension, the apply fails with the
reported values recorded. Creation sends username, email, password and package,
records identity immediately, then converges quota and suspension the same way.

No contact-email update route is documented. Changing `email` (or `username`)
is therefore an explicit replacement shown in the plan, which deletes the user
and everything it hosts. Remote email changes are reported through
`current_email` and never trigger replacement. Examples and documentation
recommend `prevent_destroy`. Users reference packages so Terraform destroys
users before packages; renaming an in-use package fails safely rather than
cascading.

## Data sources and impersonation

`mxroute_reseller_packages` and `mxroute_reseller_users` return exact name sets
from one complete list, rejecting invalid or duplicate members, with no per-item
detail requests. `mxroute_reseller_package` and `mxroute_reseller_user` expose
the same conversions as the resources. No password is ever returned.

The [reseller guide](../../examples/reseller/README.md) and runnable example use a
plain reseller default provider and a `reseller|customer` alias with the
reseller's own key. The alias username is a literal input, never a reference to
a user created in the same run; a two-stage apply (or separate root module) is
documented. Terraform's in-process test factories reattach every alias to one
provider server, so the end-to-end impersonation test installs the test
executable as a dev-override plugin and drives the Terraform CLI, giving each
alias its own provider process as in production. The previous Phase 1 alias test
now shares that harness.

## Offline evidence

| Scenario | Regression evidence |
| --- | --- |
| Package defaults, explicit `unlimited`/zero limits, single-field package PATCH, user create, quota-only PATCH, password rotation, package reassignment with override re-applied, suspend/unsuspend, no-op plans, imports of both resources, data sources, users destroyed before packages | `TestOfflineResellerLifecycle` |
| Override re-sent when reassignment resets it and not resent when preserved | `TestOfflineResellerQuotaOverrideSequencing` |
| Reassignment confirmed, quota PATCH fails; recovery sends only the quota, with no second reassignment, recreation or rotation | `TestOfflineResellerPartialUpdateRecovery` |
| External suspension/package/quota/package-limit drift repaired in place; remote email drift reported without replacement; configured email change plans replacement; external deletion recreated | `TestOfflineResellerDriftAndReplacement` |
| In-use package deletion surfaced as a business error with state and users preserved, then completed after release | `TestOfflineResellerPackageInUse` |
| Uppercase/long new usernames, missing password, existing user/package, case-variant user/package and non-canonical limits rejected before any create | `TestOfflineResellerCreationRules` |
| Malformed, forbidden, detail-404 and case-ambiguous reads preserve state and recover to an empty plan | `TestOfflineResellerReadFailuresPreserveState` |
| Staged plain/impersonated workflow in separate provider processes; alias reads the owned user's quota and domains; reseller data source on an alias, non-owned target and non-reseller account fail without state changes | `TestOfflineResellerImpersonationWorkflow` |
| Reseller resources on an impersonated provider fail at plan time with no API request | `TestOfflineResellerImpersonatedResourcesRejected` |
| Unit/limit conversions, contradictory readback, canonical limit validation, list member validation | `TestResellerConversions` |
| Exact-name lookup, absence and case-variant ambiguity | `TestResellerExactLookup` |
| Existing-username syntax distinct from creation syntax | `TestAccountBoundaries` |
| Generated credentials and passwords absent from plans, saved plan archives and state | Shared `secretChecks` on every Phase 6 lifecycle step and CLI plan/state |

The mock's package-reassignment quota behavior is a test switch, not evidence
of MXroute behavior. Both settings pass.

## Remaining live-unverified items

- C04: whether reassignment or package updates change user overrides, and
  whether user/package readback reliably reports effective quota and unlimited
  state. The provider converges either way but fails on unconfirmed readback.
- Server acceptance of decimal GB quotas, user quota `0`, and username/package
  case handling.
- Exact business-error envelope for in-use package deletion, DirectAdmin
  impersonation authorization, and list completeness (C06/C10/C13).
- Server password strength rules for reseller users.

**Scope decision, October 19, 2026:** the user chose to skip live testing of
reseller surfaces. MXroute has no API to create API keys, so a reseller test
key could only come from a separately provisioned reseller account through
the panel. Reseller packages, users and impersonation therefore stay
mock-tested and live-unverified; Phase 7 excludes them, and releases must
say so. Revisiting this needs a disposable reseller account and the Phase 7
approval process. Never test reseller operations against the existing account.

## Validation

Run October 19, 2026 with Go 1.26.9 and Terraform 1.14.6:

- `make check build`: formatting, static analysis, all packages under the race
  detector (including the complete offline Terraform suite and the refactored
  Phase 1 alias test), the eight inventory-tool tests, the 43-operation/33-case
  pinned inventory, reproducible generated documentation and the local build.
- `terraform validate` of `examples/reseller` and `examples/aliases` against the
  locally built provider through a development override, with dummy runtime
  inputs and no API requests.
- Every test and documentation path referenced by `api/coverage.json` resolves.
  All 43 operations are marked implemented and mock-tested; none remain planned.

- The complete uncached offline suite (all packages, race detector) with Go
  1.26.9 and the minimum supported Terraform 1.11.1, and again with Go 1.27.1
  and Terraform 1.16.6.
- `make vuln`: govulncheck v1.7.0 found no vulnerabilities.

Source, examples and documentation use only reserved example
domains and invented identities; no state, plans or credentials were written to
the checkout.

The Phase 6 credential-free implementation and offline gate are complete,
subject to the live-unverified items above. Mock success cannot establish
production API compatibility. Offline success never authorizes live tests,
real-account cleanup, repository visibility changes or publication.
