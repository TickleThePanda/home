# kured

kured reboots k3s nodes after unattended-upgrades installs something that
needs a reboot (usually a kernel), one node at a time.

## Schedule (Europe/London)

| When | What |
|---|---|
| Daily 01:00 / 01:30 | apt-daily / unattended-upgrades on the VMs and the Pi (`ansible/roles/unattended_upgrades/`) |
| Mon–Sat 03:00–05:00 | kured reboots any node with `/var/run/reboot-required` |

For each node, kured takes the lock, cordons, drains, reboots and
uncordons. It reboots nothing while any alert is firing, except
`HostRebootRequired` and `HostUpgradesPending`. A drain that doesn't finish
in 10 minutes is abandoned and retried 10 minutes later.

What a reboot takes down:

- **The Pi:** its `lvm-data` apps (Postgres included) stop until it is back.
  Nothing else can run them.
- **A worker:** a short blip for single-replica apps on it, such as Traefik.
- **The control plane:** about a minute of API downtime. Workloads keep
  running.

## The lock

The lock is the `weave.works/kured-node-lock` annotation on
`daemonset/kured` in `kube-system`. Whoever holds it is the only one
rebooting.

```sh
# Who holds it
kubectl -n kube-system get ds kured \
  -o jsonpath='{.metadata.annotations.weave\.works/kured-node-lock}'

# Pause all reboots (kured waits for any lock it doesn't hold)
kubectl -n kube-system annotate ds kured \
  weave.works/kured-node-lock='{"nodeID":"manual","metadata":{"unschedulable":false},"created":"'"$(date -u +%FT%TZ)"'","TTL":0}'

# Resume, or release a stuck lock
kubectl -n kube-system annotate ds kured weave.works/kured-node-lock-
```

Flux's drift detection doesn't revert the annotation, because the chart
doesn't render it.

## Reboot one node by hand

Take the lock (above) first. Then cordon and drain the node, reboot it,
uncordon it, and release the lock.
