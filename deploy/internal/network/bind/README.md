# bind

Authoritative-only BIND9 for the local zones `home.arpa` and
`1.168.192.in-addr.arpa`. No recursion — Unbound stub-zones both zones here.

Records are written by dynamic update, not by hand:

- **Kea DHCP-DDNS** (`deploy/internal/network/dhcp-kea/`) — every lease's
  hostname, qualified into `home.arpa`, plus its PTR.
- **ExternalDNS** (`deploy/internal/network/externaldns/`) — MetalLB
  `LoadBalancer` Services / Ingresses annotated with
  `external-dns.alpha.kubernetes.io/hostname`.

The only seeded record is `gateway` → `192.168.1.1` (in `zones/*.zone`) — the
router can't be a DHCP client. The seed is copied to the PVC once; `named`
owns the zone files thereafter (rewrites them + a `.jnl` journal per update),
so a change to `zones/*.zone` after first deploy needs a re-seed
(`kubectl -n bind delete pvc bind-data` + redeploy) or a manual `nsupdate`.

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
