# router

Firmware for the gateway (GL.iNet Flint 2 / GL-MT6000, `192.168.1.1`). Git
pins the OpenWrt release, package set and a few static files. CI builds the
image, flashes it with settings kept, checks the router and runs
[`../ansible/router/`](../ansible/router/) to reconcile its config.

| File | Role |
|---|---|
| `version.txt` | OpenWrt release. Renovate bumps it |
| `packages.txt` | Packages built into the image, on top of the target defaults |
| `files/` | Static files baked into the rootfs |
| `build.sh` | Image Builder run plus output checks. No secrets |
| `upgrade.sh` | `check` / `flash` / `verify` against the live router |

## Who owns what

A keep-settings sysupgrade replaces the rootfs and keeps only OpenWrt's keep
list (`sysupgrade -l`). That decides the split:

- **The image** owns packages and files the keep list drops:
  `files/etc/resolv.conf` (Quad9) and `files/etc/sysctl.d/99-tailscale.conf`
  (forwarding for the subnet router). `build.sh` also bakes in
  `ansible/files/ssh-authorized-keys`, so Ansible can reach a wiped router.
- **Ansible** owns what the keep list carries: UCI config, the Avahi config,
  `authorized_keys`, the root password and the Tailscale login.

Don't add paths to `/etc/sysupgrade.conf`. A file the router needs that the
keep list drops goes in `files/`.

## Packages

Every package the router needs is in the image. A sysupgrade replaces the
rootfs, so anything installed on the running router with `opkg`/`apk` is
gone after the next flash. Add it to `packages.txt` instead.

Image Builder installs each release's current feed versions. The image
carries its package list as `/etc/firmware.manifest`, with the exact versions
and the commit it was built from.

## How an update ships

1. **Validate (branch).** `validate.yaml` runs `build.sh`. The build fails on
   a package that doesn't resolve, an image not built for
   `glinet,gl-mt6000`, or a release that differs from `version.txt`.
   Renovate merges an OpenWrt patch once this passes. A new release
   series (24.10 → 25.12) waits for approval on the Dependency Dashboard.
2. **Deploy (`main`).** `deploy.yaml`'s `firmware` job
   (`router-firmware.yaml`) runs after infra on a push that changes
   `router/`. It rebuilds the image, and `upgrade.sh check` compares the
   router's release and `/etc/firmware.manifest` with the build. The manifest
   lists the packages and a hash of `files/` (its `@files` line). If they
   differ, the job:
   1. takes kured's lock, so no node or proxmox-01 reboot overlaps;
   2. runs `upgrade.sh flash`, then waits for the kube API and cloudflared;
   3. releases the lock and runs the router playbook again;
   4. keeps the manifest, the encrypted config backup and the log as a
      90-day artifact.
3. **Monthly rebuild.** On the 3rd at 11:00 UTC, `deploy.yaml` runs only the
   `firmware` job, which rebuilds the image for the pinned release. It flashes only if feed updates changed the
   package set, and is a no-op otherwise.

`ROUTER_AUTO_FLASH` in `router-firmware.yaml` gates steps 2 and 3. It's
`"true"`, so merges and the monthly rebuild flash unattended. Set it to
`"false"` to have a push or the schedule only report the drift as a warning;
a manual dispatch of Deploy still flashes.

The job runs inside the Deploy run, after infra and before cluster, so a flash
never overlaps other deploy work that needs the tunnel.

## What `upgrade.sh flash` does

1. Checks the board is `glinet,gl-mt6000` on squashfs.
2. Uploads the image to `/tmp`, verifies its checksum and runs
   `sysupgrade -T`.
3. Checks `sysupgrade -l` keeps the config, the dropbear host key (so
   `ansible/known_hosts` stays valid), `/etc/shadow` and
   `/etc/tailscale/tailscaled.state` (the tailnet identity).
4. Streams `sysupgrade -b -` into `age`, encrypted to
   `ansible/files/ssh-authorized-keys`. The plaintext never touches disk.
5. Starts `sysupgrade` detached, keeping settings.
6. Waits up to 15 minutes for the router to answer with a fresh uptime.
7. Verifies:
   - the release and package manifest;
   - every boot-time interface is up and WAN has an address;
   - `/etc/resolv.conf` is the image's, and internet and DNS work from the
     router;
   - every radio and SSID is up;
   - Tailscale reports `Running`;
   - SSH and LuCI listen on `192.168.1.1`.

