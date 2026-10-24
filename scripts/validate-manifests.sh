#!/usr/bin/env bash
# Build every path Flux reconciles and check the output against Kubernetes and
# CRD schemas. Run scripts/vendor.sh first.
#
# Skipped kinds:
# - Secret: SOPS files carry a top-level `sops:` key that strict mode rejects.
# - HelmChart: the catalog's schema predates k3s's `failurePolicy: retry`.
# - Any kind with no published schema.

set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
paths=(deploy/setup deploy/setup-config deploy/home deploy/internal flux-system)
crd_schemas='https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'

fail() { echo "FAIL: $*" >&2; exit 1; }

# The catalog lacks Flux Operator's CRDs. Its release ships their schemas as
# flat files (fluxinstance-fluxcd-v1.json); rename them to kubeconform's
# {{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json layout.
release() { grep -F "$1" "$repo_root/vendor.txt" | grep -oE '/download/v[^/]+/'; }
op_release=$(release flux-operator.vendored.yaml) || fail "no flux-operator line in vendor.txt"
[[ $(release flux-operator-crd-schemas) == "$op_release" ]] \
    || fail "flux-operator and its CRD schemas are on different releases in vendor.txt"

operator_schemas=$(mktemp -d)
trap 'rm -rf "$operator_schemas"' EXIT
mkdir "$operator_schemas/fluxcd.controlplane.io"
tar -xzf "$repo_root/scripts/flux-operator-crd-schemas.vendored.tar.gz" -C "$operator_schemas"
for f in "$operator_schemas"/*-fluxcd-*.json; do
    name=$(basename "$f" .json)
    mv "$f" "$operator_schemas/fluxcd.controlplane.io/${name%%-*}_${name##*-}.json"
done
operator_schemas+='/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'

# Remote resources are fetched by kustomize-controller on every reconcile.
# Upstream manifests belong in vendor.txt instead.
if remote=$(grep -rn --include=kustomization.yaml -E '^\s*-\s*https?://' \
        "$repo_root/deploy" "$repo_root/flux-system"); then
    fail "remote resources found; add them to vendor.txt instead:"$'\n'"$remote"
fi

for p in "${paths[@]}"; do
    echo "== $p"
    kustomize build "$repo_root/$p" \
        | kubeconform -strict -summary -skip Secret,HelmChart -ignore-missing-schemas \
            -schema-location default -schema-location "$operator_schemas" \
            -schema-location "$crd_schemas" \
        || fail "$p does not validate"
done

# A missing schema fails here, unlike above: the FluxInstance controls the
# whole Flux install.
echo "== flux-system/instance"
kubeconform -strict -summary -schema-location "$operator_schemas" \
        "$repo_root/flux-system/instance/flux-instance.yaml" \
    || fail "flux-system/instance does not validate"
