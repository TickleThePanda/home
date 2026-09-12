resource "cloudflare_zero_trust_device_posture_rule" "gateway" {
  account_id = var.account_id
  name       = "Gateway"
  type       = "gateway"
}

import {
  to = cloudflare_zero_trust_device_posture_rule.gateway
  id = "4611a7a2299c655529452ac81b5f4844/2c8255f9-c240-4eeb-a664-fae05721926f"
}

resource "cloudflare_zero_trust_device_posture_rule" "warp" {
  account_id = var.account_id
  name       = "Warp"
  type       = "warp"
}

import {
  to = cloudflare_zero_trust_device_posture_rule.warp
  id = "4611a7a2299c655529452ac81b5f4844/bf085d31-302a-42a4-8d79-1b15db7dabbe"
}

locals {
  # WARP client split-tunnel excludes shared by the default and custom device
  # profiles below -- standard RFC1918/link-local ranges plus DHCP broadcast,
  # kept out of the tunnel so local traffic doesn't route through WARP.
  device_profile_excludes = [
    { address = "ff05::/16" },
    { address = "ff04::/16" },
    { address = "ff03::/16" },
    { address = "ff02::/16" },
    { address = "ff01::/16" },
    { address = "fe80::/10", description = "IPv6 Link Local" },
    { address = "fd00::/8" },
    { address = "255.255.255.255/32", description = "DHCP Broadcast" },
    { address = "240.0.0.0/4" },
    { address = "224.0.0.0/24" },
    { address = "172.16.0.0/12" },
    { address = "169.254.0.0/16", description = "DHCP Unspecified" },
    { address = "100.64.0.0/10" },
    { address = "10.0.0.0/8" },
  ]
}

resource "cloudflare_zero_trust_device_default_profile" "default" {
  account_id                     = var.account_id
  allow_mode_switch              = false
  allow_updates                  = false
  allowed_to_leave               = true
  auto_connect                   = 0
  captive_portal                 = 180
  disable_auto_fallback          = false
  exclude_office_ips             = false
  register_interface_ip_with_dns = true
  sccm_vpn_boundary_support      = false
  switch_locked                  = false
  dns_search_suffixes            = []
  exclude                        = local.device_profile_excludes

  service_mode_v2 = {
    mode = "warp"
  }
}

import {
  to = cloudflare_zero_trust_device_default_profile.default
  id = "4611a7a2299c655529452ac81b5f4844"
}
