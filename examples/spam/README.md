# Shared spam controls and hourly usage

This credential-free preview is mock-tested and live-unverified. The example
assumes two already owned domains, `example.com` and `example.org`. Use the local
provider build/development override described in the repository README. Provider
credentials come from runtime environment variables; never save real identities,
credentials, inventories, plans or state in this checkout. Do not apply this
example to a real account without the separate review required by the plan.

## Ownership and access context

Whitelist and blacklist entries belong to the effective account across its
domains. `access_domain` is an API access context, not an independent owner. Keep
one resource per account/list/entry, including across aliases and independent
states. There is no authoritative whole-list resource. Terraform only removes
entries it manages and preserves unrelated entries. Existing entries fail create
with an import instruction, including differently cased duplicates.

Score saves also affect shared account scoring. Use **one** `mxroute_spam_settings`
resource per effective account. Its ID is `spam-settings`, independent of the
selected domain. This is conservative ownership based on
[MXroute's documented shared effects](https://docs.mxroute.com/docs/panel/spam-filters.html);
precise propagation and identical readback through different domains remain
live-unverified. Scores are integers from 1 through 50; lower is stricter. Only
`high_score` enters PATCH. Existing upstream delivery policy determines how scored
mail is handled; this provider does not expose unsupported policy fields.

Changing `access_domain` within the same account updates in place. Changing entry
casing does not recreate or modify it. Other entry changes replace that entry.
Changing the provider's effective account requires deliberate state migration and
import; it is not an access-domain change. Independent Terraform states/processes
cannot detect competing ownership. In-process locks serialize preflight, mutation
and readback by effective account, including impersonated aliases. Separate
processes cannot share these locks or rate budgets; coordinate runs, use conservative
parallelism and respect server throttling.

Use `*@example.net` for sender-domain matching. The API character contract allows
letters, numbers, `@ . - _ * + =`, from 1 to 254 ASCII characters. MXroute advises
using a complete domain after `@`; a wildcard there is not a reliable SpamAssassin
pattern. Entries are compared case-insensitively, preserving wire spelling on
reads and deletion. Let the provider encode wildcards and other path bytes once;
configuration and import IDs use raw, unencoded text. Lists are sets; per-email
usage is an ordered list.

## Adopt existing configuration

Configure resource blocks matching the existing values first, then import:

```sh
terraform import mxroute_spam_settings.account example.com
terraform import mxroute_spam_whitelist_entry.trusted 'example.com/*@example.net'
terraform import mxroute_spam_blacklist_entry.blocked 'example.org/blocked+tag=test@example.net'
terraform plan
```

Entry import syntax includes the access domain to make the first read possible;
the persisted ID is only the lowercase entry. No username, server or credential
is part of an ID. Import itself does not modify upstream configuration. Review
for a no-change plan before adopting production state.

Destroying spam settings only relinquishes Terraform management and **preserves
remote filtering**. No reset or delete endpoint exists. Destroying an entry
resource **deletes that exact shared entry** across the account's domains.
The account-protection policy still applies to all live destroy/replacement plans.

## Readback and recovery

Only successful complete lists can prove an entry present or absent. A missing
access domain, failed account verification, malformed list, case-ambiguous entries
or unavailable score causes an error and preserves state. Restore a usable access
context before continuing; do not interpret a context failure as a deleted entry.

Writes preflight and verify their intended value. After an uncertain response,
readback that exactly matches the requested state can confirm completion. Only a
503 busy/locking response with successful mismatching readback gets bounded
backoff/retry (at most three saves). A failed readback blocks retry; other uncertain
writes are not replayed. The operation deadline and cancellation also bound waits.
Acknowledged-but-unconfirmed creates retain their identity for recovery; Terraform
may taint an errored create. Inspect/import/reconcile before accepting replacement,
because entry replacement would delete shared configuration.

## Reporting units and freshness

`mxroute_quota` exposes username, `total_used` and `total_limit` in **bytes**, API
`percent_used`, all five category values in bytes, nullable `grace_period` with
`days_remaining` and `deadline`, and the exact upstream `updated_at` date-time.
A zero total limit means unlimited. A null grace period remains null and clears
previous grace information; unavailable fields are errors, never invented zeros.

`mxroute_email_usage.accounts` preserves the API's largest-first order, with
`email_address`, integer `size_bytes` and exact `updated_at` for each account.
Empty accounts remain an empty list. These counters differ from mailbox resource
quota/usage fields, which use MB. No byte/MB/GB conversion is implicit.

Both reporting endpoints refresh **hourly** upstream. A Terraform refresh does not
force an upstream recount. Counters are computed information and never generate
configuration writes. Do not treat these snapshots as real-time enforcement or
feed volatile usage into desired mail quotas unless that behavior is deliberate.
