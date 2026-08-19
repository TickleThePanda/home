#!/usr/bin/env bash
set -euo pipefail

# Scoped `kubectl apply` for one deploy/ subdirectory that still carries the
# ticklethepanda.dev/managed-by label -- and, for a Deployment/StatefulSet/
# DaemonSet, the matching spec.selector -- that a bare
# `kubectl apply -k deploy/<subdir>` doesn't add (that only happens through
# the top-level deploy/kustomization.yaml). Needed the first time a new
# selector-bearing workload is created: without the label baked into its
# selector from creation, the next full-pipeline apply tries to PATCH an
# immutable field and fails. See CLAUDE.md's "Deploy pattern" section.
#
# Usage: ./apply-subset.sh deploy/internal/network/dhcp-kea [kubectl apply args...]

if [[ $# -lt 1 ]]; then
  echo "usage: $(basename "$0") <deploy-subdirectory> [kubectl apply args...]" >&2
  exit 1
fi

repo_root="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
target="$(cd "$1" && pwd)"
shift

if [[ ! -f "$target/kustomization.yaml" ]]; then
  echo "error: $target has no kustomization.yaml" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

{
  echo "apiVersion: kustomize.config.k8s.io/v1beta1"
  echo "kind: Kustomization"
  echo
  echo "resources:"
  echo "  - $(realpath --relative-to="$tmp" "$target")"
  echo
  # Re-use the top-level kustomization's own labels block rather than
  # duplicating the label pairs here, so this can't drift out of sync with it.
  awk '/^labels:/,0' "$repo_root/deploy/kustomization.yaml"
} > "$tmp/kustomization.yaml"

kustomize build "$tmp" | kubectl apply -f - "$@"
