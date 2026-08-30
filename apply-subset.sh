#!/usr/bin/env bash
set -euo pipefail

# Scoped `kubectl apply` for one deploy/ subdirectory -- useful for manually
# bootstrapping or fixing a single component out of band, without touching
# the rest of the tree Flux also manages from deploy/.
#
# Usage: ./apply-subset.sh deploy/internal/network/dhcp-kea [kubectl apply args...]

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

kustomize build "$target" | kubectl apply -f - "$@"
