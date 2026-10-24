#!/usr/bin/env bash
set -euo pipefail

# Scoped `kubectl apply` for one deploy/ subdirectory -- useful for manually
# bootstrapping or fixing a single component out of band, without touching
# the rest of the tree Flux also manages from deploy/.
#
# Usage: ./apply-subset.sh deploy/internal/network/dhcp-kea [kubectl apply args...]
#
# Decrypts SOPS Secrets with the local age key (~/.config/sops/age/keys.txt,
# or SOPS_AGE_KEY_FILE), like Flux does: the *.sops.yaml files are decrypted in
# a private temporary copy of deploy/ before `kustomize build`, so the MAC is
# checked against the file as committed, not after kustomize rewrites it.
#
# Applies server-side as `kustomize-controller`, Flux's own field manager, so
# Flux adopts the objects cleanly on its next reconcile. Server-side apply also
# keeps decrypted values out of the last-applied-configuration annotation.

if [[ $# -lt 1 ]]; then
  echo "usage: $(basename "$0") <deploy-subdirectory> [kubectl apply args...]" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "$0")" && pwd)"
target="$(cd "$1" && pwd)"
shift

if [[ ! -f "$target/kustomization.yaml" ]]; then
  echo "error: $target has no kustomization.yaml" >&2
  exit 1
fi
if [[ $target != "$repo_root/deploy" && $target != "$repo_root/deploy/"* ]]; then
  echo "error: $target is not under $repo_root/deploy" >&2
  exit 1
fi

"$repo_root/scripts/vendor.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cp -r "$repo_root/deploy" "$tmp/"
find "$tmp/deploy" -name '*.sops.yaml' -exec sops --decrypt --in-place {} \;

kustomize build "$tmp/${target#"$repo_root/"}" \
  | kubectl apply --server-side --field-manager=kustomize-controller -f - "$@"
