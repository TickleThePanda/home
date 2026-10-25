#!/usr/bin/env bash
# Wait for Flux to apply a promoted commit, and report every Kustomization in
# flux-system. Exits non-zero if any of them isn't Ready on its source's
# current revision by the deadline.
#
# It never forces a Kustomization reconcile. A layer whose ExternalArtifact
# didn't change keeps its revision, counts as "unchanged" and passes as long
# as it is Ready.
#
# Order:
# 1. The OCIRepository and every ExternalArtifact from the `deploy`
#    ArtifactGenerator carry the commit as their origin revision. Then the
#    generator has handled the push.
# 2. `flux-system` applies, since it may change the other Kustomizations' specs.
# 3. Every other Kustomization is Ready on its source's revision. A transient
#    Ready=False doesn't fail the run: dependsOn retries (see
#    flux-system/sync/artifact-generator.yaml). Stalled=True does.
#
# Usage: scripts/wait-flux-deploy.sh <commit-sha>
# Env:   TIMEOUT (seconds, default 600) for the whole run.
#        GITHUB_STEP_SUMMARY, if set, gets a results table.

set -Eeuo pipefail

[[ $# -eq 1 ]] || { echo "usage: $0 <commit-sha>" >&2; exit 2; }
origin="main@sha1:$1"
ns=flux-system
deadline=$((SECONDS + ${TIMEOUT:-600}))
interval=5
origin_key='org.opencontainers.image.revision'

error() { echo "::error title=$1::$2"; }
past_deadline() { (( SECONDS >= deadline )); }

# condition <json> <type> <field>: a status condition's field, or empty.
condition() {
    jq -r --arg t "$2" --arg f "$3" \
        '.status.conditions // [] | map(select(.type == $t))[0][$f] // empty' <<<"$1"
}

# --- 1. Sources carry the commit ------------------------------------------

echo "Waiting for the sources to carry ${origin}"
while :; do
    gen=$(kubectl -n "$ns" get artifactgenerator deploy -o json)
    if [[ $(condition "$gen" Stalled status) == True ]]; then
        error "Flux artifactgenerator/deploy stalled" "$(condition "$gen" Stalled message)"
        exit 1
    fi

    sources=(ocirepository/deploy $(jq -r \
        '.status.inventory // [] | map(select((.kind // "ExternalArtifact") == "ExternalArtifact"))[]
         | "externalartifact/" + .name' <<<"$gen"))
    behind=()
    for src in "${sources[@]}"; do
        got=$(kubectl -n "$ns" get "$src" -o json \
            | jq -r --arg k "$origin_key" '.status.artifact.metadata[$k] // empty')
        [[ $got == "$origin" ]] || behind+=("$src")
    done
    (( ${#behind[@]} == 0 )) && break

    if past_deadline; then
        msg=$(condition "$gen" Ready message)
        error "Flux sources behind" "${behind[*]} never reached ${origin}; artifactgenerator/deploy: ${msg:-no Ready message}"
        exit 1
    fi
    sleep "$interval"
done

# --- 2/3. Kustomizations --------------------------------------------------

declare -A result message

# check <ks>: sets result[ks] to applied/unchanged when it's done, failed when
# Stalled, and leaves it unset while still converging. "applied" means it
# applied this commit: every source carries the commit as its origin, but a
# Kustomization only records it after reconciling a new revision.
check() {
    local ks=$1 json kind name target
    json=$(kubectl -n "$ns" get kustomization "$ks" -o json)
    kind=$(jq -r '.spec.sourceRef.kind' <<<"$json")
    name=$(jq -r '.spec.sourceRef.name' <<<"$json")
    target=$(kubectl -n "$ns" get "${kind,,}/${name}" -o jsonpath='{.status.artifact.revision}')
    message[$ks]=$(condition "$json" Ready message)

    if jq -e --arg r "$target" '
            .status.lastAppliedRevision == $r
            and .status.observedGeneration == .metadata.generation' <<<"$json" >/dev/null \
        && [[ $(condition "$json" Ready status) == True ]]; then
        if [[ $(jq -r '.status.lastAppliedOriginRevision // empty' <<<"$json") == "$origin" ]]; then
            result[$ks]=applied
        else
            result[$ks]=unchanged
        fi
    elif [[ $(condition "$json" Stalled status) == True ]]; then
        result[$ks]=failed
    fi
}

# wait_for <ks...>: polls until each is done or the deadline passes.
wait_for() {
    local pending=("$@") next ks
    while (( ${#pending[@]} > 0 )); do
        next=()
        for ks in "${pending[@]}"; do
            check "$ks"
            [[ -n ${result[$ks]:-} ]] || next+=("$ks")
        done
        pending=("${next[@]}")
        (( ${#pending[@]} == 0 )) && break
        if past_deadline; then
            for ks in "${pending[@]}"; do result[$ks]=failed; done
            break
        fi
        sleep "$interval"
    done
}

echo "Waiting for kustomization/flux-system"
wait_for flux-system

mapfile -t layers < <(kubectl -n "$ns" get kustomization -o name \
    | sed 's|.*/||' | grep -vx flux-system)
echo "Waiting for: ${layers[*]}"
wait_for "${layers[@]}"

# --- Report ---------------------------------------------------------------

rc=0
summary="| Kustomization | Result | Ready message |"$'\n'"| --- | --- | --- |"$'\n'
for ks in flux-system "${layers[@]}"; do
    r=${result[$ks]}
    msg=${message[$ks]:-}
    echo "${ks}: ${r}"
    if [[ $r == failed ]]; then
        error "Flux kustomization/${ks} failed" "${msg:-did not reconcile cleanly}"
        rc=1
    fi
    summary+="| \`${ks}\` | ${r} | ${msg//|/\\|} |"$'\n'
done
[[ -n ${GITHUB_STEP_SUMMARY:-} ]] && printf '%s' "$summary" >>"$GITHUB_STEP_SUMMARY"
exit "$rc"
