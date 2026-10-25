#!/usr/bin/env bash
# Build every path Flux reconciles and check the output against Kubernetes and
# CRD schemas. Also render every HelmRelease's chart with its values and check
# that too. Run scripts/vendor.sh first. Needs kustomize, kubeconform, helm and
# yq (mikefarah).
#
# Each deploy/ layer builds from a copy holding only that layer, since its
# ExternalArtifact (flux-system/sync/artifact-generator.yaml) contains nothing
# else. A reference outside the layer fails here, not after promotion.
#
# Skipped kinds:
# - Secret: SOPS files carry a top-level `sops:` key that strict mode rejects.
# - Any kind with no published schema.

set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
crd_schemas='https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'

fail() { echo "FAIL: $*" >&2; exit 1; }

# The catalog lacks Flux Operator's CRDs. Its release ships their schemas as
# flat files (fluxinstance-fluxcd-v1.json); rename them to kubeconform's
# {{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json layout.
# scripts/check-versions.sh checks they match the vendored operator.
operator_schemas=$(mktemp -d)
layers_root=$(mktemp -d)
charts_root=$(mktemp -d)
trap 'rm -rf "$operator_schemas" "$layers_root" "$charts_root"' EXIT
mkdir "$operator_schemas/fluxcd.controlplane.io"
tar -xzf "$repo_root/scripts/flux-operator-crd-schemas.vendored.tar.gz" -C "$operator_schemas"
for f in "$operator_schemas"/*-fluxcd-*.json; do
    name=$(basename "$f" .json)
    mv "$f" "$operator_schemas/fluxcd.controlplane.io/${name%%-*}_${name##*-}.json"
done
operator_schemas+='/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'

# Remote resources are fetched by kustomize-controller on every reconcile.
# Upstream manifests belong in vendor.txt instead.
if remote=$(grep -rn --include=kustomization.yaml -E '^\s*-\s*https?://' \
        "$repo_root/deploy" "$repo_root/flux-system"); then
    fail "remote resources found; add them to vendor.txt instead:"$'\n'"$remote"
fi

validate_file() {
    kubeconform -strict -summary -skip Secret -ignore-missing-schemas \
        -schema-location default -schema-location "$operator_schemas" \
        -schema-location "$crd_schemas" "$@"
}
validate() { kustomize build "$1" | validate_file; }

mapfile -t layers < <(grep -oE '@repo/deploy/[^/"]+' \
    "$repo_root/flux-system/sync/artifact-generator.yaml" | sed 's|^@repo/||' | sort -u)
(( ${#layers[@]} > 0 )) || fail "no deploy/ layers in flux-system/sync/artifact-generator.yaml"

for layer in "${layers[@]}"; do
    echo "== $layer"
    # A root per layer, so one layer can't resolve another's copy.
    root="$layers_root/${layer//\//_}"
    mkdir -p "$root/$(dirname "$layer")"
    cp -a "$repo_root/$layer" "$root/$layer"
    find "$root/$layer" -name '*.md' -delete
    validate "$root/$layer" || fail "$layer does not validate on its own"
done

# helm-controller renders each HelmRelease in the cluster, so the builds above
# only see the HelmRelease object. Render its chart here with the same version
# and values. --kube-version (k3s's) matters to charts that check
# .Capabilities.KubeVersion.
kube_version=$(yq '.k3s_version' "$repo_root/ansible/k3s/vars/versions.yml")
kube_version=${kube_version%%+*}
[[ $kube_version == v* ]] || fail "no k3s_version in ansible/k3s/vars/versions.yml"

mapfile -t releases < <(cd "$repo_root" && git ls-files deploy | grep -E '\.ya?ml$' \
    | xargs grep -l '^kind: HelmRelease')
(( ${#releases[@]} > 0 )) || fail "no HelmReleases found under deploy/"
for file in "${releases[@]}"; do
    echo "== $file"
    dir=$(dirname "$repo_root/$file")
    release=$(yq 'select(.kind == "HelmRelease")' "$repo_root/$file")
    name=$(yq '.spec.releaseName // .metadata.name' <<<"$release")
    namespace=$(yq '.metadata.namespace' <<<"$release")
    chart=$(yq '.spec.chart.spec.chart' <<<"$release")
    version=$(yq '.spec.chart.spec.version' <<<"$release")
    repo=$(yq '.spec.chart.spec.sourceRef.name' <<<"$release")
    url=$(yq ea "select(.kind == \"HelmRepository\" and .metadata.name == \"$repo\") | .spec.url" \
        "$dir"/*.yaml)
    [[ -n $url ]] || fail "$file: no HelmRepository '$repo' next to it"
    values="$charts_root/${name}-values.yaml"
    yq '.spec.values // {}' <<<"$release" > "$values"
    helm template "$name" "$chart" --repo "$url" --version "$version" \
            --namespace "$namespace" --kube-version "$kube_version" \
            --include-crds --values "$values" > "$charts_root/$name.yaml" \
        || fail "$file: chart $chart $version does not render"
    validate_file "$charts_root/$name.yaml" || fail "$file: rendered chart does not validate"
done

# self.yaml reads the whole OCIRepository, so flux-system builds in place.
echo "== flux-system"
validate "$repo_root/flux-system" || fail "flux-system does not validate"

# A missing schema fails here, unlike above: the FluxInstance controls the
# whole Flux install.
echo "== flux-system/instance"
kubeconform -strict -summary -schema-location "$operator_schemas" \
        "$repo_root/flux-system/instance/flux-instance.yaml" \
    || fail "flux-system/instance does not validate"
