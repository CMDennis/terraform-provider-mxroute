resource "mxroute_domain" "example" {
  # Publish the account verification TXT record before creating this resource.
  domain                    = "example.com"
  mail_hosting              = true
  verification_wait_seconds = 900
  visibility_wait_seconds   = 30

  lifecycle {
    prevent_destroy = true
  }
}
