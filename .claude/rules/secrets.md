---
paths:
  - "deploy/**"
  - "flux-system/**"
  - ".sops.yaml"
---

# Secrets

Kubernetes Secrets are committed encrypted with SOPS + age, in files named
`*.sops.yaml`. The `setup-config`, `internal` and `flux-system` Kustomizations
decrypt them with the `sops-age` Secret in `flux-system`. `.sops.yaml` holds
the public key and encrypts only `data` / `stringData`.

- **Private key:** `~/.config/sops/age/keys.txt` on the owner's machine. Keep a
  backup outside the cluster. Without it, every committed Secret is
  unrecoverable.
- **Out-of-band bootstrap Secrets:** `sops-age` and `tunnel-token`. Everything
  else comes from git.
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

```sh
kubectl -n flux-system create secret generic sops-age \
  --from-file=age.agekey=$HOME/.config/sops/age/keys.txt
```
