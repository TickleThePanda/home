# cloudflare-zero-trust

Cloudflare Zero Trust (tunnel, Access, service tokens, device posture,
Gateway) for this account -- not the DNS zones. State lives in
`ttp-home-tfstate` (see `bootstrap/terraform/`).

- `tunnel.tf` -- the "home" `cloudflared` tunnel, its ingress config, and the
  private-network routes CI and the router's dashboard use to reach the LAN.
- `access.tf` -- the "ha" and "Warp Login App" Access applications, their
  policies, and the one-time-PIN identity provider.
- `service_tokens.tf` -- the two Access service tokens; see its header
  comment before touching `client_secret_version`.
- `devices.tf` -- device posture rules and the default WARP client profile.
- `gateway.tf` -- the Gateway TLS certificate and account-level settings.

## Applying

CI applies this as the `terraform` job in `.github/workflows/deploy-terraform.yaml`
(called from `deploy.yaml`), last after `preflight`/`infra`/`cluster` -- this directory controls the
tunnel CI's own WARP connection depends on, so it runs where a blip can't
strand a layer that hasn't run yet. Authenticated to the GCS backend via
Workload Identity Federation and to Cloudflare via a `CLOUDFLARE_API_TOKEN`
repo secret.

For a local `plan`/`apply`, use a Cloudflare API token scoped to this account
with at least: Cloudflare Tunnel Write, Access: Apps and Policies Write,
Access: Organizations/Identity Providers/Groups Write, Access: Service
Tokens Write, Access: Device Posture Write, Zero Trust Write.

```
export CLOUDFLARE_API_TOKEN=...
terraform init
terraform plan
```

## Rotating a service token secret

Bump that resource's `client_secret_version` in `service_tokens.tf` and set
`previous_client_secret_expires_at` to when the old secret should stop
working, then `apply` and read the new secret from `terraform output
gha2_client_secret`. For `gha2`, re-sync GitHub Actions' `CLOUDFLARE_AUTH_CLIENT_SECRET`
(`prod` environment) immediately after -- it's what CI's WARP client
authenticates with to reach the LAN, and the old secret stops working per
`previous_client_secret_expires_at`. Do this locally, not from CI -- the
secret should never appear in a workflow log.
