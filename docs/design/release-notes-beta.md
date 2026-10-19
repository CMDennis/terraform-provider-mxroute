# MXroute provider beta

Initial all-operation beta candidate: 10 resources and 20 data sources cover the
43 operations in the pinned public MXroute API specification. Packages support
protocol v6 on Linux, macOS and Windows, amd64/arm64. Terraform >= 1.11.1 and < 2.0
is required. Pin the exact beta version; no stable support guarantee exists yet.

Resources support documented lifecycle/import behavior, explicit ownership,
write-only password rotation and safe diagnostics. Complete schema reference,
reserved-domain examples, adoption, recovery, upgrade and support guides are
included. State/plans may contain private identities even when passwords are
write-only; keep them outside this source repository and protected.

All operations are mock-tested. Authorized live evidence covers fourteen ordinary
read operations and one newly created isolated mailbox's API creation, quota/send
limit/password changes, Terraform import/no-op plan and explicitly authorized
Terraform deletion. Original configuration was confirmed unchanged and the test
mailbox is gone. Other live writes, Terraform-driven creation/update, DNS timing,
deletion cascades, cross-domain spam propagation, reseller behavior and
impersonation remain unverified. Read the full [validation record](https://github.com/CMDennis/terraform-provider-mxroute/blob/main/docs/design/phase7.md).
This beta is not a claim of complete live production validation.

Use [installation](https://github.com/CMDennis/terraform-provider-mxroute/blob/main/docs/design/installation.md), [adoption](https://github.com/CMDennis/terraform-provider-mxroute/blob/main/docs/design/migration.md), generated resource
docs and examples before planning a real-account change. Deletion/replacement can
permanently affect hosted data or routing. Mock release smoke tests do not
authorize live account operations. No account identities or private live artifacts
are included in these notes or release packages.

For issues use sanitized reproductions under SUPPORT.md; use SECURITY.md for
private vulnerability reporting. No preceding published version exists; migration
from development state should begin with a protected backup and a no-change plan.
