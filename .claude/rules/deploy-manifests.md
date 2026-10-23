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
