## Home

A monorepo for my home services, running on a Raspberry PI Kubernetes
cluster.

The cluster is installed using [k3s](https://k3s.io/). Both layers are
declarative and applied by CI — there is nothing to install by hand:

- `node/` owns the node itself (k3s version, k3s config, PV-backing
  directories, sudoers, the Argon ONE fan controller), applied with Ansible.
- `deploy/` owns everything inside the cluster, applied with kustomize.
- `apps/` holds the source for the apps built into images by CI.

### `node`

Node-level configuration for `k8s-manager-1`, applied by the `node` job in
`.github/workflows/deploy.yaml`. The k3s version lives in
`node/vars/versions.yml`; bumping it there is what upgrades the node.

See [`node/RECOVERY.md`](node/RECOVERY.md) before touching a broken cluster —
CI reaches the node *through* a pod running inside that cluster, so when k3s
is down the recovery path is LAN-local, not CI.
[`node/STORAGE.md`](node/STORAGE.md) covers the SSD: its partitions, the LVM
volume group behind both the node's own state and every PersistentVolume, and
how to rebuild it.

### `deploy`

The declarative Kubernetes configuration for deploying the applications.

### `apps`

Source for the apps this repo builds and deploys. Each has its own
`.github/workflows/build--<app>.yml`, which builds the image from
`apps/<app>/` and restarts its Deployment.

- `home-root` — a root site linking to the other services.
- `internal-index` — the equivalent index for internal-only services.

## Network

Router: 192.168.1.1 — a GL.iNet Flint 2 (GL-MT6000) running vanilla OpenWrt.
Ongoing config is a `community.openwrt` Ansible playbook
([`router/ansible/`](router/ansible/)), applied by CI over SSH. The initial
image ([`router/bootstrap/`](router/bootstrap/)) is built and flashed by hand —
the break-glass path. See [`router/README.md`](router/README.md).

Two VLANs, routed by the gateway, currently with fully open traffic between
them:

| VLAN | Subnet | Gateway | Ports / SSIDs |
|---|---|---|---|
| homelab | 192.168.1.0/24 | 192.168.1.1 | LAN2 (node), LAN3 (Home Assistant), LAN1 + the 2.5G jack (spare) |
| trusted | 192.168.10.0/24 | 192.168.10.1 | LAN4 (downstairs switch), Wi-Fi `It reaches out` / `It reaches out (2.4G)` |

Kea serves both subnets — homelab directly on its macvlan, trusted via a DHCP
relay on the router. Trusted clients resolve through routed access to Pi-hole
at 192.168.1.10.

### Kubernetes

`k3s-manager-1` Node IP / SSH: 192.168.1.2

Kube API: 192.168.1.3

PiHole: 192.168.1.10

Kea DHCP: 192.168.1.11

External ingress: 192.168.1.20

Internal ingress: 192.168.1.19

### Others

Home Assistant: 192.168.1.5
