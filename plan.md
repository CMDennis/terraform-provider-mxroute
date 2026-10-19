# MXroute Terraform Provider — Phased Implementation Plan

Date: October 18, 2026

Status: Phases 0–7 implemented October 19, 2026 within the authorized
live-test scopes; full live-lifecycle validation remains incomplete. Phase 8
release preparation is implemented locally; signing identity, repository settings,
public publication and Registry-installation gates remain owner actions.

Phase 0 artifacts and validation are recorded in
[the decision record](docs/design/decisions.md), including the exit gate,
[43-operation inventory](docs/design/operation-inventory.md),
[contract questions](docs/design/api-contracts.md), and the pinned public API
snapshot. All runtime behavior remains live-unverified; no authenticated MXroute
requests were made. The original planning policy below continues to govern later
phases.

Phase 1 architecture and schema conventions are recorded in
[schema conventions](docs/design/schema-conventions.md), with the implementation,
version adjustments and offline exit-gate evidence in
[the Phase 1 validation record](docs/design/phase1.md). The tested Terraform floor
is now 1.11.1 because 1.11.0 has a sensitive/ephemeral write-only serialization bug.
Framework and testing pins remain unchanged; indirect dependencies received
security updates. No MXroute resource or data source is exposed yet.

Phase 2 delivers typed methods and request/response tests for all 43 operations,
strict response validation, typed safe errors, bounded retries, shared rate
budgets and effective-account setting locks. Offline evidence and remaining
contract assumptions are recorded in [the Phase 2 validation record](docs/design/phase2.md).
No authenticated MXroute requests were made; resource lifecycles remain later work.

Phase 3 adds domain and pointer resources with import, mail-hosting changes,
bounded optional DNS verification and visibility waits, plus six verification,
lookup/list and DNS discovery data sources. Partial domain setup uses an explicit
pending status and warning so the next refreshed apply can recover without
replacing the new domain. Offline evidence, lifecycle choices and remaining live
assumptions are recorded in [the Phase 3 validation record](docs/design/phase3.md).
No authenticated MXroute requests or live DNS writes were made.

Phase 4 adds mailbox create/read/update/delete with write-only password rotation,
canonical lookup and import; forwarders with replacement-only destination sets;
and non-deletable catch-all management that preserves routing on state removal.
Five read-only mailbox/routing data sources complete the ordinary mail workflow.
Offline evidence and limitations are recorded in [the Phase 4 validation record](docs/design/phase4.md).
No authenticated MXroute requests were made.

Phase 5 adds shared spam sender-entry ownership, a conservatively account-owned
score singleton, and five spam/reporting data sources. Public documentation
confirms shared scoring effects; exact cross-domain propagation remains
live-unverified. Offline lifecycle, concurrency and readback evidence is recorded
in [the Phase 5 validation record](docs/design/phase5.md).
No authenticated MXroute requests were made.

Phase 6 adds reseller package and user resources with import, write-only user
password rotation, in-place package reassignment and suspension, explicit MB/GB
limit strings that keep zero distinct from unlimited, and four reseller list/detail
data sources. User updates run in a fixed, recoverable order and never cascade
package deletion. Impersonation aliases are rejected for reseller surfaces and
exercised end to end, in separate provider processes, for an owned user. All 43
operations now have Terraform workflows and offline tests; package/override
precedence remains live-unverified under C04. By user decision, reseller surfaces
are tested only against local mocks and are excluded from Phase 7 live testing.
Offline evidence is recorded in [the Phase 6 validation record](docs/design/phase6.md).
No authenticated MXroute requests were made.

Phase 7 adds guarded ordinary-account live reads, a quota response-envelope
compatibility fix, synthetic regressions, a schema compatibility baseline, large
inventory checks, field-level coverage audit, migration and troubleshooting guides,
and expanded offline compatibility CI. The October 19 real-account request
authorized read-only checks while prohibiting destructive actions or overwriting
settings. All fourteen ordinary read operations passed and the observed settings
baseline was unchanged. The initial read-only run excluded writes, imports, teardown, reseller operations
and impersonation. Its remaining gaps block a
fully live-validated stable-release claim. See the [Phase 7 record](docs/design/phase7.md).

