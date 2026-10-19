# Phase 8 — Release preparation and maintenance

Date: October 19, 2026. Local release preparation is implemented and checked.
**Public release and the Registry installation exit gate remain pending the
owner's manual settings, signing identity and publication.** The proposed first
version is `0.1.0-beta.1`; Phase 7 live gaps still prevent a fully live-validated
stable-release claim. No additional real-account requests were made in Phase 8.

Before the initial code push, the owner renamed and made the repository public
and configured the expected release-secret/variable names. Their values and
actual release execution remain unverified until the protected workflow runs.
The final publication review also replaced a credential-shaped public API
documentation example with a placeholder, retained the original upstream digest,
and extended the generic privacy check to reject such tokens.

## Delivered

- Canonical future Go module `github.com/CMDennis/terraform-provider-mxroute`,
  unchanged provider source `cmdennis/mxroute`, MIT attribution preserved.
- GoReleaser v2.17.0 packaging for Linux/macOS/Windows × amd64/arm64, versioned
  binaries, separate versioned protocol-v6 manifest, checksums covering all six
  ZIPs plus manifest, and detached binary GPG checksum signatures. Local snapshots
  are unsigned `-dev` builds and cannot upload by default.
- A manual beta-only draft workflow: main-ancestry/repository checks, offline
  validation, protected environment approval, private privacy rules before signing,
  full fingerprint comparison, asset/signature validation, packaged mock lifecycle,
  then draft-only upload. No existing release/tag/asset is overwritten. Workflow
  action dependencies are pinned to immutable official commit hashes.
- Release verifier rejects missing architectures/manifest coverage, checksum
  tampering, unexpected archive members, invalid manifest, unsigned non-development
  versions and signatures from a different trusted fingerprint. Signature tests
  generate an ephemeral RSA fixture identity outside Git.
- Actual production-package installation and schema testing using a clean
  filesystem mirror without development overrides. On Linux a temporary TLS
  CONNECT proxy accepts only the API hostname and forwards exclusively to a fixed
  loopback mock. Runtime-generated secrets, private temp state/plans, strict
  no-change plans and exact mutation counters cover create/update/password rotation,
  import and delete. The production binary has no test endpoint override.
- Owner-only private account fingerprints outside Git, commit/index/message/push
  checks, required trusted release scans of history and ZIPs, generic PR checks,
  and explicit limits/remediation guidance. No plaintext account identity/key was
  added to the source checkout. Local hooks are enabled for this checkout.
- Weekly dependency PRs, pinned compatibility/security checks, unauthenticated
  public API checksum drift detection, sanitized issue/PR templates, changelog,
  security/support/contribution policies and full user/publisher guides.

## Local verification evidence

| Check | Result |
| --- | --- |
| `make check build` — Go 1.26.9 / Terraform 1.11.1 | Passed: race/unit/offline lifecycle tests, vet, formatting, API inventory, Python regressions and generated docs |
| `make check build` — Go 1.27.1 / Terraform 1.16.6 | Passed with the same offline/compatibility checks |
| `make vuln` | Passed, no known vulnerabilities reported |
| GoReleaser configuration | Validated with pinned v2.17.0 |
| `make snapshot` | Passed: all six platform ZIPs, checksum coverage and versioned manifest |
| Native macOS arm64 package | Normal mirror `init` and schema `validate` passed |
| Linux arm64 packaged lifecycle | Passed in HashiCorp Terraform 1.11.1 container with external networking disabled, read-only root filesystem and generated mock identities |
| Signature verifier | Valid signature passed; wrong fingerprint/tampering/missing assets rejected with a disposable RSA fixture |
| Complete signed packaging dry run | A clean disposable repository/tag and RSA key produced a real beta's six archives, signed checksums and manifest; trusted-key verification passed with uploads disabled |
| GitHub workflow syntax/expressions | Actionlint v1.7.7 passed |
| Public API drift check | Current unauthenticated specification matches the pinned baseline; no file was replaced |
| Account-aware source/history/archive privacy scan | Passed; findings/values were not printed and nothing was committed/pushed |

macOS/Windows use native trust stores, so the test verifies installation/schema
there without changing trust settings; full isolated-CA lifecycle is required on
Linux. The release workflow tests the exact signed Linux amd64 package as well
as the unsigned validation snapshot. Builds for other architectures prove
packaging/compilation, not native lifecycle coverage on every platform.

## Pending owner/publication gates

The original signed `v0.1.0-beta.1` draft contains all nine validated assets, but a
separate published entry for that tag has no assets and is immutable. Recovery
uses `v0.1.0-beta.2` from the same green provider commit; no published version or
tag is overwritten. The owner must approve signing and publish the complete
workflow-created recovery draft before retrying Registry publication. The main
branch installation/publishing guides now use the recovery version.

Follow [the exact manual checklist](publishing.md): rename the repository while
private, privately commit/push the reviewed implementation, configure required
checks/tag protection/security reporting, create a protected release environment,
create/back up a dedicated RSA signing identity and supply only release secrets
plus private hash rules, then make the audited repository public when ready.
Register the public key/provider in the Registry, create a reviewed beta tag,
approve the draft job, inspect/publish the draft and confirm Registry ingestion
and a clean exact-version Registry installation. None of those external changes
was performed automatically. Actual GitHub job execution and public Registry
signature/installation remain unverified until that sequence is completed.

No earlier public provider version exists, so no release-to-release upgrade is
claimed. Future changes require migration/state-upgrade tests and preserved
versioned artifacts. Live test scope and unverified behavior remain documented
in [Phase 7](phase7.md); publication does not authorize new live account operations.
