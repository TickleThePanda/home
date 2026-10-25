#!/usr/bin/env bash
# Check renovate.json is valid and that Renovate still finds every dependency
# it should update. A manifest Renovate can't parse is otherwise silently
# never updated.
#
# Runs Renovate's local platform in extract mode: no registry lookups, no
# branches. It reads git-tracked files only, so commit (or `git add`) new
# files first.
#
# Needs Node 24.11+, npx, yq (mikefarah), jq, and GITHUB_COM_TOKEN set to any
# GitHub token (`gh auth token` works); without one, Renovate skips GitHub
# datasources. RENOVATE_VERSION picks the CLI; validate.yaml pins it.

set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

fail() { echo "FAIL: $*" >&2; exit 1; }

[[ -n ${RENOVATE_VERSION:-} ]] || fail "set RENOVATE_VERSION (see .github/workflows/validate.yaml)"
[[ -n ${GITHUB_COM_TOKEN:-} ]] || fail "set GITHUB_COM_TOKEN to a GitHub token"

renovate() { npx --yes --package "renovate@$RENOVATE_VERSION" -- "$@"; }

echo "== renovate.json"
renovate renovate-config-validator --strict || fail "renovate.json is not valid"

echo "== dependency extraction"
report=$(mktemp)
trap 'rm -f "$report"' EXIT
LOG_LEVEL=warn renovate renovate --platform=local --dry-run=extract \
        --report-type=file --report-path="$report" \
    || fail "renovate extract failed"

# One "<manager> <packageFile> <depName> <currentValue> <skipReason>" line per
# dependency. API versions are deliberately disabled in renovate.json.
deps=$(jq -r '.repositories.local.packageFiles | to_entries[] | .key as $m
    | .value[] | .packageFile as $f | .deps[] | select(.datasource != "kubernetes-api")
    | [$m, $f, .depName, .currentValue, (.skipReason // "-")] | join(" ")' "$report")
[[ -n $deps ]] || fail "Renovate found no dependencies"

# What Renovate should find, listed before any loop: a failure inside a
# process substitution would otherwise go unnoticed and check nothing.
manifests=$(git ls-files deploy | grep -E '\.ya?ml$')
image_lines=$(xargs grep -HE '^\s*(-\s+)?image:' <<<"$manifests" \
    | sed -nE 's/^([^:]+):\s*(-\s+)?image:\s*([^ #]+).*/\1 \3/p')
helm_releases=$(xargs grep -l '^kind: HelmRelease' <<<"$manifests")
kustomizations=$(git ls-files | grep -E '(^|/)kustomization\.yaml$')
[[ -n $image_lines && -n $helm_releases && -n $kustomizations ]] \
    || fail "found no image lines, HelmReleases or kustomizations in git ls-files"

failed=0
missing() { echo "FAIL: Renovate does not detect $*" >&2; failed=1; }

# Floating tags (our own :latest images, lldap) can't be bumped; anything
# else skipped means a tag or version Renovate can't parse.
while read -r manager file dep value reason; do
    [[ $reason == - || $value == latest* ]] && continue
    echo "FAIL: Renovate skips $dep $value in $file ($reason)" >&2
    failed=1
done <<<"$deps"

# Every image line in deploy/. Extract mode doesn't parse versions, so also
# require a tag that docker versioning can order: a number first, optionally
# after `v` (`v1.2.3`, `2026.10.0`, `1.39.1-native-compat`).
while read -r file image; do
    repo=${image%:*} tag=${image##*:}
    [[ $tag == latest* ]] && continue
    grep -qxF "kubernetes $file $repo $tag -" <<<"$deps" || missing "$image in $file"
    [[ $tag =~ ^v?[0-9] ]] \
        || { echo "FAIL: $file: Renovate can't order tag '$tag' of $repo" >&2; failed=1; }
done <<<"$image_lines"

# Every HelmRelease's chart.
while read -r file; do
    chart=$(yq 'select(.kind == "HelmRelease") | .spec.chart.spec.chart' "$file")
    grep -qE "^flux $file $chart " <<<"$deps" || missing "chart $chart in $file"
done <<<"$helm_releases"

# Every kustomize image pin.
while read -r file; do
    while read -r name tag; do
        grep -qxF "kustomize $file $name $tag -" <<<"$deps" || missing "$name:$tag in $file"
    done < <(yq '.images[]? | .name + " " + .newTag' "$file")
done <<<"$kustomizations"

# Every vendor.txt entry.
expected=$(grep -cvE '^\s*(#|$)' vendor.txt)
found=$(grep -c '^regex vendor.txt ' <<<"$deps" || true)
(( found == expected )) || missing "every vendor.txt entry: $found of $expected (each needs a # renovate: comment)"

# The CLI this script runs.
grep -q '^regex .github/workflows/validate.yaml renovate ' <<<"$deps" \
    || missing "RENOVATE_VERSION in .github/workflows/validate.yaml"

(( failed == 0 )) && echo "OK: Renovate detects $(wc -l <<<"$deps") dependencies"
exit "$failed"
