# Provider architecture and schema conventions

Phase 1 establishes these rules for the resources delivered in Phases 3–6.
Phases 3–6 now register ten domain, pointer, mailbox, routing, shared spam and
reseller resources and twenty data sources. See [Phase 3](phase3.md), [Phase 4](phase4.md),
[Phase 5](phase5.md) and [Phase 6](phase6.md) for lifecycle and recovery details.
The synthetic `mxroute_fixture` exists only in the test executable, exercises
the shared helpers, and is never an installable resource.

## Package boundaries

- `main.go` serves protocol v6 at `registry.terraform.io/cmdennis/mxroute`.
  Each provider instance has its own client; aliases share no mutable configuration.
- `internal/client` owns credentials in memory, the fixed production origin,
  HTTP transport, typed request/response models and all 43 API operations.
  It imports no Terraform packages. Typed operations have strict decoding, a
  two-minute deadline, a 30-second request limit, shared rate budgets, bounded
  safe retries, TLS verification and disabled redirects. See [Phase 2](phase2.md).
- `internal/provider` owns Terraform schemas, diagnostics, imports and lifecycle
  orchestration. Provider configuration never makes a request. Unknown values
  are accepted during validation but must resolve before configuration; do not
  initialize an authenticated client from partial or inherited unknown inputs.
- `internal/identity` owns identity syntax, normalization, import parsing and
  per-segment path escaping. `internal/schemautil` owns reusable Terraform schema
  and password intent rules. `internal/testutil` generates disposable secrets.
- `examples` and `templates` are documentation sources. `docs/index.md` is
  generated with pinned tfplugindocs; `docs/design` is maintained by hand.

Provider inputs are optional only to support environment fallbacks. Omitted/null
means use the corresponding `MXROUTE_*` variable. Explicit empty/whitespace or
unknown inputs never fall back to the environment. Invalid values are diagnosed
by attribute path without quoting inputs. API keys retain their complete bytes,
including colons. Provider keys are sensitive, but provider arguments cannot be
write-only: use environment variables or ephemeral inputs to keep keys out of
saved plans. Credential values are never placed in resource models or IDs.

Authentication accepts one 1–16 character alphanumeric/underscore name, or two
such names separated by one pipe. Each impersonated provider retains both the
credential owner and effective user. Ownership/roles remain server decisions;
reseller-only methods must call `RequireReseller` before making a request.
Reseller creation separately permits 1–10 lowercase letters, digits or
underscores. Shared rate budgets by server/owner and shared-setting locks by
effective account are implemented in Phase 2 within each provider process.
Separate Terraform processes cannot share these in-memory budgets or locks.

## Identities and imports

IDs are relative to the configured effective account. Never embed server,
username, API key, password, or a mutable setting in a resource ID. Moving a
resource between account aliases requires deliberate state migration/import;
changing a provider configuration does not authorize cross-account adoption.

| Resource family | Import/ID form | Normalization and replacement |
| --- | --- | --- |
| Domain | `example.com` | ASCII DNS, lowercase, remove one trailing dot during import; immutable |
| Domain pointer | `example.com/alias.example.net` | Normalize both DNS names; parent/pointer/mode changes replace |
| Mailbox | `example.com/localpart` | Domain canonical; case-insensitive local comparison; domain/local identity changes replace |
| Forwarder | `example.com/alias` | Domain canonical; alias comparison lowercase; identity and destination changes replace |
| Catch-all | `example.com` | Domain canonical; destroy relinquishes management without a remote reset |
| Spam whitelist/blacklist entry | ID `*@example.net`; import `example.com/*@example.net` | Account-relative, case-insensitive comparison; import includes access context, which is not stored identity; entry changes replace |
| Spam score singleton | ID `spam-settings`; import using an owned access domain | One conservative account owner because saves affect shared scoring; precise cross-domain propagation remains live-unverified |
| Reseller user | `ordinary` | Authentication syntax for existing objects; stricter creation syntax; username immutable |
| Reseller package | `Mail_Pro-1` | Preserve case; documented letters/digits/underscore/hyphen; name immutable |

Forms contain raw components, not URL-encoded values. `Scoped` splits exactly
once on the domain/name boundary and rejects missing/extra separators. `Path`
escapes every HTTP path segment independently, exactly once, preserving wildcard,
percent and slash bytes as data. Import helpers validate shape without contacting
an account. Resource reads confirm the identity before persisting it.

Configuration for DNS identities should require canonical lowercase ASCII
(punycode for international names). Normalize imports before setting attributes.
For mailbox/alias case differences, use semantic equality or retain configured
spelling and compare canonical keys; do not return a different known configured
value and cause Terraform's inconsistent-result errors. Preserve the server's
canonical mailbox name separately for addressing operations. Ambiguous
case-insensitive matches are errors, never evidence of absence.

Identity attributes use explicit `RequiresReplace` modifiers. Replacement is also
required for unsupported edits, including forwarder destinations and pointer
mode. Mutable fields must not inherit blanket `UseStateForUnknown` behavior.
Destroy/replacement always remains subject to the live-account policy. Catch-all
and future non-deletable settings relinquish management only; they have no API
delete/reset. No cascading deletion of unmanaged children is permitted.

