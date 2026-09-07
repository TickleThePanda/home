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

## Code comments

Most of the time, we shouldn't need a comment. Prefer clear, self-explanatory code.

When comments are necessary:

* Describe the current code, not the history of how it was developed.
* Do not let conversation context leak into comments.
* Do not mention this conversation, prior prompts, requested changes, or the sequence of decisions
  that led to the final code.
* Explain only non-obvious behaviour, constraints, invariants, or surprising implementation choices
  that a future reader of the code needs to know.
* Do not document previous implementations, rejected approaches, debugging history, or decision history.
* Assume the reader only needs to understand the final state of the code.
* Keep comments concise.


## Cluster

- **Server**: `k8s-manager-1`, `192.168.1.2`, Raspberry Pi (arm64) — the sole
  k3s server (control-plane), SQLite datastore. Also schedules workloads.
- **Agents**: `k3s-vm-control-01` / `-worker-01` / `-worker-02`
  (`192.168.1.32`–`.34`), amd64 Debian 13 VMs on `proxmox-01`.
  `k3s-vm-control-01` is cordoned — reserved for a future promotion to
  server (which needs a SQLite→etcd datastore migration first).
- k3s on every node is managed by `k3s-cluster/` (the `k3s.orchestration`
  collection), not `node/`. `node/` owns only the layer below k3s on the Pi.
- SSH: `ssh 192.168.1.2` as `panda` (Pi); the VMs have only a `deploy` user
  (the CI key). Key-based, no host aliases — use the IPs.
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

- **Five VLANs**, routed by the gateway, each its own untagged bridge (no
  802.1q):
  - `homelab` (`192.168.1.0/24`, gw `.1`, `br-lan`) — node, Home Assistant,
    all cluster IPs. Everything cluster-side stays here — MetalLB L2 only ARPs
    on the node's segment.
  - `trusted` (`192.168.10.0/24`, `br-trusted`) — the `It reaches out` Wi-Fi
    SSIDs and the LAN4 jack. `homelab` ↔ `trusted` is **fully open for now**
    (separation planned, not enforced).
  - `iot_inet` / `iot_local` / `iot_echo` (`192.168.20/30/40.0/24`,
    `br-iot-inet` / `-local` / `-echo`) — IoT devices, Wi-Fi only. Two 2.4 GHz
    **PPSK** SSIDs (`It reaches out (IoT)` client-isolated, `It reaches out
    (IoT peers)` not): full `wpad-mbedtls`, `config wifi-station` maps each
    key to a `vid`, `config wifi-vlan` bridges that vid, `dynamic_vlan=2` so
    an unclassified station is rejected (the `br-iot-park` base network is a
    dead end). `homelab` / `trusted` / tailnet may initiate into every IoT
    VLAN; **IoT may not initiate back, and IoT VLANs can't reach each other**
    (`tasks/firewall.yml`: each is `input REJECT` + `forward REJECT`, with
    narrow rules for DHCP relay, DNS to Pi-hole, NTP, mDNS). Only `iot_inet` /
    `iot_echo` get a route to the WAN. Punch further holes per device.
- **`homelab` address bands** (`192.168.1.0/24`, one flat subnet): `.1`
  gateway, `.2–.9` physical hosts, `.10–.31` cluster service addresses
  (MetalLB VIPs + Kea), `.32–.63` Proxmox guests, `.64–.127` other static
  devices, `.128–.254` DHCP pool. Convention only — the sole enforcers are the
  Kea pool (`dhcp4.json` subnet 1) and the MetalLB pool addresses
  (`deploy/setup-config/metallb/`). Put a new static address or MetalLB pool
  in its band. Full assignment list is in `README.md`.
