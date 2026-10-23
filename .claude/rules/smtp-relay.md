---
paths:
  - "deploy/internal/services/smtp-relay/**"
  - "deploy/internal/monitoring/**"
  - "deploy/internal/auth/pocketid/**"
---

# Sending mail

- `deploy/internal/services/smtp-relay/` is an in-cluster null relay
  (`boky/postfix`) forwarding to ProtonMail. ClusterIP only. Any pod can
  send through `smtp-relay.smtp-relay.svc:587` with no credential of its
  own. Consumers: Alertmanager and pocket-id. New apps should use it rather
  than ProtonMail directly.
- **Send as `hello@ticklethepanda.dev`.** A Proton SMTP token can only send
  as the address it was created for, and the relay's token belongs to
  `hello@`. Any other `From`, even another address on the same account,
  bounces with `553 Sender address rejected: not owned by user`. The
  relay's `ALLOWED_SENDER_DOMAINS` must match.
- Its credential (`smtp-relay-credentials`) is created out of band, and
  deliberately not a Reflector mirror. See the relay's `README.md`.
- Check delivery in the relay's logs (`status=sent` / `status=bounced`). To
  test end to end:
  `kubectl -n monitoring exec deploy/alertmanager -- amtool --alertmanager.url=http://localhost:9093 alert add TestAlert severity=warning`.
