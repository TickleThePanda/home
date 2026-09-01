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
warp-cli --accept-tos disconnect
ip route get 192.168.1.2    # must say `dev wlan0`/`dev eth0`, NOT CloudflareWARP
ssh panda@192.168.1.2
```

Check the route rather than trusting that WARP is off. It captures the node's
address even from a laptop already on `192.168.1.0/24`, so a session that looks
local can be running through the `cloudflared` pod — the very thing you are
about to restart. It will drop at the worst possible moment.

`panda` has password-sudo via the `sudo` group, and a scoped NOPASSWD set in
`/etc/sudoers.d/panda-k3s-admin` for the common k3s operations.

If SSH itself is unreachable, it is a keyboard-and-monitor trip: the Pi boots
from a single SSD, `/` on `/dev/sda2`, static IP `192.168.1.2/24` set in
`/etc/dhcpcd.conf`. Most of that disk is LVM — see `STORAGE.md`.

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

3. **A missing mount.** k3s's state is on LVM volumes and refuses to start
   without them. The journal names the path. See `STORAGE.md`.
   ```sh
   findmnt /var/lib/rancher/k3s/agent /var/lib/rancher/k3s/server /var/lib/kubelet
   sudo mount -a && sudo systemctl start k3s
   ```

4. **Disk full.** `df -h /var/lib/rancher/k3s/agent /var/backups /`. Each is
   its own volume, so a full one is contained. `/var/backups/k3s/` is unpruned
   and the usual culprit. Raise the size in `vars/storage.yml`; it applies
   online.

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
ansible-galaxy collection install -r requirements.yml            # once per machine
ansible-playbook site.yml --check --diff -e node_user=panda -K   # dry run
ansible-playbook site.yml --diff -e node_user=panda -K           # apply
```

The collections are not optional — ansible-core ships none, and the storage
tasks need `lvol`, `filesystem` and `mount`. CI installs them the same way.

`node_user=panda` because the `deploy` user's private key lives in the GitHub
secret `NODE_SSH_KEY`, not on the laptop. Use `-e node_user=`, **not** `-u` —
the inventory sets `ansible_user`, and an inventory var beats the `-u` flag,
so `-u panda` is silently ignored. `-K` prompts for panda's sudo password.

## Every internal service looks down, but the cluster is fine

Check DNS first. pihole runs inside the cluster, so k3s downtime takes the LAN's
resolver with it. The router advertises public resolvers too, so clients fail
over and stay there. Public names keep working while
`*.internal.ticklethepanda.co.uk` resolves to nothing.

```sh
resolvectl status          # Current DNS Server should be 192.168.1.10
sudo systemctl restart systemd-resolved
```

Check the cluster separately, bypassing DNS. A 401 is tinyauth challenging you,
so the ingress is healthy:

```sh
curl -k -o /dev/null -w '%{http_code}\n' \
    --resolve internal.ticklethepanda.co.uk:443:192.168.1.19 \
    https://internal.ticklethepanda.co.uk/
```

The real fix is the router advertising only `192.168.1.10`. That is router
config, not in this repo.

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
- Enabling I2C for the Argon ONE fan controller (`argon-fan.service`).
  `/boot/config.txt` and `/etc/modules` only take effect after a reboot,
  and a full reboot briefly kills the `cloudflared` pod CI's own connection
  runs through — worse than a k3s-only restart, where pods survive. Enable
  it by hand once:
  ```sh
  sudo sed -i 's/^#dtparam=i2c_arm=on/dtparam=i2c_arm=on/' /boot/config.txt
  echo i2c-dev | sudo tee -a /etc/modules
  sudo reboot
  ```
  Confirm with `ls /dev/i2c-1`. Until this is done, `argon-fan.service`
  crash-loops harmlessly (`SMBus` can't open the device) and self-heals once
  it's live.
- The partition table and the `data` volume group. The playbook manages logical
  volumes *on top of* them but will not create them, for the same reason — see
  `STORAGE.md`.
- `/var/lib/rancher/k3s/server/manifests/` — k3s rewrites this directory on
  every start. Never put anything there expecting it to survive.
- The out-of-band Secrets (`tunnel-token`, `cloudflare-api-token-secret`,
  `lldap-credentials`, `pocket-id-secret`, `tinyauth-secrets`,
  `label-studio-admin`, `github-commit-status`). Nothing in this repo
  records how to recreate them — a genuine gap, worth closing separately.
  `github-commit-status` (namespace `flux-system`, key `token`) is a GitHub
  PAT with commit-status write on `ticklethepanda/home`; without it the
  Flux → GitHub commit-status Alert just logs auth errors and deploys are
  unaffected.
