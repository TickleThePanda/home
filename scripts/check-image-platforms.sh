#!/usr/bin/env bash
# Check that every image in the k8s/ manifests changed since <base> exists
# and is published for linux/amd64 and linux/arm64. A stateless pod can land
# on any node, and the Pi is arm64.
#
# Usage: scripts/check-image-platforms.sh <base-ref>
#
# Checks every image in each changed file, plus kustomize `images:` pins.
# Floating `latest*` tags are skipped: this repo's own images are built
# multi-arch by their build--*.yml workflows. Needs docker (buildx), jq, yq.

set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

fail() { echo "FAIL: $*" >&2; exit 1; }

base=${1:?usage: $0 <base-ref>}
changed=$(git diff --name-only --diff-filter=d "$base" -- 'k8s/**.yaml' 'k8s/**.yml')
if [[ -z $changed ]]; then
    echo "OK: no k8s/ manifests changed"
    exit 0
fi

images=$(
    while read -r f; do
        sed -nE 's/^\s*(-\s+)?image:\s*([^ #]+).*/\2/p' "$f"
        if [[ $(basename "$f") == kustomization.yaml ]]; then
            yq '.images[]? | select(.newTag) | (.newName // .name) + ":" + .newTag' "$f"
        fi
    done <<<"$changed" | { grep -vE ':latest[^/]*$' || true; } | sort -u
)

failed=0
while read -r image; do
    [[ -n $image ]] || continue
    # The index lists one manifest per platform; a single-platform image has none.
    platforms=$(docker buildx imagetools inspect --raw "$image" \
        | jq -r '.manifests[]?.platform | select(.) | "\(.os)/\(.architecture)"') \
        || { echo "FAIL: $image: not found" >&2; failed=1; continue; }
    for want in linux/amd64 linux/arm64; do
        grep -qxF "$want" <<<"$platforms" \
            || { echo "FAIL: $image: no $want image" >&2; failed=1; }
    done
    echo "$image: ok"
done <<<"$images"

(( failed == 0 )) && echo "OK: changed images are multi-arch"
exit "$failed"
