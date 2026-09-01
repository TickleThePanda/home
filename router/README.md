# router

A bootstrapped vanilla OpenWrt image for the GL.iNet Flint 2 (GL-MT6000) at
`192.168.1.1`. `build.sh` runs the OpenWrt Image Builder in a container and bakes
the current network config into `/etc/uci-defaults` scripts that apply on first
boot. It is built and flashed by hand — CI does not touch the router.

This is a one-time migration off the stock GL.iNet firmware. Ongoing config will
later move to an Ansible playbook (`community.openwrt`) over SSH; the image build
stays as the re-bootstrap / break-glass path.

## What the image sets

- LAN `192.168.1.1/24` on `br-lan` (`lan1..lan5`); WAN PPPoE on `eth1`.
- Wi-Fi: `It reaches out` (5 GHz) and `It reaches out (2.4G)`, WPA2/WPA3
  (`sae-mixed`), country `GB`.
- **No DNS or DHCP on the router.** Kea (`192.168.1.11`) leases;
  BIND/Unbound/Pi-hole resolve. dnsmasq is disabled; the router resolves for
  itself via a static `/etc/resolv.conf` pointing at Quad9.
- SSH (key auth, `deploy_key`) and LuCI, both bound to the LAN only.
- root/LuCI password unchanged from the current router.
- Tailscale subnet router (`192.168.1.0/24`) + exit node, up on first boot.
- Firewall: OpenWrt defaults, `wan` drops, plus a `tailscale0` zone.

## Build

Prerequisites:

- `podman` (or set `CONTAINER_RUNTIME=docker`), `openssl`, `python3` with
  `jinja2` (ships with Ansible).
- `deploy_key.pub` in the repo root (from the node bootstrap).
- Tailnet ACL, once: `tagOwners` for `tag:home-router`, and

  ```jsonc
  "autoApprovers": {
    "routes":   { "192.168.1.0/24": ["tag:home-router"] },
    "exitNode": ["tag:home-router"]
  }
  ```

  then generate a tagged, reusable, non-ephemeral auth key.

Then:

```sh
cp router/secrets.env.example router/secrets.env
# fill in secrets.env (see its comments for where each value lives)
./router/build.sh
```

The image lands at
`router/out/targets/mediatek/filogic/openwrt-*-glinet_gl-mt6000-squashfs-sysupgrade.bin`.

Override the release with `VERSION=24.10.6 ./router/build.sh`.

## Flash

Stage the recovery kit first (below). Then, from a wired client:

- LuCI *System → Backup/Flash Firmware → Flash image*, **"Keep settings" off**, or
- `scp` the `.bin` to the router and `sysupgrade -n /tmp/<image>.bin`.

"Keep settings off" is required so the GL config is wiped and the
`uci-defaults` scripts run.

### Check afterwards

```sh
ssh -i deploy_key root@192.168.1.1
ifstatus wan | grep -E '"up"|"address"'      # PPPoE up, public IP
ss -tlnp | grep -E ':22|:80|:443'            # bound to 192.168.1.1, not 0.0.0.0
tailscale status                              # Running, no manual `tailscale up`
```

Confirm a new LAN client gets a `192.168.1.21–200` lease (Kea) and resolves
through Pi-hole, and that the router shows its route + exit node already
approved in the Tailscale admin console.

## Recovery

The bootloader is never touched (sysupgrade images only), so GL.iNet's U-Boot
web recovery always survives. Recovery is LAN-local and physical — do the
migration in a maintenance window at the router, with a laptop that has wired
Ethernet and can set a static IP.

Stage before flashing:

- The build's `.bin`.
- The plain OpenWrt image for this device from downloads.openwrt.org (a
  config-less fallback if the overlay is the problem).
- The GL.iNet stock firmware `.bin` (dl.gl-inet.com) and
  `backup-GL-MT6000-2026-09-01.tar.gz` — the full rollback.

Failure modes, least to most drastic:

1. **Boots, LAN works, something's wrong** — fix the script, `./build.sh`,
   re-flash (`sysupgrade -n`).
2. **Locked out** — OpenWrt failsafe: power-cycle, tap reset when the LED
   flashes during boot, connect to `192.168.1.1` (`root`, no password). Then
   `firstboot && reboot`, or `mount_root` and edit `/etc/config/*`.
3. **Won't boot** — GL U-Boot web recovery: power off, hold reset, power on
   holding ~8–10 s until the LED flashes fast. Laptop to `192.168.1.2/24`, open
   `http://192.168.1.1`, upload any image.
4. **Full rollback** — flash GL stock (via U-Boot recovery or LuCI), boot it,
   restore `backup-GL-MT6000-2026-09-01.tar.gz` from the GL UI.

While the router is down the k3s node keeps its pods running but is unreachable
from outside, and CI cannot reach the cluster.

## Files

| Path | Role |
|---|---|
| `build.sh` | render overlay + run Image Builder |
| `render.py` | Jinja2: `templates/` → `build/files/` |
| `packages.txt` | extra packages |
| `secrets.env.example` | template for the gitignored `secrets.env` |
| `templates/etc/uci-defaults/*` | first-boot config (`NN-` ordered) |
| `templates/etc/hotplug.d/iface/99-tailscale-up` | `tailscale up` on WAN up |
| `templates/etc/sysctl.d/99-tailscale.conf` | forwarding / rp_filter |
| `authorized_keys.extra` | optional extra SSH keys (gitignored) |

Every file under `templates/` is a Jinja2 template mirroring its path in the
image rootfs; secrets are injected with the `shquote` filter
(`KEY={{ ROUTER_WIFI_KEY | shquote }}`), which emits a safe POSIX
single-quoted literal.
