# Phase 1 implementation and validation

Implemented October 19, 2026. This phase establishes a buildable provider and
schema conventions. No API operation, resource or data source is advertised as
implemented. All runtime MXroute contracts remain live-unverified.

## Delivered

- Root Go module and checksum lock, protocol-v6 executable, versioned user agent,
  separate HTTP client/provider packages, shared identity and schema helpers.
- Provider inputs `server`, `username`, sensitive `api_key`, explicit environment
  fallbacks, configuration precedence, missing/empty/unknown validation, fixed
  production origin and no requests during configuration.
- Distinct owner/effective-account identities, validation of impersonation and
  reseller-operation boundary, independently configured aliases.
- [Schema conventions](schema-conventions.md) covering import/immutable IDs,
  case normalization, segment escaping, collections, null/unknown/zero handling,
  replacement, singleton removal and versioned state migration hooks.
- Shared optional write-only passwords and explicit rotation versions, including
  creation checks, unchanged-update preservation and adoption without rotation.
  Bounds are supplied per resource; actual mailbox/user lifecycles are later work.
- Runtime-generated mock secrets, fail-closed numeric loopback client injection,
  Terraform CLI lifecycle tests and a test-executable plugin for alias isolation.
  The fixture resource is absent from production builds and generated docs.
- Formatting/static analysis/race checks, pinned vulnerability scanner,
  reproducible tfplugindocs reference and examples, offline CI version matrix,
  disabled-publication snapshot configuration and protocol-v6 manifest.

## Version adjustments from Phase 0

The Framework v1.19.0, plugin-go v0.31.0 and testing v1.16.0 pins are unchanged.
The root dependency graph now selects gRPC v1.83.2, x/net v0.60.0 and x/text
v0.42.0, with compatible indirect upgrades. The first vulnerability scan found
reachable advisories in the Phase 0 graph; these updates resolve them. Historical
probe files/checksums under `testdata/compatibility` remain untouched and are not
the current provider build lock.

Terraform's supported floor is **1.11.1**, raised from the original 1.11.0
selection. The 1.11.0 test failed while encoding sensitive ephemeral write-only
values, before apply, with an internal serialization error. The identical
scenario passes on 1.11.1 and the selected current 1.16.6. HashiCorp records
the fix in the [1.11.1 changelog](https://github.com/hashicorp/terraform/blob/v1.11.1/CHANGELOG.md)
under [issue 36619](https://github.com/hashicorp/terraform/issues/36619).
Examples, CI, the provider reference and compatibility policy use the corrected
floor. No credential persistence fallback was introduced for 1.11.0.

Build tool versions: tfplugindocs v0.25.0, govulncheck v1.7.0 and GoReleaser
v2.17.0. GoReleaser v2.18.0 requires Go 1.27, so the development snapshot command
uses v2.17.0 to remain runnable with the selected Go 1.26.9 baseline.

## Offline evidence and reproduction

All test identities use reserved example domains and invented users. Passwords
and API keys are generated in memory for each test. No inherited account
credentials are used. State, saved plans, downloaded test CLIs and build outputs
are outside the checkout; no account inventory or live-test manifest was made.

Run the checks after installing the pinned Python tooling in the README:

```sh
make check build
make vuln
make snapshot
```

For a specific installed Terraform version:

```sh
TF_ACC_TERRAFORM_PATH=/path/to/terraform GOTOOLCHAIN=go1.26.9 make test
```

The tests never install Terraform automatically. Public release binaries used
for the local minimum/current checks were downloaded outside the repository and
checked against HashiCorp's release SHA-256 files. CI installs the selected
versions explicitly and contains no live-account credentials or publication job.

The checks demonstrate:

- Provider values from configuration override environment values; explicit
  empty/unknown values do not silently inherit a credential. Unknowns pass
  validation and produce actionable configuration diagnostics before any request.
- Plain, impersonated and second-server aliases can independently create the
  same synthetic identity, preserve different quota values and plan no changes.
  Alias testing launches actual provider processes. The test library's single
  reattached server is used only for single-configuration lifecycle tests.
- Password creation and explicit version rotation reach the loopback service;
  quota-only updates do not send passwords. Zero is retained as distinct from
  omission. Decreasing/removing versions, invalid lengths and missing creation
  passwords are rejected. Unknowns are deferred until apply.
- Import without a password or version produces a no-change plan; first explicit
  rotation after adoption works. Ordinary import verification and immutable-name
  replacement planning are also checked. Cleanup affects only the local fixture.
- Write-only values are null in plan/state. Decoded artifacts, raw state files,
  and saved plan ZIP members contain neither generated password nor API key.
  Secret delivery uses ephemeral inputs so configuration/variable sections in
  saved plans do not retain ordinary password variable values.
- HTTP headers preserve configured values and API-key colons; redirects are not
  followed; invalid mock origins fail closed; transport errors and diagnostics
  omit inputs, bodies and secrets. No response decoding/retry claims are made.
- The Phase 0 inventory still contains exactly 43 operations and 33 synthetic
  scenarios, and its eight regression tests pass unchanged.

## Exit gate

| Final local check | Result |
| --- | --- |
| Go 1.26.9 / Terraform 1.11.1, all packages with race detection | Passed |
| Go 1.27.1 / Terraform 1.16.6, all packages with the final dependency lock | Passed |
| `go vet`, Go/Terraform formatting, root provider build | Passed |
| govulncheck v1.7.0, final dependency graph | No vulnerabilities found |
| Documentation regeneration followed by `docs-check` | Identical generated reference |
| Pinned inventory/checksum, 33 scenarios, eight Python regression tests | Passed |
| GoReleaser configuration validation | Passed; publication disabled |
| Snapshot builds: Darwin/Linux/Windows, amd64/arm64 | All six passed |
| Checkout artifact/literal-secret review and original license checksum | Passed; no runtime artifacts or literal secret assignments found |

- [x] Provider builds and serves protocol v6 with the pinned Framework/testing APIs.
- [x] Representative configuration, environment, null/empty/unknown, alias and
  impersonation boundaries pass local tests.
- [x] Immutable/import/schema/state conventions are recorded and shared helpers
  are exercised through Terraform, including replacement planning.
- [x] Generated credentials/passwords are absent from saved resource state,
  saved plan contents and provider-generated diagnostic/format output in tests.
- [x] Password import and rotation behavior passes on the corrected minimum CLI.
- [x] Reproducible docs, CI and local snapshot configuration are prepared.

Phase 1 is complete. CI is configured for the full two-by-two version matrix;
the local pairings above are the checks actually run, not a claim that hosted
CI or a published release was executed.

Typed operation methods, response decoding, retries/rate coordination and API
contract tests are Phase 2. Public resource schemas and lifecycle acceptance
coverage belong to Phases 3–6. Real-account tests, repository visibility changes,
publication and signing remain outside this authorization.
