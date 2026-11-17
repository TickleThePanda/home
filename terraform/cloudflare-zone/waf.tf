# Custom rules for var.host. Free allows 5; this uses one per action: block,
# then JS challenge. Each block check is a named entry below and the rule ORs
# them together, so adding or removing a check is one line. Security Events
# names the rule that fired, not the check: read the event's country, method,
# path and user agent.
#
# The Cloudflare Free Managed Ruleset is "always active" zone-wide without an
# http_request_firewall_managed entrypoint. Don't add one: it could replace
# that implicit deployment for ha. too.

locals {
  on_host = "http.host eq \"${var.host}\""

  # Proton polls the calendar feeds from its servers: no browser, so no
  # challenge, and its user agent isn't something to second-guess.
  not_calendar = "not starts_with(http.request.uri.path, \"/calendar/\")"

  # GB, the EU, and EFTA (CH, NO, IS, LI). Listed explicitly because
  # ip.src.is_in_european_union needs a Business plan. CH also covers Proton's
  # calendar fetchers, so the geo check applies to /calendar/ too. WARP egress
  # is geolocated near the client; outside Europe, use the router's Tailscale
  # exit node.
  allowed_countries = [
    "GB", "CH", "NO", "IS", "LI",
    "AT", "BE", "BG", "HR", "CY", "CZ", "DK", "EE", "FI", "FR", "DE", "GR", "HU", "IE",
    "IT", "LV", "LT", "LU", "MT", "NL", "PL", "PT", "RO", "SK", "SI", "ES", "SE",
  ]

  # Scanner targets. Nothing served on this host (/, /share/, /calendar/)
  # uses these paths.
  probe_paths = [
    "/.env",
    "/.git",
    "/wp-",
    "/xmlrpc.php",
    "phpmyadmin",
    "/cgi-bin",
    ".php",
    "/actuator",
    "/vendor/phpunit",
  ]

  # Scanners and scripting clients. Verified bots (cf.client.bot) are exempt.
  bot_user_agents = [
    "sqlmap",
    "nikto",
    "nuclei",
    "masscan",
    "zgrab",
    "python-requests",
    "go-http-client",
    "curl",
    "wget",
  ]

  block_checks = {
    geo = "not ip.src.country in {${join(" ", [for c in local.allowed_countries : "\"${c}\""])}}"
    # Every route on this host is read-only.
    methods = "not http.request.method in {\"GET\" \"HEAD\"}"
    probes  = join(" or ", [for p in local.probe_paths : "lower(http.request.uri.path) contains \"${p}\""])
    bots = join(" and ", [
      local.not_calendar,
      "not cf.client.bot",
      "(http.user_agent eq \"\" or ${join(" or ", [for ua in local.bot_user_agents : "lower(http.user_agent) contains \"${ua}\""])})",
    ])
  }
}

resource "cloudflare_ruleset" "custom" {
  zone_id     = var.zone_id
  name        = "default"
  description = "Custom rules for ${var.host}"
  kind        = "zone"
  phase       = "http_request_firewall_custom"

  rules = [
    {
      ref         = "home_block"
      description = "home: block (${join(", ", keys(local.block_checks))})"
      action      = "block"
      expression  = "${local.on_host} and (${join(" or ", [for c in values(local.block_checks) : "(${c})"])})"
      enabled     = true
    },
    {
      ref         = "home_js_challenge"
      description = "home: JS challenge (all but /calendar/)"
      action      = "js_challenge"
      expression  = "${local.on_host} and ${local.not_calendar}"
      enabled     = true
    },
  ]
}
