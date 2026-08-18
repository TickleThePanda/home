## Home

A monorepo for my home services, running on a Raspberry PI Kubernetes
cluster.

The cluster is installed using [k3s](https://k3s.io/). Both layers are
declarative and applied by CI — there is nothing to install by hand:

- `node/` owns the node itself (k3s version, k3s config, PV-backing
  directories, sudoers), applied with Ansible.
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
- `odinbot` — a broadband speed test monitor.

### `rpi-timelapse`

A web controlled timelapse camera, based around the [Raspberry PI Camera].

### `speed-tester`

A broadband speed test monitor, using [Speedtest by Ookla].

## Network

Router: 192.168.1.1

### Kubernetes

`k3s-manager-1` Node IP / SSH: 192.168.1.2

Kube API: 192.168.1.3

PiHole: 192.168.1.10

External ingress: 192.168.1.20

Internal ingress: 192.168.1.19

### Others

Home Assistant: 192.168.1.5

[raspberry pi camera]: https://www.raspberrypi.org/products/camera-module-v2/
[speedtest by ookla]: https://www.speedtest.net/
