#!/bin/bash
# Flash the router's firmware under kured's lock. Run by
# router-firmware-cronjob.yaml. See README.md.
#
# Takes kured's lock, dispatches a firmware-only Deploy run and polls it to
# the end. The run checks that it holds the lock but never takes or releases
# it; this script does. A flash that failed after sysupgrade started keeps
# the lock with no TTL, which pauses kured until someone checks the router.

set -Eeuo pipefail

: "${GITHUB_TOKEN:?}" "${ALERT_FILTER:?}"

ns=kube-system
ds=kured
lock=weave.works/kured-node-lock
run_lock=$(cat /proc/sys/kernel/random/uuid)
lock_owner=router-firmware/$run_lock
# kured takes over a lock older than its TTL (a Go duration, in ns). 4.5h
# outlasts the Job's activeDeadlineSeconds, so a pod that dies mid-run
# doesn't pause kured for good.
lock_ttl=16200000000000
repo=TickleThePanda/home
api=https://api.github.com/repos/$repo/actions
# The CI step whose failure means sysupgrade started
# (.github/workflows/router-firmware.yaml).
started_step="Check the flash stopped before sysupgrade"

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

gh() {
    local method=$1 path=$2
    shift 2
    curl -fsS --max-time 15 -X "$method" \
        -H "Authorization: Bearer $GITHUB_TOKEN" \
        -H "Accept: application/vnd.github+json" \
        -H "X-GitHub-Api-Version: 2022-11-28" \
        "$api$path" "$@"
}

prom_ip=$(getent hosts prometheus.monitoring.svc.cluster.local | awk '{print $1; exit}')
[[ -n $prom_ip ]] || { log "cannot resolve Prometheus"; exit 1; }

# Every node Ready and schedulable.
nodes_ok() {
    local nodes
    nodes=$(k get nodes -o json) || return 1
    jq -e '[.items[]
        | select(([.status.conditions[] | select(.type == "Ready")][0].status != "True")
            or .spec.unschedulable == true)] | length == 0' \
        <<<"$nodes" >/dev/null
}

# Firing alerts other than the ones kured ignores (the same regex).
blocking_alerts() {
    curl -fsS --max-time 10 "http://$prom_ip:9090/api/v1/alerts" \
        | jq -r --arg f "$ALERT_FILTER" \
            '.data.alerts[] | select(.state == "firing") | .labels.alertname | select(test($f) | not)'
}

# kured's lock format (pkg/daemonsetlock, kured 1.23), with a TTL in ns.
lock_value() {
    jq -cn --arg owner "$lock_owner" --arg now "$(date -u +%FT%TZ)" --argjson ttl "$1" \
        '{nodeID: $owner, metadata: {unschedulable: false}, created: $now, TTL: $ttl}'
}

# The update carries the resourceVersion read with the annotation absent, so
# it fails if kured took the lock in between.
acquire_lock() {
    local current rv
    current=$(k -n "$ns" get ds "$ds" -o json) || return 1
    [[ -z $(jq -r --arg a "$lock" '.metadata.annotations[$a] // empty' <<<"$current") ]] || return 1
    rv=$(jq -r .metadata.resourceVersion <<<"$current")
    k -n "$ns" annotate ds "$ds" "$lock=$(lock_value "$lock_ttl")" --resource-version="$rv" >/dev/null
}

lock_holder() {
    k -n "$ns" get ds "$ds" -o json \
        | jq -r --arg a "$lock" '.metadata.annotations[$a] // empty | fromjson | .nodeID'
}

release_lock() {
    if [[ $(lock_holder) == "$lock_owner" ]]; then
        k -n "$ns" annotate ds "$ds" "$lock-" >/dev/null
    fi
}

# Rewrite our lock with no TTL, so kured never takes it over.
keep_lock() {
    if [[ $(lock_holder) == "$lock_owner" ]]; then
        k -n "$ns" annotate --overwrite ds "$ds" "$lock=$(lock_value 0)" >/dev/null
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
log "took kured's lock as $lock_owner"

trap 'log "failed before the run was dispatched: releasing the lock"; release_lock || true' EXIT

run_id=$(gh POST /workflows/deploy.yaml/dispatches \
    -d "$(jq -cn --arg lock "$run_lock" \
        '{ref: "main", inputs: {layers: "firmware", lock: $lock}, return_run_details: true}')" \
    | jq -er .workflow_run_id)
log "dispatched https://github.com/$repo/actions/runs/$run_id"

# From here the run may be flashing, so a failure leaves the lock to its TTL.
trap 'log "FAILED while following run $run_id: kured'"'"'s lock left to expire"' EXIT

# The flash cuts the internet for a few minutes, so API errors are retried
# until the Job's deadline.
dispatched=$SECONDS
cancelled=false
while :; do
    sleep 30
    run=$(gh GET "/runs/$run_id") || { log "cannot read the run; retrying"; continue; }
    status=$(jq -r .status <<<"$run")
    [[ $status == completed ]] && break
    # A run waits as queued/pending behind another Deploy run.
    if [[ $status != in_progress && $cancelled == false ]] && ((SECONDS - dispatched > 5400)); then
        log "run still $status after 90 minutes: cancelling it"
        if gh POST "/runs/$run_id/cancel" >/dev/null; then
            cancelled=true
        else
            log "cannot cancel the run; retrying"
        fi
    fi
done
conclusion=$(jq -r .conclusion <<<"$run")
log "run finished: $conclusion"

if [[ $conclusion == success ]]; then
    trap - EXIT
    release_lock
    log "released kured's lock"
    exit 0
fi

jobs=
for _ in 1 2 3 4 5; do
    jobs=$(gh GET "/runs/$run_id/jobs?per_page=100") && break
    sleep 30
done
[[ -n $jobs ]] || { log "cannot read the run's jobs"; keep_lock; exit 1; }

trap - EXIT
if jq -e --arg s "$started_step" \
    '[.jobs[].steps[]? | select(.name == $s and .conclusion == "failure")] | length > 0' \
    <<<"$jobs" >/dev/null; then
    keep_lock
    log "FAILED after sysupgrade started: kured's lock kept, so kured is paused. Check the router, then see README.md"
    exit 1
fi

release_lock
log "released kured's lock; the router was not flashed"
if [[ $cancelled == true && $conclusion == cancelled ]]; then
    log "next attempt next week"
    exit 0
fi
exit 1
