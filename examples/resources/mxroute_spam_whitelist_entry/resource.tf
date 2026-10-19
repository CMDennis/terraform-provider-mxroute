resource "mxroute_spam_whitelist_entry" "sender" {
  access_domain = "example.com"
  entry         = "*@example.net"
  # The entry is account-wide. Changing access_domain updates in place.
  # Destroy removes only this entry; unrelated entries are preserved.
}
