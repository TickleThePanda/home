#!/usr/bin/env bash
# Download the upstream manifests listed in vendor.txt to their destinations.
# A failed download fails the script, so this also checks every URL resolves.
#
# The files are gitignored (*.vendored.yaml). Run this before any local
# `kustomize build`; CI, flux-system/bootstrap.sh and apply-subset.sh run it
# themselves. Always re-downloads, so a version change takes effect.

set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
list="$repo_root/vendor.txt"

fail() { echo "FAIL: $*" >&2; exit 1; }

[[ -f $list ]] || fail "missing $list"

while read -r dest version url _; do
    [[ -z $dest || $dest == \#* ]] && continue
    [[ -n $url ]] || fail "no version and URL for $dest in $list"
    url=${url//\{version\}/$version}
    curl -fsSL --retry 3 "$url" -o "$repo_root/$dest" \
        || fail "could not download $url"
done < "$list"
