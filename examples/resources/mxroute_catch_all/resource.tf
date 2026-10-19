resource "mxroute_catch_all" "routing" {
  domain  = "example.com"
  type    = "address"
  address = "unmatched@example.net"
  # For fail/blackhole, omit address.
  # Removing this resource preserves the remote setting.
}
