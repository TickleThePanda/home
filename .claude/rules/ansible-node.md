---
paths:
  - "ansible/node/**"
  - "ansible/group_vars/pi.yml"
  - "ansible/host_vars/k8s-storage-anchor-01.yml"
---

# Node layer (the Pi, below k3s)

- `ansible/node/` owns the layer below k3s on the Pi: LVM volumes and mounts
  backing the PVs, sudoers, DNS resolver, swap, the Argon fan. k3s itself is
  `ansible/k3s/`. Applied by CI as `deploy`. Don't hand-edit the node.
- The Pi runs `k3s-agent`. `tasks/storage.yml`'s `RequiresMountsFor`
  drop-in targets `k3s-agent.service` and only the `agent` / `kubelet` LVs.
- This layer doesn't restart k3s, so it doesn't disturb CI's own transport
  (cloudflared runs in-cluster).

## Storage

`ansible/node/STORAGE.md` is the reference.

- Most of the SSD is `sda3`, one VG named `data`. Four LVs hold node state
  (sized in `vars/storage.yml`). The unallocated space **is the PV pool**,
  so don't pre-allocate it. The `server` LV is vestigial but kept, since
  shrinking fails.
- Grow a volume by editing `vars/storage.yml`. It extends online. Shrinking
  fails.
- **Ansible must not own the partition table or the VG.** CI reaches the Pi
  through a pod running on it, so a bad repartition has no remote recovery.
- `fstab` uses `nofail`, and the `RequiresMountsFor` drop-in stops
  `k3s-agent` if a volume is missing. The Pi stays reachable, rather than
  the kubelet writing to root until it fills.
- Pre-migration copies under `/mnt/disk` and `/var/lib/rancher/k3s/storage`
  (~700M) are the rollback. See the end of `STORAGE.md` to remove them.
