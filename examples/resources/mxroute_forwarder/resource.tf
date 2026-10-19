resource "mxroute_forwarder" "sales" {
  domain       = "example.com"
  alias        = "sales"
  destinations = ["recipient@example.net", "backup@example.org"]
  # Destination changes replace the alias and interrupt routing.
}
