# Recovering k8s-manager-1 without CI

`k8s-manager-1` is the Pi — a k3s **agent** and the storage anchor for every
`lvm-data` PV. The control-plane is `k3s-vm-control-01`; for a broken API or a
lost datastore see [`../k3s/RECOVERY.md`](../k3s/RECOVERY.md).

## Why this file exists

CI reaches the LAN through the `cloudflared` Deployment running **inside** the
cluster (`deploy/setup/cloudflared/tunnel.yaml`). Its logs show
`originService=warp-routing` carrying traffic to `192.168.1.2:22` and
`192.168.1.32:6443` — that pod is the Cloudflare Zero Trust private-network
connector, and it runs on the two worker VMs.

So the management path depends on the thing it manages. When the control-plane
is down, or both workers are, **GitHub Actions cannot reach the node at all**
— precisely when you need it. This was a deliberate tradeoff (the alternative
was a second connector as a node-level systemd service). The cost of that
choice is this document.

A human isn't stuck the same way CI is: the router runs its own Tailscale
subnet router + exit node (`ansible/router/tasks/tailscale.yml`), advertising
`192.168.1.0/24` independently of the cluster. Join the tailnet and the node
is reachable without being physically on the LAN — the router itself, not a
cluster workload, is what has to be up.

Everything below assumes you are **on the LAN or the tailnet**, not going
through WARP.

## First: get off WARP

```sh
warp-cli --accept-tos disconnect
ip route get 192.168.1.2    # must say `dev wlan0`/`dev eth0`/`dev tailscale0`, NOT CloudflareWARP
ssh panda@192.168.1.2
```

Check the route rather than trusting that WARP is off. It captures the node's
address even from a laptop already on `192.168.1.0/24` (or on the tailnet), so
a session that looks fine can be running through the `cloudflared` pod — the
very thing you are about to restart. It will drop at the worst possible
moment.

`panda` has password-sudo via the `sudo` group, and a scoped NOPASSWD set in
`/etc/sudoers.d/panda-k3s-admin` for the common k3s operations.

If SSH itself is unreachable, it is a keyboard-and-monitor trip: the Pi boots
from a single SSD, `/` on `/dev/sda2`, static IP `192.168.1.2/24` set in
`/etc/dhcpcd.conf`. Most of that disk is LVM — see `STORAGE.md`.

## Is it k3s-agent, or is it the tunnel?

The Pi has no API — check it against the server:

```sh
sudo systemctl status k3s-agent
KUBECONFIG=~/.kube/config kubectl get nodes            # or run from control-01
KUBECONFIG=~/.kube/config kubectl get pods -n default -l pod=cloudflared -o wide
```

- **k3s-agent down** → see "k3s-agent will not start".
- **agent up, cloudflared not Ready** → the cluster is fine and only remote
  access is broken. cloudflared runs on the worker VMs; check the
  `tunnel-token` Secret still exists (created out-of-band, *not* in this repo).

## k3s-agent will not start

```sh
sudo journalctl -u k3s-agent -n 200 --no-pager
```

Most likely causes, in order:

1. **A bad config.** `/etc/rancher/k3s/config.yaml` on the Pi is written by
   `ansible/k3s/` from `host_vars/k8s-manager-1.yml` (`agent_config_yaml` —
   the `lvm-vg` label and storage-anchor taint). Move it aside and start the
   agent to confirm:
   ```sh
   sudo mv /etc/rancher/k3s/config.yaml /tmp/
   sudo systemctl start k3s-agent
   ```
   Fix the value in `ansible/k3s/`, not on the node. Starting without it drops
   the `lvm-vg=data` label, so storage pods will not bind — re-run
   `ansible/k3s/k3s.yml` to restore it.

2. **A missing mount.** The agent's state is on LVM volumes and
   `k3s-agent.service.d/10-storage-mounts.conf` blocks it from starting
   without them. The journal names the path. See `STORAGE.md`.
   ```sh
   findmnt /var/lib/rancher/k3s/agent /var/lib/kubelet
   sudo mount -a && sudo systemctl start k3s-agent
   ```

