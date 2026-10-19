# Support and maintenance

The repository owner is the initial maintainer. Use GitHub issues for sanitized
bugs and feature requests; use the private channel in SECURITY.md for security
reports. Hosting, billing, delivery and live-account recovery belong with MXroute
support. This provider does not send messages, change DNS at a registrar, create
API keys or provide backup/restore endpoints.

Include the provider version, Terraform/OS versions, a minimal example using
`example.com` and invented identities, expected behavior and redacted diagnostics.
Do not include state, saved plans, logs, real inventories or credentials. Describe
a reproduction as offline, live read, or approved isolated live mutation. Never
repeat a destructive live operation merely to obtain a reproduction.

Proposed initial version: `0.1.0-beta.1`. Beta users should pin an exact version;
state/lifecycle behavior may change before 1.0. Patch versions are for compatible
fixes, minor beta versions for added behavior, and every breaking change needs a
documented migration. Stable 1.0 remains gated on the unresolved live validation
in the Phase 7 record. Stable releases will follow semantic versioning.

Weekly automation opens dependency PRs, runs the pinned offline compatibility
matrix and compares the public API specification with its accepted checksum.
The owner reviews failed workflow notifications in GitHub Actions. API changes
never update the pinned snapshot automatically; follow api/README.md. Review the
tested minimum/current toolchain pins and support scope before each release.
Retest and update pins deliberately rather than assuming an untested newer
version works. See [publishing](docs/design/publishing.md) for the release checklist.
