# Pinned API baseline

The public [MXroute OpenAPI specification](https://api.mxroute.com/openapi.yaml)
was downloaded on October 19, 2026 without authentication. Its linked
[documentation](https://api.mxroute.com/docs) renders that specification through a
JavaScript UI; it is not independent evidence of backend behavior.

`baseline.json` records the source, date, operation/path counts and two digests:
`upstream_sha256` identifies the original public download; `sha256` identifies the
checked-in sanitized snapshot. Before the initial push, one credential-shaped
authentication example was replaced with `[API_KEY_FROM_SECRET_STORE]` under the
project's no-keys-in-Git policy. All API schemas and operations are unchanged;
the `sanitization` field records this exception. The adjacent `.sha256` file
verifies the sanitized snapshot from the snapshots directory with
`shasum -a 256 -c openapi-2026-10-19.yaml.sha256`.

`coverage.json` is the reviewed method/path-to-client/Terraform/test/doc mapping.
All 43 targets are implemented and mock-tested with generated reference docs.
Phase 7 checked fourteen ordinary-account read operations live; per-operation
status distinguishes this from live-unverified mutation/reseller behavior.
`operations.json` and `docs/design/operation-inventory.md` are deterministic
outputs of `scripts/api_inventory.py`. The JSON preserves every request/response
schema and shared component, including schema omissions. Each operation has its
own coverage obligation even when several operations map to one resource.

Phase 7 downloaded the current public document again and confirmed identical
bytes and checksum. The [field audit](../docs/design/field-coverage.md) records
request/response coverage and the observed quota envelope discrepancy. Do not
rewrite the upstream snapshot to match a runtime deviation.

## Reviewing an upstream change

1. Download the public specification to a private temporary file outside Git
   without credentials. Record its original source digest as `upstream_sha256`.
   Review and replace credential-shaped examples before saving a **new dated
   sanitized snapshot** in Git. Record its digest as `sha256` and document every
   redaction. Do not overwrite an accepted snapshot during routine drift checks.
2. Compare method/path sets and input/output schemas, required fields, units,
   defaults, errors and descriptions with the accepted snapshot. A version string
   alone does not prove compatibility.
3. Update the contract questions, synthetic cases, coverage mappings and plan.
   New operations must receive an explicit supported workflow or documented gap;
   an operation-count change cannot pass unnoticed.
4. Deliberately update `baseline.json` and its checksum sidecar after review, then
   regenerate the two inventory outputs. Run the README's offline checks.
5. Retain prior sanitized snapshots and original digests for provenance. The
   manually triggered drift check compares public bytes against `upstream_sha256`, not the
   sanitized file digest. Re-run approved live verification only
   in Phase 7 under the account-protection policy; mock results remain separate.

No pagination, bulk/import endpoint, DNS write, API-key creation or mailbox
suspension write has been invented. Terraform state import is a provider lifecycle
feature, independent of the rate-limit prose mentioning bulk/import.
