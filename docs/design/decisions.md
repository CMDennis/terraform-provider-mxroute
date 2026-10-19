# Phase 0 decisions

Recorded October 19, 2026. These decisions freeze the implementation scope; they
do not establish that synthetic fixtures match the live API.

## Repository, identity and distribution

- Checkout: [CMDennis/mxroute-terraform](https://github.com/CMDennis/mxroute-terraform),
  branch `main`, baseline commit `caee6fd4428dbd81db355b41806ef1717d8ae78d`.
  GitHub reported it as private. The original README was a two-line placeholder;
  its replacement points to the Phase 0 artifacts. The original `plan.md` was
  retained when the checkout was attached to this workspace.
- Retain MIT as the current provisional license; the original choice was
  arbitrary, and the user permits a different license if needed for release or
  dependency requirements. Preserve required copyright and attribution notices.
  Original license SHA-256 for provenance:
  `6bbfc6ad7537df40a38468fd3249cc7830d5ccc66e24d2fdeb91b41b4dd987a1`.
- Go module: `github.com/CMDennis/mxroute-terraform`. Provider type: `mxroute`.
  Executable: `terraform-provider-mxroute`. Resource/data source prefix: `mxroute_`.
- Initial provider source address: `registry.terraform.io/cmdennis/mxroute`.
  This is the chosen logical address for development overrides/filesystem mirrors,
  **not a claim that the address is published, registered or reserved**.
- Keep the repository private during development. Develop with local overrides
  or filesystem mirrors. The intended release channel is the public Terraform
  Registry; make the repository public and arrange the required name when ready
  to publish. No publication or visibility change is authorized now. At release,
  recheck namespace ownership and arrange a public `terraform-provider-mxroute` repository.
  HashiCorp requires that [public repository naming pattern](https://developer.hashicorp.com/terraform/registry/providers/publishing).
  Revisit the Go module path before the first release if a rename is chosen.

## Version policy

| Component | Selected baseline | Policy and evidence |
| --- | --- | --- |
| Terraform CLI | `>= 1.11.1, < 2.0.0` | Phase 1 raised the original 1.11.0 selection after reproducing its sensitive/ephemeral serialization bug. See [validation evidence](phase1.md); minimum/current tests cover 1.11.1 and 1.16.6. |
| Plugin protocol | `6.0` | Use Framework protocol v6 and a v6 release manifest. |
| Go language/module floor | `1.26.0` | Supported build line selected above the dependencies' minimums; binaries do not require Go on the user's machine. |
| Go build baseline | `1.26.9` | Use the patched 1.26 toolchain; expand CI to the current 1.27 line in Phase 1. Follow supported Go release lines and patch updates. |
| Plugin Framework | `v1.19.0` | Exact initial pin, [go.mod](https://github.com/hashicorp/terraform-plugin-framework/blob/v1.19.0/go.mod) requires Go 1.25.0 and plugin-go v0.31.0. |
| Plugin testing | `v1.16.0` | Exact initial pin, [go.mod](https://github.com/hashicorp/terraform-plugin-testing/blob/v1.16.0/go.mod) requires Go 1.25.8 and plugin-go v0.31.0. |
| OpenTofu | No compatibility commitment | Add only after independent write-only, import, plan/state and lifecycle testing. |

The selected direct dependencies were checked through official Go proxy module
metadata and GitHub release metadata, rather than inferred from documentation's
“latest” selector. Both select plugin-go v0.31.0. An isolated probe module compiles
Framework `schema.StringAttribute{WriteOnly: true}`, `providerserver.ServeOpts`
with protocol 6, and `terraform-plugin-testing`'s v6 provider factories together.
See [compatibility evidence](compatibility.json) for the exact toolchain and result.
This is a dependency/API compilation check, not a provider acceptance test.
The probe source and resolved module checksums are preserved in
[`testdata/compatibility`](../../testdata/compatibility). To repeat it, run
`GOTOOLCHAIN=go1.26.9 go test -mod=readonly -v ./...` from that directory (the first
run downloads public modules and the toolchain). Phase 1 must commit the root
provider `go.mod`/`go.sum` when provider code is introduced; the probe is a separate
module and is not provider implementation or a release binary.

Phase 1 now provides the root provider module, retaining Framework v1.19.0,
testing v1.16.0 and plugin-go v0.31.0. Its indirect dependency graph includes
security patches and therefore differs from the historical Phase 0 probe. The
probe/checksums above remain a record of that original compilation check, not
the current provider's dependency lock. Use the root `go.mod`/`go.sum` for builds.

Future checks must prove password absence from saved plans/state on the minimum
Terraform version. A sensitive flag alone does not prevent persistence. Use
optional write-only password arguments with an explicit non-secret rotation
version; require a password only on creation, and permit import without it.

Use semantic versioning: domain alpha after Phase 3, ordinary-account preview
after Phase 5, all-operation beta after Phase 6, and 1.0 only after Phase 7 gates.
Pre-1.0 breaking changes still need release notes and state migration analysis.
Re-evaluate dependency pins and supported toolchains before each release; Phase 0
selection does not promise perpetual support for these exact patch versions.

## Contract decisions and feature blockers

The [contract register](api-contracts.md) separates documented facts, conservative
provider decisions and explicit runtime questions. Questions block stabilization
of the affected feature, not credential-free architecture or mock development.
Key lifecycle decisions are:

- Treat shared spam lists as effective-account objects accessed through an owned
  domain. Spam-score scope must be settled before exposing its stable schema.
- Replace forwarders when destinations change. No documented update operation
  establishes safe upsert behavior or create-before-destroy for the same alias.
- Deleting catch-all/spam singleton resources relinquishes management and
  preserves remote settings; never invent a reset/default API call.
- Do not infer domain/pointer verification, cascade behavior, system-mailbox
  mutability or package/quota precedence. Keep their explicit questions attached
  to the relevant future features and test conservative failure paths in mocks.
- No undocumented bulk/import operations. The 43-operation inventory is the
  coverage baseline. Reopen scope if upstream adds an operation.

## Credential-free test boundary

The user's permanent rule is that no API key, password, actual account information
or personal identifying information may ever enter Git. This applies to examples,
fixtures, generated artifacts and commit messages as well as implementation.
Credentials must be supplied at runtime; mock passwords are generated at runtime
instead of saved as literals. Keep live responses, inventories, ownership
manifests, Terraform state/plans and logs outside Git, and review all staged
content before any commit. [AGENTS.md](../../AGENTS.md) records these instructions
for subsequent implementation work.

Phase 0 checks are local checksum, inventory and fixture validation. Fixtures use
reserved example domains, synthetic identities and dummy values. They contain no
real API response captures or account secrets. The tooling never reads credential
environment variables and makes no API calls. The mock server and HTTP client
arrive in Phase 2; the synthetic scenarios here are inputs to that later harness.

Phases 0–6 must explicitly inject a loopback mock endpoint and dummy credentials,
reject a production endpoint, and clear/override inherited credential variables.
Do not add a production fallback or unattended live teardown to acceptance tests.
Only a real ordinary account is known to exist; reseller access, an owned user,
disposable domains and DNS control are unconfirmed. The contract register records
the prerequisites and unavailable/unsafe scenarios. Phase 7 requires a concrete
proposal and approval before any authenticated discovery or mutation, with a
baseline inventory, isolated state and an approved ownership/cleanup manifest.

## Exit gate

- [x] Repository obtained; history and unchanged upstream MIT license preserved.
- [x] Module, provider identity, development distribution, intended public release
  channel and version policy chosen.
- [x] Dated OpenAPI snapshot, SHA-256 and provenance recorded. Before the initial
  push, a credential-shaped public example was redacted; the original upstream
  digest is retained separately. See [snapshot provenance](../../api/README.md).
- [x] Exactly 43 method/path obligations map to named client, Terraform, test and
  documentation targets; every implementation status is explicitly planned.
- [x] Sparse/ambiguous contracts have synthetic cases and documented safe
  decisions or explicit blocking questions for affected features.
- [x] Future live prerequisites and unavailable scenarios are recorded; all live
  runtime assumptions remain unverified. No account credentials were used.

Phase 0 completion authorizes no live tests, publication or subsequent-phase
implementation beyond a later user request.
