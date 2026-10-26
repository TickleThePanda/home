# smtp-relay

A null-relay MTA (`boky/postfix`) that forwards everything it receives to
ProtonMail's SMTP. ClusterIP-only, no IngressRoute — never reachable outside
the cluster. Any in-cluster pod can relay through it on port 587 without its
own credential (the image's default `POSTFIX_mynetworks` already covers
k3s's pod/service CIDRs); the relay itself authenticates to ProtonMail using
the Secret below.

Consumers: Alertmanager (`k8s/apps/internal/monitoring/`) and pocket-id
(`k8s/apps/internal/auth/pocketid/statefulset.yaml`, which used to talk to
ProtonMail directly and now has no SMTP credential of its own either). Not
tied to either — any future app that wants to send mail can point at
`smtp-relay.smtp-relay.svc:587` the same way.

## Sender address

Send as `hello@ticklethepanda.dev`. A Proton SMTP token only sends as the
address it was created for, and this relay's token belongs to `hello@`. Any
other `From` bounces with `553 Sender address rejected: not owned by user`,
even another address on the same account. `ALLOWED_SENDER_DOMAINS` must
match.

Test end to end, then look for `status=sent` in this relay's logs:

```sh
kubectl -n monitoring exec deploy/alertmanager -- \
  amtool --alertmanager.url=http://localhost:9093 alert add TestAlert severity=warning
```

## `smtp-relay-credentials`

SOPS-encrypted in `secret.sops.yaml` (see `.claude/rules/secrets.md`). It is a
**manual, one-time copy** of pocket-id's own former ProtonMail credentials,
deliberately not a live Reflector mirror, so this relay's credential
lifecycle is independent of pocket-id's. If pocket-id's ProtonMail
credentials are rotated, update this with `sops`; it will not pick the change
up on its own.