The flash cuts CI's own path, because CI reaches the LAN through the
in-cluster cloudflared, which egresses through this router. So everything
after step 5 only polls. There is no remote rollback.

## When it fails

- **Before the flash** (unreachable, wrong board, `-T` rejects the image,
  keep list incomplete): nothing changed. The job releases kured's lock.
- **"sysupgrade did not reboot the router"**: the old firmware is still
  running. SSH in and run `sysupgrade -v /tmp/fw.bin` to see why.
- **"did not come back within 15 min"** or a failed check: the router needs a
  human. kured's lock stays held, which pauses node reboots on purpose.
  Release it once the router is fixed
  ([`k8s/infrastructure/controllers/kured/README.md`](../k8s/infrastructure/controllers/kured/README.md#the-lock)).
  Reach the router over Tailscale or the LAN. If it's down, follow
  [Recovery](#recovery).

Restore a backup from the run's `router-firmware-<run id>` artifact:

```sh
age -d -i ~/.ssh/id_ed25519 router-backup.tar.gz.age > backup.tar.gz
cat backup.tar.gz | ssh root@192.168.1.1 'cat > /tmp/b.tgz && sysupgrade -r /tmp/b.tgz && reboot'
```

The archive holds the PPPoE and Wi-Fi keys, `/etc/shadow` and the Tailscale
node key. Any recipient key can decrypt it, including the CI key, which
already holds all of these.

## Running it by hand

From a machine that reaches `192.168.1.1` (LAN or Tailscale):

```sh
./router/build.sh                                   # image in router/out/
SSH_KEY=deploy_key ./router/upgrade.sh check router/out
SSH_KEY=deploy_key DRY_RUN=1 ./router/upgrade.sh flash router/out   # stops before flashing
SSH_KEY=deploy_key ./router/upgrade.sh flash router/out
```

Take kured's lock first, as CI does. Then run the router playbook
([`../ansible/router/README.md`](../ansible/router/README.md)).
`VERSION=25.12.6 ./router/build.sh` builds another release without editing
`version.txt`.

## Recovery

The bootloader is never touched (sysupgrade images only), so GL.iNet's U-Boot
web recovery always survives. Recovery is physical: at the router, with a
laptop that has wired Ethernet and can set a static IP. While the router is
down, CI can't reach the cluster.

From least to most drastic:

1. **Boots, but the config is wrong.** Restore the last
   `router-firmware-<run id>` backup (above), or fix the template and run the
   playbook from the LAN.
2. **Locked out.** OpenWrt failsafe: power-cycle, tap reset when the LED
   flashes during boot, connect to `192.168.1.1` (`root`, no password). Then
   `firstboot && reboot` for a wiped router (step 4), or `mount_root` and
   edit `/etc/config/*`.
3. **Won't boot.** U-Boot web recovery: power off, hold reset, power on
   holding it 8-10 s until the LED flashes fast. Set the laptop to
   `192.168.1.2/24`, open `http://192.168.1.1` and upload `out/sysupgrade.bin`
   from `build.sh`.
4. **Wiped router** (after 3, `firstboot`, or a flash with "keep settings"
   off). This image boots with OpenWrt's defaults: LAN `192.168.1.1/24`,
   a DHCP server on it, no WAN, no Wi-Fi, and the repo's SSH keys in
   `authorized_keys`. Then:
   1. Keep it off the homelab switch until the playbook has run: its DHCP
      server would compete with Kea. Plug the laptop straight into a LAN
      port.
   2. Refresh the pinned host key, checking the fingerprint against the
      router's console: `ssh-keygen -R 192.168.1.1 -f ansible/known_hosts`,
      then `ssh-keyscan -t ed25519 192.168.1.1 >> ansible/known_hosts`.
      Commit it, or CI can't connect.
   3. Run the playbook ([`../ansible/router/README.md`](../ansible/router/README.md#running-it-by-hand))
      with `ROUTER_ROOT_PASSWORD_HASH` and `ROUTER_TS_AUTHKEY` set. It writes
      the whole config, sets the root password and logs in to the tailnet.
   4. Reconnect it to the network and check it as `upgrade.sh verify` does.

The tailnet ACL needs `tagOwners` for `tag:home-router` and `autoApprovers`
for `192.168.0.0/16` routes and the exit node, so the login needs no
approval in the admin console.
