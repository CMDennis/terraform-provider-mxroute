# Phase 2 API client and offline validation

Implemented October 19, 2026. All 43 pinned operations now have typed client
methods and named request/response tests. The provider continues to expose zero
production resources and data sources; those lifecycles belong to Phases 3–6.
No authenticated MXroute requests were made. Mock results do not verify live API
compatibility or resolve the contract questions in [api-contracts.md](api-contracts.md).

## Client contract

`internal/client` separates transport, models, operation methods, response
validation, errors, coordination and lookup helpers. It has no Terraform imports
and uses no additional module dependencies. The [operation inventory](operation-inventory.md)
records client coverage independently of future Terraform and reference-doc work.

- Wrapped reads require `success: true`, correctly typed `data` and the fields
  needed to describe the object. `/quota` and `/quota/email` decode raw objects.
  Reseller list endpoints return names, with separate detail methods for objects.
  The domain create response has its own smaller, documented model.
- Response scalar pointers preserve false/zero. Required state fields cannot be
  omitted or null. Published nullable fields must be present but may be null;
  omission fails refresh rather than clearing state. Descriptions are optional.
  Empty arrays are valid complete lists; null/malformed lists and null elements
  fail. Unknown additive fields are tolerated. JSON responses require an
  application/json or structured +json media type and are bounded to 4 MiB.
  These completeness checks are defensive client rules; OpenAPI does not mark
  every response property as required.
- Sparse mutations accept their documented status without inventing a result
  object; empty 204s skip decoding. Nonempty sparse content is inspected for
  `success: false`, including when error details have an unexpected shape.
  Other undocumented sparse content supplies no lifecycle state. Callers must
  read back successful sparse mutations when they need refreshed state.
- Optional request pointers preserve omission versus explicit zero, false and
  empty strings. Server defaults remain server defaults. Passwords are generated
  only at test runtime, omitted on unchanged updates, and never logged. Creation
  and explicit password updates validate documented character-length bounds and
  reject invalid UTF-8, consistent with the shared schema helper. Reseller
  creation uses its own stricter username rules rather than authentication rules.
- Every dynamic path segment is escaped exactly once, including wildcard spam
  patterns, addresses, slashes, percent signs and query/fragment characters.

The blacklist GET response remains a **synthetic whitelist-shaped assumption**:
its published success response has no schema. It requires a successful envelope
and a complete string array and rejects other shapes. This remains live-unverified
under C06/C10, alongside spam-score scope, package/quota interactions and other
existing blockers. The client does not claim those feature designs are stable.

## Errors and uncertain outcomes

Typed errors retain only the operation method name, HTTP status, allowlisted API
code and a safe category/advice. Statuses 401/403/404/409/422/429/500/503, business
errors, malformed responses, transport failures and ambiguous lookups are covered.
API messages, field values, URLs, bodies, credentials and low-level transport
error text are never retained in diagnostics. Wrapped context errors remain
available to `errors.Is` for cancellation/deadline handling.

`IsNotFound` is true only for absence proven by a successfully decoded complete
list. A detail 404, spam domain-context 404, authentication failure, invalid JSON
or upstream error never authorizes state removal. Domain, pointer, mailbox and
forwarder lookup helpers retain canonical spelling, reject duplicate case-folded
matches and distinguish a valid empty collection from a failed read.

An ambiguous write is attempted once. Transport failures, incomplete declared
write responses, unexpected successful statuses and upstream failures report
`Uncertain`. If context/time remains, a documented collection/settings read is
attempted, and `ReadbackAvailable` reports whether state could be observed. The
client still returns the write error: existence alone cannot prove ownership,
password delivery, every changed attribute, or side effects. It never infers
permission to adopt/delete an object or repeats a mutation based on readback.
The lifecycle caller must compare the observed object and intended change before
any later retry; passwords and unobservable effects require particular care.
Cancellation before dispatch reports cancellation without an uncertain mutation.

## Timeouts, retries and process coordination

Each typed operation has a two-minute deadline, including limiter waits, setting
locks, retries and uncertain-write readback. Individual HTTP requests have a
30-second timeout. Caller deadlines and cancellation take precedence.

