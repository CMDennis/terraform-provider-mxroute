# Reseller packages, users and impersonation

This workflow is mock-tested and live-unverified. No reseller account, owned
user or authenticated request was used to develop it. Use Terraform >= 1.11.1,
< 2.0 and the local provider development override described in the root README;
the provider is not published. All committed identities here are invented.

## Two provider identities

Reseller packages and users require a **plain reseller** provider (`username =
"reseller"`). Account and domain resources for an owned ordinary user go
through an impersonation alias (`username = "reseller|customer1"`) that uses the
reseller's own API key. The provider rejects reseller resources and data sources
on an impersonation alias before sending any request; the API also refuses them.
DirectAdmin decides whether a target is owned and impersonable: a non-owned
target fails with an authorization error and no state changes. Only ordinary
user targets are supported. The reseller and every impersonation alias share the
reseller's server-side rate-limit budget.

## Staged applies for new users

Terraform configures providers before it creates resources, and an impersonation
alias must not depend on a user created in the same run. Use a literal alias
username (here `var.customer_username`) and apply in two stages:

1. `terraform apply` with `manage_customer_account = false` creates the package
   and user through the plain reseller provider. The alias is declared but unused.
2. After the user exists, `terraform apply -var manage_customer_account=true`
   reads (and can manage) that user's account through the alias.

A separate root module for the customer's account works equally well and keeps
its state apart. Either way, configuring the alias performs no requests; the
first impersonated request happens only when a resource or data source uses it.

## Packages

Limits are strings so `unlimited` and `0` stay distinct. `quota_gb` is in **GB**
(a decimal such as `"2.5"` is accepted locally; server support is unverified).
`domains`, `email_accounts`, `email_forwarders` and `domain_pointers` are counts.
`email_accounts = "0"` permits no customer mailboxes; the system mailbox does
not consume that allowance. Omitted limits adopt the server value — the
documented creation defaults are 1 GB, 1 domain, 100 mailboxes, 100 forwarders
and 10 pointers — and are never sent on update. Only configured limits that
differ from a fresh read are PATCHed. Values must be canonical (`"5"`, not
`"05"` or `"5.0"`) so readback compares exactly.

Package names preserve case and are compared exactly. A create that finds the
same name, or a case-only variant, fails and asks for import instead of adopting
or duplicating it. Renaming a package replaces it; while users are assigned the
old package, its deletion fails with a business error and **no user is removed**.
Reassign users first, or use `create_before_destroy` on the package. Whether a
package update changes the effective quota of users already on it is
live-unverified.

## Users

New usernames must have 1–10 lowercase letters, digits or underscores. Imported
users may use the 1–16 character authentication syntax. The write-only
`password_wo` (at least 8 characters; server strength rules apply) is required
at creation. To rotate, supply a new ephemeral password and increase
`password_wo_version`; import requires no password. Supply passwords through
`TF_VAR_customer_password` or another ephemeral input, never a saved file.

`quota_mb` is in **MB** or `unlimited`, unlike package quota in GB. When set, it
is an explicit override. When omitted, the effective value is adopted from the
server and never sent. `suspended` dispatches suspend or unsuspend only when the
remote state differs. `package` changes use the reassignment endpoint in place.

An update runs in a fixed order and records each confirmed step in state:

1. unsuspend, if requested;
2. package reassignment, then a fresh read;
3. one PATCH carrying a password rotation and/or a quota override that still
   differs from the fresh read (so an override that reassignment reset is
   re-applied, and one it preserved is not resent);
4. suspend, if requested.

If a later step fails, the next apply repeats only what is still different. It
never recreates the user or repeats a confirmed rotation. Package/override
precedence is live-unverified (C04): if readback does not confirm a requested
package, quota or suspension after successful writes, the apply fails with the
reported values recorded instead of guessing.

The API has no route to change a user's contact `email`. Changing `email` in
configuration therefore plans a **replacement**, which deletes the user and
everything it hosts. Remote email changes are shown in `current_email` and do not
trigger replacement. Username changes also replace. Keep `prevent_destroy` on
users that matter: deletion permanently removes the account, its domains and
mail. Reference packages from users (`package =
mxroute_reseller_package.starter.name`) so Terraform removes users before their
packages; package deletion never cascades.

## Adopt existing users and packages

```sh
terraform import mxroute_reseller_package.starter starter
terraform import mxroute_reseller_user.customer customer1
terraform plan
```

Imports make no write requests. Configure the observed limits, package, quota
and suspension first, omit both password fields until an explicit rotation, and
confirm the next plan has no changes. The list data sources return names only
and make no per-item detail requests.

Reseller surfaces are tested only against local mocks. Live reseller testing is
out of scope for now, so verify behavior on a disposable reseller account before
relying on it in production.
