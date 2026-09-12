provider "google" {
  project = var.project_id
}

# State bucket for terraform/cloudflare-zero-trust/ (and, after the migration
# described in README.md, for this bootstrap module's own state too).
resource "google_storage_bucket" "tfstate" {
  name                        = "ttp-home-tfstate"
  project                     = var.project_id
  location                    = var.region
  storage_class               = "STANDARD"
  uniform_bucket_level_access = true
  force_destroy               = false

  versioning {
    enabled = true
  }

  lifecycle_rule {
    condition {
      num_newer_versions = 10
    }
    action {
      type = "Delete"
    }
  }

  lifecycle_rule {
    condition {
      days_since_noncurrent_time = 30
    }
    action {
      type = "Delete"
    }
  }
}

# Terraform identity for the "home" repo's Zero Trust config. Only ever
# touches its own state objects -- never a project-wide role.
resource "google_service_account" "tf_home_manager" {
  project      = var.project_id
  account_id   = "tf-home-manager"
  display_name = "Terraform - ticklethepanda/home"
  description  = "Manages state for terraform/cloudflare-zero-trust/ in github.com/ticklethepanda/home. Authenticated via WIF from GitHub Actions, no key."
}

resource "google_storage_bucket_iam_member" "tf_home_manager_tfstate" {
  bucket = google_storage_bucket.tfstate.name
  role   = "roles/storage.objectAdmin"
  member = google_service_account.tf_home_manager.member
}

# Lets GitHub Actions runs of ticklethepanda/home impersonate tf_home_manager
# via the existing shared WIF pool/provider -- no new pool or provider, no
# service account key.
resource "google_service_account_iam_member" "tf_home_manager_wif" {
  service_account_id = google_service_account.tf_home_manager.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${var.wif_pool_name}/attribute.repository/${var.github_repository}"
}
