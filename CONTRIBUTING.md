# Contributing

Use Go 1.26.9, Python 3.10+, Terraform 1.11.1+ and GPG for signature checks.
The CI matrix also tests Go 1.27.1 and Terraform 1.16.6 on Linux and macOS.
Install the pinned Python dependency from `requirements-dev.txt` in a virtual
environment. Tests use only synthetic accounts, runtime-generated secrets and
loopback services. Never supply actual credentials to ordinary tests or CI.

```sh
make check build
make vuln
make snapshot
```

`make snapshot` creates unsigned development archives outside the checkout and
verifies their layout, checksum coverage and protocol manifest. For a real native
ZIP path from that output, set `MXROUTE_RELEASE_ARCHIVE` and run
`make release-smoke`. On Linux this runs the production binary through create,
update, password rotation, no-op plan, import and delete against a fixed loopback
TLS mock. On macOS/Windows it verifies installation and schema only; Linux is
required for the full isolated-CA lifecycle. No system trust store is modified.

Change schemas with their generated docs (`make docs`), examples, coverage map,
compatibility baseline and migration notes. Avoid changing state shape or remote
ownership without an upgrade plan. [Compatibility and troubleshooting](docs/design/troubleshooting.md)
and [API refresh review](api/README.md) explain the relevant procedures. Pull
requests must state offline/live evidence separately and disclose unverified
runtime assumptions. Dependency PRs need the same checks as other changes.

## Before every commit

Read [AGENTS.md](AGENTS.md) and [the privacy guide](docs/design/privacy.md).
Never put actual credentials, account identities, inventories or Terraform
artifacts in this checkout, even in ignored files. Install the hooks locally:

```sh
git config --local core.hooksPath .githooks
python3 scripts/privacy_check.py --worktree --history --require-private-rules
```

The original maintainer has private hash-only rules outside Git. New authorized
maintainers must configure their own rules or obtain the protected fingerprint
JSON through a private channel. Hooks deliberately block commits when those rules
are missing; do not disable them or add plaintext exceptions. Generic CI checks
run without those private rules on untrusted pull requests. Trusted release jobs
require them and scan both Git history and packaged artifacts.

Review staged content and the final diff yourself as well. Checks are a safeguard,
not proof against every possible encoding or newly introduced personal identity.
If a check reports a finding, it reports locations only: sanitize the working file
and index, rerun the check, and follow SECURITY.md if anything entered history.

## Live testing and releases

Live tests require concrete approval under [plan.md](plan.md); possession of an
API key is insufficient. Never point acceptance tests at a real account.
Follow [publishing](docs/design/publishing.md) for repository settings, key
management, required checks, draft review and Registry publication. No PR job
receives release signing secrets. All published assets and tags are immutable
by project policy; corrections receive a new version.