- **DHCP**: Kea (`deploy/internal/network/dhcp-kea/`) serves every subnet. It
  is L2-attached to `homelab` via macvlan; the other VLANs reach it through
  **dnsmasq DHCP relays on the router** (`192.168.<n>.1` → `192.168.1.11`,
  relay-only, DNS listener off), and Kea picks the subnet by `giaddr`. Reverse
  DNS for each subnet is a separate zone (`<n>.168.192.in-addr.arpa`) wired
  through BIND, Unbound, ExternalDNS and Kea DDNS; forward names all land in
  `home.arpa` regardless of VLAN. Off-homelab clients get DNS `192.168.1.10`
  and resolve via routed access to Pi-hole (`listeningMode=ALL`, so it answers
  off-subnet). IoT VLANs also get NTP from the router (`192.168.<n>.1`;
  `system.ntp.enable_server`).
- **mDNS**: an Avahi reflector on the router (`router/ansible/tasks/mdns-reflector.yml`)
  forwards mDNS between `br-lan`, `br-trusted` and every `br-iot-*` — link-local
  multicast does not route on its own. This is what lets Home Assistant
  (`homelab`) rediscover ESPHome / IoT devices on the Wi-Fi VLANs after a DHCP
  address change, instead of waiting out the stale record's TTL. It also
  publishes `gateway.local` (the router's per-VLAN address) on every bridge.
  The reflector is hub-and-all, not per-pair, so mDNS *records* are visible
  between IoT VLANs too — accepted; the firewall still blocks all L3 traffic
  between them.
- **MetalLB** (L2 mode) hands out LoadBalancer IPs from pools: `external`
  (WAN-facing ingress), `internal` (LAN-only ingress), `pihole`.
- **Traefik** is the sole ingress controller. Internal-only apps use
  `Host(`<app>.internal.ticklethepanda.co.uk`)` on the `int-web-secure`
  entrypoint (TLS via a wildcard cert), routed through the `internal`
  MetalLB pool. Externally-reachable apps use the `ext-web` entrypoint via
  the `external` pool.
- **tinyauth** forward-auth gates most internal apps, backed by
  **pocket-id** (OIDC) + **lldap**.
- Internal DNS chain: clients → **Pi-hole** (`deploy/internal/network/pihole/`,
  filtering/blocklists) → **Unbound** (`deploy/internal/network/unbound/`,
  recursive resolver) → **CoreDNS** (`deploy/internal/network/coredns/`, authoritative
  for `internal.ticklethepanda.co.uk` only). Unbound stub-zones that one
  domain to CoreDNS; every other query recurses normally. **cloudflared**
  tunnels select services out to the public internet without opening
  router ports.
- **`internal.ticklethepanda.co.uk` hostnames live in the CoreDNS zone
  file** (`deploy/internal/network/coredns/zones/internal.ticklethepanda.co.uk.zone`),
  not in Pi-hole. Its wildcard (`*`) already resolves any undefined
  subdomain to the Traefik internal-ingress IP, so **a new
  `deploy/internal/<app>/` with a normal `Host(`<app>.internal...`)`
  IngressRoute needs no DNS change.** Only touch the zone file for an
  exception — a name that must resolve somewhere other than the internal
  ingress IP (e.g. a service not fronted by Traefik) — by adding an
  explicit `A` record above the wildcard and bumping the SOA serial, then
  `./apply-subset.sh deploy/internal/network/coredns`.
- Pi-hole's own local DNS records and its other non-default settings are
  code-owned: `deploy/internal/network/pihole/*.env` / `*.txt` generate
  `FTLCONF_*` env vars via `configMapGenerator` + `envFrom`.
  `misc.readOnly` is also forced `true`, which blocks *all* `pihole.toml`
  changes (not just the env-forced fields) via the UI, API, or CLI —
  settings only change through this repo now. The Admin UI stays
  available for everything that isn't `pihole.toml`-backed: query log,
  stats, block/allow list management. **The UI only greys out fields that
  are individually env-forced** (`settings.js` checks each field's own
  `flags.env_var`, not the global `misc.readOnly`) — a setting not covered
  by one of the files below still looks editable in Settings, but saving
  it fails server-side with "config is currently in read-only mode". Not a
  bug, just a Pi-hole frontend gap. To add/change a setting: edit the
  relevant committed file (a new array setting gets its own `.txt` file
  wired into the `pihole-config` generator in
  `deploy/internal/network/pihole/kustomization.yaml`; a new scalar setting gets a
  line in `pihole.env`), then `./apply-subset.sh deploy/internal/network/pihole`.
  `dns-hosts.txt` should stay limited to names outside
  `internal.ticklethepanda.co.uk` (e.g. the router) — that domain's records
  belong in the CoreDNS zone file above.

## Deploy pattern

- `.github/workflows/deploy.yaml` is the single pipeline, running sequential
  jobs on push to `deploy/**`, `node/**`, `router/ansible/**`, `proxmox/**`
  or `k3s-cluster/**`: `preflight` (the k3s/Traefik pin guard + tunnel
  reachability — no secrets, fails fast), then `node` (Ansible, Pi below
  k3s), then `router` (gateway, see `router/ansible/`), then `proxmox`
  (hypervisor apt repos + the k3s VMs, see `proxmox/`), then `k3s-cluster`
  (k3s on every node, see `k3s-cluster/`), then `cluster`. Order matters:
  `proxmox` creates the VMs `k3s-cluster` joins; `node` lands the Pi's
  storage mounts before k3s; a k3s upgrade changes which Traefik chart
  tarball the node serves, so `k3s-cluster` moves before the manifests that
  reference it. `node`, `router`, `proxmox`, `k3s-cluster` and `cluster` each
  skip when their own tree is unchanged (`cluster` = `deploy/**` +
  `flux-system/**`; all also trigger on the workflow file itself); a skipped
  job counts as a pass for the jobs that follow, and a manual
  `workflow_dispatch` runs them all.
- The `cluster` job applies everything under `deploy/` via kustomize:
  `kubectl apply -k deploy --prune -l ticklethepanda.dev/managed-by=kustomize`
- Layout: `deploy/setup/` (cluster infra — cert-manager, metallb, traefik,
  cloudflared, each a self-contained kustomization),
  `deploy/internal/<app>/` (internal-only apps, each self-contained with its
  own `namespace:`, grouped by function into `apps/`, `auth/`, `network/`,
  `services/`, plus a top-level `homepage/`), `deploy/home/`
  (externally-reachable apps — `namespace: home` is set once on
  `deploy/home/kustomization.yaml` itself, not per app, so scope applies to
  `deploy/home`, not `deploy/home/<app>`).
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
  (`home-root`). Each has its own
  `.github/workflows/build--<app>.yml`, triggered on `apps/<app>/**`, which
  builds from that directory as the Docker context, pushes
  `ticklethepanda/<app>:latest`, and restarts the Deployment. The manifests
  for these apps live under `deploy/` like any other app — nothing in
  `deploy/` references the source paths.

## Node pattern

- `node/` is the layer *below* k3s on the Pi: the LVM volumes and mounts
  backing the PVs, the sudoers entries, the DNS resolver, swap, the Argon
  fan. **k3s itself — version, `/etc/rancher/k3s/config.yaml`, install —
  moved to `k3s-cluster/`.** Applied by the `node` job in
  `.github/workflows/deploy.yaml`, connecting as `deploy` over SSH (key in
  `NODE_SSH_KEY`). Don't hand-edit on the node.
- `node/tasks/k3s-handover.yml` removes the old node-managed
  `config.yaml.d/10-k3s.yaml` once `k3s-cluster/`'s `config.yaml` is in
  place — so `disable:` never has two owners.
- **CI's own transport runs through the cluster.** The in-cluster
  `cloudflared` Deployment is the Cloudflare Zero Trust private-network
  connector (logs show `originService=warp-routing` carrying `192.168.1.2:22`
  and `:6443`). `node/` no longer restarts k3s, so its playbook doesn't
  disturb its own transport — but `k3s-cluster/` does (see that pattern).
- **An open 6443 is not a ready API.** k3s binds the port well before it
  serves, and `kubectl` fails immediately against an unavailable API rather
  than respecting `--timeout`. Readiness gates must poll
  `k3s kubectl get --raw /readyz` with retries. Running pods *do* survive a
  k3s server restart (containerd keeps them up), so cloudflared normally
  stays Ready throughout.
- The corollary: **when k3s is down, CI cannot reach the node at all.**
  Recovery is LAN-local — see `node/RECOVERY.md`.

## k3s-cluster pattern

- `k3s-cluster/` owns k3s on every node — the Pi (`server`) and the three
  Proxmox VMs (`agent`) — as one cluster, via the pinned `k3s.orchestration`
  collection (`k3s-io/k3s-ansible`). Applied by the `k3s-cluster` job, after
  `proxmox` (which creates the VMs) and `node` (Pi storage mounts), before
  `cluster`. Connects as `deploy` over SSH through the same tunnel.
- **Bumping k3s is an edit to `k3s-cluster/vars/versions.yml`.** It must move
  in the same commit as `deploy/setup/traefik/traefik-helm-chart.yaml`: k3s
  only serves the Traefik chart tarball bundled with the *installed* version.
  `node/scripts/check-traefik-pin.sh` (still invoked from `preflight`,
  repointed at the new path) enforces the pair; `--online` checks it against
  the k3s release manifest. One minor version at a time.
- `site.yml` **composes** the collection's `prereq` / `k3s_server` /
  `k3s_agent` roles rather than running `k3s.orchestration.site`, whose
  hardcoded `raspberrypi` role would edit the Pi's `/boot` cmdline and
  trigger a full **reboot** (kills the cloudflared pod — worse than a k3s
  restart, where pods survive).
- **The collection restarts k3s with a plain synchronous
  `service: state=restarted`**, not `node/`'s old detached `systemd-run`. A
  server restart keeps pods up so the tunnel only blips;
  `k3s-cluster/ansible.cfg`'s SSH keepalives cover it. Do first runs by hand
  from the LAN. Every run restarts k3s (the collection always does) — that is
  by design, not drift.
- `token` is left undefined: the server role reads the Pi's existing token
  and the agent play consumes it in the same run. No token secret, and the
  SQLite datastore is never touched — so there is no pre-change archive (the
  old `node/` upgrade script wrote one); recover a bad run by reverting.
- **No `--check` drift gate** for the `k3s-cluster` job — the collection
  skips its mutating tasks under `--check`. The job asserts four Ready nodes
  and the control-VM cordon with `kubectl` instead.
- The workflow holds `concurrency: group: cluster` so two runs can never
  touch the cluster at once. A `dorny/paths-filter` step in `preflight`
  drives the per-tree skips (see "Deploy pattern"); each downstream `if:`
  treats an upstream *skip* as a pass but still blocks on a real failure.

## Router pattern

- `router/` has two halves. `router/bootstrap/` is the OpenWrt Image Builder
  setup — a known-good baseline, built and flashed **by hand**, the
  break-glass path. `router/ansible/` is a `community.openwrt` playbook, the
  source of truth for ongoing config, applied by the `router` job in
  `.github/workflows/deploy.yaml`. The two need not stay in sync: bootstrap
  only has to get a bare router far enough for the playbook to take over.
- The `router` job connects as `root` over SSH (reusing `NODE_SSH_KEY`, whose
  public half `router/bootstrap/` bakes into the router's
  `authorized_keys`) through the **same cloudflared tunnel as `node`** — the
  Cloudflare Zero Trust private-network routes cover `192.168.1.0/24` and
  `192.168.10.0/24` (both configured in the dashboard, not this repo), so no
  separate route is needed — the IoT VLANs are deliberately *not* routed to CI.
  PPPoE and Wi-Fi secrets come from `ROUTER_PPPOE_USERNAME` /
  `ROUTER_PPPOE_PASSWORD` / `ROUTER_WIFI_KEY` / `ROUTER_WIFI_IOT_KEY_{INET,LOCAL,ECHO}`
  in the `prod` environment.
- **Same tunnel-through-the-thing-you're-changing risk as k3s.** CI egresses
  through this router's WAN. `router/ansible/site.yml`'s network / dropbear
  handlers detach with `community.openwrt.nohup` and reconnect with
  `wait_for_connection` — do not fold them into a synchronous restart. Do the
  first apply after any reflash from the LAN, not through CI.
- `community.openwrt` modules run in ash — no Python on the router. The play
  is `gather_facts: false` and includes the `community.openwrt.init` role
  with `openwrt_install_recommended_packages: false` (busybox already has
  `base64` / `sha256sum`; the opkg path can otherwise make a run report
  "changed" off a stale package list).
- The root password is the one thing `router/ansible/` does not manage —
  `router/bootstrap/` writes it once from the GL.iNet backup hash.

## Proxmox pattern

- `proxmox/` is the Ansible layer for `proxmox-01` (`192.168.1.3`), the home
  hypervisor — Proxmox VE 9 on Debian 13. Its own tree, like `node/` and
  `router/ansible/`, not a shared inventory: the three hosts differ enough
  (connection user, collections, tunnel-disruption handling) that merging
  them buys little until there is shared logic to extract.
- The `proxmox` job connects as `root` over SSH (reusing `NODE_SSH_KEY`)
  through the **same cloudflared tunnel as `node`** — the Zero Trust routes
  already cover `192.168.1.0/24`. The key's public half is already in the
  host's `authorized_keys` (a symlink into `/etc/pve/priv/`); no bootstrap
  playbook. The VM-lifecycle half instead hits the **Proxmox API** (port
  8006) from the runner with a `root@pam` token (`PROXMOX_API_TOKEN_ID` is
  the token *name*, `PROXMOX_API_TOKEN_SECRET` its value; both in `prod`) — so
  the reachability check needs
  `192.168.1.3:22` *and* `:8006`; add each to the Zero Trust policy for the
  CI service token (dashboard, not this repo).
- Two access paths in one play: apt repos + the Debian 13 template build run
  over SSH as root (`qm` — the API can't import a downloaded qcow2 as a
  disk); the three k3s VMs are cloned/configured via `community.proxmox`
  (`delegate_to: localhost`). The template build is skipped once VMID 9000
  exists; VM config runs only for a VMID that doesn't exist yet
  (`proxmox_kvm` update mode doesn't diff — it always reports changed), so
  reshaping a VM's CPU/RAM/IP means deleting it and re-running.
- Neither path touches how CI reaches the host, so no tunnel-disruption
  handling. The job runs after `node`/`router` purely so nothing overlaps.
- apt sources are deb822 `.sources` (PVE 9 / Debian 13), managed with
  `ansible.builtin.deb822_repository`. Enterprise repos are disabled **in
  place** (`Enabled: no`), not deleted — `pve-manager` recreates them on
  upgrade. `proxmox/vars/main.yml` holds `proxmox_apt_suite`; bump it on a
  major PVE / Debian upgrade.
- Deliberately not managed: the subscription key / nag, LXC containers,
  non-k3s VMs and all VM/CT storage, the cluster config and `/etc/pve`,
  `authorized_keys`, and **VM deletion** (never automated). k3s *on* the VMs
  is `k3s-cluster/`'s job.

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
  `deploy/internal/auth/pocketid/volume.yaml`.
- The pre-migration copies still sit on the root filesystem under `/mnt/disk`
  and `/var/lib/rancher/k3s/storage` (~700M). They are the rollback — see the
  end of `node/STORAGE.md` for removing them.

## Administration notes

- **k3s's own bundled addons can conflict with kustomize-managed
  resources of the same name.** k3s watches
  `/var/lib/rancher/k3s/server/manifests/` on the node and re-applies
  whatever's there on every restart, independent of CI. If the repo
  manages a resource that k3s also bundles by default (this happened with
  Traefik), add that addon to `server_config_yaml` in
  `k3s-cluster/group_vars/k3s_cluster.yml` (`disable: [servicelb, traefik,
  local-storage]`, written to `/etc/rancher/k3s/config.yaml`) and vendor the
  full resource into the repo so there's a single owner. `deploy/setup/traefik/`
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
