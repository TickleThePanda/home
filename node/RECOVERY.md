# Recovering k8s-manager-1 without CI

## Why this file exists

CI reaches the LAN through the `cloudflared` Deployment running **inside** the
cluster (`deploy/setup/cloudflared/tunnel.yaml`). Its logs show
`originService=warp-routing` carrying traffic to `192.168.1.2:22` and `:6443`
— that pod is the Cloudflare Zero Trust private-network connector.

So the management path depends on the thing it manages. When k3s is down, or
`cloudflared` is not scheduled, **GitHub Actions cannot reach the node at all**
— precisely when you need it. This was a deliberate tradeoff (the alternative
was a second connector as a node-level systemd service). The cost of that
choice is this document.

Everything below assumes you are **on the LAN**, not going through WARP.

## First: get on the LAN

```sh
# WARP off. Straight to the node's real address.
ssh panda@192.168.1.2
```

`panda` has password-sudo via the `sudo` group, and a scoped NOPASSWD set in
`/etc/sudoers.d/panda-k3s-admin` for the common k3s operations.

If SSH itself is unreachable, it is a keyboard-and-monitor trip: the Pi is a
single SD card, `/dev/root` on `mmcblk0p2`, static IP `192.168.1.2/24` set in
`/etc/dhcpcd.conf`.

## Is it k3s, or is it the tunnel?

```sh
sudo systemctl status k3s
sudo k3s kubectl get nodes
sudo k3s kubectl get pods -n default -l pod=cloudflared
```

- **k3s down** → see "k3s will not start".
- **k3s up, cloudflared not Ready** → the cluster is fine and only remote
  access is broken. Check the `tunnel-token` Secret still exists; it is
  created out-of-band and is *not* in this repo.

## A managed run left the node mid-upgrade

The upgrade runs detached as a transient unit, so it survives losing the SSH
connection. Check what it did:

```sh
sudo cat /var/log/k3s-managed-upgrade.status
sudo systemctl status k3s-managed-upgrade
```

The status file logs each step and ends in `DONE <version>` or `FAILED`. The
script's error trap always tries to `systemctl start k3s` before exiting, so a
failed upgrade should still leave the cluster running on *some* version.

## k3s will not start

```sh
sudo journalctl -u k3s -n 200 --no-pager
```

Most likely causes, in order:

1. **A bad config drop-in.** `/etc/rancher/k3s/config.yaml.d/10-k3s.yaml` is
   written by `node/tasks/k3s-config.yml`. Move it aside and start k3s to
   confirm:
   ```sh
   sudo mv /etc/rancher/k3s/config.yaml.d/10-k3s.yaml /tmp/
   sudo systemctl start k3s
   ```
   Fix the value in `node/vars/versions.yml` rather than on the node.

2. **A half-finished upgrade.** Restore the datastore (below).

3. **Disk full.** Single 30G SD card. `df -h /`. The backups this playbook
   writes to `/var/backups/k3s/` are a plausible culprit — prune old ones.

## Restore the datastore

The datastore is SQLite/kine, **not** etcd, so `k3s etcd-snapshot` does not
apply. `node/templates/k3s-managed-upgrade.sh.j2` archives it before every
version change.

```sh
ls -la /var/backups/k3s/
sudo systemctl stop k3s
sudo /usr/local/bin/k3s-killall.sh          # clears leftover containers/mounts
sudo tar -xzf /var/backups/k3s/k3s-<version>-<stamp>.tar.gz -C /
sudo systemctl start k3s
```

The archive contains `var/lib/rancher/k3s/server/db`,
`var/lib/rancher/k3s/server/token` and `etc/rancher/k3s`. The token matters:
restoring the database without it leaves the server unable to decrypt its own
stored secrets.

To go back to a previous k3s binary as well:

```sh
curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION=v1.32.6+k3s1 sh -s - server
```

## Run the playbook by hand

Everything CI does can be done from the laptop on the LAN:

```sh
cd node
ansible-playbook site.yml --check --diff -e node_user=panda -K   # dry run
ansible-playbook site.yml --diff -e node_user=panda -K           # apply
```

`node_user=panda` because the `deploy` user's private key lives in the GitHub
secret `NODE_SSH_KEY`, not on the laptop. Use `-e node_user=`, **not** `-u` —
the inventory sets `ansible_user`, and an inventory var beats the `-u` flag,
so `-u panda` is silently ignored. `-K` prompts for panda's sudo password.

## Kubeconfig has stopped working

Known and unrelated to any of the above — the client cert in the local
kubeconfig does not auto-rotate. If `kubectl` says *"server has asked for the
client to provide credentials"*:

```sh
sudo systemctl restart k3s
sudo cat /etc/rancher/k3s/k3s.yaml     # then copy to ~/.kube/config
# and change server: 127.0.0.1 -> 192.168.1.2
```

## Things deliberately not automated

- `/etc/dhcpcd.conf` — the static IP. A bad networking change costs a
  keyboard-and-monitor trip, so it is left to hand-editing.
- `/var/lib/rancher/k3s/server/manifests/` — k3s rewrites this directory on
  every start. Never put anything there expecting it to survive.
- The out-of-band Secrets (`tunnel-token`, `cloudflare-api-token-secret`,
  `lldap-credentials`, `pocket-id-secret`, `tinyauth-secrets`,
  `label-studio-admin`). Nothing in this repo records how to recreate them —
  a genuine gap, worth closing separately.
