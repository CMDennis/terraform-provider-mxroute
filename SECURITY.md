# Security policy

The provider is pre-release. Security fixes target the latest beta; no stable
version or long-term support branch exists. The repository owner maintains
releases and reviews security reports. This is a community provider and is not
an MXroute or HashiCorp support service.

Report vulnerabilities through GitHub's **Security → Report a vulnerability**
once private vulnerability reporting is enabled by the owner. Until that control
is available, do not open a public issue with exploit details or private data;
ask the repository owner for a private reporting channel. No response-time SLA
is promised. Public issues may describe sanitized symptoms without secrets.

Never attach API keys, passwords, actual server/user/domain/mailbox identities,
Terraform state/plans, debug logs, live responses or ownership manifests. Use
reserved example domains and generated dummy identities in reproductions.

If an actual secret or identity enters Git, stop publication immediately. Record
file locations and affected revisions without repeating the data. Revoke or
rotate exposed credentials with the account owner, arrange reviewed history
remediation, check remote refs and release assets, then repeat the privacy audit.
Deleting the working file or making a repository private does not remove copies
from history, clones or caches. Do not rewrite shared history without coordination.

Signing-key compromise requires stopping releases, revoking the affected key,
registering a replacement trusted key and notifying users through a sanitized
advisory. Verify subsequent releases against the new separately trusted
fingerprint. Never silently replace an existing release archive.
