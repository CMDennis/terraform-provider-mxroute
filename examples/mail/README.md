# Mailboxes and routing

This ordinary-account workflow is mock-tested and live-unverified. Use Terraform
>= 1.11.1, < 2.0 and the local provider development override described in the root
README; the provider is not published. Supply provider credentials through runtime
environment variables. All committed identities here are reserved examples.

Follow the [domain onboarding workflow](../onboarding/README.md) to publish the
verification TXT before creating a domain. For an existing domain, import
`mxroute_domain.mail` using its domain name before applying this example. Its
`prevent_destroy` protects this example's parent from accidental replacement or
deletion. References order mailbox/forwarder creation and cleanup before the
parent; they do not make domain or mailbox deletion reversible. Mailbox deletion
can permanently destroy mail. The API's system-mailbox identity and restrictions
remain unverified; no reserved mailbox is inferred or automatically adopted.

## Password creation and rotation

Supply `TF_VAR_mailbox_password` at runtime using a secret store or ephemeral
input, never a saved variable file or a literal in configuration. Mailbox passwords
must have 8–72 characters and meet the selected server's strength and character
rules. Recognized password-field rejections produce safe advice for strength or
characters without echoing the server's raw message. Unknown rejections remain
sanitized. The exact live error wording has not been verified.

Set `password_wo_version = 1` on creation. To rotate, supply the new ephemeral
password and increase the version. Supplying a different password with the same
version does not rotate it; write-only values cannot themselves produce a diff.
To omit a password on ordinary updates, remove `password_wo` and keep the current
version. Terraform may still request an ephemeral variable if its declaration
remains required; remove that unused declaration or supply it at runtime.

Passwords are read from configuration only and are null in persisted plans and
state. API credentials should also come from runtime environment variables.
Other state fields include real mail identities and account data: keep state,
plans and logs outside the checkout and protect the backend.

The `quota` unit is MB. Explicit `0` means unlimited; omission adopts the server
value (the documented create default is 1024 MB). `limit` is the daily send limit,
with a documented ceiling/default of 9600. Other numeric bounds are left to the
server. Omission preserves the current remote quota/limit on updates. Password-only
updates send neither quota nor limit. Usage (MB), daily sent count and suspension
are computed and never sent as desired configuration. The API exposes no mailbox
suspension write operation.

Local parts are resolved case-insensitively and ambiguous matches fail before
mutation. `canonical_username` records the exact server name, used for API writes;
configured `username` preserves the spelling in configuration. Changing only that
spelling is a local state update, with no mailbox PATCH or replacement. A create
that finds an existing case variant requires import, rather than adoption or a
second POST. IDs use `domain/lowercase-local-part` within the configured account.

## Forwarding and catch-all

Forwarder destinations are unordered sets. Use email addresses or exactly one
special target: `:blackhole:` silently discards and `:fail:` rejects. Mixing a
special target with other destinations is unsupported because combination
behavior is unverified. Destination changes require destroy-before-create
replacement: routing is interrupted, and a failed new create leaves the old alias
absent. Review the error and apply again to create the desired alias. Unique aliases
do not support create-before-destroy. If deletion fails, the provider preserves
the existing alias and does not attempt the new create. Unrelated aliases remain
untouched.

The API documents that forwarding to Yahoo, AOL, Gmail and similar providers
automatically enables Expert Spam Filtering for the domain. This resource does
not manage or reverse that side effect; there is no supported field for it.

Catch-all types are `fail`, `blackhole`, and `address`. The last requires a single
email `address`; omit `address` for the other modes. Mode changes clear stale
addresses from Terraform state even if the server still returns a stale address.
The non-address request sends only its type, according to the documented API.

**Removing or destroying `mxroute_catch_all` only relinquishes management and
preserves remote routing.** It does not reset the setting or stop catch-all
forwarding. To change delivery, explicitly apply the desired type before removing
management. Use one Terraform owner per domain singleton.

## Adopt existing configuration

First configure the observed quota/limit, forwarder destination set and catch-all
mode/address. For imported mailboxes, omit both password fields until an explicit
rotation. Import makes no write requests:

```sh
terraform import mxroute_domain.mail example.com
terraform import mxroute_email_account.support example.com/support
terraform import mxroute_forwarder.sales example.com/sales
terraform import mxroute_catch_all.routing example.com
terraform plan
```

Confirm the next plan has no changes before adopting the configuration. Import IDs
normalize domain and local-part casing. Resource and data source identities are
relative to the provider's effective account; choose the correct alias first.
The single/list data sources read mailboxes and forwarders; `mxroute_catch_all`
reads the existing singleton. Mailbox data sources never return passwords.

An unconfirmed write is never blindly retried or silently adopted. Reconcile its
outcome before applying again, and import only after reviewing ownership. A
confirmed create saves its identity before readback; a readback failure can still
taint it in Terraform. Review the object and tainted state before any proposed
replacement, since mailbox replacement can destroy mail. A confirmed mailbox
PATCH records its completed rotation version before readback so a failed read does
not repeat the password change. Future authenticated tests and their exact cleanup
still require the separate approval in the plan.
