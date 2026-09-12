# Home infrastructure monorepo

Four-node k3s cluster running the owner's home services: the control-plane on
an amd64 Proxmox VM, a Raspberry Pi as the arm64 storage-anchor agent, plus
two worker VMs — and the apps deployed onto it. See `README.md` for the
network map (node IPs, ingress IPs, etc.) — this file covers operational
context that isn't visible from the code alone.

## Workflow

Prefer commits on main branch as workflow (for example, do not branch).

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

- **Server**: `k3s-vm-control-01`, `192.168.1.32`, amd64 Debian 13 VM on
  `proxmox-01` — the sole k3s server (control-plane), embedded etcd with a
  single member. Tainted `ticklethepanda.dev/control-plane:NoSchedule` so it
  runs no portable workloads.
- **Agents**: `k8s-manager-1` (`192.168.1.2`, the Raspberry Pi, arm64) plus
  `k3s-vm-worker-01` / `-02` (`192.168.1.33`–`.34`, amd64 VMs). The Pi is the
  storage anchor — every `lvm-data` volume lives on its SSD, and its
  `ticklethepanda.dev/prefer-no-schedule=storage-anchor:PreferNoSchedule`
  taint steers portable pods to the workers.
- k3s on every node is managed by `ansible/k3s/` (the `k3s.orchestration`
  collection), not `ansible/node/`, which owns only the layer below k3s on
  the Pi.
- SSH: `ssh 192.168.1.2` as `panda` (Pi); every node (Pi included) also takes
  the `deploy` CI key. Key-based, no host aliases — use the IPs. Control-plane
  work goes through `deploy@192.168.1.32`.
- `panda` has scoped, passwordless sudo on the Pi
  (`/etc/sudoers.d/panda-k3s-admin`) for `k3s-agent` service control and
  reading k3s's own config. Anything broader (general root shell, package
  installs) needs the owner interactively — kept narrow deliberately, since
  those directories are close to root-equivalent (k3s treats their contents
  as trusted input).
- The local kubeconfig's client cert doesn't auto-rotate. If `kubectl`
  reports "server has asked for the client to provide credentials", restart
  k3s on `k3s-vm-control-01` and re-pull `/etc/rancher/k3s/k3s.yaml` into
  `~/.kube/config` (fix the `server:` field to `192.168.1.32`, it defaults
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
- **mDNS**: an Avahi reflector on the router (`ansible/router/tasks/mdns-reflector.yml`)
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

- `.github/workflows/deploy.yaml` is the pipeline's entry point: trigger,
  paths-filter, and the `preflight` job (the k3s/Traefik pin guard + tunnel
  reachability — fails fast) live there. The other three jobs —
  `infra` (every Ansible layer), `cluster` (kustomize/Flux), `terraform`
  (`terraform/cloudflare-zero-trust/`, last) — are each a separate file
  (`deploy-infra.yaml`, `deploy-cluster.yaml`, `deploy-terraform.yaml`) called
  from `deploy.yaml` via `workflow_call`. **This split is deliberate:**
  `dorny/paths-filter` works at file granularity, so with every job in one
  file, editing any one job's section matched every other job's filter entry
  for that file too — editing the terraform job, for instance, reran
  proxmox/node/k3s/router as well. Each layer's filter now points at just its
  own file; `preflight` stays in `deploy.yaml` itself since it computes every
  other job's gating outputs, so a change to it should still conservatively
  rerun everything, same as this file.
- **`infra` is one job for all four Ansible layers, in its own file.** It
  applies them in the order `ansible/site.yml` documents: `proxmox` (creates
  the VMs), `node` (the Pi's storage mounts, which a `RequiresMountsFor`
  drop-in makes `k3s-agent` depend on), `k3s`, then `router` last. A k3s
  upgrade changes which Traefik chart tarball is served, so `k3s` lands
  before the `cluster` manifests that reference it. The router goes last
  because nothing depends on it and it is the only Ansible layer that can
  drop CI's own egress — a WAN blip there cannot strand layers that had yet
  to run.
- One job rather than four for the Ansible layers is deliberate: runner
  provisioning, checkout, the Cloudflare WARP connect and `ansible-setup` are
  most of the cost of a no-op deploy, and they are now paid once. **Do not
  split the four Ansible layers back into separate jobs** to get per-layer
  isolation — they are sequential `ansible-playbook` invocations under
  `set -euo pipefail`, which already blocks later layers on a failure. (This
  is unrelated to the top-level infra/cluster/terraform file split above —
  that split is between jobs that already ran in separate files' worth of
  concerns; the four Ansible layers stay fused into one.)
- **`terraform` runs last, same reasoning as `router`:** it owns the "home"
  Cloudflare tunnel and the private-network routes CI's own WARP connection
  reaches the LAN through, so it is the one job that can drop CI's own
  egress. It doesn't need WARP itself (Cloudflare's and GCP's APIs are
  reached over the public internet), but it still waits on
  preflight/infra/cluster succeeding-or-skipping before touching that config.
