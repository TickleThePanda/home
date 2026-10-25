#!/usr/bin/env bash
# Build every path Flux reconciles and check the output against Kubernetes and
# CRD schemas. Run scripts/vendor.sh first.
#
# Each deploy/ layer builds from a copy holding only that layer, since its
# ExternalArtifact (flux-system/sync/artifact-generator.yaml) contains nothing
# else. A reference outside the layer fails here, not after promotion.
#
# Skipped kinds:
# - Secret: SOPS files carry a top-level `sops:` key that strict mode rejects.
# - Any kind with no published schema.

set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
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
layers_root=$(mktemp -d)
trap 'rm -rf "$operator_schemas" "$layers_root"' EXIT
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

validate() {
    kustomize build "$1" \
        | kubeconform -strict -summary -skip Secret -ignore-missing-schemas \
            -schema-location default -schema-location "$operator_schemas" \
            -schema-location "$crd_schemas"
}

mapfile -t layers < <(grep -oE '@repo/deploy/[^/"]+' \
    "$repo_root/flux-system/sync/artifact-generator.yaml" | sed 's|^@repo/||' | sort -u)
(( ${#layers[@]} > 0 )) || fail "no deploy/ layers in flux-system/sync/artifact-generator.yaml"

for layer in "${layers[@]}"; do
    echo "== $layer"
    # A root per layer, so one layer can't resolve another's copy.
    root="$layers_root/${layer//\//_}"
    mkdir -p "$root/$(dirname "$layer")"
    cp -a "$repo_root/$layer" "$root/$layer"
    find "$root/$layer" -name '*.md' -delete
    validate "$root/$layer" || fail "$layer does not validate on its own"
done

# self.yaml reads the whole OCIRepository, so flux-system builds in place.
echo "== flux-system"
validate "$repo_root/flux-system" || fail "flux-system does not validate"

# A missing schema fails here, unlike above: the FluxInstance controls the
# whole Flux install.
echo "== flux-system/instance"
kubeconform -strict -summary -schema-location "$operator_schemas" \
        "$repo_root/flux-system/instance/flux-instance.yaml" \
    || fail "flux-system/instance does not validate"
