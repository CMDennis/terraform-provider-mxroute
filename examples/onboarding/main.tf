terraform {
  required_version = ">= 1.11.1, < 2.0.0"
  required_providers {
    mxroute = { source = "cmdennis/mxroute" }
    aws     = { source = "hashicorp/aws", version = "~> 6.0" }
  }
}

# Runtime credentials only: MXROUTE_* for MXroute and AWS's normal credential chain.
provider "mxroute" {}
provider "aws" { region = "us-east-1" }

variable "zone_id" {
  type        = string
  description = "Existing Route 53 public hosted zone ID for example.com. Supply at runtime."
}

# 1. Obtain the account key, before the domain exists.
data "mxroute_verification_key" "current" {}

# 2. Publish the verification TXT through a DNS provider.
resource "aws_route53_record" "verification" {
  zone_id = var.zone_id
  name    = "${data.mxroute_verification_key.current.record.name}.example.com"
  type    = data.mxroute_verification_key.current.record.type
  ttl     = 60
  records = [data.mxroute_verification_key.current.record.value]
}

# 3. Wait for the exact TXT value to be resolvable, then create the domain.
resource "mxroute_domain" "example" {
  domain                    = "example.com"
  mail_hosting              = true
  verification_wait_seconds = 900
  depends_on                = [aws_route53_record.verification]

  lifecycle { prevent_destroy = true }
}

# 4. Discover mail records only after domain creation.
data "mxroute_dns_records" "example" {
  domain = mxroute_domain.example.domain
}

resource "aws_route53_record" "mx" {
  zone_id = var.zone_id
  name    = mxroute_domain.example.domain
  type    = "MX"
  ttl     = 300
  records = [for mx in data.mxroute_dns_records.example.mx_records : "${mx.priority} ${mx.hostname}"]
}

resource "aws_route53_record" "spf" {
  zone_id = var.zone_id
  name    = mxroute_domain.example.domain
  type    = data.mxroute_dns_records.example.spf.type
  ttl     = 300
  records = [data.mxroute_dns_records.example.spf.value]
}

# DNS record contents are returned unaltered; never add quotes to the TXT value.
# DKIM can be null, and its availability is only known after domain creation.
output "dkim" { value = data.mxroute_dns_records.example.dkim }
output "mx_records" { value = data.mxroute_dns_records.example.mx_records }
