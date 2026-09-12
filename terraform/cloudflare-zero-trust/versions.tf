terraform {
  required_version = ">= 1.5.0"

  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.0"
    }
  }

  backend "gcs" {
    bucket = "ttp-home-tfstate"
    prefix = "cloudflare-zero-trust"
  }
}

provider "cloudflare" {}
