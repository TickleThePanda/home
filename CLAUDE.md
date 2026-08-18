# Home infrastructure monorepo

Single-node k3s cluster running the owner's home services on a Raspberry Pi,
plus the apps deployed onto it. See `README.md` for the network map (node
IPs, ingress IPs, etc.) — this file covers operational context that isn't
visible from the code alone.

## Documentation

Write documentation that is simple, concise, and practical.

- Prefer short sentences and short paragraphs.
- Explain only what the reader needs to use or understand the feature.
- Do not explain obvious implementation details.
- Do not repeat information in different words.
- Avoid introductory or concluding filler.
- Avoid phrases like "This allows you to..." when the code or heading already makes that clear.
- Prefer a concrete example over a long explanation.
- Use bullets only when they make the content easier to scan.
- Keep each section as short as possible without losing necessary information.
- Do not document every edge case unless it is important to normal usage.
- Assume the reader is a competent software developer.

## Cluster

- Node: `k8s-manager-1`, `192.168.1.2`, Raspberry Pi (arm64), single node
  acting as both control-plane and worker. k3s.
- SSH: `ssh 192.168.1.2` as user `panda` (key-based, no host alias
  configured locally — use the IP).
- `panda` has scoped, passwordless sudo on the node
  (`/etc/sudoers.d/panda-k3s-admin`) for k3s service control and reading/
  editing k3s's own manifest/config directories. Anything broader (general
  root shell, package installs) needs the owner interactively — kept
  narrow deliberately, since those directories are close to root-equivalent
  (k3s's server process treats their contents as trusted input).
- The local kubeconfig's client cert doesn't auto-rotate. If `kubectl`
  reports "server has asked for the client to provide credentials", restart
  k3s on the node and re-pull `/etc/rancher/k3s/k3s.yaml` into
  `~/.kube/config` (fix the `server:` field to the node's IP, it defaults
  to `127.0.0.1`).

## Networking

- **MetalLB** (L2 mode) hands out LoadBalancer IPs from pools: `external`
  (WAN-facing ingress), `internal` (LAN-only ingress), `pihole`, `kube-api`.
- **Traefik** is the sole ingress controller. Internal-only apps use
  `Host(`<app>.internal.ticklethepanda.co.uk`)` on the `int-web-secure`
  entrypoint (TLS via a wildcard cert), routed through the `internal`
  MetalLB pool. Externally-reachable apps use the `ext-web` entrypoint via
  the `external` pool.
- **tinyauth** forward-auth gates most internal apps, backed by
  **pocket-id** (OIDC) + **lldap**.
- **pihole** is internal DNS. **cloudflared** tunnels select services out
  to the public internet without opening router ports.

## Deploy pattern

- `.github/workflows/deploy.yaml` is the single pipeline for both layers,
  running three sequential jobs on push to `deploy/**` or `node/**`:
  `check` (the k3s/Traefik pin guard — no secrets, fails fast), then `node`
  (Ansible, see below), then `cluster`. Order matters: a k3s upgrade changes
  which Traefik chart tarball the node serves, so the node must move before
  the manifests that reference it.
- The `cluster` job applies everything under `deploy/` via kustomize:
  `kubectl apply -k deploy --prune -l ticklethepanda.dev/managed-by=kustomize`
- Layout: `deploy/setup/` (cluster infra — cert-manager, metallb, traefik,
  api-proxy, cloudflared, each a self-contained kustomization),
  `deploy/internal/<app>/` (internal-only apps, each self-contained with its
  own `namespace:`), `deploy/home/` (externally-reachable apps — `namespace:
  home` is set once on `deploy/home/kustomization.yaml` itself, not per
  app, so scope applies to `deploy/home`, not `deploy/home/<app>`).
- For a targeted fix, prefer a scoped apply over the full `-k deploy
  --prune` — the full run reconciles the entire repo at once which is slow
  and can hit intermittent issues. Do not scope the apply with a direct
  `kubectl apply -f <file>` or `kubectl apply -k deploy/<subdir>`. Instead
  use `./apply-subset.sh <deploy-subdir> [kubectl apply args...]`.
- A single-replica Deployment with a ReadWriteOnce PVC must set `strategy:
  type: Recreate`. The default RollingUpdate waits for the new pod before
  killing the old one, but the new pod can't mount the volume until the old
  one releases it — permanent deadlock at `replicas: 1`.
- A ConfigMap mounted via `subPath` must be defined with a
  `configMapGenerator` in that app's `kustomization.yaml`, not a plain
  `ConfigMap` resource. `subPath` mounts never live-refresh, and a
  generator's content-hashed name changes the pod template on every content
  change, forcing an actual rollout instead of silently going stale.

## Apps

- `apps/<app>/` holds the source for the images this repo builds
  (`home-root`, `internal-index`, `odinbot`). Each has its own
  `.github/workflows/build--<app>.yml`, triggered on `apps/<app>/**`, which
  builds from that directory as the Docker context, pushes
  `ticklethepanda/<app>:latest`, and restarts the Deployment. The manifests
  for these apps live under `deploy/` like any other app — nothing in
  `deploy/` references the source paths.

## Node pattern

- Everything under `node/` is the layer *below* `deploy/`: the k3s version,
  k3s's own config files, the directories backing local-volume PVs, and the
  sudoers entries. Applied with Ansible by the `node` job in
  `.github/workflows/deploy.yaml`, which connects as the `deploy` user over
  SSH (key in the `NODE_SSH_KEY` secret). Don't hand-edit these on the node —
  the playbook purges `config.yaml.d` files it doesn't manage.
- **Bumping k3s is an edit to `node/vars/versions.yml`**, nothing more. But
  it must move in the same commit as
  `deploy/setup/traefik/traefik-helm-chart.yaml`: k3s only serves the Traefik
  chart tarball bundled with the *installed* version, so the two pins are
  coupled. `node/scripts/check-traefik-pin.sh` enforces this in CI and
  `--online` verifies the pair against the k3s release manifest.
- Kubernetes doesn't support skipping minor versions — upgrade one at a time.
- **CI's own transport runs through the cluster.** The in-cluster
  `cloudflared` Deployment is the Cloudflare Zero Trust private-network
  connector (its logs show `originService=warp-routing` carrying
  `192.168.1.2:22` and `:6443`). Restarting k3s therefore disturbs the very
  connection the playbook runs over, which is why the restart handler
  detaches via `systemd-run` and the upgrade runs as its own detached unit
  writing to a status file. Don't "simplify" either into a plain
  `systemctl restart`.
- **An open 6443 is not a ready API.** k3s binds the port well before it
  serves, and `kubectl` fails immediately against an unavailable API rather
  than respecting `--timeout`. Readiness gates must poll
  `k3s kubectl get --raw /readyz` with retries — the first CI run failed
  exactly here. Running pods *do* survive a k3s server restart (containerd
  keeps them up), so cloudflared itself normally stays Ready throughout.
- The corollary: **when k3s is down, CI cannot reach the node at all.**
  Recovery is LAN-local — see `node/RECOVERY.md`.
- The workflow holds `concurrency: group: cluster` so two runs can never
  touch the cluster at once. The `node` job runs on every push to either
  path, not just `node/**` — it's idempotent, and a no-op run doubles as
  drift detection.

## Storage

`node/STORAGE.md` is the reference. The essentials:

- Most of the SSD is `sda3`, one LVM volume group named `data`. Four LVs hold
  node state (sized in `node/vars/storage.yml`); the unallocated ~327G **is
  the PersistentVolume pool**, not spare capacity. Don't pre-allocate it.
- **Growing a volume is an edit to `node/vars/storage.yml`.** It extends
  online. Shrinking fails.
- **Ansible does not own the partition table or the VG**, and must not — CI
  reaches this node through a pod running on it, so a bad repartition has no
  remote recovery.
- `fstab` uses `nofail` and a k3s drop-in sets `RequiresMountsFor`, so a
  missing volume leaves the Pi reachable but stops k3s. The alternative is k3s
  writing a second copy of its state to root and looking healthy.
- **`lvm-data` is the default StorageClass** and the only one — every volume is
  a real LV. `local-path` and the static `local-storage` PVs in `/mnt/disk` are
  gone, and `local-storage` is in `k3s_disable`. A PVC needs no
  `storageClassName` at all now.
- **A `storage:` request is now a real device size.** Under `local-path` it was
  advisory, so claims drifted far past it — `pihole-etc` asked for `128Mi` while
  holding 422M. Under `lvm-data` that would be ENOSPC. Size claims for real, and
  grow them with `kubectl patch pvc` (the class is `allowVolumeExpansion: true`).
- **A StatefulSet's `volumeClaimTemplate` PVC is a `--prune` target.** The
  controller stamps `spec.selector.matchLabels` — which includes
  `ticklethepanda.dev/managed-by` — onto the PVCs it creates, but those PVCs are
  not in the applied set, so the pipeline's prune deletes them. Declare such a
  claim explicitly alongside the StatefulSet; see
  `deploy/internal/pocketid/volume.yaml`.
- The pre-migration copies still sit on the root filesystem under `/mnt/disk`
  and `/var/lib/rancher/k3s/storage` (~700M). They are the rollback — see the
  end of `node/STORAGE.md` for removing them.

## Administration notes

- **k3s's own bundled addons can conflict with kustomize-managed
  resources of the same name.** k3s watches
  `/var/lib/rancher/k3s/server/manifests/` on the node and re-applies
  whatever's there on every restart, independent of CI. If the repo
  manages a resource that k3s also bundles by default (this happened with
  Traefik), disable that addon in `/etc/rancher/k3s/config.yaml.d/`
  (`disable: [traefik]`, alongside `servicelb`) and vendor the full
  resource into the repo so there's a single owner. `deploy/setup/traefik/`
  is the current example of this pattern.
- **Disabling a k3s addon triggers a real Helm uninstall**, which can
  cascade — e.g. removing Traefik's addon deleted its CRDs, which
  garbage-collected every `IngressRoute`/`Middleware`/`TLSStore` cluster-
  wide, not just Traefik's own resources. After a change like this,
  expect to reapply the affected app manifests and re-check TLS (a missing
  `TLSStore` silently falls back to a self-signed cert rather than erroring).
- **Objects can get stuck `Terminating` and drift invisibly for a long
  time** if their owning controller can't finish cleanup — `kubectl apply`
  will keep "succeeding" without the live object actually changing. Spot
  check: `kubectl get svc -A -o json | jq -r '.items[] | select(.metadata.deletionTimestamp != null) | "\(.metadata.namespace)/\(.metadata.name)"'`
  (swap `svc` for other kinds). Clearing a stuck finalizer is a real
  mutation — confirm with the owner before doing it.
- **`raw.githubusercontent.com` in a kustomize `resources:` list is
  fragile** — it can get misparsed as a git-clone target and fail with
  `429`/`503` during GitHub hiccups. Check
  `https://www.githubstatus.com/api/v2/status.json` before assuming it's
  local throttling rather than a real outage.
