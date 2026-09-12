output "gha2_client_id" {
  description = "CLOUDFLARE_AUTH_CLIENT_ID -- CI's WARP client authenticates with this."
  value       = cloudflare_zero_trust_access_service_token.gha2.client_id
}

output "gha2_client_secret" {
  description = "CLOUDFLARE_AUTH_CLIENT_SECRET -- re-sync the GitHub Actions secret whenever this changes (first apply after import, or any deliberate client_secret_version bump)."
  value       = cloudflare_zero_trust_access_service_token.gha2.client_secret
  sensitive   = true
}
