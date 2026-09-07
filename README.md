## Home

A monorepo for my home services. The Kubernetes cluster spans a Raspberry Pi
(the k3s server) and three Debian VMs on a Proxmox host (agents).

The cluster is installed using [k3s](https://k3s.io/). Every layer is
declarative and applied by CI — there is nothing to install by hand:

- `node/` owns the Pi *below* k3s (LVM/PV-backing volumes, sudoers, the DNS
  resolver, swap, the Argon ONE fan), applied with Ansible.
- `k3s-cluster/` owns k3s on every node (version, config, agent join), via
  the `k3s.orchestration` collection, applied with Ansible.
- `deploy/` owns everything inside the cluster, applied with kustomize.
- `apps/` holds the source for the apps built into images by CI.
- `router/` owns the gateway (OpenWrt), applied with Ansible.
- `proxmox/` owns the hypervisor `proxmox-01` and the k3s VMs on it, applied
  with Ansible.

### `node`

Pi-level configuration for `k8s-manager-1` *below* k3s, applied by the `node`
job in `.github/workflows/deploy.yaml`.

See [`node/RECOVERY.md`](node/RECOVERY.md) before touching a broken cluster —
CI reaches the node *through* a pod running inside that cluster, so when k3s
is down the recovery path is LAN-local, not CI.
[`node/STORAGE.md`](node/STORAGE.md) covers the SSD: its partitions, the LVM
volume group behind both the node's own state and every PersistentVolume, and
how to rebuild it.

### `k3s-cluster`

k3s itself, on the Pi (server) and the three Proxmox VM agents, as one
cluster — applied by the `k3s-cluster` job. The k3s version lives in
[`k3s-cluster/vars/versions.yml`](k3s-cluster/vars/versions.yml); bumping it
there upgrades every node. See
[`k3s-cluster/README.md`](k3s-cluster/README.md).

### `deploy`

The declarative Kubernetes configuration for deploying the applications.

### `apps`

Source for the apps this repo builds and deploys. Each has its own
`.github/workflows/build--<app>.yml`, which builds the image from
`apps/<app>/` and restarts its Deployment.

- `home-root` — a root site linking to the other services.

### `proxmox`

Ansible for the hypervisor `proxmox-01` (192.168.1.3) — its apt repos, plus
the Debian 13 template and the three k3s agent VMs on it. Applied by the
`proxmox` job in `.github/workflows/deploy.yaml`. See
[`proxmox/README.md`](proxmox/README.md).

## Network

Router: 192.168.1.1 — a GL.iNet Flint 2 (GL-MT6000) running vanilla OpenWrt.
Ongoing config is a `community.openwrt` Ansible playbook
([`router/ansible/`](router/ansible/)), applied by CI over SSH. The initial
image ([`router/bootstrap/`](router/bootstrap/)) is built and flashed by hand —
the break-glass path. See [`router/README.md`](router/README.md).

VLANs, routed by the gateway. `homelab` ↔ `trusted` is fully open for now; the
IoT VLANs are inbound-only (see below).

| VLAN | Subnet | Gateway | Ports / SSIDs |
|---|---|---|---|
| homelab | 192.168.1.0/24 | 192.168.1.1 | LAN2 (node), LAN3 (Home Assistant), LAN1 + the 2.5G jack (spare) |
| trusted | 192.168.10.0/24 | 192.168.10.1 | LAN4 (downstairs switch), Wi-Fi `It reaches out` / `It reaches out (2.4G)` |
| iot_inet | 192.168.20.0/24 | 192.168.20.1 | Wi-Fi `It reaches out (IoT)`, PPSK — internet only |
| iot_local | 192.168.30.0/24 | 192.168.30.1 | Wi-Fi `It reaches out (IoT)`, PPSK — no internet |
| iot_echo | 192.168.40.0/24 | 192.168.40.1 | Wi-Fi `It reaches out (IoT peers)`, PPSK — internet + peer-to-peer |

The three IoT VLANs share two 2.4 GHz PPSK SSIDs (the key a device joins with
selects its VLAN). `homelab`, `trusted` and the tailnet may initiate into every
IoT VLAN; IoT VLANs may not initiate back, and cannot reach each other. Holes
are punched per device as needed. mDNS is reflected from every VLAN to
`homelab`/`trusted` (one avahi instance, so IoT-to-IoT discovery records also
leak — L3 stays firewalled).

Kea serves every subnet — homelab directly on its macvlan, the rest via DHCP
relays on the router. Off-homelab clients resolve through routed access to
Pi-hole at 192.168.1.10; the IoT VLANs also use the router (192.168.N.1) as
their NTP server.

### homelab addressing

`192.168.1.0/24` is one flat subnet, carved into bands by convention. Only two
things enforce the boundaries: the Kea DHCP pool and the MetalLB pool
addresses. Nothing is firewalled between bands.

| Range | Use |
|---|---|
| `.1` | gateway (router) |
| `.2`–`.9` | physical hosts |
| `.10`–`.31` | cluster service addresses (MetalLB VIPs + Kea) |
| `.32`–`.63` | virtual machines (Proxmox guests) |
| `.64`–`.127` | other static devices — switches, APs, printers, NAS |
| `.128`–`.254` | DHCP dynamic pool |

Assigned:

| IP | Host |
|---|---|
| 192.168.1.1 | gateway |
| 192.168.1.2 | `k8s-manager-1` — Pi, k3s server / SSH |
| 192.168.1.3 | Proxmox `proxmox-01` |
| 192.168.1.5 | Home Assistant (Kea reservation) |
| 192.168.1.10 | Pi-hole (MetalLB) |
| 192.168.1.11 | Kea DHCP (macvlan) |
| 192.168.1.19 | Traefik internal ingress (MetalLB) |
| 192.168.1.20 | Traefik external ingress (MetalLB) |
| 192.168.1.32 | `k3s-vm-control-01` — k3s agent (cordoned), Proxmox guest |
| 192.168.1.33 | `k3s-vm-worker-01` — k3s agent, Proxmox guest |
| 192.168.1.34 | `k3s-vm-worker-02` — k3s agent, Proxmox guest |
