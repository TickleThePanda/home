# bind

Authoritative-only BIND9 for `home.arpa` and the per-VLAN reverse zones
(`1.` / `10.` / `20.` / `30.` / `40.` `.168.192.in-addr.arpa`). No recursion —
Unbound stub-zones each of them here.

**Every record is written by dynamic update** — the zone files carry only
NS/SOA. Three writers, each with its own TSIG key:

- **Kea DHCP-DDNS** (`deploy/internal/network/dhcp-kea/`) — every lease's
  hostname, qualified into `home.arpa`, plus its PTR.
- **external-dns** (`deploy/internal/network/externaldns/`) — MetalLB
  `LoadBalancer` Services / Ingresses annotated with
  `external-dns.kubernetes.io/hostname` (v0.22 dropped the `.alpha`).
- **external-dns-static** (`deploy/internal/network/dns-static/`) — a
  hand-maintained list of non-DHCP hosts (`gateway`, the k3s VMs) as
  `DNSEndpoint` resources.

`entrypoint.sh` seeds each zone file into the PVC **only if it isn't already
there** — `named` owns the copy afterwards. So **adding a zone** (new
`zones/*.zone` + `named.conf` block + kustomization entry) is an ordinary
deploy: the configMap hash rolls the pod, the entrypoint copies just the new
file, existing zones and their live records are untouched. There is nothing
to "re-seed" for a record change — that goes through one of the writers above.

## TSIG keys (out-of-band, like `tunnel-token`)

Each writer authenticates with its own hmac-sha256 key. Generate once and
create the Secrets:

```sh
tsig-keygen -a hmac-sha256 kea         > kea.conf
tsig-keygen -a hmac-sha256 externaldns > externaldns.conf
cat kea.conf externaldns.conf          > keys.conf

# BIND: both key {} blocks, for named.conf's include
kubectl -n bind create secret generic bind-tsig --from-file=keys.conf

# Kea D2: just kea's base64 (tsig-keys secret-file)
sed -n 's/.*secret "\(.*\)";/\1/p' kea.conf | tr -d '\n' > kea.secret
kubectl -n kea create secret generic kea-ddns-tsig --from-file=ddns-secret=kea.secret

# ExternalDNS: name + base64 as env (see that dir's manifests)
sed -n 's/.*secret "\(.*\)";/\1/p' externaldns.conf | tr -d '\n' > externaldns.secret
kubectl -n externaldns create secret generic externaldns-tsig \
  --from-literal=key-name=externaldns \
  --from-file=key-secret=externaldns.secret
```

`named.conf`'s `update-policy` grants each key `zonesub` on the relevant
types. Tighten later (e.g. ExternalDNS to a `k8s.` sublabel).

The live cluster secrets are the source of truth for the key material —
back them up (`kubectl get secret -n bind bind-tsig -o yaml`), don't commit.
