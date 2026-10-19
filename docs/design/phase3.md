# Phase 3 domain onboarding and offline validation

Implemented October 19, 2026. This is a mock-tested alpha, with no authenticated
MXroute requests or live DNS writes. Production API compatibility, DNS propagation
as seen by MXroute, pointer prerequisites, deletion cascades and recovery remain
live-unverified under C03/C06/C10/C12. The repository remains unpublished.

## Terraform surfaces

Two resources are registered: `mxroute_domain` and `mxroute_domain_pointer`.
Both start at schema version 0, expose the initial state-upgrade hook, normalize
imports, require canonical configured DNS names, and use account-relative IDs
without credentials or provider account details. Domain identity changes replace;
pointer parent, name and alias/redirect mode changes replace. There are no new
module dependencies or provider endpoint overrides.

Six data sources expose the verification key and its record, domain lookup/list,
pointer lookup/list, and DNS records. Unordered collections are sets. DNS record
objects retain type, name, exact value and nullable optional description; MX
records retain priority and hostname. DKIM and verification objects explicitly
refresh to null when the API returns null. No DNS writes are implied by discovery.
A domain's SSL flag and pointer names are computed information. Managing pointer
resources separately preserves unrelated pointers and avoids competing ownership.

Domain `mail_hosting` is optional/computed: omission adopts the server value;
explicit true/false drives only the documented mail-status PATCH when needed.
Reads refresh drift rather than preserving stale values. Import uses
`example.com` or `example.com/alias.example.net` and requires no mutation; configure
the observed hosting/mode before confirming a no-change plan.

## Recovery after domain creation

The provider records the stable identity immediately after a successful create
response, before visibility readback or mail-hosting changes. A failed follow-up
step must not cause destructive recreation. Terraform taints resources when
Create returns error diagnostics even if state was saved; offline testing
reproduced that replacement behavior. Consequently, a confirmed domain create
with a failed subsequent step saves `mail_hosting_pending = true` and emits a
warning. Until refresh, `mail_hosting` in this pending state records requested
intent and is explicitly **not proof of the remote value**. The next normal
refreshed plan reads the actual remote value and proposes the necessary hosting
update. The recovery test proves one domain POST across both applies and no
replacement or manual untaint/import. Avoid `-refresh=false` during recovery.

An unconfirmed POST failure is still an error and is never silently adopted or
replayed. After an uncertain write, review the remote object and import if
appropriate; existence alone is not proof that this Terraform run created it.
Updates preserve prior state on API failure, and the next refresh reconciles any
partially applied hosting change. Pointer creation saves its ID before readback;
failed pointer readback returns an error and may require reviewing Terraform's
tainted state before applying a replacement.

## Bounded readiness and absence handling

`verification_wait_seconds` defaults to 0 (disabled), accepts 0–1800 seconds and
opts into polling the system DNS resolver for the account's exact TXT name/value
before creation. Only missing TXT answers and wrong TXT values are retried;
resolver failures stop. Enabling it for a pointer checks the pointer's own DNS
name, independently of the parent. It is an optional preflight, not a claim that
MXroute's unknown pointer prerequisites have been established. The API remains
authoritative and arbitrary create validation errors are never retried. This
avoids inventing an undocumented DNS-verification API error code.

`visibility_wait_seconds` defaults to 30, accepts 0–1800 seconds and bounds
post-create visibility polling. Zero checks once. Only complete successful lists
proving absence are retried; authentication, malformed responses, ambiguous names
and other failures stop. Caller cancellation and deadlines bound both waits.
HTTP deadlines/retry budgets from Phase 2 still apply inside these bounds.

A domain detail 404 triggers a complete domain-list lookup. Only confirmed
absence permits state removal. A pointer read confirms its parent before using
the complete child list; a parent authentication/ambiguous/unavailable response
cannot masquerade as a missing pointer. Even confirmed parent deletion requires a complete successful child list
proving pointer absence before removing pointer state. A parent-context 404
on the child route remains an error; no deletion cascade is assumed. Deletion calls only the requested documented
domain or pointer endpoint; there is no client-side purge of unmanaged children.
References between child and parent order cleanup. Domain deletion can affect
mail; important domains should use `prevent_destroy`.

## Offline evidence

`make check build` passed with Go 1.26.9 and Terraform 1.14.6, including formatting,
static analysis, all packages under the race detector, eight inventory-tool
regressions, all 43 API mappings/33 contract scenarios, reproducible generated
provider/resource/data-source docs and the local provider build. Phase 3 tests
also pass on Terraform 1.11.1 with Go 1.26.9 and Terraform 1.16.6 with Go 1.27.1.

| Scenario | Evidence |
| --- | --- |
| Verification key → simulated external TXT publication → exact TXT check → domain/pointer creation | `TestOfflineOnboardingDNSAndVisibilityWait`; built-in Terraform objects stand in for the external DNS provider, with an injected DNS resolver and delayed HTTP visibility |
| Domain create, repeated no-op, mail-hosting updates, both imports followed by no-op, all six data sources, ordered cleanup | `TestOfflineDomainWorkflow` |
| Failed second-step hosting change, pending state, next-apply recovery without recreation | `TestOfflineDomainPartialCreateRecovery` |
| Hosting/mode drift, pointer deletion, parent deletion and recreation | `TestOfflineDomainDriftAndDeletion` |
| Malformed and forbidden refresh preserves resource identities | `TestOfflineDomainReadFailuresPreserveState` |
| Exact DNS contents and nullable DKIM/verification clearing | `TestOfflineDomainWorkflow`, `TestOfflineDNSNullRefresh` |
| Missing DNS prevents create; unrelated API validation errors are not retried | `TestOfflineFailedDNSPreventsCreation`, `TestOfflineDomainValidationErrorNotRetried` |
| Omitted hosting adopts server value without a PATCH or perpetual diff | `TestOfflineDomainServerDefault` |
| Canonical names, bounded waits, modes, cancellation, unrelated failures and strict absence proof | `TestDomainValidators`, `TestVerificationWait`, `TestReadinessWaitStopsOnUnrelatedErrors`, `TestDomainAbsenceRequiresCompleteList`, `TestPointerDeletionNeverAssumesParentCascade` |

The [onboarding example](../../examples/onboarding/README.md) documents a real
external-DNS configuration, nullable DKIM staging, dependencies, imports,
replacement/deletion consequences and pending-setup recovery. Its Route 53 writes
are illustrative and have not been executed. Generated docs cover every Phase 3
surface and include reserved-domain examples/import commands. Inventory mappings
now distinguish implemented Phase 3 surfaces from later planned surfaces.
Documentation checks compare all generated reference pages, rather than only
the provider index.

The Phase 3 offline exit gate is satisfied. None of these checks authorizes live
testing, release publication, visibility changes or cleanup of real account data.
