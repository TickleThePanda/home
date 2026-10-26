---
paths:
  - "k8s/**"
---

# Kubernetes manifests

- A single-replica Deployment with a ReadWriteOnce PVC must use `strategy:
  type: Recreate`. RollingUpdate deadlocks, because the new pod can't mount
  the volume until the old one releases it.
- A ConfigMap mounted via `subPath` must come from a `configMapGenerator`.
  `subPath` mounts never refresh; the generator's hashed name forces a
  rollout on each change.
- `storage:` requests are real LV sizes on the Pi's `data` VG. Size claims
  for real; they can grow (`kubectl patch pvc`) but never shrink. No
  `storageClassName` is needed, since `lvm-data` is the default.
- Removing a PVC from the manifests makes Flux prune it, data included.
- Upstream manifests (operator installs, CRDs) go in `vendor.txt`, never as
  URLs in `resources:`. CI rejects remote resources. Give each entry a
  `# renovate:` comment (see the file's header) so Renovate bumps it. Run
  `scripts/vendor.sh` before a local `kustomize build`.
- Give a third-party image an exact, multi-arch version tag. Renovate picks
  it up with no config; add a `packageRules` entry in `renovate.json` only
  for an unusual tag scheme (like Kea's even minors).
- Send app mail through `smtp-relay.smtp-relay.svc:587` as
  `hello@ticklethepanda.dev` (see the relay's README).
- `k8s/apps/home/` sets `namespace: home` once at its top level. Internal apps
  each set their own.

## Adding a service

A new service needs:

- Its own directory with `namespace.yaml` and a `kustomization.yaml`, listed
  in its parent's `resources:` (`k8s/apps/internal/kustomization.yaml`).
- A multi-arch image with an exact version tag (above).
- If it has state, a sized `lvm-data` PVC and `Recreate` strategy (above).
- If it has a web UI, an IngressRoute on `int-web-secure` with `tls: {}`,
  ``Host(`<app>.internal.ticklethepanda.co.uk`)`` and the `tinyauth`
  middleware (`namespace: tinyauth`). No DNS change. Copy
  `k8s/apps/internal/apps/headlamp/ingress.yaml`.
- A Homepage card: `gethomepage.dev/*` annotations on that IngressRoute
  (`enabled`, `href`, `name`, `group`, `icon`, `description`,
  `pod-selector`). `group` is `Applications` or `Administration`, per
  `.claude/rules/homepage.md`. `icon` is a dashboard-icons name or `mdi-*`.
- Its external/base URL set to the internal hostname, if it builds absolute
  links (emails, redirects).
- If it uses another app's Secret, a Reflector mirror, not a copy.
- `prometheus.io/scrape: "true"` (plus `port` / `path` if needed) on the pod
  template. Prometheus discovers it without a config change.
- If internet-facing, an `ext-web` route and a tunnel hostname in
  `terraform/cloudflare-zero-trust/tunnel.tf`.
- A `network-policy.yaml`: `default-deny-ingress` plus allows for Traefik,
  Prometheus and any in-cluster clients. Using a protected service (Postgres,
  SMTP relay, LLDAP) also means adding the app to that service's policy. See
  `k8s/apps/README.md`.
