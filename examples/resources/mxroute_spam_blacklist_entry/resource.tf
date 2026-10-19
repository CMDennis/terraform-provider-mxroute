resource "mxroute_spam_blacklist_entry" "sender" {
  access_domain = "example.com"
  entry         = "blocked+tag=test@example.org"
  # The entry is account-wide. Changing access_domain updates in place.
  # Destroy removes only this entry; unrelated entries are preserved.
}
