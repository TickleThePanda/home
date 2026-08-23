# Storage

The node boots from a 512 GB SSD (`/dev/sda`, MBR):

```
sda1   256M   vfat   /boot
sda2  29.6G   ext4   /
sda3  ~447G   LVM    PV -> VG "data"
```

`/` and `/boot` stay off LVM. Pi OS boots via `root=PARTUUID=` with no
initramfs, and root-on-LVM would need one built and maintained.

## Volume group `data`

Node state. Sizes live in `vars/storage.yml`, and `tasks/storage.yml` manages
the volumes, filesystems and mounts.

| LV | Mount | Size | Holds |
|---|---|---|---|
| `agent` | `/var/lib/rancher/k3s/agent` | 64G | containerd image store |
| `kubelet` | `/var/lib/kubelet` | 16G | emptyDir, PV bind mounts |
| `server` | `/var/lib/rancher/k3s/server` | 8G | kine datastore |
| `backups` | `/var/backups` | 32G | k3s upgrade archives |

The remaining ~327G is the PersistentVolume pool. The OpenEBS LVM LocalPV
driver (`deploy/setup/lvm-localpv/`) creates one LV per PVC on the `lvm-data`
StorageClass, which is the default. Don't pre-allocate it.

Volumes are thick, so `vgs` free space is real.

## Growing a volume

Raise the size in `vars/storage.yml` and let CI apply it. `lvol` runs with
`resizefs`, so it extends online. Shrinking fails rather than truncating.

## What Ansible does not own

The partition table and the volume group. CI reaches the node through a pod
running on it, so a bad automated repartition has no remote recovery. The
playbook fails with a pointer here when the VG is missing.

## Rebuilding the volume group

Only for a fresh disk. Run from the LAN, not through WARP — see `RECOVERY.md`.

```sh
sudo apt install -y lvm2
sudo parted /dev/sda unit MiB print free
```

The trailing free-space row becomes `sda3`. Originally it ran from 30560MiB to
the end of the disk.

```sh
sudo parted -a optimal /dev/sda -- mkpart primary <START>MiB 100%
```

`parted` warns that the result is `not properly aligned ... % 65535s != 0s`.
Ignore it. The USB-SATA bridge reports `optimal_io_size` as 33553920 bytes —
65535 × 512, a placeholder. Real alignment is fine: 512-byte blocks, zero
alignment offset, 1 MiB-aligned start. `sda1` and `sda2` fail the same check.

```sh
sudo parted /dev/sda set 3 lvm on
sudo reboot
sudo pvcreate /dev/sda3
sudo vgcreate data /dev/sda3
```

Then run the playbook (`RECOVERY.md`, "run the playbook by hand"). It builds
everything else.

## Troubleshooting

**k3s won't start and the journal names a mount.** The drop-in
`/etc/systemd/system/k3s.service.d/10-storage-mounts.conf` sets
`RequiresMountsFor` on each mount point, so k3s refuses to start without them —
otherwise it would write a second copy of its state to an empty directory on
root. Check `findmnt`, fix the mount, start k3s.

**A volume is missing but the Pi is reachable.** Intended. `fstab` uses
`nofail`, so the machine stays up while the cluster stays down.

**Reading an `lvm-data` volume without Kubernetes.** They are plain ext4 on
plain LVs:

```sh
sudo lvs data
sudo mount /dev/data/<lv> /mnt/somewhere
```

## Pre-migration leftovers

Every volume used to live on the root filesystem, either as a static
`local-storage` PV under `/mnt/disk` or a `local-path` directory under
`/var/lib/rancher/k3s/storage`. Both are now empty of anything the cluster
references, but the data is still there and is the rollback for the migration.

Reclaim ~700M once the migrated volumes have proven themselves:

```sh
sudo rm -rf /mnt/disk /var/lib/rancher/k3s/storage
```
