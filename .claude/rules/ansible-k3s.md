---
paths:
  - "ansible/k3s/**"
  - "ansible/group_vars/k3s_cluster.yml"
  - "ansible/host_vars/**"
  - "deploy/setup/traefik/**"
---

# k3s layer

- `ansible/k3s/` owns k3s on every node (`k3s-vm-control-01` as `server`,
  the Pi and both workers as `agent`) via the pinned `k3s.orchestration`
  collection.
- `k3s.yml` composes the collection's `prereq` / `k3s_server` / `k3s_agent`
  roles rather than running `k3s.orchestration.site`. That playbook's
  `raspberrypi` role edits the Pi's `/boot` cmdline and reboots it, which
  kills cloudflared.
- **Bump k3s in `vars/versions.yml`, in the same commit as
  `deploy/setup/traefik/traefik-helm-chart.yaml`.** k3s only serves the
  Traefik chart bundled with the installed version.
  `scripts/check-traefik-pin.sh` enforces the pair (`--online` checks the
  release manifest). One minor version at a time.
- **Node DNS is Quad9, not Pi-hole.** `tasks/node-dns.yml` writes a netplan
  drop-in and asserts it took. A node whose resolver is a cluster workload
  can't cold-start the cluster. Pods still use CoreDNS.
- The collection restarts k3s on every run, synchronously. That is by
  design, not drift. Pods survive a server restart, so the tunnel only
  blips; `ansible.cfg`'s SSH keepalives cover it. Do first runs by hand from
  the LAN. `forks = 4` hits all three agents at once. Drop to `forks = 1` if
  a second server is ever added, since rolling past a lone etcd member
  loses quorum.
- **An open 6443 is not a ready API.** k3s binds early, and `kubectl` fails
  immediately rather than honouring `--timeout`. Gate on
  `k3s kubectl get --raw /readyz` with retries.
- `token` is left undefined. The server role reads the existing token and
  the agent play consumes it in the same run.
- Datastore: embedded etcd, single member (`cluster-init: true`).
  `server_config_yaml` snapshots every 6h (retention 4) to
  `/var/lib/rancher/k3s/server/db/snapshots`. See `RECOVERY.md`.
- **The Pi's node identity is load-bearing.** It must rejoin as
  `k8s-storage-anchor-01` with `ticklethepanda.dev/lvm-vg=data`, since the
  OpenEBS PVs' `nodeAffinity` and the `lvm-data` topology key off both.
  `host_vars/k8s-storage-anchor-01.yml` sets the label and taint at
  registration.
- `k3s.yml` writes `/etc/rancher/k3s/registries.yaml` with a read-only
  Docker Hub PAT (`DOCKER_PULL_USERNAME` / `DOCKER_PULL_TOKEN`: `prod`
  secrets in CI, the gitignored root `.env` by hand). With no creds the
  file is left as-is.
- No `--check` drift gate, since the collection skips its tasks under
  `--check`. CI asserts four Ready nodes and the server taint instead.
- Bundled addons the repo owns are disabled in `server_config_yaml`
  (`disable: [servicelb, traefik, local-storage]`).
