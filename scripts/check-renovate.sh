#!/usr/bin/env bash
# Check renovate.json is valid and that Renovate still finds every dependency
# it should update: images, charts and kustomize pins in k8s/, vendor.txt,
# GitHub Actions, Ansible collections, app Dockerfiles and single version
# pins. A file Renovate can't parse is otherwise silently never updated.
#
# Runs Renovate's local platform in extract mode: no registry lookups, no
# branches. It reads git-tracked files only, so commit (or `git add`) new
# files first.
#
# Needs Node 24.11+, npx, yq (mikefarah), jq, and GITHUB_COM_TOKEN set to any
# GitHub token (`gh auth token` works); without one, Renovate skips GitHub
# datasources. RENOVATE_VERSION picks the CLI; validate.yaml sets it.

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
# dependency, with spaces dropped from ranges (`~> 5.0` -> `~>5.0`). API
# versions are deliberately disabled in renovate.json.
deps=$(jq -r '.repositories.local.packageFiles | to_entries[] | .key as $m
    | .value[] | .packageFile as $f | .deps[] | select(.datasource != "kubernetes-api")
    | [$m, $f, .depName, (.currentValue // "-" | gsub(" "; "")), (.skipReason // "-")] | join(" ")' "$report")
[[ -n $deps ]] || fail "Renovate found no dependencies"

# What Renovate should find, listed before any loop: a failure inside a
# process substitution would otherwise go unnoticed and check nothing.
manifests=$(git ls-files k8s | grep -E '\.ya?ml$')
image_lines=$(xargs grep -HE '^\s*(-\s+)?image:' <<<"$manifests" \
    | sed -nE 's/^([^:]+):\s*(-\s+)?image:\s*([^ #]+).*/\1 \3/p')
helm_releases=$(xargs grep -l '^kind: HelmRelease' <<<"$manifests")
kustomizations=$(git ls-files | grep -E '(^|/)kustomization\.yaml$')
# "<file> <owner>/<repo> <ref>" for every remote action, minus any subpath.
action_lines=$(git ls-files .github | grep -E '\.ya?ml$' | xargs grep -HE '^\s*(-\s+)?uses:\s*[^.]' \
    | sed -nE 's#^([^:]+):\s*(-\s+)?uses:\s*([^/@ ]+/[^/@ ]+)[^@ ]*@([^ #]+).*#\1 \3 \4#p')
# "<name> <version>"; Renovate names a git collection by its host and path.
collections=$(yq '.collections[] | .name + " " + .version' ansible/requirements.yml \
    | sed -E 's#^https://([^ ]+)\.git #\1 #')
# "<file> <image> <tag>" for every FROM in apps/.
from_lines=$(git ls-files apps | grep -E '/Dockerfile[^/]*$' | xargs grep -HE '^FROM ' \
    | sed -nE 's#^([^:]+):FROM\s+([^: ]+):([^ ]+).*#\1 \2 \3#p')
[[ -n $image_lines && -n $helm_releases && -n $kustomizations && -n $action_lines \
    && -n $collections && -n $from_lines ]] \
    || fail "found no image lines, HelmReleases, kustomizations, actions, collections or FROM lines"

failed=0
missing() { echo "FAIL: Renovate does not detect $*" >&2; failed=1; }

# Floating tags (our own :latest images, lldap) can't be bumped, and unpinned
# deps (apt packages, provider blocks) have nothing to bump. Anything else
# skipped means a tag or version Renovate can't parse.
while read -r manager file dep value reason; do
    [[ $reason == - || $reason == unspecified-version || $value == latest* ]] && continue
    echo "FAIL: Renovate skips $dep $value in $file ($reason)" >&2
    failed=1
done <<<"$deps"

# Every image line in k8s/. Extract mode doesn't parse versions, so also
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

# Every remote GitHub Action.
while read -r file action ref; do
    grep -qxF "github-actions $file $action $ref -" <<<"$deps" || missing "$action@$ref in $file"
done <<<"$action_lines"

# Every Ansible collection.
while read -r name version; do
    grep -qxF "ansible-galaxy ansible/requirements.yml $name $version -" <<<"$deps" \
        || missing "collection $name $version"
done <<<"$collections"

# Every app base image.
while read -r file image tag; do
    grep -qxF "dockerfile $file $image $tag -" <<<"$deps" || missing "$image:$tag in $file"
done <<<"$from_lines"

# Single version pins, as "<manager> <file> <dep>".
while read -r manager file dep; do
    grep -qE "^$manager $file $dep [^ ]+ -$" <<<"$deps" || missing "$dep in $file"
done <<'PINS'
regex .github/workflows/validate.yaml renovate
regex ansible/k3s/vars/versions.yml k3s-io/k3s
regex k8s/cluster/instance/flux-instance.yaml fluxcd/flux2
regex .github/workflows/deploy-cluster.yaml fluxcd/flux2
regex .github/actions/validate-cluster/action.yml yannh/kubeconform
regex apps/sync-runner/Dockerfile.librofm librofm
regex router/version.txt openwrt/openwrt
pip_requirements .github/actions/ansible-setup/requirements.txt ansible-core
github-actions .github/workflows/deploy-terraform.yaml terraform
gomod apps/sync-runner/go.mod go
terraform terraform/cloudflare-zero-trust/versions.tf cloudflare
terraform bootstrap/terraform/versions.tf google
PINS

(( failed == 0 )) && echo "OK: Renovate detects $(wc -l <<<"$deps") dependencies"
exit "$failed"
