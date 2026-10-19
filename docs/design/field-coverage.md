# Phase 7 field-level API audit

Audited October 19, 2026 against the pinned OpenAPI 3.0.3/API 1.0.0 document.
The current upstream bytes match SHA-256
`25272af35138b6ef4dc79eccd6ea1876fa1b30c97ae56be6dc0ba8a01d6e879d`.
There are no added/removed operations, parameter changes, schema changes,
default changes or nullability changes. The baseline remains 43 operations over
26 paths. The operation count alone is not the field audit: the tables below
cover documented request fields and success-schema leaves, including inline
schemas. `[]` means collection elements; ordinary `success/data` envelopes are
validated by the client rather than exposed as Terraform state.

## Request inputs

| API surface | Every documented input | Terraform mapping / behavior |
| --- | --- | --- |
| Domain create / mail status | `domain`; `enabled` | `mxroute_domain.domain`; `mail_hosting` |
| Pointer create | `pointer`, `alias` | `pointer`, `type` (`alias`/`redirect`); omitted API alias defaults true |
| Mailbox create | `username`, `password`, `quota`, `limit` | `username`, write-only `password_wo`, MB `quota`, daily `limit`; omitted optional fields adopt server values |
| Mailbox update | `password`, `quota`, `limit` | Due rotation and configured changed fields only; no omission-to-zero conversion |
| Forwarder create | `alias`, `destinations[]` | `alias`, unordered `destinations`; changes replace |
| Spam score update | `high_score` | Account-owned `high_score`; bounded 1–50 |
| Spam whitelist/blacklist add | `entry` | Individual `entry`, max 254 documented characters; preserves unrelated entries |
| Catch-all update | `type`, `address` | Same names; address required only for address mode |
| Reseller user create | `username`, `email`, `password`, `package` | Same names except write-only `password_wo` |
| Reseller user update | `quota`, `password` | `quota_mb` (MB string or `unlimited`), due write-only rotation |
| Reseller package assignment | `package` | In-place user `package` change; readback before quota override |
| Reseller package create | `name`, `quota`, `domains`, `email_accounts`, `email_forwarders`, `domain_pointers` | `name`, GB `quota_gb`, same-name limits; omission adopts documented defaults |
| Reseller package update | `quota`, `domains`, `email_accounts`, `email_forwarders`, `domain_pointers` | Same mapping; only configured differing values are sent |

Suspend/unsuspend and delete operations have no body fields. Path selectors
`domain`, `pointer`, `user`, `alias`, `entry`, `username`, `name` map to resource
identities or data-source selectors; wildcard entries are encoded exactly once.
No query pagination parameters are documented. Authentication headers are provider
inputs and remain in memory; no unsupported API key creation endpoint is invented.

## Success outputs

| Response family | Every documented data field | Terraform mapping / limitations |
| --- | --- | --- |
| Verification key | `key`, `record.{type,name,value}`, `description` | Same data-source fields; optional record description is additionally tolerated |
| Domain names | `data[]` strings | `mxroute_domains.domains` set |
| Domain created | `domain`, `ssl_enabled` | Identity retained immediately; detail read establishes full resource state |
| Domain detail | `domain`, `mail_hosting`, `ssl_enabled`, `pointers[]` | Same resource/data-source fields |
| Pointer list | `pointer`, `type`, `target` | Same fields in single/list data sources and resource readback |
| Mailbox list/detail | `username`, `email`, `quota`, `usage`, `limit`, `sent`, `suspended` | Same data-source fields; resource exposes canonical username separately; quota/usage MB, counters and suspension computed |
| Forwarder list | `alias`, `email`, `destinations[]` | Same fields, canonical alias separate in the resource; destination sets |
| Spam settings | `high_score` | Resource/data source `high_score` |
| Sender lists | `data[]` strings | Whitelist/blacklist `entries` sets; individual entry ownership |
| DNS MX | `mx_records[].{priority,hostname,description}` | Same nested set fields; optional description becomes null |
| DNS SPF / DKIM | `spf.{type,name,value}`, nullable `dkim.{type,name,value}` | Same nested fields, exact DNS content; optional descriptions tolerated |
| DNS verification | Nullable `verification.{type,name,value,description}` | Same nested fields; null clears prior state |
| Catch-all | `type`, nullable `address`, `description` | Data source exposes all; resource manages only type/address. Description is server prose, deliberately not a managed resource field. Non-address modes normalize address to null. |
| Account quota | `username`, `total_used`, `total_limit`, `percent_used`, `breakdown.{email,web,databases,backups,other}`, nullable `grace_period.{days_remaining,deadline}`, `updated_at` | All same-name data-source fields; storage bytes, zero unlimited, timestamps preserved; no writes |
| Per-email usage | `username`, `accounts[].{email_address,size_bytes,updated_at}` | All same-name data-source fields; ordered largest-first list, storage bytes |
| Reseller username/package lists | `data[]` strings | `users` / `packages` sets; no per-item detail fanout |
| Reseller user / assignment response | `username`, `email`, `domain`, `package`, `suspended`, `quota.{limit,used,unlimited}` | Detail data source exposes units explicitly; resource uses `current_email`, `quota_mb`, `quota_used_mb`; unlimited flag maps to `unlimited`. Assignment's returned object is not trusted as final state: a fresh detail GET establishes post-assignment values. |
| Reseller package | `name`, `settings.{quota_gb,quota_unlimited,domains,email_accounts,email_forwarders,domain_pointers}` | Same limits with explicit GB; null/unlimited converted to `unlimited`, zero retained distinctly |

Sparse mutation responses and 204 responses expose no invented fields. API error
`success` and `error.{code,message,field}` are inspected for classification;
allowlisted codes and fixed advice are rendered. Raw messages/fields are not
retained or exposed because they may echo secrets or identities. Password-field
rejections yield fixed categories only. Unknown additive success fields are
accepted but have no promised Terraform mapping until inventoried.

## Observed discrepancy and boundaries

Live ordinary-account quota reads use `{success:true,data:{...}}` while the pinned
specification describes raw objects. The client accepts both complete shapes,
validates every required field and nullable value, and rejects missing data,
invalid success markers and `success:false`. Synthetic S034/S035 fixtures and
`TestQuotaReadsAcceptRawAndWrappedButRejectIncompleteEnvelopes` cover the difference;
`TestOfflineUsageReporting` checks both forms through Terraform. No raw response
or real account values were used as fixture content.

This audit concerns published field coverage. A later isolated test verified the
mailbox create and single-field quota/limit/password PATCH requests; it did not
verify all write semantics. See the [mailbox test record](mailbox-live-testing.md).
Live score propagation, pointer creation prerequisites, domain deletion cascades,
system mailbox restrictions and reseller package/override precedence remain
unresolved C01/C03/C04 questions in the contract register. API create-key, arbitrary
DirectAdmin settings, DNS provider writes and mailbox suspension writes remain
outside the documented API scope.
