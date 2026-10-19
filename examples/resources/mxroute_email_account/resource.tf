variable "mailbox_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "mxroute_email_account" "support" {
  domain              = "example.com"
  username            = "support"
  quota               = 1024 # MB; use 0 for unlimited
  limit               = 120
  password_wo         = var.mailbox_password
  password_wo_version = 1 # increase to rotate
}