- Each layer is skipped when the push changed nothing under its own tree
  (including its own workflow file). Anything shared — `ansible/inventory.yml`,
  `group_vars/`, `host_vars/`, `roles/`, `ansible.cfg`, `site.yml`,
  `requirements.yml`, `deploy.yaml` itself, the composite actions — matches
  the `shared` paths-filter and runs **every** Ansible layer. That catch-all
  is load-bearing: without it a change to a shared role would apply to no
  host at all. A `workflow_dispatch` runs everything.
- `.github/actions/ansible-setup` installs a pinned `ansible-core`
  (`.github/actions/ansible-setup/requirements.txt`, bumped in its own commit
  like the collections) plus the Galaxy collections, with pip / collection /
  fact caches restored. `.github/actions/ansible-apply` applies one layer and
  then proves idempotence with a `--check` re-run — **but only when the apply
  actually changed something.** A no-op apply is idempotent by definition, so
  re-running the whole playbook over the tunnel to confirm it is pure
  wall-clock cost.
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
- **Every image must be multi-arch (`linux/amd64` + `linux/arm64`)** — the Pi
  is arm64, the worker VMs are amd64, and a stateless pod can land on either.
  Each `build--<app>.yml` builds both platforms and fails if the pushed
  manifest is missing one. The same applies to third-party images referenced
  from `deploy/`. Workloads that must stay on the Pi are held there by their
  `lvm-data` PVC (storage topology), not by an arch `nodeSelector`.

## Ansible layout

- **`ansible/` is one Ansible project for every managed host** — one
  `ansible.cfg`, one `inventory.yml`, one `known_hosts`, one
  `requirements.yml` (the union of every layer's collections), shared
  `group_vars/`, `host_vars/` and `roles/`. Each layer keeps its own
  directory: `ansible/proxmox/`, `ansible/node/`, `ansible/k3s/`,
  `ansible/router/`, each holding a playbook named after the layer plus its
  `tasks/`, `templates/`, `vars/`. `ansible/site.yml` imports all four in
  dependency order. `bootstrap/router/` is *not* part of this — it is
  hand-flashed, not CI-driven.
- Connection differences are `group_vars`, not separate trees: the router is
  `ansible_user: root` + `ansible_python_interpreter: /dev/null` (OpenWrt ships
  no Python, every task is a `community.openwrt` module running in ash),
  proxmox-01 is root, everything else is `deploy`.
- **Every play that gathers must use the same `gather_subset`
  (`!all, min, network`).** The fact cache is shared across layers and
  `gathering = smart` means a cached entry wins over the play's own subset — so
  a layer that gathered less would starve one that needs more.
  `k3s.orchestration`'s prereq role reads `all_ipv6_addresses` and (in
  `k3s.nft.j2`) `default_ipv4`, both of which live in `network`; the other
  layers need only `min`, but they share one cache so they share one subset.
  There is **no `ansible.cfg` key** for this — `gather_subset` under
  `[defaults]` is silently ignored (there is no `DEFAULT_GATHER_SUBSET` in
  ansible-core), and dropping the per-play setting falls back to a full
  hardware scan on every host. It has to be repeated per play.
- **Never scope a run with `--limit` alone.** `k8s-manager-1` is in both `pi`
  and `agent` — limiting to `pi` still matches the k3s agent play and would
  drag the Pi through a k3s reinstall, then fail on an undefined `token` (the
  agent role interpolates it unconditionally but only defines it when the
  server play ran). Select whole layer playbooks instead; that is what CI does.
