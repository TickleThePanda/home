# Recovering the k3s control-plane without CI

The control-plane is `k3s-vm-control-01` (`192.168.1.32`) — the sole k3s
server, running embedded etcd with a **single member**. There is no peer to
fail over to, so recovery means restoring an etcd snapshot.

CI reaches the LAN through the in-cluster `cloudflared` Deployment (see
`ansible/node/RECOVERY.md`). When the API is down CI cannot reach anything —
work from the LAN, or from the tailnet via the router's Tailscale subnet route
(independent of the cluster — see `ansible/node/RECOVERY.md`), with WARP
disconnected, using the `deploy` key:

```sh
warp-cli disconnect ; ip route get 192.168.1.32   # must NOT be CloudflareWARP
ssh deploy@192.168.1.32
```

## Snapshots

`server_config_yaml` schedules `k3s etcd-snapshot` every 6h (retention 4).
They land on the root filesystem:

```sh
sudo k3s etcd-snapshot ls
ls -la /var/lib/rancher/k3s/server/db/snapshots/
sudo k3s etcd-snapshot save              # take one now, before anything risky
```

Copy the newest off the box — `scp deploy@192.168.1.32:/var/lib/rancher/k3s/server/db/snapshots/<name> .`

## k3s will not start / etcd is corrupt

```sh
sudo journalctl -u k3s -n 200 --no-pager
```

- **Bad `config.yaml`.** Written by `ansible/k3s/`'s `k3s_server` role. Move it
  aside to confirm, then fix the value in `group_vars/k3s_cluster.yml` /
  `vars/versions.yml` and re-run `k3s.yml` — not on the node.
  `cluster-init: true` must stay set; removing it makes k3s fall back to
  SQLite and ignore the etcd data.
- **etcd will not come up.** Restore the newest snapshot:
  ```sh
  sudo systemctl stop k3s
  sudo k3s server \
    --cluster-reset \
    --cluster-reset-restore-path=/var/lib/rancher/k3s/server/db/snapshots/<name>
  # wait for "etcd is running, restart without --cluster-reset flag now"
  sudo systemctl start k3s
  ```
  Loses whatever changed since that snapshot. The agents (Pi + workers)
  reconnect on their own.

## The control-plane VM is gone

Recreate it through the `proxmox` job — same VMID (132), IP, name, and SSH
key, so it comes back as `k3s-vm-control-01` at `192.168.1.32`. Then, before
letting `ansible/k3s/k3s.yml` reinstall it clean, restore etcd from the
newest snapshot with `--cluster-reset` as above (copy the snapshot onto the
fresh VM first). Node objects for the agents persist in the restored
datastore; they reconnect.

## No HA — this is accepted

A single etcd member has no fault tolerance: the VM being down is the API
being down. The tradeoff buys a simpler cluster (no etcd quorum to manage on
every `k3s.yml` run) for a home setup where the recovery path is a snapshot
restore, not a failover. Adding a second server later needs an **odd** total
(3) and `--forks=1` runs — see the `k3s.orchestration` collection README.

## CI transport

`cloudflared` (2 replicas) runs on the two worker VMs — it is tainted off
`k3s-vm-control-01` and Prefer-off the Pi. If **both** workers are down, CI
cannot reach the LAN even if the control-plane is healthy. A human still can,
via the router's Tailscale subnet route — see `ansible/node/RECOVERY.md`.