A later explicit follow-up authorized one new test mailbox and changes only to
it. Creation, isolated quota/send-limit/password PATCHes and preservation of
pre-existing configuration passed live. The new mailbox was imported into
separate private Terraform state and produced a no-change saved plan. No apply,
destroy or deletion was performed in that initial test. A subsequent explicit
cleanup request authorized deletion of only the owned test mailbox. Its reviewed
single-object plan applied successfully; independent reads confirmed absence and
the unchanged original configuration. No test objects remain; private provenance
and the empty isolated Terraform state are retained. Other mutations remain unverified. See the
[mailbox test record](docs/design/mailbox-live-testing.md).

## Objective and starting point

Phase 8 now provides six-platform packaging, signed-checksum verification,
protocol-v6 manifest distribution, production-package mock installation/lifecycle
tests, private-account commit/push/release checks, a manual protected draft workflow,
maintenance automation and comprehensive release/user/support documentation.
[The Phase 8 record](docs/design/phase8.md) separates passing local evidence from
pending publication gates; [publishing](docs/design/publishing.md) lists the exact
manual GitHub, GPG and Registry steps. No repository visibility change, release,
tag, commit or push was performed during this preparation.

Build a reliable Go Terraform provider that covers every operation in the MXroute Email Hosting API, with predictable plans, import support, drift detection, secure credential handling, complete documentation, and repeatable releases.