- `localhost` is in its own `control` group, needed for `community.proxmox`'s
  `delegate_to: localhost`. No play uses `hosts: all` — that would run `become`
  tasks on the runner.
- Anything genuinely shared between layers becomes a role under
  `ansible/roles/`, parameterised by `group_vars`. `glances` is the current
  example: identical on the Pi and proxmox-01 except the web-server package
  (`python3-bottle` on Glances 3, `python3-uvicorn` on Glances 4), which is
  `glances_web_package`. A naive merge there breaks a host at *runtime*, not at
  apply time.

## Node pattern

- `ansible/node/` is the layer *below* k3s on the Pi: the LVM volumes and
  mounts backing the PVs, the sudoers entries, the DNS resolver, swap, the
  Argon fan. **k3s itself — version, `/etc/rancher/k3s/config.yaml`, install —
  is `ansible/k3s/`.** Applied by the `infra` job in `deploy-infra.yaml`,
  connecting as `deploy` over SSH (key in `NODE_SSH_KEY`). Don't hand-edit on
  the node.
- The Pi runs `k3s-agent`, not a server. `ansible/node/tasks/storage.yml`'s
  `RequiresMountsFor` guard targets `k3s-agent.service` and only the `agent` /
  `kubelet` LVs; the `server` LV is vestigial (kept — shrinking an LV fails).
- **CI's own transport runs through the cluster.** The in-cluster
  `cloudflared` Deployment is the Cloudflare Zero Trust private-network
  connector (logs show `originService=warp-routing` carrying `192.168.1.2:22`
  and `192.168.1.32:6443`). `ansible/node/` no longer restarts k3s, so its playbook
  doesn't disturb its own transport — but `ansible/k3s/` does (see that
  pattern). cloudflared has two replicas and is tainted off the server, so it
  runs on the two worker VMs.
- **An open 6443 is not a ready API.** k3s binds the port well before it
  serves, and `kubectl` fails immediately against an unavailable API rather
  than respecting `--timeout`. Readiness gates must poll
  `k3s kubectl get --raw /readyz` with retries. Running pods *do* survive a
  k3s server restart (containerd keeps them up), so cloudflared normally
  stays Ready throughout.
- The corollary: **when the server or both worker VMs are down, CI cannot
  reach the LAN at all** — its only path is the in-cluster `cloudflared`
  Deployment. A human isn't stuck the same way: the router runs its own
  Tailscale subnet router + exit node (`ansible/router/tasks/tailscale.yml`,
  independent of the cluster), advertising `192.168.1.0/24` and the other
  VLANs — connect to the tailnet and the LAN is reachable without being
  physically present. See `ansible/node/RECOVERY.md` and
  `ansible/k3s/RECOVERY.md`.

## K3s layer pattern

- `ansible/k3s/` owns k3s on every node — `k3s-vm-control-01` (`server`), and
  the Pi plus the two worker VMs (`agent`) — as one cluster, via the pinned
  `k3s.orchestration` collection (`k3s-io/k3s-ansible`). Applied by the
  `infra` job, after the proxmox layer (which creates the VMs) and the node
  layer (Pi storage mounts), before `cluster`. Connects as `deploy` over SSH
  through the same tunnel.
- **Node DNS is pinned to Quad9, not the in-cluster Pi-hole.** The VMs' first
  `pre_task` (`tasks/node-dns.yml`) writes a netplan drop-in and asserts it
  took — a node whose resolver is a cluster workload cannot cold-start the
  cluster (the kubelet can't resolve a registry to pull the image that brings
  the resolver back). `ansible/proxmox/` sets the same for a fresh clone. Pods still
  use CoreDNS.
- **Bumping k3s is an edit to `ansible/k3s/vars/versions.yml`.** It must move
  in the same commit as `deploy/setup/traefik/traefik-helm-chart.yaml`: k3s
  only serves the Traefik chart tarball bundled with the *installed* version.
  `scripts/check-traefik-pin.sh` (still invoked from `preflight`,
  repointed at the new path) enforces the pair; `--online` checks it against
  the k3s release manifest. One minor version at a time.
