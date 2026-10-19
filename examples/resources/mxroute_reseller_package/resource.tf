# Use a plain reseller provider, not a reseller|user impersonation alias.
resource "mxroute_reseller_package" "starter" {
  name             = "starter"
  quota_gb         = "5" # GB, or "unlimited"
  domains          = "3"
  email_accounts   = "0" # no customer mailboxes; distinct from "unlimited"
  email_forwarders = "unlimited"
  # Omitted limits adopt the server value (documented creation defaults).
}
