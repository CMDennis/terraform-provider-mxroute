terraform {
  required_version = ">= 1.11.1, < 2.0.0"
  required_providers {
    mxroute = { source = "registry.terraform.io/cmdennis/mxroute" }
  }
}

# Supply provider credentials through runtime MXROUTE_* variables.
provider "mxroute" {}

resource "mxroute_spam_settings" "account" {
  access_domain = "example.com"
  high_score    = 15
}

resource "mxroute_spam_whitelist_entry" "trusted" {
  access_domain = "example.com"
  entry         = "*@example.net"
}

resource "mxroute_spam_blacklist_entry" "blocked" {
  access_domain = "example.org"
  entry         = "blocked+tag=test@example.net"
}

data "mxroute_spam_whitelist" "account" {
  access_domain = "example.org"
  depends_on    = [mxroute_spam_whitelist_entry.trusted]
}

data "mxroute_quota" "account" {}
data "mxroute_email_usage" "account" {}

output "storage_bytes" {
  value = data.mxroute_quota.account.total_used
}
output "quota_limit_bytes" {
  value = data.mxroute_quota.account.total_limit
}
output "mailbox_usage" {
  value = data.mxroute_email_usage.account.accounts
}
