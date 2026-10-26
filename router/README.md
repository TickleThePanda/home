# router

Firmware for the gateway (GL.iNet Flint 2 / GL-MT6000, `192.168.1.1`). Git
pins the OpenWrt release and package set. CI builds the image, flashes it
with settings kept, checks the router and runs
[`../ansible/router/`](../ansible/router/) to reconcile its config.

| File | Role |
|---|---|
| `version.txt` | OpenWrt release. Renovate bumps it |
| `packages.txt` | Packages built into the image, on top of the target defaults |
| `build.sh` | Image Builder run plus output checks. No secrets |
| `upgrade.sh` | `check` / `flash` / `verify` against the live router |

[`../bootstrap/router/`](../bootstrap/router/) builds the same image with a
first-boot config overlay for a wiped or new router. Use that path for
recovery, not for upgrades.

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
   `glinet,gl-mt6000`, or a release that differs from `version.txt`. It also
   fails if a package in `router_firmware_packages` is missing from the
   image. Renovate merges an OpenWrt patch once this passes. A new release
   series (24.10 → 25.12) waits for approval on the Dependency Dashboard.
2. **Deploy (`main`).** `deploy-infra.yaml` rebuilds the image after the
   router playbook, and `upgrade.sh check` compares the router's release and
   `/etc/firmware.manifest` with the build. If they differ, the job:
   1. takes kured's lock, so no node or proxmox-01 reboot overlaps;
   2. runs `upgrade.sh flash`, then waits for the kube API and cloudflared;
   3. releases the lock and runs the router playbook again;
   4. keeps the manifest, the encrypted config backup and the log as a
      90-day artifact.
3. **Monthly rebuild.** On the 3rd at 11:00 UTC, `deploy.yaml` rebuilds the
   image for the pinned release. It flashes only if feed updates changed the
   package set, and is a no-op otherwise.

`ROUTER_AUTO_FLASH` in `deploy-infra.yaml` gates steps 2 and 3. While it's
`"false"`, only a manual dispatch of Deploy flashes. A push or the schedule
reports the drift as a warning.

## What `upgrade.sh flash` does

1. Checks the board is `glinet,gl-mt6000` on squashfs.
2. Uploads the image to `/tmp`, verifies its checksum and runs
   `sysupgrade -T`.
3. Checks `sysupgrade -l` keeps the config, the dropbear host key (so
   `ansible/known_hosts` stays valid), `/etc/shadow`,
   `/etc/tailscale/tailscaled.state` (the tailnet identity) and the files
   `ansible/router/tasks/firmware.yml` adds to `/etc/sysupgrade.conf`.
4. Streams `sysupgrade -b -` into `age`, encrypted to
   `ansible/files/ssh-authorized-keys`. The plaintext never touches disk.
5. Starts `sysupgrade` detached, keeping settings.
6. Waits up to 15 minutes for the router to answer with a fresh uptime.
7. Verifies:
   - the release and package manifest;
   - every boot-time interface is up and WAN has an address;
   - internet and DNS work from the router;
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
  ([`deploy/setup/kured/README.md`](../deploy/setup/kured/README.md#the-lock)).
  Reach the router over Tailscale or the LAN. If it's down, follow
  [`../bootstrap/router/README.md#recovery`](../bootstrap/router/README.md#recovery).

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
