# bootstrap

Creates the GCS state bucket and the `tf-home-manager` service account +
Workload Identity Federation binding that `terraform/cloudflare-zero-trust/`
runs as in CI. Reuses the `ticklethepanda-web` project's existing shared WIF
pool/provider (`pool/providers/github`, already scoped to
`assertion.repository_owner=='TickleThePanda'`) — no new pool or provider.

## Applying

Manual only, not run from CI (see the top-level plan/CLAUDE.md context) —
this is the layer that creates the bucket everything else depends on, so it
can't depend on that bucket existing yet.

```
gcloud auth application-default login   # panda@ticklethepanda.dev, roles/owner on ticklethepanda-web
terraform init
terraform apply
```

State starts local (`terraform.tfstate`, gitignored). After the first apply,
move this module's own state into the bucket it just created so nothing
stays local long-term:

```
cat >> versions.tf <<'EOF'

terraform {
  backend "gcs" {
    bucket = "ttp-home-tfstate"
    prefix = "bootstrap"
  }
}
EOF
terraform init -migrate-state
```

Re-run `terraform apply` after any future change to this module.
