# Storage layout for k8s-manager-1

## The shape

The node boots from a 512 GB SSD (`/dev/sda`, MBR):

```
sda1   256M   vfat   /boot            firmware reads this directly
sda2  29.6G   ext4   /                the OS, and nothing that grows
sda3  ~447G   LVM    PV -> VG "data"
```

Three of four MBR primaries; 512 GB is far under the 2 TiB MBR ceiling, so
there is no reason to convert to GPT.

**`/` and `/boot` are deliberately not on LVM.** Raspberry Pi OS boots via
`root=PARTUUID=` with no initramfs at all. Root-on-LVM means building and
maintaining one, which adds a boot-time failure mode to a machine whose
recovery path is a keyboard and a monitor (see `RECOVERY.md`). Nothing on root
grows, so it buys nothing.

Volume group `data` holds two different kinds of thing.

**Node state, carved by hand and managed by `tasks/storage.yml`:**

| LV | Mount | Size | Holds |
|---|---|---|---|
| `agent` | `/var/lib/rancher/k3s/agent` | 64G | containerd image store, container writable layers — kubelet's `imagefs` |
| `kubelet` | `/var/lib/kubelet` | 16G | emptyDir, pod ephemeral storage, PV bind mounts — kubelet's `nodefs` |
| `server` | `/var/lib/rancher/k3s/server` | 8G | the kine datastore and token |
| `backups` | `/var/backups` | 32G | `k3s-managed-upgrade.sh.j2` archives, which nothing prunes |

**Everything else — roughly 327G — is left unallocated on purpose.** That free
space *is* the persistent volume pool: the OpenEBS LVM LocalPV driver
(`deploy/setup/lvm-localpv/`) carves one real LV out of `data` per PVC on the
`lvm-data` StorageClass. Do not pre-carve it.

Thick volumes only, no thin pool. `dm_thin_pool` is not loaded, and on a single
disk with no capacity alerting a full thin pool means filesystems flipping
read-only with no warning. Thick means `vgs` free space is the truth.

## Who owns what

`tasks/storage.yml` owns the **steady state**: the `lvm2` package, the logical
volumes at the sizes in `vars/storage.yml`, their filesystems, their `fstab`
entries, the k3s drop-in and the TRIM timer. All of it is idempotent and runs
from CI.

It does **not** own the partition table or the volume group, and never will.
Creating `sda3` is the one operation with no recovery path that does not
involve physically visiting the Pi, and CI reaches this node through a pod
running *on* the node. That step is the runbook below, run by hand, once.

If the volume group is missing the playbook fails with a pointer back here
rather than trying to build it.

## Growing a volume

Raise the number in `vars/storage.yml` and let CI apply it. `lvol` runs with
`resizefs: true`, so the logical volume and the ext4 filesystem on it both
extend **online** — no unmount, no downtime, no reboot.

Shrinking is not supported and will fail rather than truncate. If you genuinely
need a volume smaller, that is a manual, offline job.

## One-time build (LAN-local)

Everything below happens on the LAN with WARP **off**. Step 5 stops k3s, which
takes down the `cloudflared` pod that CI arrives through — this cannot be run
from a GitHub runner.

```sh
ssh panda@192.168.1.2
```

### 1. LVM tooling

```sh
sudo apt update && sudo apt install -y lvm2
```

### 2. Create sda3

```sh
sudo parted /dev/sda unit MiB print free
```

Note the `Start` of the trailing free-space row, then:

```sh
sudo parted -a optimal /dev/sda -- mkpart primary <START>MiB 100%
sudo parted /dev/sda set 3 lvm on
sudo reboot
```

Reboot rather than `partx -a`: rereading the partition table of a live boot
disk is the kind of thing that works nine times out of ten.

### 3. Volume group and volumes

```sh
sudo pvcreate /dev/sda3
sudo vgcreate data /dev/sda3

sudo lvcreate -n agent   -L 64G data
sudo lvcreate -n kubelet -L 16G data
sudo lvcreate -n server  -L  8G data
sudo lvcreate -n backups -L 32G data

for lv in agent kubelet server backups; do
    sudo mkfs.ext4 -m 0 /dev/data/$lv
done
```

`-m 0` drops ext4's 5% root reserve, which does nothing useful on a data volume
and would cost ~6G across these four.

Confirm before going further — this is the last checkpoint where nothing has
moved:

```sh
sudo vgs data && sudo lvs data     # ~327G free
```

### 4. Stop the cluster

```sh
sudo systemctl stop k3s
sudo /usr/local/bin/k3s-killall.sh
```

