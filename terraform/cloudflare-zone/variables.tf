variable "zone_id" {
  description = "ticklethepanda.co.uk zone ID."
  type        = string
  default     = "6ded6d575af2616633c40ab328907c15"
}

variable "host" {
  description = "The only hostname these rules apply to. Every rule is scoped to it, so the zone's other hostnames (ha.) are unaffected."
  type        = string
  default     = "home.ticklethepanda.co.uk"
}
