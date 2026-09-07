# k3s-cluster

k3s across every node: `k8s-manager-1` (the Pi, sole server) and the three
Proxmox VMs (`k3s-vm-control-01/-worker-01/-worker-02`, agents). One cluster.

Owns the k3s version, `/etc/rancher/k3s/config.yaml`, and installing/joining
k3s on each node, via the official `k3s.orchestration` collection
(`k3s-io/k3s-ansible`, pinned in `requirements.yml`). Everything *below* k3s
on the Pi -- LVM/mounts, the DNS resolver, swap, the Argon fan, sudoers --
stays in `node/`. Everything *inside* the cluster stays in `deploy/`.

Applied by the `k3s-cluster` job in `.github/workflows/deploy.yaml`, over SSH
as `deploy` (the Pi's automation user; the VMs' cloud-init user) through the
same cloudflared tunnel as `node` / `router` / `proxmox`.

## What it manages

| Area | Where |
|---|---|
| k3s version (all nodes) | `vars/versions.yml` (`k3s_version`) |
| server config -- disabled addons | `group_vars/k3s_cluster.yml` (`server_config_yaml`) |
| agent join (VMs -> Pi API) | `k3s.orchestration.k3s_agent`, `api_endpoint` |
| lvm-vg node label + `k3s-vm-control-01` cordon | `tasks/node-labels.yml` |

Not managed: the VMs themselves (that is `proxmox/`), the SQLite datastore,
the cluster token (left as the Pi's existing one), anything in `deploy/`.

## Running it by hand

From the LAN (WARP disconnected), with the `deploy` private key:

```sh
cd k3s-cluster
ansible-galaxy collection install -r requirements.yml
pip install netaddr
ansible-playbook site.yml --syntax-check
ansible-playbook site.yml --diff -e node_user=deploy
```

`kubectl get nodes` should show four `Ready` nodes on the pinned
`k3s_version`, with `k3s-vm-control-01` `SchedulingDisabled`.

## Notes

- **The server play restarts k3s on the Pi.** A k3s *server* restart keeps
  running pods (cloudflared included) up -- containerd holds them -- so the
  cloudflared tunnel only blips. The roles use a plain synchronous restart,
  not `node/`'s detached one; `ansible.cfg`'s SSH keepalives cover the blip.
  A full Pi *reboot* is a different matter and is why the collection's
  `raspberrypi` role is deliberately not used (`site.yml` header).
- **No `--check` idempotence gate.** The collection's roles skip their
  install/restart tasks under `--check`, so a drift gate would be
  meaningless. The CI job asserts the real end state with `kubectl` instead.
- **No pre-change datastore archive.** `node/` used to tar the SQLite
  datastore before every k3s change; the collection does not. `token` is
  left undefined so the datastore is untouched -- a bad run is recovered by
  reverting the commit and re-running. `node/RECOVERY.md`'s restore section
  still applies.
- `known_hosts` pins each node's ed25519 key. A reflashed Pi or rebuilt VM
  regenerates its key: re-scan (`ssh-keyscan -t ed25519 <ip>`), verify the
  fingerprint from the console, re-commit.
- `site.yml`'s pre_task fetches `/usr/local/bin/k3s-install.sh` (`force:
  false`) before the roles: they run the script unconditionally but only
  fetch it on a version change, and a node already at the target version
  (the Pi, on first adoption) would otherwise not have it.
- k3s upgrades: bump `k3s_version` (and the coupled
  `traefik_chart_version` -- `node/scripts/check-traefik-pin.sh` enforces the
  pair). One minor version at a time.
