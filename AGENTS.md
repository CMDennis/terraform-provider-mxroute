# Project instructions

## Secrets and account privacy

- NEVER add an API key, any password, or the user's actual account information
  or identifying information to Git, including commits, branches, tags, patches,
  fixtures, examples, documentation, generated files, or commit messages.
- This includes MXroute usernames, account email addresses, account/server details,
  real managed domains, account inventories, ownership manifests, and raw API
  responses that could identify the user. Use reserved example domains and
  invented identities in repository artifacts.
- Supply authorized credentials through runtime environment variables or an
  approved secret store. Do not write their values into this checkout, even in
  ignored files. Do not print them in tool output, logs, diagnostics, or chat.
- Do not store literal passwords in fixtures, even dummy passwords. Generate
  disposable mock passwords at runtime and keep them out of saved artifacts.
- Keep Terraform state, saved plans, variable files, logs, account inventories,
  and live-test manifests outside Git. State and plans can expose account
  identities even when password arguments are write-only.
- Before staging or committing, review every changed file and generated artifact
  for secrets and actual account identifiers. Ignore rules are an extra safeguard;
  they do not make it safe to stage sensitive data. Report potential findings
  using filenames and locations only, never the sensitive values themselves.
- If sensitive data is found in Git, stop publication and report it without
  repeating the data. Coordinate credential rotation and history remediation;
  deleting the working file alone does not remove committed history.

## Publication and license

- The repository is private during development. The intended release channel is
  the public Terraform Registry. Change visibility and arrange the required
  repository name when the user is ready to publish, not during development.
- MIT is the current, provisional license choice. The user permits selecting a
  different license if release or dependency requirements call for it. Preserve
  required existing copyright and attribution notices.

## Testing

- Follow the account-protection policy in `plan.md`. Phases 0–6 use explicit local
  mocks and runtime dummy credentials, never the real account.
- Authenticated live testing and its exact cleanup require prior approval under
  that policy. Possession of credentials is not authorization.
