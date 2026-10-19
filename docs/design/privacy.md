# Account privacy and Git safeguards

The supplied real-account credentials, its server/user/domain/mailbox identities,
inventories, live responses and private test manifests must never enter Git or a
release asset. Use actual credentials only through authorized runtime environment
variables or a protected secret store. Even ignored checkout files are unsuitable
for private live artifacts; keep them outside the repository with owner-only
permissions. API keys/passwords are never needed by ordinary tests or CI.

## Protection installed for this checkout

`core.hooksPath` points to `.githooks`. The pre-commit hook inspects the actual
staged bytes (including forced additions of ignored files). The commit-message
hook checks the message before it is committed. The pre-push hook checks working
files, reachable history, messages and ref names. All three require private
account fingerprint rules and fail closed if those rules are missing/invalid.
The initial Phase 8 preparation left code uncommitted for review before its
authorized publication push.

Generic history checks cover local/remote branches, tags and HEAD. Codex's local
undo trees are checked separately for protected account fingerprints and private
key material; they are never part of the explicit `main` push. An older public
documentation example may remain in local undo metadata after redaction, but is
absent from the published branch history. Never use `git push --mirror` to publish
local app metadata.

Private rules are stored at
`~/.config/mxroute-terraform/privacy-fingerprints.json`, outside the checkout,
with owner-only permissions. They contain lengths and SHA-256 hashes of protected
identities, including case-normalized and encoded variants; no plaintext account
values or credentials are saved. Treat these rules as private too: low-entropy
identities can be guessed from hashes. Keep the file out of Git and public artifacts.

`scripts/privacy_check.py` also blocks private runtime file types, private-key
material, literal account environment assignments and live provenance JSON, and
looks inside ZIP assets. Findings print only locations and categories. Ordinary
CI uses generic checks without private rules; the protected release job receives
hash-only rules through its environment secret and requires them before signing.
Never pass those rules to an untrusted pull-request job.

## Recheck and maintain

```sh
git config --get core.hooksPath
python3 scripts/privacy_check.py --worktree --staged --history --require-private-rules
python3 scripts/privacy_check.py --artifact-dir /tmp/mxroute-provider-dist --require-private-rules
```

For a new authorized account or key, supply values only in the runtime environment
and run `python3 scripts/configure_privacy.py`. An optional `--manifest` accepts an
existing private manifest path outside the checkout and protects its identities
without copying its contents. Rules merge with previous fingerprints; rotating a
key does not remove protection for the old key. Update the protected GitHub
`PRIVACY_FINGERPRINTS` secret after extending rules.

Other contributors need to enable hooks in their clones. Hooks do not travel as
Git configuration, can be bypassed, and cannot recognize every arbitrary new
identity or encoding. Always inspect staged content manually; hooks are additional
protection. Branch/tag names and annotations require the same review as files.
Avoid blanket staging when private artifacts may exist.

If a finding exists only in the working tree/index, remove it from both, retain
any necessary private evidence outside Git, and recheck. If it reached history,
stop publication and follow [SECURITY.md](../../SECURITY.md); deleting the visible
file is insufficient. Do not print the sensitive value in a fix commit, issue or
remediation report. Existing license attribution and the project's public source
namespace are retained; neither exposes the private test account.
