#!/usr/bin/env bash
set -euo pipefail

# Installs Flux Operator, has it install Flux, and points Flux at the
# promoted `deploy` OCI artifact. Flux then manages k8s/cluster/ itself
# (sync/self.yaml), so CI never applies it. Run this by hand when Flux can't
# recover on its own:
#
# - a fresh or rebuilt cluster
# - Flux's CRDs, controllers, the operator or `sops-age` are gone
# - a bad change under k8s/cluster/ broke Flux's own reconciliation
#
# Idempotent: on a healthy cluster it changes nothing. It uses the `deploy` tag
# already in Docker Hub and pushes nothing.
#
# Inputs:
#   current kubectl context    the target cluster
#   SOPS_AGE_KEY_FILE          age private key (default ~/.config/sops/age/keys.txt)
#   DOCKER_USERNAME / DOCKER_PASSWORD
#                              Docker Hub credentials for the private artifact.
#                              Only needed if dockerhub-deploy-pull is missing;
#                              CI refreshes it on every deploy.
#
# Usage: k8s/cluster/bootstrap.sh

cd "$(dirname "$0")"

key_file="${SOPS_AGE_KEY_FILE:-$HOME/.config/sops/age/keys.txt}"
if [[ ! -r "$key_file" ]]; then
  echo "error: no age key at $key_file (set SOPS_AGE_KEY_FILE)" >&2
  exit 1
fi

echo "Context: $(kubectl config current-context)"

../../scripts/vendor.sh

# The operator manifests leave out the Namespace (the FluxInstance creates
# it), but need it to exist.
kubectl get namespace flux-system >/dev/null 2>&1 || kubectl create namespace flux-system

# Separate applies with waits between them: a single apply races the CRDs, and
# kubectl gives up on the custom resources before the CRDs are Established.
kubectl apply -k operator
kubectl wait --for=condition=Established --timeout=60s \
  crd/fluxinstances.fluxcd.controlplane.io
kubectl -n flux-system rollout status deployment/flux-operator --timeout=120s

# Server-side apply: client-side apply would copy the key into the
# last-applied-configuration annotation.
apply_secret() {
  kubectl apply --server-side --force-conflicts --field-manager=flux-bootstrap -f -
}

kubectl -n flux-system create secret generic sops-age \
  --from-file=age.agekey="$key_file" \
  --dry-run=client -o yaml | apply_secret

if [[ -n "${DOCKER_USERNAME:-}" && -n "${DOCKER_PASSWORD:-}" ]]; then
  kubectl -n flux-system create secret docker-registry dockerhub-deploy-pull \
    --docker-username="$DOCKER_USERNAME" \
    --docker-password="$DOCKER_PASSWORD" \
    --dry-run=client -o yaml | apply_secret
elif ! kubectl -n flux-system get secret dockerhub-deploy-pull >/dev/null 2>&1; then
  echo "error: dockerhub-deploy-pull is missing; set DOCKER_USERNAME and DOCKER_PASSWORD" >&2
  exit 1
fi

# Flux Operator installs the controllers and Flux's CRDs from the FluxInstance.
kubectl apply -k instance
kubectl -n flux-system wait --for=condition=Ready --timeout=5m fluxinstance/flux
kubectl wait --for=condition=Established --timeout=60s \
  crd/gitrepositories.source.toolkit.fluxcd.io \
  crd/ocirepositories.source.toolkit.fluxcd.io \
  crd/artifactgenerators.source.extensions.fluxcd.io \
  crd/externalartifacts.source.toolkit.fluxcd.io \
  crd/kustomizations.kustomize.toolkit.fluxcd.io \
  crd/imagerepositories.image.toolkit.fluxcd.io \
  crd/imagepolicies.image.toolkit.fluxcd.io \
  crd/imageupdateautomations.image.toolkit.fluxcd.io

# sync/ holds no Secrets: plain kubectl can't decrypt SOPS. Flux's own Secrets
# live in secrets/, which only the flux-system Kustomization applies.
kubectl apply -k sync

flux reconcile source oci deploy -n flux-system
flux reconcile kustomization flux-system -n flux-system
