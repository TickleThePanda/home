# client_secret is only returned by the API at creation or at a rotation
# (a client_secret_version bump); it's never returned by a plain read. To
# rotate a token, bump its client_secret_version and set
# previous_client_secret_expires_at to the grace-period end for the old
# secret, then read the new value from outputs.tf.

# Named "github actions" but not the token CI's WARP client actually
# authenticates with -- see access.tf's github_actions_service_token policy,
# which is bound to "gha2" below, not this one.
resource "cloudflare_zero_trust_access_service_token" "github_actions" {
  account_id = var.account_id
  name       = "github actions"
  duration   = "forever"
  enabled    = true
}

import {
  to = cloudflare_zero_trust_access_service_token.github_actions
  id = "accounts/4611a7a2299c655529452ac81b5f4844/65bbd9c4-ec75-4826-95fe-42f5a6b5a68d"
}

# CI's WARP client authenticates with this token's current secret
# (CLOUDFLARE_AUTH_CLIENT_ID/_SECRET in GitHub Actions).
resource "cloudflare_zero_trust_access_service_token" "gha2" {
  account_id = var.account_id
  name       = "gha2"
  duration   = "forever"
  enabled    = true

  client_secret_version             = 2
  previous_client_secret_expires_at = "2026-09-12T16:55:01Z"
}

import {
  to = cloudflare_zero_trust_access_service_token.gha2
  id = "accounts/4611a7a2299c655529452ac81b5f4844/ee59b92e-76c2-4ad8-92a3-eba66ebc9fae"
}
