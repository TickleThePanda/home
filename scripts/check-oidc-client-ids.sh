#!/usr/bin/env bash
# Check that Headlamp's Pocket ID client ID is the one the API server trusts.
# Ansible writes k3s_oidc_client_id into the API server's authentication
# config, and pocket-id-operator registers the client from the
# PocketIDOIDCClient. If they differ, Headlamp logins get tokens the API
# server rejects. Headlamp's own -oidc-client-id flag must match too.
#
# Needs yq (mikefarah or the Python wrapper) and grep.

set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

ansible_id=$(yq -r '.k3s_oidc_client_id' ansible/group_vars/k3s_cluster.yml)
client_id=$(yq -r '.spec.clientID' k8s/apps/internal/auth/pocketid-clients/headlamp.yaml)
flag_id=$(sed -nE 's/.*"-oidc-client-id=([^"]+)".*/\1/p' k8s/apps/internal/apps/headlamp/deployment.yaml)

failed=0
[[ -n $ansible_id && $ansible_id != null ]] || { echo "FAIL: no k3s_oidc_client_id in ansible/group_vars/k3s_cluster.yml" >&2; failed=1; }
[[ $client_id == "$ansible_id" ]] \
    || { echo "FAIL: Headlamp's PocketIDOIDCClient has clientID '$client_id', but k3s_oidc_client_id is '$ansible_id'" >&2; failed=1; }
[[ $flag_id == "$ansible_id" ]] \
    || { echo "FAIL: Headlamp's -oidc-client-id is '$flag_id', but k3s_oidc_client_id is '$ansible_id'" >&2; failed=1; }

(( failed == 0 )) && echo "OK: Headlamp's client ID matches k3s_oidc_client_id"
exit "$failed"
