#!/usr/bin/env bash
# Assert that the Traefik chart pinned in operators/ matches the k3s version
# pinned in node/.
#
# k3s serves chart tarballs from its own /var/lib/rancher/k3s/server/static/
# charts/, which only ever contains the charts bundled with the INSTALLED k3s
# version. So bumping k3s without bumping the chart URL leaves operators/
# pointing at a tarball the node no longer has -- and vice versa.
#
# With --online, also asks the k3s repo whether the pair is genuinely what that
# release bundles. Skipped by default so the check stays useful offline and
# does not fail CI on a GitHub API hiccup.

set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
versions="$repo_root/node/vars/versions.yml"
chart="$repo_root/operators/traefik/traefik-helm-chart.yaml"

fail() { echo "FAIL: $*" >&2; exit 1; }

[[ -f $versions ]] || fail "missing $versions"
[[ -f $chart ]] || fail "missing $chart"

k3s_version=$(grep -oP '^k3s_version:\s*"\K[^"]+' "$versions") \
    || fail "could not read k3s_version from $versions"
want=$(grep -oP '^traefik_chart_version:\s*"\K[^"]+' "$versions") \
    || fail "could not read traefik_chart_version from $versions"

# Both traefik-<v>.tgz and traefik-crd-<v>.tgz must agree.
mapfile -t found < <(grep -oP 'static/charts/traefik(-crd)?-\K[^.]+\.[^.]+\.[^-]+\+up[0-9.]+(?=\.tgz)' "$chart")

[[ ${#found[@]} -eq 2 ]] \
    || fail "expected 2 chart references in $chart, found ${#found[@]}"

for f in "${found[@]}"; do
    [[ $f == "$want" ]] \
        || fail "chart pin mismatch: $chart has '$f', node/vars/versions.yml says '$want'"
done

echo "OK: k3s $k3s_version <-> traefik chart $want (both references agree)"

if [[ ${1:-} == --online ]]; then
    echo "Checking against the k3s release manifest..."
    ref=${k3s_version//+/%2B}
    upstream=$(curl -sfL --max-time 20 \
        "https://api.github.com/repos/k3s-io/k3s/contents/manifests/traefik.yaml?ref=${ref}" \
        -H 'Accept: application/vnd.github.raw' \
        | grep -v 'traefik-crd-' \
        | grep -oP 'static/charts/traefik-\K[^.]+\.[^.]+\.[^-]+\+up[0-9.]+(?=\.tgz)' | head -1) || true

    if [[ -z $upstream ]]; then
        echo "WARN: could not reach the k3s repo; skipping the online check." >&2
        exit 0
    fi

    [[ $upstream == "$want" ]] \
        || fail "k3s $k3s_version actually bundles traefik chart '$upstream', not '$want'"

    echo "OK: k3s $k3s_version really does bundle traefik chart $upstream"
fi
