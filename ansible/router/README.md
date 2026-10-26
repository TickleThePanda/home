# ansible/router

The gateway's config (GL.iNet Flint 2, OpenWrt, `192.168.1.1`). The firmware
in [`../../router/`](../../router/) owns packages and static files; this
playbook owns everything a keep-settings sysupgrade carries over. See
[Who owns what](../../router/README.md#who-owns-what).

Applied by the `router` step in `.github/workflows/deploy-infra.yaml` on every
push touching `ansible/router/**`, and by
`.github/workflows/router-firmware.yaml` after a firmware flash, over SSH as
`root` through the same cloudflared tunnel the `node` job uses.

## What it manages

| What | Where |
|---|---|
| `/etc/config/network`, whole: LAN, PPPoE WAN, MACs, ULA, `br-trusted` and the `br-iot-*` VLAN bridges | `templates/config/network.j2` |
| `/etc/config/wireless`, whole: radios, the two trusted APs, the two 2.4 GHz IoT PPSK SSIDs | `templates/config/wireless.j2` |
| `/etc/config/dhcp`, whole: no DHCP or DNS served, dnsmasq as a relay to Kea for the trusted and IoT VLANs | `templates/config/dhcp.j2` |
| `/etc/config/firewall`, whole: `wan` drops, `tailscale0`, `trusted` and IoT zones, IoT punch-through rules | `templates/config/firewall.j2` |
| hostname, timezone, NTP server; dropbear and LuCI on the homelab and trusted addresses | options in `templates/apply.sh.j2` |
| Avahi mDNS reflector (homelab ↔ trusted ↔ every IoT VLAN) | `templates/avahi-daemon.conf.j2` |
| `authorized_keys`, root password, `/etc/sysupgrade.conf` (empty), dnsmasq/avahi/tailscale enabled | `templates/apply.sh.j2` |
| Tailscale login, advertised subnets and exit node | `templates/apply.sh.j2` |

A whole file is rendered and replaces the router's. Anything else in it is
drift: a LuCI edit there shows up in the next run's diff and gets reverted.
`system`, `dropbear` and `uhttpd` are mostly OpenWrt defaults, so only the
options the script sets are managed.

## How a run works

Each `community.openwrt` module task costs 8-10 SSH round trips through the
tunnel. So `router.yml` renders everything into one shell script, uploads it
with a few `raw` calls (dropbear caps a command at 9000 bytes) and runs it.
The script:

1. refuses to run over uncommitted `uci` changes (a half-finished LuCI edit);
2. compares each whole file by `uci export`, which ignores formatting and
   comments, and installs the ones that differ;
3. sets the partly managed options, the plain files, the root password, the
   services and Tailscale;
4. prints a masked before/after of each change, which the play turns into a
   diff on the controller.

The play then reloads only what changed. `network` and `dropbear` restart
detached, because they can drop the tunnel or the session, and the play
waits for the router to answer again.

`--check` runs the same script in compare-only mode, so CI's idempotence
re-run is a real check.

## Secrets

From the environment. CI reads them from the `prod` GitHub Environment. The
SSH key is `NODE_SSH_KEY`, whose public half the firmware bakes in.

| Env var / GitHub secret | |
|---|---|
| `ROUTER_PPPOE_USERNAME`, `ROUTER_PPPOE_PASSWORD` | PPPoE login |
| `ROUTER_WIFI_KEY` | trusted SSIDs' PSK |
| `ROUTER_WIFI_IOT_KEY_INET`, `_LOCAL`, `_ECHO` | IoT PPSKs, one per VLAN |
| `ROUTER_ROOT_PASSWORD_HASH` | optional: root's crypt hash. Unset leaves the password alone |
| `ROUTER_TS_AUTHKEY` | optional: tagged `tag:home-router`, reusable, non-ephemeral. Only used when the router is logged out of the tailnet; the run fails then if it's unset |

## Running it by hand

From a machine that reaches `192.168.1.1` (LAN or Tailscale):

```sh
cd ansible
cp router/secrets.env.example router/secrets.env   # once; fill it in
set -a; . router/secrets.env; set +a
ansible-galaxy collection install -r requirements.yml
ansible-playbook router/router.yml --check --diff --private-key ../deploy_key
ansible-playbook router/router.yml --diff --private-key ../deploy_key
```

## Notes

- `known_hosts` pins the router's dropbear host key. A keep-settings flash
  keeps it; a wiped router has a new one (see
  [Recovery](../../router/README.md#recovery)).
- `wireless.j2` hardcodes this board's radio paths: `radio0` is 2.4 GHz and
  `radio1` 5 GHz. Another router needs them re-read from a fresh
  `wifi config`.
- After a release upgrade, OpenWrt's migrations may touch an owned file. The
  next run shows that as a diff and reverts it; fold it into the template if
  it's wanted.
