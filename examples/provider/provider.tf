terraform {
  required_version = ">= 1.11.1, < 2.0.0"
  required_providers {
    mxroute = {
      source = "cmdennis/mxroute"
    }
  }
}

# Supply MXROUTE_SERVER, MXROUTE_USERNAME and MXROUTE_API_KEY at runtime.
# This address requires a local development override until publication.
provider "mxroute" {}
