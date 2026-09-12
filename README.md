## Home

A monorepo for my home services. The Kubernetes cluster spans three Debian VMs
on a Proxmox host (one the k3s server, two agents) and a Raspberry Pi (an
agent, the storage anchor).

The cluster is installed using [k3s](https://k3s.io/). Every layer is
declarative and applied by CI — there is nothing to install by hand:

- `ansible/` owns every host outside the cluster, as one Ansible project with
  four layers:
  - `ansible/proxmox/` — the hypervisor `proxmox-01` and the k3s VMs on it.
  - `ansible/node/` — the Pi *below* k3s (LVM/PV-backing volumes, sudoers, the
    DNS resolver, swap, the Argon ONE fan).
  - `ansible/k3s/` — k3s on every node (version, config, agent join), via the
    `k3s.orchestration` collection.
  - `ansible/router/` — the gateway (OpenWrt).
- `deploy/` owns everything inside the cluster, applied with kustomize.
- `apps/` holds the source for the apps built into images by CI.
- `router/bootstrap/` is the hand-flashed OpenWrt image — the break-glass path,
  not applied by CI.

All four Ansible layers are applied by the single `infra` job in
`.github/workflows/deploy.yaml`, in the order `ansible/site.yml` documents.

### `ansible/node`

Pi-level configuration for `k8s-manager-1` *below* k3s.

See [`ansible/node/RECOVERY.md`](ansible/node/RECOVERY.md) and
[`ansible/k3s/RECOVERY.md`](ansible/k3s/RECOVERY.md) before touching a broken
cluster — CI reaches the nodes *through* a pod running inside that cluster, so
when the control-plane is down, CI can't reach it either. A human can, via
the router's Tailscale subnet route (independent of the cluster) instead of
being physically on the LAN.
[`ansible/node/STORAGE.md`](ansible/node/STORAGE.md) covers the SSD: its partitions, the LVM
volume group behind both the node's own state and every PersistentVolume, and
how to rebuild it.

### `ansible/k3s`

k3s itself, on `k3s-vm-control-01` (server) and the Pi plus two worker VMs
(agents), as one cluster. The k3s version lives in
[`ansible/k3s/vars/versions.yml`](ansible/k3s/vars/versions.yml); bumping it
there upgrades every node. See
[`ansible/k3s/README.md`](ansible/k3s/README.md).

### `deploy`

The declarative Kubernetes configuration for deploying the applications.

### `apps`

Source for the apps this repo builds and deploys. Each has its own
`.github/workflows/build--<app>.yml`, which builds the image from
`apps/<app>/` and restarts its Deployment.

- `home-root` — a root site linking to the other services.

### `ansible/proxmox`

Ansible for the hypervisor `proxmox-01` (192.168.1.3) — its apt repos, plus
the Debian 13 template and the three k3s VMs on it (the server and two
agents). See
[`ansible/proxmox/README.md`](ansible/proxmox/README.md).

## Network

Router: 192.168.1.1 — a GL.iNet Flint 2 (GL-MT6000) running vanilla OpenWrt.
Ongoing config is a `community.openwrt` Ansible playbook
([`ansible/router/`](ansible/router/)), applied by CI over SSH. The initial
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
| 192.168.1.2 | `k8s-manager-1` — Pi, k3s agent (lvm-data storage anchor) / SSH |
| 192.168.1.3 | Proxmox `proxmox-01` |
| 192.168.1.5 | Home Assistant (Kea reservation) |
| 192.168.1.10 | Pi-hole (MetalLB) |
| 192.168.1.11 | Kea DHCP (macvlan) |
| 192.168.1.19 | Traefik internal ingress (MetalLB) |
| 192.168.1.20 | Traefik external ingress (MetalLB) |
| 192.168.1.32 | `k3s-vm-control-01` — k3s server (control-plane, tainted), Proxmox guest |
| 192.168.1.33 | `k3s-vm-worker-01` — k3s agent, Proxmox guest |
| 192.168.1.34 | `k3s-vm-worker-02` — k3s agent, Proxmox guest |
