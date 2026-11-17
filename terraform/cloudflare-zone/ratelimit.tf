# Free allows one rate-limit rule, with a fixed 10s period and 10s block,
# counted per IP per Cloudflare data centre. A page load is under 20 requests;
# Proton polls each calendar feed every few hours.
resource "cloudflare_ruleset" "ratelimit" {
  zone_id     = var.zone_id
  name        = "default"
  description = "Rate limiting for ${var.host}"
  kind        = "zone"
  phase       = "http_ratelimit"

  rules = [
    {
      ref         = "home_ratelimit"
      description = "home: 60 requests / 10s per IP"
      action      = "block"
      expression  = local.on_host
      enabled     = true
      ratelimit = {
        characteristics     = ["ip.src", "cf.colo.id"]
        period              = 10
        requests_per_period = 60
        mitigation_timeout  = 10
      }
    },
  ]
}
