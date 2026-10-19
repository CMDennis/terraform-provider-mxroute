# Use one owner per effective account, across all owned domains.
resource "mxroute_spam_settings" "account" {
  access_domain = "example.com"
  high_score    = 15
  # Destroy relinquishes management and preserves remote filtering.
}