- `k3s.yml` **composes** the collection's `prereq` / `k3s_server` /
  `k3s_agent` roles rather than running `k3s.orchestration.site`, whose
  hardcoded `raspberrypi` role would edit the Pi's `/boot` cmdline and
  trigger a full **reboot** (kills the cloudflared pod — worse than a k3s
  restart, where pods survive).
- **The collection restarts k3s with a plain synchronous
  `service: state=restarted`**, not `ansible/node/`'s detached
  `systemd-run`. A server restart keeps pods up so the tunnel only blips;
  `ansible/ansible.cfg`'s SSH keepalives cover it. Do first runs by hand
  from the LAN. Every run restarts k3s (the collection always does) — that is
  by design, not drift. `ansible.cfg` sets `forks = 4` so the agent play hits
  the three agents at once; serialising only matters with a second server
  (rolling a restart past a lone etcd member loses quorum) — drop back to
  `forks = 1` if one is ever added.
- `token` is left undefined: the server role reads the existing token off
  `k3s-vm-control-01` and the agent play consumes it in the same run. No
  token secret.
- **The datastore is embedded etcd with a single member.** `cluster-init:
  true` in `server_config_yaml` keeps it selected. There is no peer to
  recover from, so `server_config_yaml` schedules `k3s etcd-snapshot` every
  6h (retention 4) to `/var/lib/rancher/k3s/server/db/snapshots` on the
  server's root disk. Recover a bad run by reverting; recover a lost
  datastore from a snapshot — see `ansible/k3s/RECOVERY.md`.
