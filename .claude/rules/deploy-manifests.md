---
paths:
  - "deploy/**"
  - "flux-system/**"
  - "apply-subset.sh"
---

# Kubernetes manifests

## Layout

- `deploy/setup/`: cluster infra (cert-manager, MetalLB, Traefik,
  cloudflared, CNPG, OpenEBS `lvm-localpv`, multus, reflector, ...), each a
  self-contained kustomization.
- `deploy/setup-config/`: CRs that need `setup`'s controllers running
  first (ClusterIssuer, MetalLB pools).
- `deploy/internal/<group>/<app>/`: internal-only apps, grouped into
  `apps/`, `auth/`, `network/`, `services/`, `monitoring/`, plus a top-level
  `homepage/`. Each sets its own `namespace:`.
- `deploy/home/`: externally reachable apps. `namespace: home` is set once
  on `deploy/home/kustomization.yaml`, so scope applies to `deploy/home`,
  not `deploy/home/<app>`.
- `flux-system/install` (Flux itself) and `flux-system/sync` (the
  OCIRepository, Kustomizations, image automation, notifications).

## Rules

- A single-replica Deployment with a ReadWriteOnce PVC must use `strategy:
  type: Recreate`. RollingUpdate deadlocks: the new pod can't mount the
  volume until the old one releases it.
- A ConfigMap mounted via `subPath` must come from a `configMapGenerator`,
  not a plain `ConfigMap`. `subPath` mounts never refresh. The generator's
  hashed name forces a rollout on every change.
- Third-party images must be multi-arch too.

## Image automation

- Each image line carries `# {"$imagepolicy": "flux-system:<name>"}`. Flux
  commits new minor/patch tags straight to `main` (config in
  `flux-system/sync/images.yaml` and `image-automation.yaml`), which runs
  the deploy pipeline like any push.
- Majors are manual: edit the tag and the policy's `range` together. To add
  an image, add an `ImageRepository`, an `ImagePolicy` and the marker.
- Not covered: `ticklethepanda/*:latest`, images tied to a vendored manifest
  or the k3s version (Traefik, cert-manager, MetalLB, ...), the CNPG
  `imageName`, and lldap (unreleased build).
- A bad bump shows as a red commit status after the 5m `wait`. `Recreate`
  apps are down in the meantime. Roll back by reverting the commit **and**
  excluding the version in its policy, or Flux re-bumps it. Apps that
  migrate their DB on start may not survive a rollback.

## Storage

- `lvm-data` is the only StorageClass and the default, so a PVC needs no
  `storageClassName`. Every claim is a real LV carved from the Pi's `data`
  VG.
- A `storage:` request is a real device size. Size it for real, and grow
  it with `kubectl patch pvc` (`allowVolumeExpansion: true`). LVs can't
  shrink.
- CNPG finds its PVCs only through a controller `ownerReference` to the
  `Cluster`. A PVC recreated by hand without one wedges the operator. It
  then loops silently with a stale phase (e.g. "Instance Status Extraction
  Error: HTTP communication issue") and never goes Ready, which also fails
  Flux's `internal` health check. Carry the ownerReference over when
  migrating a CNPG volume.
