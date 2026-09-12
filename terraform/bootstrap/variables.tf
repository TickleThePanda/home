variable "project_id" {
  description = "GCP project holding the state bucket and this repo's Terraform service account."
  type        = string
  default     = "ticklethepanda-web"
}

variable "region" {
  description = "Region for the state bucket, matching the other ttp-* buckets in this project."
  type        = string
  default     = "europe-west1"
}

variable "github_repository" {
  description = "GitHub \"owner/repo\" that terraform/cloudflare-zero-trust/ runs from in CI."
  type        = string
  default     = "ticklethepanda/home"
}

variable "wif_pool_name" {
  description = "Full resource name of the existing shared Workload Identity Pool this project already uses for other Terraform-managed repos."
  type        = string
  default     = "projects/388566333789/locations/global/workloadIdentityPools/pool"
}
