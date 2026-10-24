#!/usr/bin/env bash
set -euo pipefail

# Scoped `kubectl apply` for one deploy/ subdirectory -- useful for manually
# bootstrapping or fixing a single component out of band, without touching
# the rest of the tree Flux also manages from deploy/.
#
# Usage: ./apply-subset.sh deploy/internal/network/dhcp-kea [kubectl apply args...]
#
# Skips SOPS-encrypted Secrets: plain kubectl can't decrypt them, and only Flux
# does (see .claude/rules/secrets.md). Edit those with `sops` and push.

if [[ $# -lt 1 ]]; then
  echo "usage: $(basename "$0") <deploy-subdirectory> [kubectl apply args...]" >&2
  exit 1
fi

target="$(cd "$1" && pwd)"
shift

if [[ ! -f "$target/kustomization.yaml" ]]; then
  echo "error: $target has no kustomization.yaml" >&2
  exit 1
fi

# kustomize keeps each Secret's top-level `sops:` key, which marks it encrypted.
kustomize build "$target" \
  | awk 'BEGIN { RS = "\n---\n"; ORS = "\n---\n" } !/(^|\n)sops:/' \
  | kubectl apply -f - "$@"
