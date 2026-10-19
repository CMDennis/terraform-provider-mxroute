# Authorized read-only live checks

The October 19, 2026 request authorized Phase 7 checks on a real ordinary account
while forbidding destructive actions and overwriting existing settings. Its safe
scope is read-only discovery. That original read-only scope authorized no disposable identities, writes or
cleanup. A later explicit request authorized one new mailbox and updates only
to it; see the [isolated mailbox test](mailbox-live-testing.md). Cleanup was separately authorized afterward for only that new mailbox; its
removal and the unchanged original baseline were independently confirmed. Reseller operations remain excluded by the earlier
scope decision. Credentials and account identifiers are deliberately absent here.

## Concrete scope

The opt-in checker `go run ./cmd/mxroute-readcheck --authorized-read-only` takes
credentials only from `MXROUTE_SERVER`, `MXROUTE_USERNAME`, `MXROUTE_API_KEY`.
Run it only after explicit authorization; possession of those variables or the
flag alone is not approval. It is never called by CI, `make check`, or acceptance
tests. It creates no Terraform state, saved plans, configuration, response dumps,
inventory files or manifests. Collection values and fingerprints remain in memory.

It performs two passes over the following ordinary-account GETs:

- Account domain inventory, verification key, quota and email usage.
- Each discovered owned domain's detail, pointers, mailbox list, forwarders,
  catch-all, DNS records, spam score, whitelist and blacklist.
- One existing mailbox detail per domain with a nonempty mailbox list, selected
  deterministically. No per-mailbox fanout is used.

A dedicated transport permits only these documented GET route families at the
fixed HTTPS API origin. It rejects every other HTTP method, requests with bodies,
reseller routes, arbitrary URLs, query strings and redirects. The runner rejects
impersonation, malformed/duplicate domain identities and inventories over twenty
domains before further discovery. Maximum logical request count is 408 for two
passes; the existing rate limiter applies to every attempt and retries remain
bounded. Cancellation is supported with a ten-minute overall deadline. Every
response is validated by the normal typed client, including its 4 MiB size cap.

## Baseline and results

The first pass fingerprints all observed configuration. The second compares the
same configuration and membership, with unordered collections canonicalized and
64-bit integers preserved exactly. Mailbox size/sent counters, hourly quota usage
and usage timestamps are excluded because normal mail activity can change them.
Quota limit is compared; per-email collection membership is checked through the
mailbox list. Passwords and settings not exposed by these API reads cannot be
compared. Two passes cannot detect a change that occurs and reverts between reads.

Output contains fixed operation names, pass/fail categories, HTTP status on error,
completion and baseline-comparison booleans. It never prints inventory identities,
counts, hashes, server messages or response bodies. On an error or unexpected
baseline difference the process exits nonzero. It does not attempt repairs or
cleanup. No test objects exist after a read-only run.

The successful Phase 7 run exercised all fourteen read operations and returned
`completed: true` and `baseline_unchanged: true`. The earlier failed quota-shape
check stopped without writes; the resulting compatibility fix and synthetic
regressions are described in the [field audit](field-coverage.md).

## Future mutation testing

Outside the isolated new-mailbox test, mutations remain live-unverified. Before
any later live lifecycle test, prepare
the exact disposable identities, operations, expected indirect effects, baseline,
separate protected state, ownership manifest, saved-plan review and cleanup for
approval under `plan.md`. A prefix is not proof of ownership. Creating a test
object does not authorize its deletion. Never run the full acceptance harness,
blanket destroy or a sweeper against the existing account. Account-wide spam,
forwarding side effects, deletion cascades and reseller workflows need a suitable
isolated sandbox; passing read checks cannot authorize them.
