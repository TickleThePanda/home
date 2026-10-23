---
paths:
  - "ansible/router/**"
  - "ansible/group_vars/router.yml"
  - "bootstrap/router/**"
---

# Router (OpenWrt)

- Two halves. `bootstrap/router/` is the Image Builder baseline, built and
  flashed **by hand**: the break-glass path. `ansible/router/` is a
  `community.openwrt` playbook and the source of truth for ongoing config,
  applied by CI. They need not stay in sync.
- CI connects as `root` through the same cloudflared tunnel as other
  layers. Zero Trust routes cover `192.168.1.0/24` and `192.168.10.0/24`
  (configured in the dashboard). IoT VLANs are deliberately not routed to
  CI. Secrets: `ROUTER_PPPOE_USERNAME` / `_PASSWORD`, `ROUTER_WIFI_KEY`,
  `ROUTER_WIFI_IOT_KEY_{INET,LOCAL,ECHO}` in `prod`.
- **CI egresses through this router's WAN.** The network and dropbear
  handlers detach with `community.openwrt.nohup` and reconnect with
  `wait_for_connection`. Don't fold them into a synchronous restart. Do the
  first apply after any reflash from the LAN.
- No Python: modules run in ash. The play is `gather_facts: false` and uses
  `community.openwrt.init` with `openwrt_install_recommended_packages:
  false`. Busybox already has what's needed, and the opkg path can report
  "changed" off a stale package list.
- Each task costs several SSH round trips. Prefer one templated `uci batch`
  or a dict `set` over loops.
- The root password isn't managed here. `bootstrap/router/` writes it once.

## VLANs and Wi-Fi

- Five untagged bridges: `br-lan` (`homelab`), `br-trusted`, `br-iot-inet` /
  `-local` / `-echo`. Cluster-side IPs stay on `homelab`, since MetalLB L2
  only ARPs on the node's segment.
- IoT is Wi-Fi only, on two 2.4 GHz PPSK SSIDs: `It reaches out (IoT)`
  (client-isolated) and `It reaches out (IoT peers)`. Full `wpad-mbedtls`;
  `config wifi-station` maps each key to a `vid`, `config wifi-vlan` bridges
  it. `dynamic_vlan=2` rejects unclassified stations; `br-iot-park` is a dead
  end.
- `tasks/firewall.yml`: each IoT zone is `input REJECT` + `forward REJECT`,
  with narrow rules for DHCP relay, DNS to Pi-hole, NTP and mDNS. Only
  `iot_inet` / `iot_echo` reach the WAN. Punch further holes per device.
- IoT VLANs get NTP from the router (`192.168.<n>.1`), and DHCP via dnsmasq
  relays to Kea.
- `tasks/tailscale.yml` runs a Tailscale subnet router and exit node,
  independent of the cluster. It is the human break-glass path onto the LAN.
