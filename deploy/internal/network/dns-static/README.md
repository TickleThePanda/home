# dns-static

A second `external-dns` instance (`external-dns-static`) whose only source is
`DNSEndpoint` CRs. It owns the forward + reverse records for hosts that aren't
DHCP clients — the gateway and the k3s VMs — pushing them into BIND by dynamic
update, the same channel Kea and the discovery `external-dns` use.

This replaces the old zone-file seeding: `bind/zones/*.zone` now carry only
NS/SOA, and adding or changing a static record is an edit to `records.yaml`
(no PVC re-seed).

## Managing records

Edit `records.yaml` — one `DNSEndpoint` per host, each with its `A` and `PTR`.
`--policy=sync`, so removing a block removes the records.

## How it stays separate from the discovery external-dns

- Different `--txt-owner-id` / `--txt-prefix` (`_sdns.` vs `_edns.`), so each
  instance only ever touches records it created.
- `--source=crd` only — it never looks at Services or Ingresses.
- Reuses the `externaldns` TSIG key: `named.conf` already grants it
  `A AAAA TXT` on `home.arpa` and `PTR TXT` on `1.168.192.in-addr.arpa`.

## Deps

`DNSEndpoint` CRD is vendored in `deploy/setup/dnsendpoint-crd/` (Established
before the `internal` Kustomization runs). Runs in the `external-dns`
namespace to share the `externaldns-tsig` secret.