At most three attempts are allowed for safe reads after transport errors or 5xx,
and for mutations clearly rejected with HTTP 429. Other mutations, 2xx failure
envelopes, password changes and destructive requests receive no automatic replay.
Retries use exponential jitter capped at five seconds plus server-directed waits.
`Retry-After` supports seconds and HTTP dates; `X-RateLimit-Reset` supports Unix
seconds. The later applicable hint wins. HTTP 429 without a usable time hint
pauses the shared budget for one minute. Zero hints allow a jittered retry.
Exhausted response headers also block the appropriate shared budget.

The rolling-minute limits are **180 reads and 48 writes**, below the documented
200/60 ceilings. Budgets are shared by API origin, normalized server and credential
owner, including reseller impersonation aliases and different keys owned by the
same account. Reads and writes have separate budgets. Shared spam mutations
serialize by server and effective account, even across plain/impersonated aliases
with different credential owners. Coordination keys are hashes, with no plaintext
account identity or API key stored in the process-wide maps.

These budgets and locks exist only within one provider process. Independent
Terraform processes and machines cannot share them. Avoid simultaneous applies
for the same account; begin with `-parallelism=1` when multiple runs share a
credential owner. Server-directed limits remain authoritative. In-memory setting
locks supplement the API's own locking and do not provide distributed exclusion.

## Authentication and transport

`X-Server`, `X-Username` and the complete `X-API-Key` are sent as configured;
colons are preserved and no legacy key-ID prefix is reconstructed. User-Agent
includes the provider version. TLS verification stays enabled. Redirects are
never followed, including same-origin redirects, so credentials cannot be
forwarded to a redirect target. The client emits no debug/body logs; formatting
clients and password-bearing request models produces redacted text.

The [pinned public API authentication contract](../../api/snapshots/openapi-2026-10-19.yaml)
says account-password access requires DirectAdmin `api_with_password=yes` in
`user.conf`; login keys are unaffected by that setting. Key restrictions must
permit the requested actions, session queries and impersonation as applicable.
Only reseller-owned ordinary users are supported as impersonation targets.
Reseller-only methods reject an impersonated provider locally. A plain username
is required for reseller endpoints; the remote service still verifies its role.

Tests inject an explicit numeric loopback HTTP origin, generate their own dummy
credentials and never fall back to production or inherited account inputs.
Production has a fixed API origin with no configurable endpoint override.

## Verification

Reproduce with the pinned development tools from the README:

```sh
make check build
make vuln
```

The client suite includes a named test for every inventory method, all 33 original
synthetic scenarios, safe read retry and rejected-write body replay, ambiguous
create/delete/password-update readback, response-size/partial-body limits,
malformed JSON/media types, nullable field presence, canonical/ambiguous/absent
lookups, credential redaction, URL encoding, local budgets, alias throttle sharing,
effective-account serialization, and cancellation while requesting or waiting.
All use local mocks; state/plans and development artifacts stay outside Git.

## Exit gate

| Local validation | Result |
| --- | --- |
| Named request/response tests for all 43 inventory operations | Passed |
| Original 33 contract scenarios executed through typed client methods | Passed |
| Full race suite: Go 1.26.9 / Terraform 1.11.1 | Passed |
| Full race suite: Go 1.27.1 / Terraform 1.16.6 | Passed |
| Final client regressions on both Go versions, including streaming cancellation and Unicode password bounds | Passed |
| Formatting, `go vet`, provider build | Passed |
| Pinned inventory/checksum and eight Python regressions | Passed |
| Generated provider documentation check | Unchanged reference |
| govulncheck v1.7.0 | No vulnerabilities found |
| Phase 2 source/documentation review for literal credential assignments | No findings |

The full Terraform suites above exercised the existing provider foundation and
synthetic fixture lifecycles. Final client-only regression additions were rerun
on both Go versions, followed by static analysis, formatting and the provider
build. The dependency lock and production Terraform schema remain unchanged.
Builds, coverage data and Python tooling were kept outside the checkout.

Phase 2 is complete against local mocks. Live contract verification remains
Phase 7 work requiring the approval and account protections in the plan.
