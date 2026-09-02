# router

Config for the home gateway — a GL.iNet Flint 2 (GL-MT6000) running vanilla
OpenWrt at `192.168.1.1`, hostname `gateway`.

| Directory | What it is |
|---|---|
| [`bootstrap/`](bootstrap/) | The OpenWrt Image Builder setup. Bakes a known-good baseline config into `/etc/uci-defaults` scripts, built and flashed **by hand**. The re-bootstrap / break-glass path — migrating off stock firmware, or rebuilding a bricked router from nothing. |
| [`ansible/`](ansible/) | The `community.openwrt` playbook — the source of truth for the router's **ongoing** config. Applied by the `router` job in `.github/workflows/deploy.yaml` on every push touching `router/ansible/**`, over SSH through the same cloudflared tunnel the `node` job uses. |

`bootstrap/` is a point-in-time baseline, enough to bring a router up to where
the playbook can take over. It does not track later changes — ongoing config
evolves in `ansible/`, and after a re-flash the next playbook run converges the
router to current state. The one thing `ansible/` deliberately does not manage is
the root password (set once by `bootstrap/` from the GL.iNet backup hash).

The router does no DNS or DHCP — Kea leases, BIND/Unbound/Pi-hole resolve. See
the root `README.md` and `CLAUDE.md` for the wider network picture.
