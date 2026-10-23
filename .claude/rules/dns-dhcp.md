---
paths:
  - "deploy/internal/network/**"
  - "ansible/router/tasks/mdns-reflector.yml"
---

# DNS and DHCP

## CoreDNS (`internal.ticklethepanda.co.uk`)

- Hostnames live in
  `deploy/internal/network/coredns/zones/internal.ticklethepanda.co.uk.zone`,
  not Pi-hole. Unbound stub-zones this domain to CoreDNS; everything else
  recurses normally.
- The `*` wildcard resolves to the Traefik internal-ingress IP. Only add a
  record for an exception, i.e. a name that must resolve elsewhere: an `A`
  record above the wildcard, a bumped SOA serial, then
  `./apply-subset.sh deploy/internal/network/coredns`.
- In-cluster resolvers never see public records for this domain, e.g.
  cert-manager's DNS-01 TXT records. cert-manager checks propagation
  against public resolvers for that reason.

## Pi-hole

- Its settings are code-owned: `deploy/internal/network/pihole/*.env` and
  `*.txt` generate `FTLCONF_*` env vars via `configMapGenerator` +
  `envFrom`.
- `misc.readOnly=true` blocks every `pihole.toml` change from the UI, API
  or CLI. Query log, stats and block/allow lists still work in the UI.
- The UI greys out only individually env-forced fields. Anything else
  looks editable but fails to save with "config is currently in read-only
  mode". This is expected, not a bug.
- To change a setting: a scalar gets a line in `pihole.env`; a new array
  gets its own `.txt` wired into the `pihole-config` generator in
  `kustomization.yaml`. Then `./apply-subset.sh deploy/internal/network/pihole`.
- `dns-hosts.txt` is only for names outside `internal.ticklethepanda.co.uk`
  (e.g. the router).
- `listeningMode=ALL`, so it answers off-subnet clients. Every VLAN gets DNS
  `192.168.1.10`.

## DHCP (Kea) and reverse DNS

- Kea (`dhcp-kea/`) serves every subnet. It is macvlan-attached to
  `homelab`. Other VLANs reach it through dnsmasq relays on the router
  (`192.168.<n>.1` → `192.168.1.11`, relay-only), and Kea picks the subnet
  by `giaddr`. The pool in `dhcp4.json` subnet 1 enforces the `homelab`
  DHCP band.
- Each subnet's reverse zone (`<n>.168.192.in-addr.arpa`) is wired through
  BIND, Unbound, ExternalDNS and Kea DDNS. Forward names all land in
  `home.arpa` regardless of VLAN.

## mDNS

- An Avahi reflector on the router forwards mDNS between `br-lan`,
  `br-trusted` and every `br-iot-*`. That is how Home Assistant rediscovers
  IoT devices after an address change. It also publishes `gateway.local`.
- It is hub-and-all, so IoT VLANs see each other's mDNS records. That is
  accepted, since the firewall still blocks L3 between them.