The [repository](https://github.com/CMDennis/mxroute-terraform) currently contains a README and LICENSE and is private. The local project directory was empty when this plan was prepared. Treat this as a new provider rather than a migration of existing implementation.

The [MXroute OpenAPI specification](https://api.mxroute.com/openapi.yaml), linked from the [API documentation](https://api.mxroute.com/docs), declares version 1.0.0 and currently describes **43 HTTP operations across 26 paths**. This is the initial coverage baseline. API details in this plan come from that specification; runtime behavior has not been tested. Proposed Terraform names and lifecycle choices below are design recommendations to finalize before implementation.

“Complete coverage” means every documented operation has a tested client method and a supported Terraform workflow. Durable configuration belongs in resources, reads belong in data sources, and operational endpoints that change readable configuration belong in resource updates. It does not require one Terraform resource per HTTP endpoint.

## Credential-free development and live-account testing policy

The user currently has credentials only for a real account, not a disposable sandbox. Protect that account and all existing resources. This policy applies to every phase, test runner, cleanup routine, and release smoke test below.

- **Build through Phase 6 without real API credentials.** Use synthetic fixtures, a local mock API, and offline Terraform acceptance tests. Tests must explicitly select the mock endpoint, use dummy credentials, and fail closed rather than fall back to production or inherited credentials. All Phase 0–6 testing and exit gates mean offline verification; runtime assumptions remain marked as unverified until Phase 7. Mock success is not proof of live API compatibility.
- **Defer all authenticated live testing to Phase 7 and ask the user first.** Prepare the exact test scope, affected resource identities, expected changes and side effects, cleanup actions, and recovery limits for review before requesting approval. Possession of credentials is not approval to use them. The current request authorizes planning only, not any future live test.
- **Live testing must be non-destructive to the existing account and its resources. If a Terraform plan contains a destroy, assume it is a real remote destruction.** This includes replacement actions, implicit cleanup, and test-framework teardown. Never apply a destroy or replacement affecting pre-existing objects. Proposed singleton state-only removal behavior is not grounds to bypass this review rule.
- **Temporary resource creation and destruction may be allowed only with prior, explicit approval of the specific test and its cleanup.** Use isolated, disposable test objects with no real mail or user data. Creating a test object does not authorize destroying it unless that cleanup was included in the approved scope. Do not delete, replace, suspend, or otherwise jeopardize the user's account or any pre-existing domain, mailbox, user, package, routing, or shared setting. Tests with irreversible effects on real account data are out of scope.
- **Inventory and isolate before writes.** After approval for read-only discovery, capture a baseline inventory and relevant settings. Use a separate test state/backend and an explicit manifest of approved identities created by that run. Never import existing resources into a state that an acceptance harness will automatically destroy. Names or prefixes alone are insufficient proof of ownership.
- **Review every mutation, including cleanup.** Before applying, inspect the exact saved plan for deletes, replacements, updates, and indirect effects. Restrict writes to the approved manifest and re-check ownership and dependencies immediately before deletion. For API calls or test harnesses without a reviewable plan, enforce equivalent operation/identity allowlists or keep those tests offline. Fail closed on unknown identities, unexpected diffs, or possible cascades into existing resources. Any changed scope requires renewed approval.
- **No blanket live destroy or automatic sweeping.** Disable unattended teardown against the real account unless the specific cleanup is approved and guarded. On failure or uncertainty, stop, retain the test state and ownership manifest, and report remaining objects. Do not broaden cleanup to make a test pass. Backups, `prevent_destroy`, and separate state are additional safeguards, not guarantees that a destructive operation is reversible.
- **Keep unsafe or unavailable tests offline.** Account-wide spam changes, forwarder side effects, package changes, deletion cascades, and reseller-account lifecycle tests require particular scrutiny. If they cannot be isolated safely, record them as mock-tested/live-unverified until a suitable sandbox exists. Missing reseller credentials or denied live testing must never trigger experimentation on the real account or be reported as a passing live test.

Credentials, if approved for Phase 7, must be supplied at runtime through environment variables or protected secrets. Never save API keys, any passwords, actual account details or personal identifying information to Git, including examples, fixtures, documentation, generated artifacts and commit messages. Generate mock passwords at runtime rather than storing literals. Keep live responses, account inventories, ownership manifests, Terraform state/plans and logs outside Git. Review staged content before every commit; ignore rules are an additional safeguard only. See [AGENTS.md](AGENTS.md).

## Phase 0 — Freeze scope and settle API contracts

**Dependencies:** None. **Outcome:** An implementation-ready API inventory and design decisions.

1. Obtain the repository checkout, preserve its license, and decide the Go module path, provider namespace, and distribution model. Proposed provider type: `mxroute`; binary: `terraform-provider-mxroute`.
2. Save a dated OpenAPI snapshot and checksum during implementation. Generate an operation inventory keyed by HTTP method and path, since the specification does not provide operation IDs. Track each operation's client method, resource/data source, tests, documentation, and status.
3. Identify sparse or ambiguous contracts from public documentation and model them with synthetic fixtures. Record runtime assumptions for approved verification in Phase 7; no real credentials are required here. Prioritize success response bodies, 204 responses, error envelopes, missing-resource behavior, canonical names, defaults, nullable fields, and list completeness. Do not assume pagination that the specification does not define.
4. Resolve the design questions below before stabilizing the schema:
   - Whether the spam score is account-wide like the explicitly shared sender lists, or has distinct domain scope.
   - Whether forwarder creation can update an existing alias. Until verified, model destination changes as replacement because no update endpoint is documented.
   - Domain/pointer verification prerequisites, domain deletion cascades, and any system mailbox restrictions.
   - Package-change effects on user quota overrides, package updates' effects on existing users, and which returned quota values can be read back reliably.
   - Catch-all and spam singleton removal semantics, where no API delete/reset operation exists.
   - Whether any API endpoints are missing from OpenAPI. Bulk/import appears only in the rate-limit description; no bulk/import operations are documented. Do not invent them. Terraform state import is a separate capability.
5. Define future live-test prerequisites: ordinary account, reseller access, owned ordinary user for impersonation, isolated test domains, DNS control, and clear ownership boundaries. Only a real account is currently available; do not assume reseller access or a sandbox exists. Implement all scenarios with mocks now and track unavailable or unsafe live scenarios for Phase 7 under the testing policy above.
6. Decide the supported Terraform, Go, and Plugin Framework versions. Recommended initial Terraform floor: 1.11 to support write-only passwords; verify selected dependency compatibility before pinning versions. Treat OpenTofu compatibility as a separate tested commitment if desired.

**Exit gate:** All 43 operations are inventoried; uncertain behavior has either a verified contract or an explicit blocking question for the affected feature; provider identity and version policy are recorded.

## API coverage matrix

Each method listed below is a separate coverage obligation. All proposed Terraform names use the `mxroute_` prefix. A resource and data source may share a name because their Terraform namespaces are separate. Singular lookups can use list endpoints when the API has no detail endpoint.

| API path | Methods | Proposed Terraform surface | Phase |
| --- | --- | --- | --- |
| `/verification-key` | GET | `mxroute_verification_key` data source | 3 |
| `/domains` | GET, POST | `mxroute_domains` data source; `mxroute_domain` resource create | 3 |
| `/domains/{domain}` | GET, DELETE | `mxroute_domain` resource read/delete and data source | 3 |
| `/domains/{domain}/mail-status` | PATCH | `mxroute_domain.mail_hosting` attribute | 3 |
| `/domains/{domain}/pointers` | GET, POST | `mxroute_domain_pointers` and `mxroute_domain_pointer` data sources; pointer resource read/create | 3 |
| `/domains/{domain}/pointers/{pointer}` | DELETE | `mxroute_domain_pointer` resource delete | 3 |
| `/domains/{domain}/email-accounts` | GET, POST | `mxroute_email_accounts` data source; `mxroute_email_account` resource create | 4 |
| `/domains/{domain}/email-accounts/{user}` | GET, PATCH, DELETE | `mxroute_email_account` resource read/update/delete and data source | 4 |
| `/domains/{domain}/forwarders` | GET, POST | `mxroute_forwarders` and `mxroute_forwarder` data sources; forwarder resource read/create | 4 |
| `/domains/{domain}/forwarders/{alias}` | DELETE | `mxroute_forwarder` resource delete | 4 |
| `/domains/{domain}/spam/settings` | GET, PATCH | `mxroute_spam_settings` resource and data source | 5 |
| `/domains/{domain}/spam/whitelist` | GET, POST | `mxroute_spam_whitelist` data source; `mxroute_spam_whitelist_entry` resource read/create | 5 |
| `/domains/{domain}/spam/whitelist/{entry}` | DELETE | `mxroute_spam_whitelist_entry` resource delete | 5 |
| `/domains/{domain}/spam/blacklist` | GET, POST | `mxroute_spam_blacklist` data source; `mxroute_spam_blacklist_entry` resource read/create | 5 |
| `/domains/{domain}/spam/blacklist/{entry}` | DELETE | `mxroute_spam_blacklist_entry` resource delete | 5 |
| `/domains/{domain}/dns` | GET | `mxroute_dns_records` data source | 3 |
| `/domains/{domain}/catch-all` | GET, PATCH | `mxroute_catch_all` resource and data source | 4 |
| `/quota` | GET | `mxroute_quota` data source | 5 |
| `/quota/email` | GET | `mxroute_email_usage` data source | 5 |
| `/reseller/users` | GET, POST | `mxroute_reseller_users` data source; `mxroute_reseller_user` resource create | 6 |
| `/reseller/users/{username}` | GET, PATCH, DELETE | `mxroute_reseller_user` resource read/update/delete and data source | 6 |
| `/reseller/users/{username}/suspend` | POST | Set `mxroute_reseller_user.suspended = true` | 6 |
| `/reseller/users/{username}/unsuspend` | POST | Set `mxroute_reseller_user.suspended = false` | 6 |
| `/reseller/users/{username}/package` | PATCH | Update `mxroute_reseller_user.package` | 6 |
| `/reseller/packages` | GET, POST | `mxroute_reseller_packages` data source; `mxroute_reseller_package` resource create | 6 |
| `/reseller/packages/{name}` | GET, PATCH, DELETE | `mxroute_reseller_package` resource read/update/delete and data source | 6 |

No documented endpoint manages DNS records at a registrar, API key creation, mailbox suspension, or arbitrary DirectAdmin settings. DNS values and mailbox suspension can be exposed as returned information, but unsupported writes must not be implied. New endpoints discovered later must be added to this matrix before claiming complete coverage.

## Phase 1 — Establish provider architecture and schema conventions

**Dependencies:** Phase 0 decisions. **Outcome:** A small, maintainable foundation.

1. Use the [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework), protocol v6, and `terraform-plugin-testing`. Keep HTTP transport and API models separate from Terraform schemas and lifecycle code.
2. Establish a conventional layout: provider entry point; `internal/client` for transport and typed API methods; `internal/provider` for resources/data sources; shared validation and identity helpers; `examples`; generated `docs`; sanitized `testdata`; CI and release configuration. Prefer a typed handwritten client initially; evaluate generation only where it handles the incomplete schemas cleanly.
3. Define provider inputs `server`, `username`, and sensitive `api_key`, with explicit environment fallbacks such as `MXROUTE_SERVER`, `MXROUTE_USERNAME`, and `MXROUTE_API_KEY`. Configuration takes precedence over environment values. Validate missing, empty, and unknown values without breaking Terraform planning. Use the fixed production API URL by default and inject test endpoints through the client test harness.
4. Support aliased providers for multiple servers/accounts and `reseller|user` impersonation. Keep credential-owner identity distinct from effective-account identity. Do not store provider credentials in resource IDs or computed attributes.
5. Standardize immutable identifiers, import syntax, case normalization, path escaping, nested types, and `Required`/`Optional`/`Computed` behavior. Use sets only where order has no meaning. Preserve differences between omitted, zero, empty, null, and unknown values.
6. Define replacement rules explicitly. Domain names, mailbox identities, package names, and other unchangeable identities require replacement. Avoid blanket state-preservation plan modifiers that hide drift. Use state schema versions and migration hooks from the first release.
7. Adopt write-only mailbox and reseller-user passwords with an explicit rotation version attribute. Require a password at creation without forcing imported objects to rotate it. Omitted passwords on update preserve the remote credential. Verify passwords never enter persisted plans/state; sensitive flags alone do not prevent storage. See [write-only argument guidance](https://developer.hashicorp.com/terraform/plugin/framework/resources/write-only-arguments).
8. Set up formatting, linting, unit tests, vulnerability checks, dependency pinning, and documentation generation early. Keep generated documentation reproducible and reviewable.

**Exit gate:** Provider builds, validates representative configurations, handles aliased configurations correctly, and has agreed schema/identity conventions. Credentials remain absent from logs and resource state.

## Phase 2 — Implement and harden the API client

**Dependencies:** Phase 1. **Outcome:** A tested client supporting all 43 operations.

1. Implement typed request/response models and methods for every matrix entry. Explicitly handle wrapped `success/data` responses, unwrapped quota responses, sparse bodies, and empty 204 responses. Treat `success: false` as an error even if the HTTP status is unexpectedly successful.
2. Send `X-Server`, `X-Username`, and `X-API-Key` exactly as configured. Preserve colons in API keys; do not reconstruct obsolete `keyid:password` credentials. Document DirectAdmin restrictions and account-password access requirements.
3. Validate impersonation syntax separately from reseller user creation rules. Use plain reseller identity for reseller-only endpoints and return useful errors when those endpoints are requested through an impersonated configuration.
4. Map API codes and HTTP statuses to typed errors and actionable diagnostics. Only confirmed absence removes a resource from state. Authentication, authorization, parsing, ambiguous lookup, and upstream errors must preserve state and fail the operation.
5. Add context cancellation, request/operation timeouts, bounded backoff with jitter, and rate-limit coordination. Respect `Retry-After` and reset headers. The documented ceilings are 200 reads and 60 writes per minute; use conservative budgets with headroom.
6. Share rate-limit budgets within a provider process by server and credential owner, including impersonated aliases. Serialize shared-setting mutations by effective account. Separate processes cannot share an in-memory limiter; document conservative parallelism and server-directed backoff for concurrent runs.
7. Define retries by endpoint semantics. Retry safe reads and clearly rejected throttled requests within deadlines. After ambiguous write failures, reconcile remote state before deciding whether to retry. Do not blindly retry creates, destructive operations, or password changes. Report uncertainty honestly when readback cannot verify an outcome.
8. Prevent credential leakage through debug logs, diagnostic bodies, redirects, or fixtures. Keep TLS verification enabled and reject unexpected credential-bearing redirects. Include provider version in the user agent and log only sanitized request metadata.
9. Build an HTTP test harness for success, empty/malformed JSON, unexpected content types, 401/403/404/409/422/429/500/503, timeouts, cancellation, partial writes, and upstream schema variation. Cover URL encoding for addresses and wildcard spam entries.

**Exit gate:** Every documented operation has request/response tests; retry behavior and redaction are verified; malformed responses cannot silently erase Terraform state.

## Phase 3 — Deliver domain onboarding, pointers, and DNS discovery

**Dependencies:** Phases 1–2. **Outcome:** First complete user workflow and alpha milestone.

1. Implement the verification-key data source with record name/type/value. Demonstrate the actual onboarding order: obtain key, create verification TXT through a DNS provider, wait for propagation, then create the MXroute domain.
2. Implement domain create/read/delete, import, and mail-hosting updates. Model SSL status and pointer summaries as computed information, not independently managed pointer configuration.
3. Handle partial domain creation: record the stable identity once creation succeeds, then apply requested mail-hosting configuration. If the second step fails, preserve enough state for the next apply to recover.
4. Provide configurable bounded waits for recognized DNS-verification/visibility conditions. Avoid unconditional sleeps or retrying unrelated validation errors. Cross-provider dependency ordering does not itself guarantee DNS propagation.
5. Implement pointer resources with alias/redirect mode, list-backed reads, import, and replacement for unsupported changes. Verify creation prerequisites independently of the primary domain.
6. Implement domain/pointer lookup and list data sources plus DNS discovery for MX, SPF, nullable DKIM, and nullable verification records. Preserve DNS content and make outputs convenient for external DNS providers.
7. Document domain deletion consequences, parent-child dependencies, and `prevent_destroy` for important mail domains. Do not silently purge unmanaged children to make deletion succeed.
8. Test creation, second-apply no-op, mail-hosting changes, import followed by no-op plan, external drift, external deletion, failed DNS verification, and dependency-ordered cleanup.

**Exit gate:** A fresh domain can be onboarded from verification through DNS outputs against the mock service using documented examples; import and drift behavior pass offline acceptance tests. Any alpha must explicitly disclose pending live validation.

## Phase 4 — Deliver mailbox and routing management

**Dependencies:** Phase 3. **Outcome:** Complete ordinary mail configuration lifecycle.

1. Implement mailbox resources and single/list data sources: password creation/rotation, quota, daily send limit, computed usage/sent counts/suspension status, import, and deletion.
2. Respect the documented mailbox password length of 8–72 characters while surfacing server-specific strength/character errors. Validate the documented daily-send ceiling of 9600 and distinguish quota zero (unlimited) from omission. Verify other bounds before enforcing them.
3. Preserve canonical mailbox names: updates resolve local parts case-insensitively, and ambiguous lookup must fail. Avoid replacing or duplicating mailboxes merely because input casing differs. Send only intended changed fields so password-only updates retain quota and send limits.
4. Implement forwarders with destination sets, special `:blackhole:` and `:fail:` targets, list-backed reads, import, and deletion. Validate supported combinations. Unless Phase 0 proves an update contract, destination changes require replacement and documentation of the resulting routing interruption; do not promise create-before-destroy for a unique alias.
5. Document automatic Expert Spam Filtering activation for certain forwarding destinations. Do not claim to manage that side effect through an unsupported API field.
6. Implement catch-all configuration and reads for `fail`, `blackhole`, and `address`, with conditional address validation and correct clearing of stale address values when switching modes.
7. For catch-all and other non-deletable singleton settings, use an explicit lifecycle contract: create means applying desired settings; destroy relinquishes Terraform management while preserving remote settings. Document this prominently and allow import of the existing singleton. Do not invent an upstream default or silently reset configuration during destroy.
8. Test imports without known passwords, rotation without persistent secrets, quota-only/password-only updates, routing replacement failures, catch-all transitions, canonicalization, and stable repeated plans. Keep volatile usage fields computed and out of writable desired state.

**Exit gate:** Mailboxes, forwarders, and catch-all are fully documented and tested, including secret handling and the unavoidable limitations of replacement and singleton removal.

## Phase 5 — Deliver spam controls and usage visibility

**Dependencies:** Phases 2–4; spam-scope decision from Phase 0. **Outcome:** Safe shared spam management and complete reporting.

1. Implement spam settings read/update with score validation from 1 through 50. Send only documented fields and respect the 2048-byte request limit. Scope the resource to the verified underlying setting, not merely the domain-shaped URL.
2. Implement whitelist and blacklist entry resources and list data sources. Entries are shared across the effective account's domains; use an owned domain as the API access context, not as an independent owner of a second copy of the list.
3. Define stable import IDs and canonical entry comparison, including case-insensitive duplicates and wildcard encoding. A change of access domain within the same account must not delete and recreate a shared entry. Document one Terraform owner per account/entry and limits of duplicate detection across independent states.
4. Prefer per-entry ownership so Terraform preserves unrelated entries. Do not also expose an authoritative whole-list resource that competes with entry resources. Verify requested entry presence/absence using complete successful list reads.
5. Treat an unavailable setting or failed account verification as an error. A missing access domain does not prove a shared entry is gone. After an unconfirmed spam save, read before retrying; handle busy/locking responses with bounded backoff.
6. Apply the documented relinquish-management behavior to non-deletable spam settings. Deleting an entry resource does remove that specific entry.
7. Implement account quota and per-email usage data sources, preserving bytes versus MB/GB, unlimited semantics, category breakdowns, nullable grace periods, timestamps, and hourly refresh limitations.
8. Test shared access across two domains, concurrent entry changes, preservation of unrelated entries/settings, case-insensitive duplicates, account isolation, failed readback, and unavailable upstream values. Ensure stale hourly usage does not trigger configuration changes.

**Exit gate:** Shared spam state cannot be accidentally managed as separate domain copies; all reporting fields are accurately typed and documented.

## Phase 6 — Deliver reseller lifecycle and impersonation workflows

**Dependencies:** Phases 2–5. **Outcome:** All documented API operations exposed through Terraform workflows.

1. Implement reseller packages with CRUD, single/list data sources, and import. Convert request quota in GB and response settings deliberately. Preserve unlimited/null semantics and distinguish zero customer mailboxes from unlimited; the system mailbox does not consume the customer-mailbox allowance.
2. Implement reseller users with creation, read, deletion, password/quota updates, single/list data sources, and import. User quota writes use MB or `unlimited`, unlike package quota in GB. Use unambiguous Terraform attribute names and tested conversions.
3. Manage suspension as a readable `suspended` boolean, dispatching suspend/unsuspend only when needed. Manage package reassignment through the package endpoint. No standalone action resource is needed for these observable state transitions.
4. Treat unsupported edits, including contact email if no update route is verified, as explicit replacement or creation-only behavior with clear documentation. Apply the documented 1–10 lowercase username creation constraint separately from authentication's 1–16 character rule.
5. Define deterministic sequencing for package assignment and explicit user quota overrides based on verified API behavior. Recover from partial multi-step updates without recreating the user or erasing successfully changed state.
6. Express user-to-package references so destruction removes users before packages. Surface package-in-use errors without cascading deletion of users. Document consequences of user deletion and changes to shared packages.
7. Provide an end-to-end example with a plain reseller provider and aliased impersonated providers for owned accounts. Explicitly document any staged applies needed for newly created identities; do not assume provider configuration can depend on unknown resources created in the same run.
8. Test permission boundaries, impersonation target ownership, plain versus impersonated endpoints, package switching, quota overrides, password rotation, suspension transitions, imports, and package-in-use failures.

**Exit gate:** Every row in the coverage matrix has implemented Terraform behavior and offline endpoint/lifecycle tests. Phase 6 is the credential-free implementation milestone. Beta evaluation can begin with live-validation gaps clearly disclosed; it must not claim verified production compatibility.

## Phase 7 — Prove correctness and prepare users for production

**Dependencies:** Phases 3–6. Testing and documentation also accompany every earlier phase.

1. Run offline unit and HTTP contract tests on every change and Terraform acceptance tests against the controlled mock service. Before any authenticated live testing, present the concrete test scope and obtain the user's approval under the live-account policy above. Start with approved read-only discovery. Do not run the full acceptance suite against the real account: [Terraform's provider testing framework](https://developer.hashicorp.com/terraform/plugin/testing) exercises actual lifecycle operations, including destruction.
2. Require each resource to demonstrate create/read/update-or-replace/delete, import, second-apply no-op, external modification detection, external deletion handling, and documented singleton behavior in mocks. Repeat only specifically approved, isolated scenarios live. Creation and cleanup of disposable objects may proceed together only when both are approved in advance; account deletion and destruction of existing resources remain prohibited. Test all writable fields and important cross-field combinations offline even when live verification is unsafe or unavailable.
3. Verify unknown/null values, defaults, normalized sets, case changes, zero/unlimited boundaries, import without secrets, and refresh of nullable output fields. Add regression fixtures for every discovered API discrepancy.
4. Exercise cancellation, throttling, bounded retries, failed response decoding, ambiguous writes, partial multi-step updates, concurrent aliases, and large account inventories. Measure request budgets and avoid uncontrolled per-item detail calls. Cache only when correctness and account isolation are preserved.
5. Verify release-to-release state compatibility with fixtures and state upgrade tests whenever schema changes occur. Test the chosen minimum and current supported Terraform versions and supported operating systems.
6. Keep live credentials out of untrusted pull-request jobs and default CI runs. Use separate test state, baseline inventory, explicit ownership manifests, and operation allowlists. Inspect every saved plan, treating all destroys/replacements as real. Block changes to pre-existing objects and any unexpected side effects. Cleanup must stay within the approved disposable-object scope, with no blanket destroy or sweeper. After testing, compare against the baseline to confirm existing resources/settings remain intact; report any differences and leftover test objects without attempting unapproved repairs.
7. Generate reference docs and runnable examples for every resource/data source. Include authentication, provider aliases, impersonation, DNS onboarding, imports, secret rotation, shared settings, replacement downtime, deletion semantics, rate limits, quota units, and troubleshooting.
8. Add practical migration guides for importing existing domains, mailboxes, routing, spam entries, users, and packages without recreation. Demonstrate a no-change plan after adoption and explicitly describe unreadable secrets.
9. Recompare the pinned OpenAPI inventory with the current upstream version. Classify additions and breaking changes; reconcile the coverage matrix before the stable release. Audit field-level coverage as well as the 43-operation count.

**Exit gate:** No unresolved state-loss, secret-leakage, destructive-replacement, or perpetual-diff defects; approved live tests pass and baseline comparison confirms existing resources/settings are intact. Record every scenario as offline-passed, live-passed, or live-unverified. Unsafe or unavailable scenarios remain live-unverified and block a claim of fully live-validated coverage; obtain an appropriate sandbox later rather than relaxing the account-protection rules. Documentation must disclose those limits before deciding release readiness.

## Phase 8 — Release, distribute, and maintain

**Dependencies:** Phase 7. **Outcome:** Installable, verifiable provider and an ongoing support process.

1. Finalize distribution before publishing. The current private `mxroute-terraform` repository does not satisfy public Registry repository requirements. A public Registry release needs an appropriately named public `terraform-provider-mxroute` repository; alternatives include retaining private distribution through a registry or mirror. Decide repository naming/visibility and licensing deliberately during implementation.
2. Establish semantic versioning and staged milestones: domain alpha after Phase 3, ordinary-account preview after Phase 5, full API beta after Phase 6, and 1.0 after Phase 7's gates. Publish a compatibility/support policy and a changelog describing behavior and migration impact.
3. Automate cross-platform builds with GoReleaser, signed checksums, a protocol-v6 Registry manifest, and immutable versioned release artifacts. Keep signing credentials restricted to trusted release jobs. Follow the [Registry publishing requirements](https://developer.hashicorp.com/terraform/registry/providers/publishing).
4. Smoke-test installation through the selected distribution channel on a clean machine, then validate examples and perform a small lifecycle/import workflow using the released binary against the mock service. Any live release smoke test remains subject to the Phase 7 approval and account-protection policy. Verify artifact checksums and signatures.
5. Establish dependency/security updates, an API-spec change review process, scheduled compatibility testing, issue templates, support ownership, and a release checklist. Breaking schema/lifecycle changes require a migration story and appropriate versioning.
6. For newly added upstream operational endpoints, evaluate Terraform actions only if they genuinely lack a durable readable resource state and the supported Terraform version permits them. Avoid placeholder resources that rerun side effects on every apply; see [Framework actions](https://developer.hashicorp.com/terraform/plugin/framework/actions).

**Exit gate:** A clean user environment can install the provider, follow documented workflows, and upgrade without unexpected changes; releases and ongoing API coverage checks are repeatable.

## Cross-phase definition of done

- [ ] Every documented method/path maps to a tested client method and supported Terraform workflow.
- [ ] Every documented input/output field is supported or has an explicit, justified limitation.
- [ ] Every resource has a stable identity, import syntax, drift behavior, and documented destruction/replacement semantics.
- [ ] API errors never masquerade as successful empty reads or remove valid state.
- [ ] Reapplying unchanged configuration produces no changes; updates preserve unrelated fields and objects.
- [ ] Passwords and API credentials are absent from logs, fixtures, resource IDs, and persisted secret attributes.
- [ ] Ordinary, reseller, and impersonated account boundaries are tested.
- [ ] Rate limits, cancellation, concurrent access, and uncertain write outcomes have bounded, tested handling.
- [ ] Complete examples, generated reference docs, release artifacts, and upgrade guidance pass the release checks.
- [ ] The upstream API baseline and any verified deviations are recorded, with no undisclosed coverage gaps.
- [ ] Phases 0–6 run without real credentials; live results are distinguished from mock results.
- [ ] Every live test and its cleanup have explicit prior approval, with existing account resources protected and baseline integrity checked.
- [ ] Destroy/replacement plans and implicit teardown are treated as real destructive operations; only approved disposable test objects may be removed.

## Recommended execution order

Complete Phases 0–2 first, then deliver Phases 3–6 as vertical slices with their own offline tests and documentation, without requesting or using a real API key. Carry unverified runtime assumptions forward explicitly. At Phase 7, prepare a concrete, non-destructive live-test proposal and ask the user before using the real account; proceed only within the approved scope. Use Phase 8 to publish and maintain the provider with its validation limits disclosed. Do not postpone import, secret handling, or lifecycle decisions until after resource schemas are public; those choices are expensive to change once users have state.

Historical scope: this file was the only artifact created for the original planning request. Subsequent October 19, 2026 requests authorized Phases 0–6 implementation, now recorded above. The later Phase 7 request authorized ordinary-account read-only testing with the explicit account-protection constraint recorded above. The subsequent follow-up authorized only one new test mailbox and its isolated updates/adoption. The next explicit request authorized removal of only that test mailbox, now independently confirmed. No request authorizes changes to pre-existing objects, wider cleanup, repository visibility changes or publication.
