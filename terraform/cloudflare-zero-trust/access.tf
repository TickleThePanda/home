resource "cloudflare_zero_trust_access_identity_provider" "onetimepin" {
  account_id = var.account_id
  name       = ""
  type       = "onetimepin"
  config     = {}
}

import {
  to = cloudflare_zero_trust_access_identity_provider.onetimepin
  id = "accounts/4611a7a2299c655529452ac81b5f4844/80c050c9-6616-4711-8593-12082f442dca"
}

resource "cloudflare_zero_trust_access_policy" "any_trusted_device" {
  account_id = var.account_id
  name       = "Any trusted device"
  decision   = "bypass"
  connection_rules = {
    rdp = {}
  }
  include = [
    {
      device_posture = {
        integration_uid = cloudflare_zero_trust_device_posture_rule.gateway.id
      }
    },
    {
      device_posture = {
        integration_uid = cloudflare_zero_trust_device_posture_rule.warp.id
      }
    },
  ]
}

import {
  to = cloudflare_zero_trust_access_policy.any_trusted_device
  id = "4611a7a2299c655529452ac81b5f4844/49841253-b0d6-469e-ba91-713c1911a1df"
}

resource "cloudflare_zero_trust_access_policy" "anthropic_ips" {
  account_id = var.account_id
  name       = "Anthropic IPs"
  decision   = "bypass"
  connection_rules = {
    rdp = {}
  }
  include = [{
    ip = {
      ip = "160.79.104.0/21"
    }
  }]
}

import {
  to = cloudflare_zero_trust_access_policy.anthropic_ips
  id = "4611a7a2299c655529452ac81b5f4844/987da7f2-0972-4a15-b2db-a23b4ecad2f7"
}

resource "cloudflare_zero_trust_access_policy" "ticklethepanda_dev_emails" {
  account_id       = var.account_id
  name             = "ticklethepanda.dev emails"
  decision         = "allow"
  session_duration = "24h"
  include = [{
    email_domain = {
      domain = "ticklethepanda.dev"
    }
  }]
}

import {
  to = cloudflare_zero_trust_access_policy.ticklethepanda_dev_emails
  id = "4611a7a2299c655529452ac81b5f4844/e88aa34b-f52c-4e4e-b122-de7996dd8166"
}

# Named "github actions" but actually keyed on the "gha2" service token --
# the standalone "github actions" token (service_tokens.tf) isn't referenced
# by any policy. Reflects the live config, not a naming choice made here.
resource "cloudflare_zero_trust_access_policy" "github_actions_service_token" {
  account_id       = var.account_id
  name             = "github actions"
  decision         = "non_identity"
  session_duration = "24h"
  include = [{
    service_token = {
      token_id = cloudflare_zero_trust_access_service_token.gha2.id
    }
  }]
}

import {
  to = cloudflare_zero_trust_access_policy.github_actions_service_token
  id = "4611a7a2299c655529452ac81b5f4844/d90bad76-3d57-4227-a715-62dbd924599b"
}

resource "cloudflare_zero_trust_access_application" "ha" {
  account_id                 = var.account_id
  name                       = "ha"
  domain                     = "ha.ticklethepanda.co.uk"
  type                       = "self_hosted"
  session_duration           = "24h"
  app_launcher_visible       = true
  auto_redirect_to_identity  = false
  enable_binding_cookie      = false
  http_only_cookie_attribute = false
  options_preflight_bypass   = false

  destinations = [{
    type = "public"
    uri  = "ha.ticklethepanda.co.uk"
  }]

  policies = [
    { id = cloudflare_zero_trust_access_policy.any_trusted_device.id },
    { id = cloudflare_zero_trust_access_policy.anthropic_ips.id },
  ]
}

import {
  to = cloudflare_zero_trust_access_application.ha
  id = "accounts/4611a7a2299c655529452ac81b5f4844/79353bea-e2c0-4cee-bb4a-d0f70d53e198"
}

resource "cloudflare_zero_trust_access_application" "warp_login" {
  account_id                = var.account_id
  name                      = "Warp Login App"
  domain                    = "ticklethepanda.cloudflareaccess.com/warp"
  type                      = "warp"
  session_duration          = "24h"
  auto_redirect_to_identity = false

  policies = [
    { id = cloudflare_zero_trust_access_policy.ticklethepanda_dev_emails.id },
    { id = cloudflare_zero_trust_access_policy.github_actions_service_token.id },
  ]
}

import {
  to = cloudflare_zero_trust_access_application.warp_login
  id = "accounts/4611a7a2299c655529452ac81b5f4844/2f0b83c1-f235-4fc5-9fa9-ebbce03178e1"
}
