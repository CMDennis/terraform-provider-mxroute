variable "customer_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

# Use a plain reseller provider, not a reseller|user impersonation alias.
resource "mxroute_reseller_user" "customer" {
  username            = "customer1"
  email               = "owner@example.net" # changing it REPLACES the user
  package             = mxroute_reseller_package.starter.name
  quota_mb            = "2048" # MB, or "unlimited"; omit to adopt the package value
  suspended           = false
  password_wo         = var.customer_password
  password_wo_version = 1 # increase to rotate

  lifecycle {
    prevent_destroy = true # deletion removes the account and everything it hosts
  }
}
