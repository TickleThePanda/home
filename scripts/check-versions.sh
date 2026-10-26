#!/usr/bin/env bash
# Check version invariants that a single dependency bump could break. Renovate
# updates coupled dependencies in one branch (renovate.json), but if upstream
# publishes them at different times, the branch holds only one half. This
# makes that branch fail instead of merging.
#
# Needs yq (mikefarah), git and curl (it reads raw.githubusercontent.com).
# Doesn't need scripts/vendor.sh.

set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

failed=0
fail() { echo "FAIL: $*" >&2; failed=1; }

# vendor.txt's version column for a destination.
vendored() { awk -v dest="$1" '$1 == dest { print $2 }' vendor.txt; }

# Every `image: <repo>:<tag>` in tracked k8s/ manifests, as "<repo> <tag>".
images=$(git ls-files k8s | grep -E '\.ya?ml$' | xargs sed -nE 's/^\s*(-\s+)?image:\s*([^ #]+).*/\2/p' \
    | sed -E 's/:([^:/]+)$/ \1/' | sort -u)
[[ -n $images ]] || { echo "FAIL: no image lines found in k8s/" >&2; exit 1; }
tag() { awk -v repo="$1" '$1 == repo { print $2 }' <<<"$images"; }

# One image, one tag: external-dns runs twice, and both must move together.
while read -r repo count; do
    fail "$repo has $count different tags in k8s/: $(tag "$repo" | tr '\n' ' ')"
done < <(awk '{ print $1 }' <<<"$images" | uniq -c | awk '$1 > 1 { print $2, $1 }')

# The Flux Operator install and the CRD schemas validate-manifests.sh checks
# against must come from the same release.
op=$(vendored k8s/cluster/operator/flux-operator.vendored.yaml)
schemas=$(vendored scripts/flux-operator-crd-schemas.vendored.tar.gz)
[[ -n $op && $op == "$schemas" ]] \
    || fail "flux-operator ($op) and its CRD schemas ($schemas) differ in vendor.txt"

# lvm-localpv's manifest ships `openebs/lvm-driver:ci`; its kustomization pins
# the image to the manifest's release.
lvm=$(vendored k8s/infrastructure/controllers/lvm-localpv/lvm-operator.vendored.yaml)
lvm_tag=$(yq '.images[] | select(.name == "openebs/lvm-driver") | .newTag' \
    k8s/infrastructure/controllers/lvm-localpv/kustomization.yaml)
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
releases=$(git ls-files k8s | xargs grep -l '^kind: HelmRelease')
for f in $releases; do
    version=$(yq 'select(.kind == "HelmRelease") | .spec.chart.spec.version' "$f")
    [[ $version =~ $exact ]] || fail "$f: chart version '$version' is not an exact version"
done
version=$(yq '.spec.distribution.version' k8s/cluster/instance/flux-instance.yaml)
[[ $version =~ $exact ]] || fail "FluxInstance version '$version' is not an exact version"

# The flux CLI that CI pushes artifacts with is the cluster's Flux release:
# the action ref and the version it installs.
flux_step='.jobs[].steps[] | select((.uses // "") | test("^fluxcd/flux2/action@"))'
flux_ref=$(yq "$flux_step | .uses | sub(\"^fluxcd/flux2/action@v\"; \"\")" .github/workflows/deploy-cluster.yaml)
flux_cli=$(yq "$flux_step | .with.version" .github/workflows/deploy-cluster.yaml)
[[ $flux_ref == "$version" && $flux_cli == "$version" ]] \
    || fail "flux action ref ($flux_ref) and CLI version ($flux_cli) differ from the FluxInstance ($version)"

# Flux Operator installs only the Flux releases it embeds (config/data/flux/ in
# its repo). Any other version fails the FluxInstance.
if ! curl -fsSL -o /dev/null --retry 3 \
    "https://raw.githubusercontent.com/controlplaneio-fluxcd/flux-operator/v$op/config/data/flux/v$version/source-controller.yaml"; then
    fail "flux-operator $op doesn't embed Flux $version; wait for an operator release that does"
fi

# Traefik's chart pins its own image, so the chart version is the only thing
# to upgrade. An image override would stop chart upgrades from moving it.
[[ $(yq '.spec.values.image // ""' k8s/infrastructure/controllers/traefik/helm-release.yaml) == "" ]] \
    || fail "k8s/infrastructure/controllers/traefik/helm-release.yaml overrides the chart's image"

# k3s and alpine/k8s (kubectl) minors automerge, but Kubernetes can't skip a
# minor. Renovate opens a branch per minor at once, so fail all but the next
# one; each rebase after a merge lets the following minor through.
minor() { sed -nE 's/^v?([0-9]+\.[0-9]+).*/\1/p' <<<"$1"; }
k3s_vars=ansible/k3s/vars/versions.yml
k3s=$(minor "$(yq '.k3s_version' "$k3s_vars")")
kubectl=$(minor "$(tag alpine/k8s)")
if git rev-parse --verify -q origin/main >/dev/null; then
    main_k3s=$(minor "$(git show "origin/main:$k3s_vars" | yq '.k3s_version')")
    main_kubectl=$(minor "$(git grep -h -E 'image:\s*alpine/k8s:' origin/main -- k8s \
        | sed -nE 's/.*alpine\/k8s:([^ #]+).*/\1/p' | head -n1)")
    step_ok() { [[ ${1%.*} == "${2%.*}" ]] && (( ${2#*.} - ${1#*.} <= 1 )); }
    step_ok "$main_k3s" "$k3s" || fail "k3s moves from $main_k3s to $k3s; upgrade one minor at a time"
    step_ok "$main_kubectl" "$kubectl" \
        || fail "alpine/k8s moves from $main_kubectl to $kubectl; upgrade one minor at a time"
else
    echo "NOTE: no origin/main, skipping the one-minor-at-a-time check" >&2
fi
# kubectl supports a server one minor either side of it.
(( ${kubectl#*.} - ${k3s#*.} <= 1 && ${k3s#*.} - ${kubectl#*.} <= 1 )) \
    || fail "alpine/k8s ($kubectl) is more than one minor from k3s ($k3s)"

(( failed == 0 )) && echo "OK: version invariants hold"
exit "$failed"
