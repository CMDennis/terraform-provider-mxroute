terraform {
  required_version = ">= 1.11.1, < 2.0.0"
  required_providers {
    mxroute = {
      source = "cmdennis/mxroute"
    }
  }
}

# Invented identities only. Supply runtime secrets through a secret manager or
# ephemeral input; never save actual account details in this repository.
variable "reseller_api_key" {
  type      = string
  sensitive = true
  ephemeral = true
}

provider "mxroute" {
  server   = "mail.example.com"
  username = "reseller"
  api_key  = var.reseller_api_key
}

provider "mxroute" {
  alias    = "ordinary"
  server   = "mail.example.com"
  username = "reseller|ordinary"
  api_key  = var.reseller_api_key
}

# Account and domain resources select this account with provider = mxroute.ordinary.
# Reseller users and packages require the plain provider above. A user created
# in the same configuration requires a separate apply before this alias uses it;
# see examples/reseller/README.md.
