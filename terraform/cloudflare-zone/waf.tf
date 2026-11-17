# Custom rules for var.host. Free allows 5; this uses one per action. Each
# check is a named entry below and the rule ORs them together, so adding or
# removing a check is one line. Security Events names the rule that fired,
# not the check: read the event's country, method, path and user agent.
#
# The Cloudflare Free Managed Ruleset is "always active" zone-wide without an
# http_request_firewall_managed entrypoint. Don't add one: it could replace
# that implicit deployment for ha. too.

locals {
  on_host = "http.host eq \"${var.host}\""

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

  block_checks = {
    # GB, the EU and the rest of Europe's EEA/EFTA. CH also covers Proton's
    # calendar fetchers, so this applies to /calendar/ too. WARP egress is
    # geolocated near the client; outside Europe, use the router's Tailscale
    # exit node.
    geo = "not ip.src.is_in_european_union and not ip.src.country in {\"GB\" \"CH\" \"NO\" \"IS\" \"LI\"}"
    # Every route on this host is read-only.
    methods = "not http.request.method in {\"GET\" \"HEAD\"}"
    probes  = join(" or ", [for p in local.probe_paths : "lower(http.request.uri.path) contains \"${p}\""])
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
  ]
}
