# router/ansible

The `community.openwrt` playbook that holds the gateway's ongoing config. It
reproduces the steady state of [`../bootstrap/`](../bootstrap/) and is the source
of truth from there on.

Applied by the `router` job in `.github/workflows/deploy.yaml` on every push
touching `router/ansible/**`, over SSH as `root` through the same cloudflared
tunnel the `node` job uses.

## What it manages

| Area | File |
|---|---|
| hostname, timezone | `tasks/system.yml` |
| dropbear + LuCI bound to the LAN | `tasks/mgmt-access.yml` |
| LAN address, PPPoE WAN, MACs, ULA | `tasks/network.yml` |
| radios + both APs | `tasks/wireless.yml` |
| `dhcp.lan` ignored, dnsmasq off, `/etc/resolv.conf` → Quad9 | `tasks/dhcp-dns.yml` |
| `wan` drops, `tailscale0` zone + forwardings | `tasks/firewall.yml` |
| forwarding sysctls, `tailscaled` enabled | `tasks/tailscale.yml` |

Not managed: the **root password** (`../bootstrap/` sets it once from the
GL.iNet backup hash) and the **Tailscale login / advertised routes** (persist in
`/etc/tailscale/tailscaled.state`; `../bootstrap/`'s hotplug script logs in on
first WAN up — change routes later with `tailscale set` by hand).

## Secrets

Three values come from the environment. CI reads them from the `prod` GitHub
Environment; the SSH key is `NODE_SSH_KEY` (its public half is already in the
router's `authorized_keys`).

| Env var | GitHub secret |
|---|---|
| `ROUTER_PPPOE_USERNAME` | `ROUTER_PPPOE_USERNAME` |
| `ROUTER_PPPOE_PASSWORD` | `ROUTER_PPPOE_PASSWORD` |
| `ROUTER_WIFI_KEY` | `ROUTER_WIFI_KEY` |

## Running it by hand

Do the first apply after any reflash from the LAN, not through CI — a `network`
reload can briefly drop the tunnel CI rides on.

```sh
cd router/ansible
set -a; . ../bootstrap/secrets.env; set +a   # same var names
ansible-galaxy collection install -r requirements.yml
ansible-playbook site.yml --diff --private-key ../../deploy_key
# then confirm it is idempotent:
ansible-playbook site.yml --check --diff --private-key ../../deploy_key
```

The router already carries this config from the image, so a first run against a
freshly-flashed router should report few or no changes and no interface bounce.

## Notes

- `known_hosts` pins the router's dropbear host key. After a reflash regenerates
  it, refresh with `ssh-keyscan -t ed25519 192.168.1.1` and verify the
  fingerprint from the console.
- The playbook assumes `radio0` is 2.4 GHz and `radio1` is 5 GHz; `wireless.yml`
  asserts this and fails clearly if a particular router differs.
- Anonymous UCI sections (the `tailscale0` firewall zone, the tailnet
  forwardings, the `eth1` device) are matched by their attributes, not by a
  positional index, so a run stays idempotent regardless of ordering.
