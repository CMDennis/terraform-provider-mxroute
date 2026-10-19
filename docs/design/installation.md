# Installation, first use and upgrades

The first beta is prepared but **not published**. Registry installation below
becomes available only after the owner completes [publishing](publishing.md).
The provider address is `registry.terraform.io/cmdennis/mxroute`, protocol v6.
Supported packages are Linux, macOS and Windows on amd64 and arm64. The tested
Terraform minimum is 1.11.1; 1.11.0 fails write-only password persistence checks.
Use Terraform >= 1.11.1 and < 2.0; the current tested pin is 1.16.6.

## Registry install after publication

Use a new working directory **outside the source checkout** because configurations,
state and plans may contain real account identities. Protect the directory and
backend access. Create `main.tf` with an exact beta constraint:

```hcl
terraform {
  required_version = ">= 1.11.1, < 2.0.0"
  required_providers {
    mxroute = {
      source  = "cmdennis/mxroute"
      version = "0.1.0-beta.1"
    }
  }
}

provider "mxroute" {}
```

Remove any `dev_overrides` or `TF_REATTACH_PROVIDERS` inherited from development.
Run `terraform init -backend=false` and `terraform validate` to verify installation
and schema before supplying credentials. Review the signing identity shown by
Terraform and the lockfile; keep the lockfile with sanitized user configurations.
The source checkout intentionally ignores generated local lockfiles.

Beta versions require an explicit prerelease constraint; pin the exact version.
Do not claim a local development binary or mirror is a Registry download. See
[HashiCorp's provider publishing rules](https://developer.hashicorp.com/terraform/registry/providers/publishing)
for Registry version and signature behavior.

## Configure credentials safely

The provider reads `MXROUTE_SERVER`, `MXROUTE_USERNAME` and `MXROUTE_API_KEY` from
the process environment. Set them through a secret manager or an interactive
shell prompt; never put their literal values in a Terraform file, shell history,
repository `.env`/variable file, CI log or issue. Provider aliases support explicit
configuration when needed; the [alias example](../../examples/aliases/provider.tf)
shows an ephemeral key input. See the [provider reference](../index.md) for all
options and timeout/rate-limit behavior.

`terraform plan` can read the account; run it only when that access is authorized.
Start with [adoption](migration.md) for existing objects. Import then require an
ordinary no-change plan; do not apply a plan just to reconcile an import mismatch.
Use `prevent_destroy` where appropriate, and review every saved plan before apply.
Removal semantics differ by resource: domain/mailbox/user deletion may remove
data; forwarder changes can replace routing; removing catch-all/spam settings
relinquishes management while retaining remote settings. Read each resource guide.

For new authorized objects, choose the relevant
[domain](../../examples/onboarding/README.md), [mail](../../examples/mail/README.md),
[spam](../../examples/spam/README.md) or [reseller](../../examples/reseller/README.md)
example. Replace reserved examples privately in the external working directory.
Passwords use ephemeral `password_wo` plus a monotonically increased
`password_wo_version`; never set a guessed password/version during adoption.
Real state/plans may expose identities even with write-only passwords, so store
them in a protected backend/directory outside this source repository.

## Verify downloaded release assets

Obtain the signing public key and full fingerprint through the separately trusted
Registry signing-key entry or maintainer channel. A key bundled next to a ZIP is
not sufficient evidence of trust. Download the six ZIPs, versioned manifest,
SHA256SUMS and binary `.sig` into a private temporary directory. With `release_dir`,
`public_key_file` and `GPG_FINGERPRINT` set to those paths/trusted fingerprint:

```sh
python3 scripts/verify_release.py --directory "$release_dir" --version 0.1.0-beta.1 \
  --public-key "$public_key_file" --fingerprint "$GPG_FINGERPRINT"
```

The verifier requires exact checksum coverage of all six platform archives and
the versioned protocol manifest, validates ZIP membership and LICENSE, then
checks the detached signature against the expected full primary/signing
fingerprint in an isolated temporary keyring. The `--unsigned-snapshot` option
accepts only `-dev` versions and is for local testing; it is not release verification.

## Offline filesystem mirror or local development

For unpublished development use `make snapshot`, which writes packages under
`/tmp/mxroute-provider-dist`. These unsigned `0.1.0-beta.1-dev` packages are not
production release assets. To test normal installation, place your native ZIP at:

```text
MIRROR/registry.terraform.io/cmdennis/mxroute/terraform-provider-mxroute_VERSION_OS_ARCH.zip
```

Create a Terraform CLI configuration outside Git, replace `/absolute/path/to/mirror`
with the mirror root and select it through `TF_CLI_CONFIG_FILE`:

```hcl
provider_installation {
  filesystem_mirror {
    path    = "/absolute/path/to/mirror"
    include = ["registry.terraform.io/cmdennis/mxroute"]
  }
}
disable_checkpoint = true
```

Pin the exact mirrored version in `required_providers` and run `terraform init`.
The absence of `direct` intentionally prevents Registry fallback. Mirror installs
calculate local checksums and do not independently establish publisher identity;
verify signed release assets separately. HashiCorp documents mirror and
development-override behavior in [CLI configuration](https://developer.hashicorp.com/terraform/cli/config/config-file).

For faster source development, `make build` writes a binary outside Git at
`/tmp/mxroute-provider-build`. A CLI `dev_overrides` entry may point to that
directory; it bypasses version/checksum selection and cannot prove release
installation. Remove it before testing Registry or mirror packages. There is no
production API endpoint override. The packaged Linux smoke test uses a temporary
TLS proxy that connects only to a fixed loopback mock, with no real credentials.

## Upgrade and recovery

Back up state securely, read CHANGELOG.md and migration notes, change the exact
version constraint, then run `terraform init -upgrade` in the external working
directory. Review the lockfile signature/checksum changes and save/review an
ordinary plan before applying anything. Stop on unexpected replacement, deletion,
secret persistence or perpetual diffs; do not repeatedly apply to suppress drift.

Schema version 0 is the current compatibility baseline; no prior public release
exists to upgrade from. Future state-shape changes need upgrade tests and an
explicit migration. Downgrading the binary cannot reverse a newer state schema
or remote writes. Retain protected backups and follow version-specific recovery
guidance. See [troubleshooting](troubleshooting.md), [support](../../SUPPORT.md)
and [security reporting](../../SECURITY.md) for further help.
