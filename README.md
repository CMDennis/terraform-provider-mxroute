# MXroute Terraform provider

Phase 8 release preparation is implemented locally. The proposed first release is
`0.1.0-beta.1`; it is **not published**. Six platform packages, signed-checksum
tooling, protected manual draft releases, privacy hooks and ongoing maintenance
checks are ready. Owner setup and actual Registry installation remain release
gates. Start with the [manual publishing checklist](docs/design/publishing.md).

Phases 0–7 are implemented within the authorized live-test scopes. The provider supports domain
onboarding, mail-hosting updates, domain pointers, imports, verification-key
discovery, domain/pointer
lookups and lists, and MX/SPF/nullable DKIM/verification DNS outputs. Phase 4 adds
mailboxes with write-only password rotation, forwarders with destination sets,
catch-all settings and their single/list data sources. The hardened client covers
all 43 API operations. Phase 5 adds shared spam score/entry management, sender-list
reads, account quota and hourly per-email usage. Phase 6 adds reseller packages
and users (suspension, package reassignment, quota overrides, password rotation),
their list/detail data sources and impersonation-alias workflows, so every
documented operation now has a Terraform workflow.
This is an all-operation beta candidate. All 43 operations are mock-tested;
Phase 7 additionally verified all 14 ordinary-account read operations and found
no changes in the before/after settings baseline. A later authorized test created
one new mailbox, updated only its quota/send limit/password, and confirmed that
pre-existing configuration stayed unchanged. Live import and a no-change Terraform
plan also passed for that mailbox. A later explicitly authorized cleanup removed
it and confirmed the original inventory/configuration was restored. Other live
mutations, reseller operations and impersonation remain unverified; fully validated
production compatibility is not claimed.

- [Implementation plan](plan.md)
- [Phase 8 release preparation and verification](docs/design/phase8.md)
- [Manual GitHub, signing-key and Registry setup](docs/design/publishing.md)
- [Installation, first use, verification and upgrades](docs/design/installation.md)
- [Privacy safeguards and commit protection](docs/design/privacy.md)
- [Changelog](CHANGELOG.md), [contributing](CONTRIBUTING.md), [support](SUPPORT.md) and [security](SECURITY.md)
- [Phase 0 decisions and exit gate](docs/design/decisions.md)
- [Phase 1 architecture, schema and identity conventions](docs/design/schema-conventions.md)
- [Phase 1 validation and exit gate](docs/design/phase1.md)
- [Phase 2 API client and offline validation](docs/design/phase2.md)
- [Phase 3 domain lifecycle and offline validation](docs/design/phase3.md)
- [Phase 4 mailbox and routing validation](docs/design/phase4.md)
- [Phase 5 shared spam and reporting validation](docs/design/phase5.md)
- [Phase 6 reseller lifecycle and impersonation validation](docs/design/phase6.md)
- [Phase 7 validation, scenario matrix and release limits](docs/design/phase7.md)
- [Read-only live testing scope and safeguards](docs/design/live-testing.md)
- [Isolated mailbox live test and retained-object policy](docs/design/mailbox-live-testing.md)
- [Adopt existing objects without recreation](docs/design/migration.md)
- [Field-level API coverage audit](docs/design/field-coverage.md)
- [Troubleshooting and compatibility policy](docs/design/troubleshooting.md)
- [Reseller packages, users and impersonation guide](examples/reseller/README.md)
- [Spam ownership, imports and usage guide](examples/spam/README.md)
- [Mail configuration, rotation and adoption guide](examples/mail/README.md)
- [Domain onboarding example and recovery guidance](examples/onboarding/README.md)
- [Generated provider reference](docs/index.md)
- [All 43 operations](docs/design/operation-inventory.md)
- [API contracts and feature blockers](docs/design/api-contracts.md)
- [Pinned API baseline and refresh procedure](api/README.md)
- [Synthetic contract scenarios](testdata/contracts/scenarios.json)

## Build and verify locally

Use Go 1.26.9 (or the tested 1.27.1 line), Terraform >= 1.11.1 and < 2.0,
Python >= 3.10, and the pinned Python dependency below. Go modules and development
tools are pinned in `go.mod` and `Makefile`. Initial downloads require network
access; all account operations in tests use a synthetic loopback service.

```sh
make check build
make vuln
```

`make check` includes unit tests, race detection, offline Terraform lifecycle and
alias tests, static analysis, formatting, inventory validation and a generated
documentation check. Set `TF_ACC_TERRAFORM_PATH` to an installed Terraform binary
to select a version; tests do not auto-download Terraform. Tests generate dummy
credentials in memory, override inherited credentials and keep state/plans in
temporary directories outside the checkout. No real account is needed.

The binary is written to `/tmp/mxroute-provider-build/terraform-provider-mxroute`.
Use a local Terraform CLI development override for
`registry.terraform.io/cmdennis/mxroute` pointing to that directory. Do not run
Registry installation for this unpublished provider. `examples/provider` uses
runtime credential environment variables; `examples/aliases` demonstrates an
ephemeral API-key input. Provider configuration validates inputs without API calls.

`make docs` regenerates the schema reference. `make snapshot` packages and verifies
six unsigned development ZIPs outside Git with publication disabled. With the
native ZIP in `MXROUTE_RELEASE_ARCHIVE`, `make release-smoke` checks normal mirror
installation and schema validation; Linux additionally exercises the full
mock lifecycle/import workflow using the production binary. The protected release
workflow signs and verifies real versioned packages before creating a draft.
The minimum Terraform version is 1.11.1 because
1.11.0 fails the sensitive/ephemeral write-only password persistence test; see
[the validation record](docs/design/phase1.md).

## Verify the Phase 0 inventory offline

Use Python 3.10 or later. Installing the pinned tooling dependency requires a
package download once; subsequent checks do not access the network or read API
credentials.

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -r requirements-dev.txt
.venv/bin/python scripts/api_inventory.py --check
.venv/bin/python -m unittest discover -s scripts -p 'test_*.py' -v
```

To regenerate the inventory after an intentional mapping change:

```sh
.venv/bin/python scripts/api_inventory.py
```

Phase 0 tooling only validates the baseline, mappings and fixture structure. It
does not execute a client, emulate the API, or demonstrate production compatibility.
Default tests use dummy credentials and an explicitly selected mock endpoint.
The Phase 7 live checks used a separate opt-in, GET-only runner after authorization;
they are never part of CI or the acceptance suite. Future authenticated testing
requires concrete approval under the plan, including any disposable-object cleanup.

Provider type: `mxroute`. Go module: `github.com/CMDennis/terraform-provider-mxroute`.
Development uses local overrides or filesystem mirrors. The repository is now
public with the required name. The intended release channel is the public
Terraform Registry; no provider version has been published yet.

## Secrets and account privacy

Never commit API keys, passwords, actual account details, or personal identifying
information. Supply credentials only at runtime. Fixtures use invented identities
and reserved example domains; mock passwords are generated at runtime. Keep state,
plans, logs, account inventories, and live-test manifests out of Git. See
[project instructions](AGENTS.md) for the rules applying to every future phase.
Local commit/message/push hooks require private hash-only account rules stored
outside Git; trusted releases require the same rules and scan packaged ZIPs.
See the [privacy guide](docs/design/privacy.md) for setup, limits and remediation.

Currently licensed under [MIT](LICENSE), preserved from the upstream checkout;
the license can be reconsidered if release requirements call for a change.
