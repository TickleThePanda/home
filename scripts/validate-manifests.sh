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
            -schema-location default -schema-location "$crd_schemas" \
        || fail "$p does not validate"
done
