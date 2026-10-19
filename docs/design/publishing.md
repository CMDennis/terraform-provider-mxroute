# Publishing and release checklist

Phase 8 prepares a **beta** release. The repository has been renamed and made
public by its owner. An empty `v0.1.0-beta.1` release was published separately
from the signed draft and locked as immutable; it cannot be installed.
The recovery tag is `v0.1.0-beta.2`; the live gaps in
[Phase 7](phase7.md) still block a fully live-validated stable-release claim.
MIT remains the license, including its existing attribution. The repository owner
is responsible for signing, publication and follow-up support.

## One-time manual changes

Perform these in order. Items involving public visibility or release publication
are deliberate owner actions, not side effects of running local tests.

1. **Rename the repository while it is private.** In GitHub repository
   **Settings → General → Repository name**, change `mxroute-terraform` to
   `terraform-provider-mxroute`. The source address remains `cmdennis/mxroute`.
   The Go module/imports are already prepared for the new name. Update the local
   remote after the rename:

   ```sh
   git remote set-url origin https://github.com/CMDennis/terraform-provider-mxroute.git
   ```

2. **Review and commit the implementation.** For the initial push, review the
   complete implementation and its new files before staging. Ensure local
   privacy hooks are enabled, run the full private-history/staged check below,
   review the staged diff and every new file, then commit and push. Never stage
   a whole private-artifact directory. Private state/manifests remain outside Git.
   Stop the push if any finding remains unresolved.

3. **Configure GitHub controls.** Under **Settings → Rules → Rulesets** (or
   branch protection), protect `main` against force pushes/deletion and require
   pull-request review plus the checks from `.github/workflows/ci.yml`, including
   the provider matrix, vulnerability and packaging jobs. Protect `v*` tags
   against updates/deletion and restrict their creation to release maintainers.
   Enable **Security → private vulnerability reporting** and dependency/security
   alerts. Allow the checked-in Actions workflows and Dependabot to run. Enable
   immutable GitHub releases if available; the project forbids asset/tag replacement
   regardless of that setting. Verify these controls again after changing visibility.