## Values and state evolution

Use Framework typed values throughout Terraform models. Required identifies
necessary desired configuration; Optional means omission is meaningful;
Computed is read-only information. Optional+Computed is reserved for a documented
server default that can be read back. Do not invent defaults for sparse responses.
Send only explicitly changed/requested PATCH fields. Preserve zero (including
quota zero/unlimited), empty strings, null and unknown as distinct values until
an operation's documented semantics deliberately convert them.

Use typed single nested objects for structured records, maps for keyed values,
sets for unordered unique collections such as forwarding destinations and spam
entries, and lists only where order is meaningful. Write-only values cannot be
members of sets. Response null and absent properties require presence-aware
client models; neither automatically becomes a zero value or an empty collection.
Usage counters and server status are computed; they must not become desired
configuration or trigger writes. Transport errors and incomplete reads preserve
prior state and return errors.

Every resource starts with explicit schema version `0`, implements
`ResourceWithUpgradeState`, and returns an empty upgrade map initially. When
changing persisted schemas, increment the version and implement/test migrations
from each supported prior schema with sanitized state fixtures. Never add a
fictional no-op migration before a prior schema exists. Test fixtures exercise
the initial hook; production resource migrations arrive with those resources.

## Password creation, adoption and rotation

Both mailbox and reseller-user resources use shared `PasswordAttributes`:
optional sensitive **write-only** `password_wo`, and optional persisted non-secret
`password_wo_version`. There is no ordinary password attribute or saved hash.
The [Framework write-only contract](https://developer.hashicorp.com/terraform/plugin/framework/resources/write-only-arguments)
requires reading the secret from configuration; plan and state values are null.

1. At creation/replacement require a password and positive version. Unknown
   values may pass planning but must resolve before a write. Missing secrets fail
   before any mutation. Use mailbox bounds 8–72 characters, and reseller minimum
   8 with no invented maximum. Server strength requirements remain server checks.
2. On update send a password only when its version increases (or is first set
   on an imported object). An unchanged version ignores a supplied secret.
   A changed secret alone produces no Terraform diff. Versions cannot decrease
   or be removed once recorded.
3. Omitting a password while retaining the version preserves the remote
   credential. Unrelated quota/routing updates never send a password.
4. Import accepts no password and no rotation version. A no-change plan must
   succeed after import. Adding a password/version later is an explicit rotation.
5. Validate at planning and again at apply with `PasswordChange`; never read a
   password from plan/state, log it, put it in private state, or synthesize a hash.

The offline tests use an ephemeral sensitive variable populated with an in-memory
generated password, inspect decoded plans/state, and inspect saved plan ZIP
members and raw state files for generated secrets. Normal non-ephemeral input
variables or literal configuration may still be saved in a plan's configuration
section even when the resource argument is write-only. Use ephemeral values for
password delivery; write-only is not a substitute for secure input handling.

## Development and release boundary

`make check build` runs formatting, Go static analysis, race-tested unit/offline
Terraform tests, the Phase 0 inventory checks and reproducible docs generation.
Install pinned Python tooling first. `make vuln` consults the public Go
vulnerability database. Tool/module downloads are development dependencies,
not authenticated MXroute requests. CI covers both selected Go lines and the
minimum/current Terraform versions. Tests override inherited credentials and
disable debug logs; their client factory requires an explicit numeric loopback
origin and never uses the production client constructor.

`make snapshot` uses pinned GoReleaser for unsigned local cross-platform builds
outside Git. The protocol-v6 manifest is prepared, but publication is disabled;
signing, Registry registration and releases remain Phase 8. Keep the repository
private and preserve its license. Runtime API compatibility stays live-unverified.


## Phase 4 concrete mailbox and routing schemas

Mailbox and forwarder identity inputs preserve configured local-part casing.
Separate computed canonical fields preserve server spelling; case-only edits do
not replace objects. Account-relative IDs use normalized domain/local-part import
syntax. Optional/computed mailbox quota and limit adopt server values on omission
and never become accidental zeros on a password-only PATCH. Mailbox usage and
suspension are read-only. Passwords use the write-only/version convention above.
Catch-all address is optional and noncomputed: required in address mode, omitted
in fail/blackhole modes, and null after a non-address refresh. Destroying this
singleton preserves remote settings. See [Phase 4](phase4.md) for tests and limits.

## Phase 6 concrete reseller schemas

Reseller packages and users are account-relative to a plain reseller provider;
their IDs are the exact package name and username. Both reject impersonation
aliases at plan time and before any request. Package and username lookup is
exact: a case-only variant is neither adopted nor treated as absence, and blocks
creation. Package limits and user quota are strings in the API's write form, a
canonical number or `unlimited`, so zero and unlimited remain distinct and
readback compares exactly. Units appear in the names: `quota_gb` for packages,
`quota_mb` for users. Optional/computed limits adopt server values on omission
and are sent only when configured and different from a fresh read. User
`package` and `suspended` update in place through their dedicated endpoints;
`username` and the creation-only contact `email` replace, with the server value
exposed separately as `current_email`. Reseller-user passwords use the
write-only/version convention with an 8-character minimum and no invented
maximum. See [Phase 6](phase6.md) for ordering, recovery and tests.
