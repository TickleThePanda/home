# cluster

Flux itself: Flux Operator (`operator/`), the FluxInstance (`instance/`),
Flux's own Secrets (`secrets/`) and the sync objects (`sync/`). Flux manages
this directory from the `deploy` artifact; `bootstrap.sh` is the manual path
when it can't.

## Signed artifacts

CI (`.github/workflows/deploy-cluster.yaml`) pushes each deploy artifact,
signs its digest with keyless Cosign, verifies the signature, and only then
moves the `deploy` tag. The `deploy` OCIRepository (`sync/oci-repository.yaml`)
accepts an artifact only when it carries a signature whose certificate names
`deploy-cluster.yaml@refs/heads/main`, issued by GitHub Actions. The `prod`
environment only runs on `main`, so no branch can produce that identity.

Flux pulls with a read-only Docker Hub token
(`secrets/dockerhub-deploy-pull.sops.yaml`). Only CI holds the publishing
credential. CI checks after each deploy that the token can pull and nothing
more.

Check a published artifact by hand:

```sh
cosign verify docker.io/ticklethepanda/home-deploy:deploy --new-bundle-format=false \
  --registry-username ticklethepanda --registry-password "$READ_ONLY_TOKEN" \
  --certificate-identity https://github.com/TickleThePanda/home/.github/workflows/deploy-cluster.yaml@refs/heads/main \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

### When it fails

A failed check never changes the cluster: Flux keeps the last verified
artifact, and every Kustomization keeps applying it.

- **CI fails to sign or verify** (a Sigstore outage, say). The deploy fails
  before moving `deploy`. Re-run it once Sigstore is back.
- **Flux rejects the artifact** (`kubectl -n flux-system get ocirepository
  deploy` says `failed to verify the signature`). If `deploy` was moved to
  something CI didn't sign, re-run the last good Deploy, which re-signs and
  re-promotes `main`.
- **The trust policy is wrong** (the workflow or repo was renamed). Flux
  can't apply a fix through an artifact it rejects. Commit the corrected
  `matchOIDCIdentity`, let the Deploy promote it, then run `bootstrap.sh`,
  which applies `sync/` straight from your checkout.
- **The registry token is broken or rotated.** Create a new read-only token
  in Docker Hub, rewrite `secrets/dockerhub-deploy-pull.sops.yaml` (below),
  commit and push, then run `bootstrap.sh`: Flux can't pull the commit that
  carries the new token.

None of these turn verification off. Never remove `verify` to get a deploy
through.

Rewrite the registry token (zsh):

```sh
read -rs 'TOKEN?Docker Hub read-only token: '; echo
kubectl create secret docker-registry dockerhub-deploy-pull -n flux-system \
  --docker-server=https://index.docker.io/v1/ \
  --docker-username=ticklethepanda --docker-password="$TOKEN" \
  --dry-run=client -o yaml > k8s/cluster/secrets/dockerhub-deploy-pull.sops.yaml
unset TOKEN
sops -e -i k8s/cluster/secrets/dockerhub-deploy-pull.sops.yaml
```

### What it doesn't cover

It protects the Docker Hub → Flux boundary. It doesn't protect against a
compromised deploy workflow, a malicious change that reaches `main` and
passes validation, a compromised GitHub Actions or Sigstore, or a
compromised cluster.
