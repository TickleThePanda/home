#!/usr/bin/env bash
set -euo pipefail

# Scoped `kubectl apply` for one k8s/ subdirectory -- useful for manually
# bootstrapping or fixing a single component out of band, without touching
# the rest of the tree Flux also manages from k8s/.
#
# Usage: ./apply-subset.sh k8s/apps/internal/network/dhcp-kea [kubectl apply args...]
#
# Decrypts SOPS Secrets with the local age key (~/.config/sops/age/keys.txt,
# or SOPS_AGE_KEY_FILE), like Flux does: the *.sops.yaml files are decrypted in
# a private temporary copy of k8s/ before `kustomize build`, so the MAC is
# checked against the file as committed, not after kustomize rewrites it.
#
# Applies server-side as `kustomize-controller`, Flux's own field manager, so
# Flux adopts the objects cleanly on its next reconcile. Server-side apply also
# keeps decrypted values out of the last-applied-configuration annotation.

if [[ $# -lt 1 ]]; then
  echo "usage: $(basename "$0") <k8s-subdirectory> [kubectl apply args...]" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "$0")" && pwd)"
target="$(cd "$1" && pwd)"
shift

if [[ ! -f "$target/kustomization.yaml" ]]; then
  echo "error: $target has no kustomization.yaml" >&2
  exit 1
fi
# Not k8s/cluster: that is Flux itself (k8s/cluster/bootstrap.sh).
if [[ $target != "$repo_root/k8s/infrastructure/"* && $target != "$repo_root/k8s/apps/"* ]]; then
  echo "error: $target is not under $repo_root/k8s/infrastructure or $repo_root/k8s/apps" >&2
  exit 1
fi

"$repo_root/scripts/vendor.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cp -r "$repo_root/k8s" "$tmp/"
find "$tmp/k8s" -name '*.sops.yaml' -exec sops --decrypt --in-place {} \;

kustomize build "$tmp/${target#"$repo_root/"}" \
  | kubectl apply --server-side --field-manager=kustomize-controller -f - "$@"
