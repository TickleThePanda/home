# bind

Authoritative-only BIND9 for `home.arpa` and the per-VLAN reverse zones
(`1.` / `10.` / `20.` / `30.` / `40.` `.168.192.in-addr.arpa`). No recursion —
Unbound stub-zones each of them here.

**Every record is written by dynamic update** — the zone files carry only
NS/SOA. Three writers, each with its own TSIG key:

- **Kea DHCP-DDNS** (`k8s/apps/internal/network/dhcp-kea/`) — every lease's
  hostname, qualified into `home.arpa`, plus its PTR.
- **external-dns** (`k8s/apps/internal/network/externaldns/`) — MetalLB
  `LoadBalancer` Services / Ingresses annotated with
  `external-dns.kubernetes.io/hostname` (v0.22 dropped the `.alpha`).
- **external-dns-static** (`k8s/apps/internal/network/dns-static/`) — a
  hand-maintained list of non-DHCP hosts (`gateway`, the k3s VMs) as
  `DNSEndpoint` resources.

`entrypoint.sh` seeds each zone file into the PVC **only if it isn't already
there** — `named` owns the copy afterwards. So **adding a zone** (new
`zones/*.zone` + `named.conf` block + kustomization entry) is an ordinary
deploy: the configMap hash rolls the pod, the entrypoint copies just the new
file, existing zones and their live records are untouched. There is nothing
to "re-seed" for a record change — that goes through one of the writers above.

## TSIG keys

Each writer authenticates with its own hmac-sha256 key. The Secrets are
SOPS-encrypted in git (see `.claude/rules/secrets.md`):

- `bind-tsig` (`secret.sops.yaml` here): both `key {}` blocks, for
  `named.conf`'s include.
- `kea-ddns-tsig` (`../dhcp-kea/`): Kea's base64 secret only.
- `externaldns-tsig` (`../externaldns/`): key name plus base64 secret, also
  used by `../dns-static/`.

To rotate, generate new keys, then update all three with `sops`:

```sh
tsig-keygen -a hmac-sha256 kea         > kea.conf
tsig-keygen -a hmac-sha256 externaldns > externaldns.conf
cat kea.conf externaldns.conf          > keys.conf       # bind-tsig: keys.conf
sed -n 's/.*secret "\(.*\)";/\1/p' kea.conf | tr -d '\n'         # kea-ddns-tsig: ddns-secret
sed -n 's/.*secret "\(.*\)";/\1/p' externaldns.conf | tr -d '\n' # externaldns-tsig: key-secret
```

`named.conf`'s `update-policy` grants each key `zonesub` on the relevant
types. Tighten later (e.g. ExternalDNS to a `k8s.` sublabel).
