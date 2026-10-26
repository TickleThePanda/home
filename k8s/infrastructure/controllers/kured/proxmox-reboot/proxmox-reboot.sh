#!/bin/bash
# Reboot proxmox-01 under kured's lock, if it needs a reboot. Run by
# cronjob.yaml on the Pi, the one node that stays up while proxmox-01 and
# its three VMs reboot. See ../README.md.
#
# Before the reboot, any failure uncordons the Pi and releases the lock.
# After it, a failure keeps the lock, which pauses kured until someone
# releases it by hand.

set -Eeuo pipefail

: "${PVE_TOKEN:?}" "${ALERT_FILTER:?}"
DRY_RUN=${DRY_RUN:-false}

ns=kube-system
ds=kured
lock=weave.works/kured-node-lock
lock_owner=proxmox-01
pi=k8s-storage-anchor-01
pve=https://192.168.1.3:8006/api2/json/nodes/proxmox-01
ca=/config/pve-root-ca.pem

log() { echo "$(date -u +%FT%TZ) $*"; }

# kubectl falls back to the in-cluster config only when no flag changes the
# default config, and k() needs --request-timeout. So write the service
# account's config out. tokenFile makes kubectl re-read the rotating token.
sa=/var/run/secrets/kubernetes.io/serviceaccount
export KUBECONFIG=/tmp/kubeconfig
kubectl config set-cluster in-cluster --server="https://$KUBERNETES_SERVICE_HOST:$KUBERNETES_SERVICE_PORT" \
    --certificate-authority="$sa/ca.crt" >/dev/null
kubectl config set-credentials sa --token="" >/dev/null
kubectl config set "users.sa.tokenFile" "$sa/token" >/dev/null
kubectl config set-context in-cluster --cluster=in-cluster --user=sa >/dev/null
kubectl config use-context in-cluster >/dev/null
k() { kubectl --request-timeout=15s "$@"; }

# Prometheus by ClusterIP: CoreDNS runs on the workers and goes down with
# them, but kube-proxy on the Pi keeps routing ClusterIPs.
prom_ip=$(getent hosts prometheus.monitoring.svc.cluster.local | awk '{print $1; exit}')
[[ -n $prom_ip ]] || { log "cannot resolve Prometheus"; exit 1; }
prom() {
    curl -fsS --max-time 10 "http://$prom_ip:9090/api/v1/query" --data-urlencode "query=$1" \
        | jq -r '.data.result[0].value[1] // empty'
}

# Every node Ready (and, unless $1 is "any", schedulable).
nodes_ok() {
    local nodes
    nodes=$(k get nodes -o json) || return 1
    jq -e --arg mode "${1:-}" '[.items[]
        | select(([.status.conditions[] | select(.type == "Ready")][0].status != "True")
            or ($mode != "any" and .spec.unschedulable == true))] | length == 0' \
        <<<"$nodes" >/dev/null
}

# Firing alerts other than the ones kured ignores (the same regex).
blocking_alerts() {
    curl -fsS --max-time 10 "http://$prom_ip:9090/api/v1/alerts" \
        | jq -r --arg f "$ALERT_FILTER" \
            '.data.alerts[] | select(.state == "firing") | .labels.alertname | select(test($f) | not)'
}

# kured's lock format (pkg/daemonsetlock, kured 1.23). The update carries the
# resourceVersion read with the annotation absent, so it fails if kured took
# the lock in between.
acquire_lock() {
    local current rv value
    current=$(k -n "$ns" get ds "$ds" -o json) || return 1
    [[ -z $(jq -r --arg a "$lock" '.metadata.annotations[$a] // empty' <<<"$current") ]] || return 1
    rv=$(jq -r .metadata.resourceVersion <<<"$current")
    value=$(jq -cn --arg owner "$lock_owner" --arg now "$(date -u +%FT%TZ)" \
        '{nodeID: $owner, metadata: {unschedulable: false}, created: $now, TTL: 0}')
    k -n "$ns" annotate ds "$ds" "$lock=$value" --resource-version="$rv" >/dev/null
}

release_lock() {
    local holder
    holder=$(k -n "$ns" get ds "$ds" -o json \
        | jq -r --arg a "$lock" '.metadata.annotations[$a] // empty | fromjson | .nodeID')
    if [[ $holder == "$lock_owner" ]]; then
        k -n "$ns" annotate ds "$ds" "$lock-" >/dev/null
    fi
}

# Wait until $2 succeeds, checking every $3 seconds for up to $1 seconds.
wait_for() {
    local deadline=$((SECONDS + $1)) check=$2 every=$3
    until $check; do
        ((SECONDS < deadline)) || return 1
        sleep "$every"
    done
}

# DRY_RUN goes through the lock and cordon even when no reboot is due.
if [[ $DRY_RUN == true ]]; then
    log "DRY_RUN: skipping the reboot-required check"
elif [[ $(prom 'node_reboot_required{hostname="proxmox-01"}') != 1 ]]; then
    log "proxmox-01 does not need a reboot"
    exit 0
else
    log "proxmox-01 needs a reboot"
fi

ready_to_lock() {
    if ! nodes_ok; then log "waiting: a node is NotReady or cordoned"; return 1; fi
    local alerts
    alerts=$(blocking_alerts) || { log "waiting: cannot read alerts"; return 1; }
    if [[ -n $alerts ]]; then log "waiting: firing alerts: ${alerts//$'\n'/ }"; return 1; fi
    if ! acquire_lock; then log "waiting: kured's lock is held"; return 1; fi
}
if ! wait_for 3600 ready_to_lock 300; then
    log "gave up after an hour; next attempt next week"
    exit 0
fi
log "took kured's lock"

abort() {
    log "failed before the reboot: uncordoning $pi and releasing the lock"
    k uncordon "$pi" || true
    release_lock || true
}
trap abort EXIT

# Pods evicted from the NotReady VMs then wait for them instead of piling
# onto the Pi.
k cordon "$pi" >/dev/null
log "cordoned $pi"

boot_before=$(prom 'node_boot_time_seconds{hostname="proxmox-01"}')
[[ -n $boot_before ]] || { log "no node_boot_time_seconds for proxmox-01"; exit 1; }

if [[ $DRY_RUN == true ]]; then
    log "DRY_RUN: not rebooting proxmox-01"
else
    curl -fsS --max-time 30 --cacert "$ca" -X POST \
        -H "Authorization: PVEAPIToken=$PVE_TOKEN" \
        --data-urlencode command=reboot "$pve/status" >/dev/null
    trap 'log "FAILED after the reboot: kured lock kept, so kured is paused. See k8s/infrastructure/controllers/kured/README.md"' EXIT
    log "rebooting proxmox-01"

    rebooted() {
        local now
        now=$(prom 'node_boot_time_seconds{hostname="proxmox-01"}') || return 1
        [[ -n $now ]] && awk -v a="$now" -v b="$boot_before" 'BEGIN { exit !(a > b) }'
    }
    wait_for 1200 rebooted 30 || { log "proxmox-01 did not come back within 20 minutes"; exit 1; }
    log "proxmox-01 is back"

    all_ready() { nodes_ok any; }
    wait_for 1800 all_ready 30 || { log "nodes not all Ready within 30 minutes"; exit 1; }
    log "all nodes Ready"
fi

trap - EXIT
k uncordon "$pi" >/dev/null
release_lock
log "uncordoned $pi and released kured's lock"