4. **Create the release signing identity locally.** Install GnuPG, use a protected
   keyring outside this checkout, and generate a dedicated RSA signing key with a
   project-only user ID. Do not use an MXroute account/email/domain or other test
   identity. Choose a strong passphrase in the prompt and retain an offline backup
   and revocation certificate:

   ```sh
   gpg --quick-generate-key 'MXroute Terraform Provider Release Signing' rsa4096 sign 2y
   gpg --list-secret-keys --keyid-format LONG --fingerprint
   ```

   Record the **full primary fingerprint**, not a short key ID. Key creation and
   expiration syntax are documented in the [GnuPG manual](https://www.gnupg.org/documentation/manuals/gnupg/OpenPGP-Key-Management.html).
   Local signature tests use disposable keys only; they are not a release identity.

5. **Create a protected GitHub Environment named `provider-release`.** Under
   **Settings → Environments**, add a required maintainer reviewer and select
   **Deployment branches and tags → Selected branches and tags → main (branch)**.
   This workflow is dispatched from `main`; checking out a tag does not change
   that execution ref. The workflow separately validates that the supplied beta
   tag is on `main`, and the tag ruleset protects its identity. Prevent other
   branches from using the environment. If your plan/visibility does not support
   required reviewers, keep signing disabled until those protections are available.
   A single-owner project needs a reviewer arrangement that permits the intended
   manual approval; do not enable a self-review ban without another reviewer.
   Add these **environment** secrets/variable:

   | Name | Kind | Value supplied by you |
   | --- | --- | --- |
   | `GPG_PRIVATE_KEY` | Secret | ASCII-armored export of only the dedicated release private key |
   | `PASSPHRASE` | Secret | The key's passphrase |
   | `PRIVACY_FINGERPRINTS` | Secret | Existing private hash-only JSON rules, never plaintext identities |
   | `GPG_FINGERPRINT` | Variable | Full trusted primary fingerprint, compared with the imported key |

   Use GitHub's secret fields or pipe directly with the GitHub CLI; never export a
   private key into the repository or paste it into chat, issues or workflow YAML.
   With `GPG_FINGERPRINT` set to your chosen full fingerprint in your shell:

   ```sh
   gpg --armor --export-secret-keys "$GPG_FINGERPRINT" |
     gh secret set GPG_PRIVATE_KEY --env provider-release
   gh secret set PASSPHRASE --env provider-release
   gh secret set PRIVACY_FINGERPRINTS --env provider-release < "$HOME/.config/mxroute-terraform/privacy-fingerprints.json"
   gh variable set GPG_FINGERPRINT --env provider-release --body "$GPG_FINGERPRINT"
   ```

   `gh secret set PASSPHRASE` prompts privately. Do not put the passphrase in a
   command argument. Never configure an MXroute API key, server or username in
   GitHub Actions. Environment protection is the credential boundary; see
   [GitHub's environment documentation](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/manage-environments).

6. **Make the audited repository public when ready to publish.** In
   **Settings → General → Danger Zone → Change repository visibility**, select
   Public only after the privacy checks, private commit review and CI pass.
   Public Registry distribution requires this visibility and the canonical name.
   If you prefer to keep it private, use a private registry or filesystem mirror
   and do not proceed with the public Registry steps.

7. **Register the public signing key and provider.** Sign in to the
   [Terraform Registry](https://registry.terraform.io/) with the GitHub identity
   authorized for the repository. In user settings add the ASCII-armored **public**
   signing key (export with `gpg --armor --export "$GPG_FINGERPRINT"`). Choose
   **Publish → Provider** and select `terraform-provider-mxroute` for the
   `cmdennis/mxroute` namespace. Review the requested GitHub authorization and
   Registry webhook. Registration may need the first published release before
   validation completes; finish step 8 and retry if needed. Follow the
   [official publishing requirements](https://developer.hashicorp.com/terraform/registry/providers/publishing)
   if the UI changes. The Registry never needs the private key or MXroute credentials.

8. **Create a reviewed beta tag, prepare its draft, then publish manually.** Use
   the per-release checklist below. After the draft and nine assets are verified,
   publish the draft in GitHub Releases, retain its pre-release designation and
   confirm successful ingestion in the Registry. A signed Git tag is separate
   from the required binary signature on the release checksums.

## Before each tag

- Review `CHANGELOG.md`, [release notes](release-notes-beta.md), known live gaps,
  dependency changes, MIT attribution and version/upgrade compatibility. The
  checked-in workflow permits beta tags only. Stable support requires a separate
  reviewed readiness decision and corresponding workflow/docs changes.
- Run `make check build`, `make vuln`, `make snapshot`, and native
  `make release-smoke` with the package path in `MXROUTE_RELEASE_ARCHIVE`.
  Linux must pass the full packaged-binary mock lifecycle. Require the complete
  CI matrix and packaging check for the exact commit to be tagged.
- Run the required private checks from the project root:

  ```sh
  python3 scripts/privacy_check.py --worktree --staged --history --require-private-rules
  python3 scripts/privacy_check.py --artifact-dir /tmp/mxroute-provider-dist --require-private-rules
  ```

- Inspect the final staged diff and all untracked candidate files. Verify no
  credentials, real account details, inventories or private runtime artifacts
  appear in Git, including commit/tag messages and refs. Hooks must be active.
- Commit reviewed source and notes. Ensure the release commit is clean and on
  `main`. Create an annotated or signed new tag; for the first beta:

  ```sh
  git tag -a v0.1.0-beta.2 -m 'MXroute provider 0.1.0 beta 2'
  git push origin v0.1.0-beta.2
  ```

  Do not reuse a tag or overwrite an existing release, including a draft. Fixes
  receive a new beta number. Run the private history check again before pushing.

## Draft workflow and artifact review

In **Actions → Prepare beta release draft → Run workflow**, select `main` and
enter the existing tag. The read-only job checks canonical repository, beta tag
format and main ancestry, then runs offline checks and packaged mock smoke.
The `provider-release` environment pauses the signing job for manual review.
After approval it requires private fingerprint rules, imports the dedicated key,
compares the full fingerprint, builds six packages, signs and verifies checksums,
scans history/archives and tests the exact signed Linux package. It creates a
**draft pre-release** only. Existing releases are never overwritten. Local
GoReleaser uploads are disabled by default; the workflow builds with
`--skip=publish` and explicitly creates the reviewed draft after verification.

For version `0.1.0-beta.2`, the nine public assets are:

- Six `terraform-provider-mxroute_0.1.0-beta.2_OS_ARCH.zip` files, for
  `darwin`, `linux`, `windows` × `amd64`, `arm64`.
- `terraform-provider-mxroute_0.1.0-beta.2_manifest.json` (protocol v6).
- `terraform-provider-mxroute_0.1.0-beta.2_SHA256SUMS` covering all six ZIPs and
  that exact versioned manifest.
- `terraform-provider-mxroute_0.1.0-beta.2_SHA256SUMS.sig`, a detached binary GPG
  signature. Every ZIP contains only its versioned production binary and LICENSE.

Never upload the whole distribution directory: build metadata, unpacked binaries
and tool configuration are not release assets. Compare the draft notes and asset
names, verify signatures against the separately registered fingerprint, and use
`scripts/verify_release.py` as shown in [installation](installation.md). Publish
the draft manually only after those checks pass. Watch Registry ingestion errors
and GitHub Actions; keep a failed or unreviewed draft unpublished.

## Publish the existing signed draft

Wait for the complete release workflow to succeed, then open its draft in GitHub
Releases. Edit that existing draft. Verify the tag, six platform ZIPs, versioned
manifest, SHA256SUMS and binary signature before clicking **Publish release**.
Keep the pre-release designation. Creating a separate release entry for the same
tag does not copy assets from the draft; it can publish an empty release instead.

If the Registry reports missing SHASUMS, signature and platforms, compare the
published release's attached assets with the signed draft. Source-code archives
automatically generated by GitHub do not contain the built provider packages.
An immutable published release cannot have assets added or replaced. Prepare a
new beta version, verify and publish its complete draft, and retry Registry
publication or resync against that version. Preserve the earlier tag and release.

## After publication and upgrades

From a clean directory outside the checkout, with no development override/plugin
cache, follow [installation](installation.md) and confirm an exact-version
`terraform init`, schema validation and verified lockfile. Registry installation
must be tested after actual publication; a local mirror alone cannot prove Registry
ingestion or its signature metadata. Re-run the mock lifecycle with the downloaded
native package on Linux. Any live smoke test needs fresh concrete approval under
the Phase 7 policy; ordinary publication does not authorize account changes.

Update the release record with actual tag, CI result, fingerprint and Registry
installation evidence, using no private account identifiers. For subsequent
versions test old-state upgrades and no-op adoption before release, retain prior
assets and document migrations. To roll back, pin the prior provider version only
if its schema can read the current state; restore a protected state backup when
the migration guide calls for it. Version pinning does not undo remote changes.

If a release fails before publication, leave the draft unpublished and diagnose.
If a published release is defective, publish a corrected version and a migration
notice; never replace assets or retarget its tag. Follow SECURITY.md for exposed
credentials/signing material. Dependency and public API drift alerts are reviewed
weekly by the repository owner; no baseline or live account is changed by those jobs.
