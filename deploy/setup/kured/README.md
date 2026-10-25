# kured

kured reboots k3s nodes after unattended-upgrades installs something that
needs a reboot (usually a kernel), one node at a time. The `proxmox-reboot`
CronJob does the same for proxmox-01, under the same lock.

## Schedule (Europe/London)

| When | What |
|---|---|
| Daily 01:00 / 01:30 | apt-daily / unattended-upgrades on the VMs and the Pi (`ansible/roles/unattended_upgrades/`) |
| Mon–Sat 03:00–05:00 | kured reboots any node with `/var/run/reboot-required` |
| Sun 01:30 | proxmox-01 runs `apt full-upgrade` (`ansible/proxmox/tasks/auto-upgrade.yml`) |
| Sun 03:00 | `proxmox-reboot` reboots proxmox-01 if it needs it |

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

## proxmox-01

proxmox-01 isn't a k3s node, so kured can't reboot it. The `proxmox-reboot`
CronJob (`proxmox-reboot/`) runs on the Pi, the only node that survives the
reboot:

1. Exits unless Prometheus reports `node_reboot_required` for proxmox-01.
2. Waits up to an hour until every node is Ready and schedulable, no
   blocking alert is firing, and it can take kured's lock.
3. Cordons the Pi, so pods evicted from the VMs wait for them instead of
   piling onto it.
4. Reboots proxmox-01 through the Proxmox API, as `kured@pve!reboot`. That
   identity holds only `Sys.PowerMgmt` and `Sys.Audit` on the node.
5. Waits for proxmox-01's boot time to change and for every node to be
   Ready, then uncordons the Pi and releases the lock.

The VMs stop workers first, then the control plane, each with up to 150s
to shut down gracefully, and start back up in the opposite order.

While it runs, the control plane, both workers, CI's tunnel and home DNS
(Unbound, CoreDNS) are all down. That takes 5–10 minutes.

A failure before step 4 undoes steps 2–3. A failure after it keeps the
lock, so kured stays paused, and fires `ProxmoxRebootFailed`. Read the
Job's log, fix the problem, then release the lock (below).

### The API token

The token's secret is shown only once, so it is created by hand. Ansible
creates the user, role and ACLs (`ansible/proxmox/tasks/auto-upgrade.yml`)
and grants the token its ACL once it exists.

```sh
ssh root@192.168.1.3 pveum user token add kured@pve reboot --privsep 1
```

Then:

1. Put `kured@pve!reboot=<value>` in `proxmox-reboot/secret.sops.yaml`
   (`kind: Secret`, name `proxmox-reboot`, key `token`).
2. Add the file to `proxmox-reboot/kustomization.yaml`.
3. Remove `suspend: true` from `proxmox-reboot/cronjob.yaml`.
4. Push. The infra layer grants the token its ACL.

To rotate the token, run `pveum user token remove kured@pve reboot` and
repeat these steps.

Test it without rebooting:

```sh
kubectl -n kube-system create job pr-test --from=cronjob/proxmox-reboot \
  --dry-run=client -o json \
  | jq '.spec.template.spec.containers[0].env += [{"name":"DRY_RUN","value":"true"}]' \
  | kubectl apply -f -
kubectl -n kube-system logs -f job/pr-test
kubectl -n kube-system delete job pr-test
```

A DRY_RUN run skips the reboot-required check. It takes the lock, cordons
the Pi, then uncordons it and releases the lock, without rebooting
anything.

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