3. **Wrong server address / token.** The agent joins
   `https://192.168.1.32:6443` (`api_endpoint`), token in
   `/etc/systemd/system/k3s-agent.service.env`. If `k3s-vm-control-01` was
   rebuilt with a new token, re-run `ansible/k3s/k3s.yml`.

4. **Disk full.** `df -h /var/lib/rancher/k3s/agent /var/lib/kubelet /`. Each
   is its own volume, so a full one is contained. Raise the size in
   `vars/storage.yml`; it applies online.

## The Pi died

Rebuild the layer below k3s (`ansible/node/` — see `STORAGE.md` for the SSD), then
`ansible/k3s/k3s.yml` rejoins it as an agent named `k8s-manager-1`. The
`lvm-data` PV data lives in VG `data` on the SSD: it survives a reinstall if
the disk is intact, and is lost with the disk — there is no replication, so
restore those apps from their own backups.

## Run the playbook by hand

Everything CI does can be done from the laptop on the LAN or the tailnet:

```sh
cd ansible
ansible-galaxy collection install -r requirements.yml                     # once per machine
ansible-playbook node/node.yml --check --diff -e node_user=panda -K       # dry run
ansible-playbook node/node.yml --diff -e node_user=panda -K               # apply
```

The collections are not optional — ansible-core ships none, and the storage
tasks need `lvol`, `filesystem` and `mount`. CI installs them the same way.

`node_user=panda` because the `deploy` user's private key lives in the GitHub
secret `NODE_SSH_KEY`, not on the laptop. Use `-e node_user=`, **not** `-u` —
the inventory sets `ansible_user`, and an inventory var beats the `-u` flag,
so `-u panda` is silently ignored. `-K` prompts for panda's sudo password.

For anything k3s-level (version, `config.yaml`, a stuck agent), the playbook
is `ansible/k3s/k3s.yml` instead — it manages every node as one cluster. It
needs the `deploy` key (the VMs have no `panda` user).

Don't scope this one with `--limit k8s-manager-1` to "just touch the Pi" — the
agent role reads a token fact that only the server play sets, and filtering
the server host out of the run leaves it undefined (`'token' is undefined`,
confirmed live). Run the whole playbook; it's idempotent, and every run
restarts k3s everywhere anyway (see the k3s layer pattern in `CLAUDE.md`).

## Every internal service looks down, but the cluster is fine

Check DNS first. pihole runs inside the cluster (on the Pi), so if the Pi or
its pods are down the LAN loses its resolver. The router advertises public
resolvers too, so clients fail over and stay there. Public names keep working
while `*.internal.ticklethepanda.co.uk` resolves to nothing.

The **k3s nodes** do not depend on pihole — `ansible/k3s/` pins the VMs to
Quad9 and `ansible/node/` pins the Pi — so a pihole outage never wedges the cluster
itself, only client name resolution.

```sh
resolvectl status          # client's Current DNS Server should be 192.168.1.10
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
client to provide credentials"*, on `k3s-vm-control-01`:

```sh
sudo systemctl restart k3s
sudo cat /etc/rancher/k3s/k3s.yaml     # then copy to ~/.kube/config
# and change server: 127.0.0.1 -> 192.168.1.32
```

The CI `KUBE_CONFIG` prod secret embeds the same cert; regenerate it the same
way (`gh secret set KUBE_CONFIG --env prod`, server `https://192.168.1.32:6443`).

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
- The out-of-band Secrets (`tunnel-token`, `cloudflare-api-token-secret`,
  `lldap-credentials`, `pocket-id-secret`, `tinyauth-secrets`,
  `label-studio-admin`, `github-commit-status`). Nothing in this repo
  records how to recreate them — a genuine gap, worth closing separately.
  `github-commit-status` (namespace `flux-system`, key `token`) is a GitHub
  PAT with commit-status write on `ticklethepanda/home`; without it the
  Flux → GitHub commit-status Alert just logs auth errors and deploys are
  unaffected.
