terraform {
  required_version = ">= 1.11.1, < 2.0.0"
  required_providers {
    mxroute = {
      source = "registry.terraform.io/cmdennis/mxroute"
    }
  }
}

# Invented identities only. Supply MXROUTE_SERVER and MXROUTE_API_KEY at runtime.
# The default provider is the plain reseller: required for users and packages.
provider "mxroute" {
  username = var.reseller_username
}

# The alias manages an owned user's account with the reseller's own key. Its
# username is a literal input, never a reference to a resource created in the
# same run; see README.md for the two-stage apply.
provider "mxroute" {
  alias    = "customer"
  username = "${var.reseller_username}|${var.customer_username}"
}

variable "reseller_username" {
  type    = string
  default = "reseller"
}

variable "customer_username" {
  type    = string
  default = "customer1"
}

# Stage 2 only: set to true after the user exists.
variable "manage_customer_account" {
  type    = bool
  default = false
}

variable "customer_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "mxroute_reseller_package" "starter" {
  name           = "starter"
  quota_gb       = "5"
  domains        = "3"
  email_accounts = "50"
}

resource "mxroute_reseller_user" "customer" {
  username            = var.customer_username
  email               = "owner@example.net"
  package             = mxroute_reseller_package.starter.name
  password_wo         = var.customer_password
  password_wo_version = 1

  lifecycle {
    prevent_destroy = true
  }
}

# Account-level reads and resources for the owned user go through the alias.
data "mxroute_quota" "customer" {
  count    = var.manage_customer_account ? 1 : 0
  provider = mxroute.customer
}

data "mxroute_domains" "customer" {
  count    = var.manage_customer_account ? 1 : 0
  provider = mxroute.customer
}

output "customer_domains" {
  value = var.manage_customer_account ? data.mxroute_domains.customer[0].domains : null
}
