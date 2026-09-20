# smtp-relay

A null-relay MTA (`boky/postfix`) that forwards everything it receives to
ProtonMail's SMTP. ClusterIP-only, no IngressRoute — never reachable outside
the cluster. Any in-cluster pod can relay through it on port 587 without its
own credential (the image's default `POSTFIX_mynetworks` already covers
k3s's pod/service CIDRs); the relay itself authenticates to ProtonMail using
the Secret below.

Consumers: Alertmanager (`deploy/internal/monitoring/`) and pocket-id
(`deploy/internal/auth/pocketid/statefulset.yaml`, which used to talk to
ProtonMail directly and now has no SMTP credential of its own either). Not
tied to either — any future app that wants to send mail can point at
`smtp-relay.smtp-relay.svc:587` the same way.

## `smtp-relay-credentials` (out-of-band)

A **manual, one-time copy** of pocket-id's own former ProtonMail credentials
(still in `pocket-id-secret`, just no longer referenced by pocket-id's own
manifest) — deliberately not a live Reflector mirror, so this relay's
credential lifecycle is independent of pocket-id's:

```sh
kubectl -n pocket-id get secret pocket-id-secret -o jsonpath='{.data.SMTP_USER}' | base64 -d; echo
kubectl -n pocket-id get secret pocket-id-secret -o jsonpath='{.data.SMTP_PASSWORD}' | base64 -d; echo

kubectl -n smtp-relay create secret generic smtp-relay-credentials \
  --from-literal=RELAYHOST_USERNAME='<value above>' \
  --from-literal=RELAYHOST_PASSWORD='<value above>'
```

Not in this repo — see `ansible/node/RECOVERY.md`'s out-of-band-secrets
list. If pocket-id's ProtonMail credentials are ever rotated, this needs
re-copying by hand; it will not pick the change up on its own.
