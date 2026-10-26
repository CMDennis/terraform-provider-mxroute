# Publishing releases

Releases follow the EASYDNS provider's tag-driven process: push a reviewed `v*`
version tag on `main`, and GitHub Actions tests, builds, signs, verifies and
publishes the release automatically. There is no workflow dispatch, signing
approval or manual draft-publication step.

Tags such as `v0.1.0-beta.3` produce pre-releases; tags such as `v1.0.0` produce
stable releases. A stable tag still requires a reviewed readiness decision: the
[Phase 7 live-validation gaps](phase7.md) currently prevent a fully live-validated
stable-release claim. Automation does not change that evidence or authorize live
testing. MIT remains the license, including its existing attribution.

## One-time setup

The repository must be public and named `terraform-provider-mxroute` for public
Registry distribution, with provider source `cmdennis/mxroute`. Its owner has
already renamed it and made it public. Keep `main` and `v*` tags protected against
force pushes, deletion and tag replacement; require the checks from
`.github/workflows/ci.yml` before releasing a reviewed commit. Allow only trusted
release maintainers to create version tags.

The existing `provider-release` GitHub Environment holds the signing material.
To support tag-triggered releases, configure **Deployment branches and tags →
Selected branches and tags → `v*` (tag)**, and remove the old `main` branch rule
and required-reviewer gate. The release workflow independently requires the
canonical repository and verifies that the tagged commit belongs to `main`.
Environment secrets remain isolated from ordinary CI and pull-request jobs.
See [GitHub's environment documentation](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/manage-environments).

Retain these environment secrets and variable:

| Name | Kind | Purpose |
| --- | --- | --- |
| `GPG_PRIVATE_KEY` | Secret | ASCII-armored dedicated release private key |
| `PASSPHRASE` | Secret | Release key passphrase |
| `PRIVACY_FINGERPRINTS` | Secret | Existing private hash-only JSON rules, never plaintext identities |
| `GPG_FINGERPRINT` | Variable | Separately trusted full primary fingerprint |

Use a dedicated protected signing key outside the checkout, with a project-only
user ID, offline backup and revocation certificate. Never use MXroute account
identities for signing or place a private key in the repository. Configure secrets
through GitHub's secret fields or private prompts/pipes. Never configure an MXroute
API key, server or username in Actions. The workflow compares the imported signing
key with the separately configured full fingerprint before building signed assets.

Register the ASCII-armored **public** signing key in the
[Terraform Registry](https://registry.terraform.io/), then choose **Publish →
Provider** and select the canonical repository for `cmdennis/mxroute`. The Registry
uses the public key and release webhook; it never needs private signing material
or MXroute credentials. Follow the
[Registry publishing requirements](https://developer.hashicorp.com/terraform/registry/providers/publishing)
for registration and ingestion errors.

## Release a version

1. Update `CHANGELOG.md`, review compatibility/migrations and live-validation
   limits, and merge the reviewed changes to `main`. Require all CI jobs to pass
   for the exact commit that will be tagged.
2. For a local rehearsal, run `make check build`, `make vuln`, `make snapshot` and
   `make release-smoke` with the native snapshot ZIP in `MXROUTE_RELEASE_ARCHIVE`.
   Linux must pass the complete packaged-binary mock lifecycle. Snapshots never
   publish and all test credentials are generated at runtime.
3. Review source, notes and candidate files for private information. Run the
   private checks before committing or pushing:

   ```sh
   python3 scripts/privacy_check.py --worktree --staged --history --require-private-rules
   python3 scripts/privacy_check.py --artifact-dir /tmp/mxroute-provider-dist --require-private-rules
   ```

4. From a clean checkout of the reviewed commit on `main`, create and push a new
   annotated or signed semantic-version tag. For example, using a beta number
   that has never been used:

   ```sh
   git tag -a v0.1.0-beta.3 -m 'MXroute provider 0.1.0 beta 3'
   git push origin v0.1.0-beta.3
   ```

5. Watch **Actions → Release**. A successful run publishes all nine verified
   assets together and generates release notes from GitHub changes, including a
   link to the validation limits at the release commit. A hyphenated version is
   marked as a pre-release and never becomes the latest stable release.

A tag push is the publication action. There is no need to create a separate
GitHub release entry. Do not reuse a tag or overwrite an existing release,
including a draft. Any failure requiring changed bytes receives a new version.

## Automated checks and assets

The read-only validation job checks repository identity, tag format and `main`
ancestry, then runs offline checks, vulnerability analysis, six-platform snapshot
packaging and the production-package mock lifecycle. Only after validation does
the signing job access the `provider-release` environment. It requires private
privacy rules, scans source/history, imports and checks the signing key, builds
and signs the real version, verifies checksums and the trusted signature, scans
packaged archives, and tests the exact signed Linux package against a loopback
mock. Every account operation remains offline.

GoReleaser uploads remain disabled in its configuration. After every check passes,
the workflow publishes the explicit asset list with `gh release create --verify-tag`.
This avoids publishing before validation or creating an empty
release. Existing releases are never overwritten.

Each release has exactly nine public assets:

- Six `terraform-provider-mxroute_VERSION_OS_ARCH.zip` files for
  `darwin`, `linux`, `windows` × `amd64`, `arm64`.
- `terraform-provider-mxroute_VERSION_manifest.json` declaring protocol v6.
- `terraform-provider-mxroute_VERSION_SHA256SUMS` covering all ZIPs and the exact
  versioned manifest.
- `terraform-provider-mxroute_VERSION_SHA256SUMS.sig`, a detached binary GPG
  signature. Each ZIP contains only its versioned production binary and LICENSE.

Build metadata and unpacked binaries are never uploaded. For independent asset
verification, follow [installation](installation.md).

## Recovery and Registry verification

The original `v0.1.0-beta.1` publication had no provider assets and could not be
installed. `v0.1.0-beta.2` is now published with all nine assets. Preserve existing
tags/releases. Do not create a second release entry for a draft's tag or try to
replace immutable assets. Choose
a fresh version after inspecting existing tags and releases. The older manual
beta-draft workflow is superseded by the tag-driven workflow.

If validation or signing fails, inspect the failed Actions step; no release is
published by those stages. If publication partially fails, inspect the release
and use a fresh version when corrections are required. Never retarget a tag or
replace previously published bytes. Follow SECURITY.md for exposed credentials
or signing material.

After publication, confirm Registry ingestion and all expected platforms. From a
clean directory outside the checkout, with no development override or plugin
cache, follow [installation](installation.md): install the exact version, validate
its schema and verify the lockfile/signature. On Linux, repeat the mock lifecycle
with the downloaded native package. Record the tag, CI result and Registry
installation evidence without private identities. Any authenticated live smoke
test still requires specific prior approval under [plan.md](../../plan.md).

For upgrades, verify old-state compatibility and no-change adoption, document
migrations and retain prior assets. Pinning an older provider does not undo remote
changes and is safe only if it can read the current state.

## Maintenance

The weekly compatibility/public-API scan is disabled. **Actions → Manual
compatibility and public API drift** can be run on demand. Normal
push/pull-request CI and weekly Dependabot update PRs remain configured. Neither
maintenance nor release jobs change an API baseline or access the live account.
