resource "mxroute_domain_pointer" "example" {
  domain  = mxroute_domain.example.domain
  pointer = "alias.example.net"
  type    = "alias"

  # Opt in only when a TXT verification record is published for this pointer.
  verification_wait_seconds = 900
}
