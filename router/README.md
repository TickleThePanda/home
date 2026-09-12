# router

Config for the home gateway — a GL.iNet Flint 2 (GL-MT6000) running vanilla
OpenWrt at `192.168.1.1`, hostname `gateway`.

This directory holds only [`bootstrap/`](bootstrap/): the OpenWrt Image Builder
setup, which bakes a known-good baseline into `/etc/uci-defaults` scripts and is
built and flashed **by hand**. It is the re-bootstrap / break-glass path —
migrating off stock firmware, or rebuilding a bricked router from nothing.

The router's **ongoing** config lives in
[`ansible/router/`](../ansible/router/), applied by CI on every push touching
`ansible/router/**`.

`bootstrap/` is a point-in-time baseline, enough to bring a router up to where
the playbook can take over. It does not track later changes — after a re-flash
the next playbook run converges the router to current state. The one thing the
playbook deliberately does not manage is the root password (set once by
`bootstrap/` from the GL.iNet backup hash).

The router does no DNS or DHCP — Kea leases, BIND/Unbound/Pi-hole resolve. See
the root `README.md` and `CLAUDE.md` for the wider network picture.
