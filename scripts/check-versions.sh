#!/usr/bin/env bash
# Check version invariants that a single dependency bump could break. Renovate
# updates coupled dependencies in one branch (renovate.json), but if upstream
# publishes them at different times, the branch holds only one half. This
# makes that branch fail instead of merging.
#
# Needs yq (mikefarah) and git. Doesn't need scripts/vendor.sh.

set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

failed=0
fail() { echo "FAIL: $*" >&2; failed=1; }

# vendor.txt's version column for a destination.
vendored() { awk -v dest="$1" '$1 == dest { print $2 }' vendor.txt; }

# Every `image: <repo>:<tag>` in tracked deploy/ manifests, as "<repo> <tag>".
images=$(git ls-files deploy | grep -E '\.ya?ml$' | xargs sed -nE 's/^\s*(-\s+)?image:\s*([^ #]+).*/\2/p' \
    | sed -E 's/:([^:/]+)$/ \1/' | sort -u)
[[ -n $images ]] || { echo "FAIL: no image lines found in deploy/" >&2; exit 1; }
tag() { awk -v repo="$1" '$1 == repo { print $2 }' <<<"$images"; }

# One image, one tag: external-dns runs twice, and both must move together.
while read -r repo count; do
    fail "$repo has $count different tags in deploy/: $(tag "$repo" | tr '\n' ' ')"
done < <(awk '{ print $1 }' <<<"$images" | uniq -c | awk '$1 > 1 { print $2, $1 }')

# The Flux Operator install and the CRD schemas validate-manifests.sh checks
# against must come from the same release.
op=$(vendored flux-system/operator/flux-operator.vendored.yaml)
schemas=$(vendored scripts/flux-operator-crd-schemas.vendored.tar.gz)
[[ -n $op && $op == "$schemas" ]] \
    || fail "flux-operator ($op) and its CRD schemas ($schemas) differ in vendor.txt"

# lvm-localpv's manifest ships `openebs/lvm-driver:ci`; its kustomization pins
# the image to the manifest's release.
lvm=$(vendored deploy/setup/lvm-localpv/lvm-operator.vendored.yaml)
lvm_tag=$(yq '.images[] | select(.name == "openebs/lvm-driver") | .newTag' \
    deploy/setup/lvm-localpv/kustomization.yaml)
[[ -n $lvm && $lvm == "$lvm_tag" ]] \
    || fail "lvm-localpv manifest ($lvm) and openebs/lvm-driver image ($lvm_tag) differ"

# Kea's DHCPv4 and DDNS daemons talk to each other and release together.
[[ $(tag jonasal/kea-dhcp4) == "$(tag jonasal/kea-dhcp-ddns)" ]] \
    || fail "kea-dhcp4 ($(tag jonasal/kea-dhcp4)) and kea-dhcp-ddns ($(tag jonasal/kea-dhcp-ddns)) differ"

# GeoPulse's UI and backend release together; the backend tag adds a variant.
backend=$(tag tess1o/geopulse-backend)
[[ ${backend%-native-compat} == "$(tag tess1o/geopulse-ui)" ]] \
    || fail "geopulse-backend ($backend) and geopulse-ui ($(tag tess1o/geopulse-ui)) differ"

# Exact pins only: a range lets helm-controller or Flux Operator upgrade on
# their own, outside this repo's validation.
exact='^v?[0-9]+\.[0-9]+\.[0-9]+$'
releases=$(git ls-files deploy | xargs grep -l '^kind: HelmRelease')
for f in $releases; do
    version=$(yq 'select(.kind == "HelmRelease") | .spec.chart.spec.version' "$f")
    [[ $version =~ $exact ]] || fail "$f: chart version '$version' is not an exact version"
done
version=$(yq '.spec.distribution.version' flux-system/instance/flux-instance.yaml)
[[ $version =~ $exact ]] || fail "FluxInstance version '$version' is not an exact version"

# The flux CLI that CI pushes artifacts with is the cluster's Flux release:
# the action ref and the version it installs.
flux_step='.jobs[].steps[] | select((.uses // "") | test("^fluxcd/flux2/action@"))'
flux_ref=$(yq "$flux_step | .uses | sub(\"^fluxcd/flux2/action@v\"; \"\")" .github/workflows/deploy-cluster.yaml)
flux_cli=$(yq "$flux_step | .with.version" .github/workflows/deploy-cluster.yaml)
[[ $flux_ref == "$version" && $flux_cli == "$version" ]] \
    || fail "flux action ref ($flux_ref) and CLI version ($flux_cli) differ from the FluxInstance ($version)"

# Traefik's chart pins its own image, so the chart version is the only thing
# to upgrade. An image override would stop chart upgrades from moving it.
[[ $(yq '.spec.values.image // ""' deploy/setup/traefik/helm-release.yaml) == "" ]] \
    || fail "deploy/setup/traefik/helm-release.yaml overrides the chart's image"

(( failed == 0 )) && echo "OK: version invariants hold"
exit "$failed"
