# Changelog

## Unreleased

- Match the EASYDNS release process: pushing a reviewed version tag runs all
  validation/signing checks and publishes complete assets automatically. Versions
  with a prerelease suffix remain pre-releases; stable tags publish stable releases.
- Disable the weekly compatibility/API-drift scan; retain it for manual runs.
- Update the build/release toolchain to Go 1.26.10 and the newer CI lane to
  Go 1.27.3, fixing GO-2026-6809 so the release vulnerability gate passes.

## v0.1.0-beta.2

Recovery release for the immutable, assetless `v0.1.0-beta.1` publication. It uses
the same validated provider code and prepares complete signed platform packages
under a new version. The previous tag and release remain unchanged.

- Implement all 43 documented API operations through 10 resources and 20 data
  sources: domains, pointers, mailboxes, forwarders, catch-all, shared spam
  settings/lists, reporting, reseller packages/users and impersonation aliases.
- Support imports, schema version 0, explicit drift handling, write-only password
  rotation, bounded retries/rate limits, and safe credential diagnostics.
- Provide operation and field coverage records, complete generated schema
  reference, adoption/recovery examples and explicit live-validation limits.
- Prepare six platform packages, signed checksums, a protocol-v6 manifest,
  protected manual draft releases, offline package installation tests, privacy
  checks, dependency updates and scheduled compatibility/API-drift checks.

Published with all nine provider assets. Read [the beta release notes](docs/design/release-notes-beta.md)
for the verified scope and remaining live gaps. No prior released state format
exists. Future entries must describe schema/lifecycle changes and migrations.

## v0.1.0-beta.1 — incomplete publication

An empty public release was created separately from the correctly signed draft.
Its immutable published entry has no provider packages, checksum file or signature
and cannot be installed. No valid assets or checksums are being replaced; use the
complete recovery release once it is published and indexed.
