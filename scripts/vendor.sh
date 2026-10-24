#!/usr/bin/env bash
# Download the upstream manifests listed in vendor.txt to their destinations.
#
# The files are gitignored (*.vendored.yaml). Run this before any local
# `kustomize build`; CI, flux-system/bootstrap.sh and apply-subset.sh run it
# themselves. Always re-downloads, so a URL change takes effect.

set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
list="$repo_root/vendor.txt"

fail() { echo "FAIL: $*" >&2; exit 1; }

[[ -f $list ]] || fail "missing $list"

while read -r dest url _; do
    [[ -z $dest || $dest == \#* ]] && continue
    [[ -n $url ]] || fail "no URL for $dest in $list"
    curl -fsSL --retry 3 "$url" -o "$repo_root/$dest" \
        || fail "could not download $url"
done < "$list"