`k3s-killall.sh` is required, not optional: the PV bind mounts under
`/var/lib/kubelet/pods/` hold the source directory busy. It only unmounts
paths matching `/run/k3s`, `/var/lib/kubelet/pods`, `/var/lib/kubelet/plugins`
and `/run/netns/cni-`, so it is safe against these mount points both now and
later — none of them collide.

### 5. Copy the data

```sh
sudo mkdir -p /mnt/migrate
for pair in agent:/var/lib/rancher/k3s/agent \
            kubelet:/var/lib/kubelet \
            server:/var/lib/rancher/k3s/server \
            backups:/var/backups; do
    lv=${pair%%:*}; src=${pair#*:}
    sudo mount /dev/data/$lv /mnt/migrate
    sudo rsync -aHAX --numeric-ids --info=progress2 "$src"/ /mnt/migrate/
    sudo umount /mnt/migrate
done
sudo rmdir /mnt/migrate
```

`--numeric-ids` matters. `vars/storage.yml` documents why: the uids and gids in
play are container-side and do not map to users on the node, so letting rsync
resolve them through `/etc/passwd` would rewrite them into something else.

The `agent` copy is ~11G onto a Pi — expect it to take a while.

### 6. Swap them in

```sh
for pair in agent:/var/lib/rancher/k3s/agent \
            kubelet:/var/lib/kubelet \
            server:/var/lib/rancher/k3s/server \
            backups:/var/backups; do
    src=${pair#*:}
    sudo mv "$src" "$src.old"
    sudo mkdir -p "$src"
done
```

Add to `/etc/fstab`:

```
/dev/data/agent    /var/lib/rancher/k3s/agent   ext4  noatime,nofail  0  2
/dev/data/kubelet  /var/lib/kubelet             ext4  noatime,nofail  0  2
/dev/data/server   /var/lib/rancher/k3s/server  ext4  noatime,nofail  0  2
/dev/data/backups  /var/backups                 ext4  noatime,nofail  0  2
```

Then `sudo mount -a` and check `findmnt` before starting anything.

(The playbook writes these same entries itself. Doing it here means the node
comes back up on its own, without waiting for a deploy.)

### 7. Start, then prove it

```sh
sudo systemctl start k3s
sudo k3s kubectl get nodes
sudo k3s kubectl get pods -A
sudo reboot
```

**The reboot is not optional.** It is the only thing that actually proves the
`fstab` entries are right; a working `mount -a` does not. After it comes back,
re-check `findmnt` and that pods are running.

### 8. Hand it to Ansible

From the laptop, still on the LAN:

```sh
cd node
ansible-galaxy collection install -r requirements.yml
ansible-playbook site.yml --check --diff -e node_user=panda -K
ansible-playbook site.yml --diff -e node_user=panda -K
```

The first apply adds the k3s drop-in and the TRIM timer; the volumes and mounts
should already report as unchanged. Once a second `--check` run is clean, merge
the branch and let CI take it from there.

### 9. Reclaim

Once you are satisfied — a few days is reasonable — delete the copies:

```sh
sudo du -sh /var/lib/rancher/k3s/*.old /var/lib/kubelet.old /var/backups.old
sudo rm -rf /var/lib/rancher/k3s/agent.old /var/lib/rancher/k3s/server.old \
            /var/lib/kubelet.old /var/backups.old
sudo rm -rf /boot.bak          # leftover from the SD -> SSD move
df -h /                        # ~15G used -> ~4G
```

## When something is wrong

**k3s will not start, and the journal mentions a mount.** The drop-in at
`/etc/systemd/system/k3s.service.d/10-storage-mounts.conf` names every mount
point in `node_lvs`, so k3s refuses to start when a volume is missing. Each
entry has to be a real mount point — `RequiresMountsFor` resolves a path and
its parents, so naming `/var/lib/rancher/k3s` would only require `/` and the
guard would quietly do nothing. This is deliberate: the alternative is k3s
starting against
an empty directory on root and quietly writing a second copy of its state.
Check `findmnt` and `journalctl -u k3s`, fix the mount, start k3s.

**The Pi booted but a volume is missing.** `fstab` carries `nofail`, so a bad
volume leaves the node up and reachable over SSH instead of dropping a headless
box to an emergency console. That is the trade `nofail` and `RequiresMountsFor`
make together — the machine stays reachable, the cluster stays stopped.

**The volume group is gone or degraded.** `sudo pvs`, `sudo vgs`, `sudo lvs`.
A missing PV means the partition table or the disk itself, not LVM.

**A `lvm-data` volume needs reading without Kubernetes.** They are ordinary
ext4 on ordinary LVs. `sudo lvs data` to find it, then
`sudo mount /dev/data/<lv> /mnt/somewhere`. Nothing about the CSI driver has to
be working.
