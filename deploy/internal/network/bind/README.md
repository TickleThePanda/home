# bind

Authoritative-only BIND9 for `home.arpa` and the per-VLAN reverse zones
(`1.` / `10.` / `20.` / `30.` / `40.` `.168.192.in-addr.arpa`). No recursion —
Unbound stub-zones each of them here.

Records are written by dynamic update, not by hand:

- **Kea DHCP-DDNS** (`deploy/internal/network/dhcp-kea/`) — every lease's
  hostname, qualified into `home.arpa`, plus its PTR.
- **ExternalDNS** (`deploy/internal/network/externaldns/`) — MetalLB
  `LoadBalancer` Services / Ingresses annotated with
  `external-dns.kubernetes.io/hostname` (v0.22 dropped the `.alpha`).

Seeded records (hosts that aren't DHCP clients, so DDNS never sees them):
`gateway` → `192.168.1.1`, and the three k3s VMs `k3s-vm-control-01` /
`-worker-01` / `-worker-02` → `192.168.1.32`–`.34` (static cloud-init IPs).
Plus each reverse zone's NS/SOA. `entrypoint.sh` seeds each zone file into
the PVC **only if it isn't already there**, so:

- **Adding a zone** (new `zones/*.zone` + `named.conf` block + kustomization
  entry) is an ordinary deploy — the configMap hash change rolls the pod, the
  entrypoint copies just the new file, existing zones and their live dynamic
  records are untouched.
- **Changing an existing seed** (e.g. adding the k3s-VM records to an
  already-seeded `home.arpa`) needs a re-seed of that copy:
  `kubectl -n bind delete pvc bind-data` + redeploy, or a manual `nsupdate`
  (`named` owns the file after first seed, rewriting it + a `.jnl` journal).

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
