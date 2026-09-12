# ansible/k3s

k3s across every node: `k3s-vm-control-01` (sole server, embedded etcd with a
single member) and `k8s-manager-1` (the Pi) plus `k3s-vm-worker-01/-02`
(agents). One cluster.

Owns the k3s version, `/etc/rancher/k3s/config.yaml`, the VMs' node resolver,
and installing/joining k3s on each node, via the official `k3s.orchestration`
collection (`k3s-io/k3s-ansible`, pinned in `requirements.yml`). Everything
*below* k3s on the Pi -- LVM/mounts, the Pi's DNS resolver, swap, the Argon
fan, sudoers -- stays in `ansible/node/`. Everything *inside* the cluster stays in
`deploy/`.

Applied by the `infra` job in `.github/workflows/deploy.yaml`, over SSH
as `deploy` through the same cloudflared tunnel as `node` / `router` /
`proxmox`.

## What it manages

| Area | Where |
|---|---|
| k3s version (all nodes) | `vars/versions.yml` (`k3s_version`) |
| server config -- addons, tls-san, etcd snapshots | `group_vars/k3s_cluster.yml` (`server_config_yaml`) |
| agent join (-> `192.168.1.32` API) | `k3s.orchestration.k3s_agent`, `api_endpoint` |
| VM node resolver (Quad9) | `tasks/node-dns.yml` |
| Pi lvm-vg label + storage-anchor taint at join | `host_vars/k8s-manager-1.yml` |
| lvm-vg label, server taint, Pi taint (via API) | `tasks/node-labels.yml` |
| docker.io pull auth (all nodes) | `k3s.yml` -> `/etc/rancher/k3s/registries.yaml` |

Not managed: the VMs themselves (that is `ansible/proxmox/`), the etcd datastore
contents (snapshots are config, above), the cluster token (left as the
existing one), anything in `deploy/`.

## Running it by hand

From the LAN (WARP disconnected), with the `deploy` private key:

```sh
cd ansible
ansible-galaxy collection install -r requirements.yml
pip install netaddr
ansible-playbook k3s/k3s.yml --syntax-check
set -a; . ../.env; set +a   # DOCKER_PULL_* -- else registries.yaml is skipped
ansible-playbook k3s/k3s.yml --diff -e node_user=deploy
```

`kubectl get nodes` should show four `Ready` nodes on the pinned
`k3s_version`, with `k3s-vm-control-01` carrying the
`ticklethepanda.dev/control-plane:NoSchedule` taint.

## Notes

- **Every run restarts k3s.** `ansible.cfg` sets `forks = 1` so hosts are
  done one at a time. A k3s restart keeps running pods (cloudflared included)
  up -- containerd holds them -- so the cloudflared tunnel only blips;
  `ansible.cfg`'s SSH keepalives cover it. A full *reboot* is a different
  matter and is why the collection's `raspberrypi` role is deliberately not
  used (`k3s.yml` header).
- **No `--check` idempotence gate.** The collection's roles skip their
  install/restart tasks under `--check`, so a drift gate would be
  meaningless. The CI job asserts the real end state with `kubectl` instead.
- **The datastore is single-member embedded etcd.** No peer to recover from,
  so `server_config_yaml` schedules `k3s etcd-snapshot` (every 6h, retention
  4). A bad run is recovered by reverting the commit; a lost datastore from a
  snapshot -- see `RECOVERY.md`.
- `known_hosts` pins each node's ed25519 key. A reflashed Pi or rebuilt VM
  regenerates its key: re-scan (`ssh-keyscan -t ed25519 <ip>`), verify the
  fingerprint from the console, re-commit.
- **VM node DNS.** `tasks/node-dns.yml` (the first pre_task, VMs only) pins
  eth0's resolver to Quad9 via a netplan drop-in and fails the run if
  `192.168.1.10` (in-cluster Pi-hole) ever comes back. Pi-hole is a cluster
  workload, so a node that resolves through it can't cold-start the cluster.
- `k3s.yml`'s pre_task fetches `/usr/local/bin/k3s-install.sh` (`force:
  false`) before the roles: they run the script unconditionally but only
  fetch it on a version change, and a node already at the target version
  would otherwise not have it.
- **docker.io pull auth.** `k3s.yml` writes `/etc/rancher/k3s/registries.yaml`
  on every node from `DOCKER_PULL_USERNAME` / `DOCKER_PULL_TOKEN` (a read-only
  Docker Hub PAT; `prod` env secrets in CI, `../.env` for a hand run) so the
  cluster's Docker Hub pulls draw on the account's rate limit rather than the
  shared anonymous per-IP one. Absent creds -> the file is skipped, any
  existing one left in place. Rotate with `gh secret set DOCKER_PULL_TOKEN
  --env prod` then a `workflow_dispatch`.
- k3s upgrades: bump `k3s_version` (and the coupled
  `traefik_chart_version` -- `scripts/check-traefik-pin.sh` enforces the
  pair). One minor version at a time.
