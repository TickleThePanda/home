---
paths:
  - "deploy/**"
  - "flux-system/**"
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
- A new third-party image gets an `ImageRepository` + `ImagePolicy` in
  `flux-system/sync/images.yaml` and a `# {"$imagepolicy": "flux-system:<name>"}`
  marker on its image line.
- An app that sends mail goes through `smtp-relay.smtp-relay.svc:587` as
  `hello@ticklethepanda.dev` (see the relay's README).
- `deploy/home/` sets `namespace: home` once at its top level. Internal apps
  each set their own.

## Adding a service

- [ ] **Kustomize:** own directory with `namespace.yaml` and a
  `kustomization.yaml`, listed in its parent's `resources:`
  (`deploy/internal/kustomization.yaml`).
- [ ] **Image:** multi-arch, with an `ImageRepository` + `ImagePolicy` and
  marker (above).
- [ ] **Storage:** sized `lvm-data` PVC and `Recreate` strategy (above).
- [ ] **Ingress:** an IngressRoute on `int-web-secure` with `tls: {}`,
  ``Host(`<app>.internal.ticklethepanda.co.uk`)`` and the `tinyauth`
  middleware (`namespace: tinyauth`). No DNS change. Copy
  `deploy/internal/apps/headlamp/ingress.yaml`.
- [ ] **Homepage card:** `gethomepage.dev/*` annotations on that
  IngressRoute (`enabled`, `href`, `name`, `group`, `icon`, `description`,
  `pod-selector`). `group` is `Applications` or `Administration`, per
  `.claude/rules/homepage.md`. `icon` is a dashboard-icons name or `mdi-*`.
- [ ] **Base URL:** if the app builds absolute links (emails, redirects),
  set its external/base URL to the internal hostname.
- [ ] **Secrets:** credentials owned by another app are mirrored with
  Reflector, not copied.
- [ ] **Metrics:** `prometheus.io/scrape: "true"` (plus `port` / `path` if
  needed) on the pod template. Prometheus picks it up with no config change.
- [ ] **Internet-facing:** an `ext-web` route, plus a tunnel hostname in
  `terraform/cloudflare-zero-trust/tunnel.tf`.
