# Adopt existing configuration without recreation

Use Terraform >= 1.11.1, < 2.0 with the provider development override in the
root README. The provider is unpublished. This procedure is mock-tested;
the later isolated-mailbox test additionally verified live import and a no-change
plan for its proven-new mailbox, without adopting any existing object.
See HashiCorp's [import workflow](https://developer.hashicorp.com/terraform/cli/import).

## Prepare a separate working directory

Keep configuration containing real identities, Terraform state, saved plans and
backups outside this repository. Protect the state backend and use locking.
Supply `MXROUTE_SERVER`, `MXROUTE_USERNAME`, and `MXROUTE_API_KEY` through runtime
environment variables from a secret store. Use a plain ordinary account for mail
objects; reseller resources require a plain reseller provider. Select the intended
provider alias on each resource before importing. Never bind one remote object
to multiple addresses, aliases or states.

Read the existing objects using the data sources or panel. Write resource blocks
matching those values. Import adopts state; it does not discover desired
configuration or authorize changes. Add `lifecycle { prevent_destroy = true }`
to important adopted resources. That guard helps while the resource block exists;
removing configuration can still propose destruction.

| Resource address (example) | Import identifier | Configuration needed for a no-change plan |
| --- | --- | --- |
| `mxroute_domain.mail` | `example.com` | `domain`; omit optional `mail_hosting` to adopt it, or match the observed value |
| `mxroute_domain_pointer.alias` | `example.com/alias.example.net` | `domain`, `pointer`, observed `type` (`alias` or `redirect`) |
| `mxroute_email_account.support` | `example.com/support` | `domain`, `username`; omit optional quota/limit to adopt, or match their observed values; omit both password fields |
| `mxroute_forwarder.sales` | `example.com/sales` | `domain`, `alias`, exact observed unordered `destinations` set |
| `mxroute_catch_all.routing` | `example.com` | `domain`, observed `type`, and `address` only for address mode |
| `mxroute_spam_whitelist_entry.sender` | `example.com/*@example.net` | `access_domain`, existing `entry`; one owner across the entire effective account |
| `mxroute_spam_blacklist_entry.sender` | `example.com/blocked@example.net` | Same shared-entry rule, for the blacklist |
| `mxroute_spam_settings.account` | `example.com` | `access_domain`, observed `high_score`; one owner across the account |
| `mxroute_reseller_package.starter` | `starter` | Exact `name`; omit optional limits to adopt, or match current values in the declared units |
| `mxroute_reseller_user.customer` | `customer1` | Exact `username`, current `email`, assigned `package`; omit optional quota/suspension to adopt; omit both password fields |

Domains and domain/local-part import identifiers normalize case. Reseller names
retain exact spelling. A spam entry's stored ID is the lowercase entry alone;
its import ID additionally supplies an owned access domain. The spam settings
stored ID is `spam-settings`, independent of the access domain.

## Import and review

Start with the parent domain and work through pointers, mailboxes and routing.
Import reseller packages before users. The following example adopts a mailbox
without supplying or changing its password:

```hcl
resource "mxroute_email_account" "support" {
  domain   = "example.com"
  username = "support"
  # Optional quota/limit adopt current server values on import.
  # Do not set password_wo or password_wo_version for adoption.
  lifecycle {
    prevent_destroy = true
  }
}
```

```sh
terraform import mxroute_email_account.support example.com/support
terraform plan -input=false -detailed-exitcode -out=adoption.tfplan
terraform show adoption.tfplan
```

For a completed adoption, the ordinary plan must exit **0** with **No changes**.
Exit **2** means a proposed change: inspect it and adjust configuration to match
the existing object. Exit **1** is an error. Do not apply a plan containing a
create, update, delete or replacement to resolve an adoption mismatch. A
refresh-only plan alone does not prove the desired configuration is a no-op.
Saved plans include private identities even when passwords are write-only;
protect them outside Git. No apply is needed after a successful CLI import and
empty ordinary plan.

A declarative `import` block is also supported and is exercised in the offline
mailbox adoption test. Inspect its saved plan before applying: it must show only
the intended import and no configuration changes. CLI import provides the most
direct path when no remote changes are authorized.

## Secrets, units and settings

Passwords cannot be recovered through import. Leave both password fields absent
until an explicit rotation is authorized. Then supply an ephemeral `password_wo`
and set `password_wo_version = 1`; later rotations increase the version. Never
persist the password or invent its current version.

Mailbox quota is MB and `0` means unlimited. Reseller user quota is MB represented
as a string or `unlimited`; package quota is GB. Package mailbox limit `0` means
zero customer mailboxes, distinct from `unlimited`. Unknown or unavailable quota
readback is an error, never an instruction to overwrite it with zero.

Forwarder destination changes replace the alias and interrupt routing. Forwarding
to some large providers can enable Expert Spam Filtering as a server side effect.
Catch-all and spam-score resource removal relinquish management and retain the
remote setting. Sender-entry removal deletes that entry; unrelated entries remain.
Domain, mailbox and reseller-user deletion may permanently remove hosted data.
Never use `terraform destroy` as an adoption check.

See the [mail guide](../../examples/mail/README.md),
[spam guide](../../examples/spam/README.md), and
[reseller guide](../../examples/reseller/README.md) for full examples and recovery
limits. Reseller and impersonation behavior remains entirely live-unverified.
