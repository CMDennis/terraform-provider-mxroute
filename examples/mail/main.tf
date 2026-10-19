terraform {
  required_version = ">= 1.11.1, < 2.0.0"
  required_providers {
    mxroute = {
      source = "registry.terraform.io/cmdennis/mxroute"
    }
  }
}

# Supply provider credentials through runtime MXROUTE_* environment variables.
provider "mxroute" {}

variable "mailbox_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "mxroute_domain" "mail" {
  domain       = "example.com"
  mail_hosting = true

  lifecycle {
    prevent_destroy = true
  }
}

resource "mxroute_email_account" "support" {
  domain              = mxroute_domain.mail.domain
  username            = "support"
  quota               = 1024
  limit               = 120
  password_wo         = var.mailbox_password
  password_wo_version = 1
}

resource "mxroute_forwarder" "sales" {
  domain       = mxroute_domain.mail.domain
  alias        = "sales"
  destinations = [mxroute_email_account.support.email, "archive@example.net"]
}

resource "mxroute_catch_all" "routing" {
  domain = mxroute_domain.mail.domain
  type   = "fail"
}

data "mxroute_email_accounts" "mailboxes" {
  domain     = mxroute_domain.mail.domain
  depends_on = [mxroute_email_account.support]
}
