---
paths:
  - "deploy/**"
  - "flux-system/**"
  - ".sops.yaml"
---

# Secrets

Kubernetes Secrets are committed encrypted with SOPS + age, in files named
`*.sops.yaml`. The `setup`, `setup-config`, `internal` and `flux-system`
Kustomizations decrypt them with the `sops-age` Secret in `flux-system`. `.sops.yaml` holds
the public key and encrypts only `data` / `stringData`.

- **Private key:** `~/.config/sops/age/keys.txt` on the owner's machine. Keep a
  backup outside the cluster. Without it, every committed Secret is
  unrecoverable.
- **Out-of-band bootstrap Secret:** `sops-age` only. Everything else comes
  from git. Secrets created by operators or CI (cert-manager TLS,
  CNPG, `dockerhub-deploy-pull`) stay out of git.
- **Flux's own Secrets** live in `flux-system/secrets/`, never under
  `flux-system/operator`, `flux-system/instance` or `flux-system/sync`.
  `flux-system/bootstrap.sh` applies those with plain `kubectl apply -k`,
  which can't decrypt.
- **Never commit plaintext.** Don't `git add` a decrypted file, and strip
  `kubectl.kubernetes.io/last-applied-configuration` from anything exported
  from the cluster.

## Add or edit

```sh
sops deploy/internal/services/foo/secret.sops.yaml   # creates or edits
```

Write a normal `kind: Secret` with `stringData`. Add the file to the
directory's `kustomization.yaml`. SOPS needs `SOPS_AGE_KEY_FILE` unset or
pointing at `keys.txt`.

## Rotate

Edit the value with `sops`, commit, push. Pods that read the Secret through
env vars need a restart. Mounted files update on their own.

## Recreate `sops-age`

Run `flux-system/bootstrap.sh`. It recreates `sops-age` from `keys.txt`
(override with `SOPS_AGE_KEY_FILE`) and is a no-op for everything else on a
healthy cluster.
