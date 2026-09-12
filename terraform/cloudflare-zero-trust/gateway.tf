resource "cloudflare_zero_trust_gateway_certificate" "gateway" {
  account_id = var.account_id
}

import {
  to = cloudflare_zero_trust_gateway_certificate.gateway
  id = "4611a7a2299c655529452ac81b5f4844/bb96b678-0419-4088-a854-855316a3ac0d"
}

resource "cloudflare_zero_trust_gateway_settings" "account" {
  account_id = var.account_id
  settings = {
    activity_log = null
    antivirus    = null
    block_page   = null
    fips         = null
    certificate = {
      binding_status = "pending_deployment"
      id             = cloudflare_zero_trust_gateway_certificate.gateway.id
      qs_pack_id     = "5e764a76-ce86-44c0-992c-ad835ed42afd"
      updated_at     = "0001-01-01T00:00:00Z"
    }
    tls_decrypt = {
      enabled = false
    }
  }
}

import {
  to = cloudflare_zero_trust_gateway_settings.account
  id = "4611a7a2299c655529452ac81b5f4844"
}