- **The Pi's node identity is load-bearing.** It rejoins as an agent named
  `k8s-manager-1` with `ticklethepanda.dev/lvm-vg=data` — the OpenEBS PVs'
  `nodeAffinity` and the `lvm-data` StorageClass topology both key off that
  name and label. `host_vars/k8s-manager-1.yml` sets the label and the
  storage-anchor taint at registration (the label gates PV binding, so it
  can't wait for `node-labels.yml`).
- **`k3s.yml` writes `/etc/rancher/k3s/registries.yaml` on every node** to
  authenticate docker.io pulls against a read-only Docker Hub PAT
  (`DOCKER_PULL_USERNAME` / `DOCKER_PULL_TOKEN` — `prod` env secrets in CI,
  a gitignored root `.env` for a hand run), lifting the cluster's pulls off
  the shared anonymous per-IP rate limit. Absent creds → the file is skipped
  (any existing one left as-is). k3s only re-reads it on restart, which every
  run does anyway.
- **No `--check` drift gate** for the k3s layer — the collection
  skips its mutating tasks under `--check`. The job asserts four Ready nodes
  and the server's `control-plane:NoSchedule` taint with `kubectl` instead.
- The workflow holds `concurrency: group: cluster` so two runs can never
  touch the cluster at once. A `dorny/paths-filter` step in `preflight`
  drives the per-tree skips (see "Deploy pattern"); each downstream `if:`
  treats an upstream *skip* as a pass but still blocks on a real failure.

## Router pattern

- The router's config has two halves. `bootstrap/router/` is the OpenWrt Image
  Builder setup — a known-good baseline, built and flashed **by hand**, the
  break-glass path. `ansible/router/` is a `community.openwrt` playbook, the
  source of truth for ongoing config, applied by the `router` job in
  `deploy-infra.yaml`. The two need not stay in sync: bootstrap
  only has to get a bare router far enough for the playbook to take over.
- The `router` job connects as `root` over SSH (reusing `NODE_SSH_KEY`, whose
  public half `bootstrap/router/` bakes into the router's
  `authorized_keys`) through the **same cloudflared tunnel as `node`** — the
  Cloudflare Zero Trust private-network routes cover `192.168.1.0/24` and
  `192.168.10.0/24` (both configured in the dashboard, not this repo), so no
  separate route is needed — the IoT VLANs are deliberately *not* routed to CI.
  PPPoE and Wi-Fi secrets come from `ROUTER_PPPOE_USERNAME` /
  `ROUTER_PPPOE_PASSWORD` / `ROUTER_WIFI_KEY` / `ROUTER_WIFI_IOT_KEY_{INET,LOCAL,ECHO}`
  in the `prod` environment.
- **Same tunnel-through-the-thing-you're-changing risk as k3s.** CI egresses
  through this router's WAN. `ansible/router/router.yml`'s network / dropbear
  handlers detach with `community.openwrt.nohup` and reconnect with
  `wait_for_connection` — do not fold them into a synchronous restart. Do the
  first apply after any reflash from the LAN, not through CI.
- `community.openwrt` modules run in ash — no Python on the router. The play
  is `gather_facts: false` and includes the `community.openwrt.init` role
  with `openwrt_install_recommended_packages: false` (busybox already has
  `base64` / `sha256sum`; the opkg path can otherwise make a run report
  "changed" off a stale package list).
- The root password is the one thing `ansible/router/` does not manage —
  `bootstrap/router/` writes it once from the GL.iNet backup hash.

## Proxmox pattern

- `ansible/proxmox/` is the Ansible layer for `proxmox-01` (`192.168.1.3`),
  the home hypervisor — Proxmox VE 9 on Debian 13. One layer of the shared
  `ansible/` project; its `root` login lives in `group_vars/proxmox.yml`.
- It connects as `root` over SSH (reusing `NODE_SSH_KEY`)
  through the **same cloudflared tunnel as the node layer** — the Zero Trust routes
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
  reshaping a VM's CPU/RAM/IP means deleting it and re-running. For
  `k3s-vm-control-01` (the server) that also means restoring etcd from a
  snapshot — see `ansible/k3s/RECOVERY.md`.
- **The k3s VMs' cloud-init resolver is Quad9** (`proxmox_vm_nameservers`),
  not the in-cluster Pi-hole — a fresh clone must not depend on a cluster
  workload to resolve. `ansible/k3s/` re-asserts it on running VMs.
- Neither path touches how CI reaches the host, so no tunnel-disruption
  handling. The job runs after `node`/`router` purely so nothing overlaps.
- apt sources are deb822 `.sources` (PVE 9 / Debian 13), managed with
  `ansible.builtin.deb822_repository`. Enterprise repos are disabled **in
  place** (`Enabled: no`), not deleted — `pve-manager` recreates them on
  upgrade. `ansible/proxmox/vars/main.yml` holds `proxmox_apt_suite`; bump it on a
  major PVE / Debian upgrade.
- Deliberately not managed: the subscription key / nag, LXC containers,
  non-k3s VMs and all VM/CT storage, the cluster config and `/etc/pve`,
  `authorized_keys`, and **VM deletion** (never automated). k3s *on* the VMs
  is `ansible/k3s/`'s job.

## Storage

`ansible/node/STORAGE.md` is the reference. The essentials:

- Most of the SSD is `sda3`, one LVM volume group named `data`. Four LVs hold
  node state (sized in `ansible/node/vars/storage.yml`); the unallocated ~327G **is
  the PersistentVolume pool**, not spare capacity. Don't pre-allocate it. The
  `server` LV is vestigial since the control-plane left the Pi — kept, not
  reclaimed (shrinking fails).
- **Growing a volume is an edit to `ansible/node/vars/storage.yml`.** It extends
  online. Shrinking fails.
- **Ansible does not own the partition table or the VG**, and must not — CI
  reaches this node through a pod running on it, so a bad repartition has no
  remote recovery.
- `fstab` uses `nofail` and a `k3s-agent` drop-in sets `RequiresMountsFor` on
  the `agent` / `kubelet` LVs, so a missing volume leaves the Pi reachable but
  stops `k3s-agent`. The alternative is the kubelet writing its state to root
  and looking healthy until the root filesystem fills.
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
  end of `ansible/node/STORAGE.md` for removing them.

## Administration notes

- **k3s's own bundled addons can conflict with kustomize-managed
  resources of the same name.** k3s watches
  `/var/lib/rancher/k3s/server/manifests/` on the server (`k3s-vm-control-01`)
  and re-applies whatever's there on every restart, independent of CI. If the repo
  manages a resource that k3s also bundles by default (this happened with
  Traefik), add that addon to `server_config_yaml` in
  `ansible/group_vars/k3s_cluster.yml` (`disable: [servicelb, traefik,
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
