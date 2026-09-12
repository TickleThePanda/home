output "tfstate_bucket" {
  description = "GCS bucket terraform/cloudflare-zero-trust/ (and this module, after migration) store state in."
  value       = google_storage_bucket.tfstate.name
}

output "tf_home_manager_email" {
  description = "Service account terraform/cloudflare-zero-trust/'s CI workflow authenticates as."
  value       = google_service_account.tf_home_manager.email
}

output "workload_identity_provider" {
  description = "Full resource name to pass as workload_identity_provider in google-github-actions/auth."
  value       = "${var.wif_pool_name}/providers/github"
}
