#!/usr/bin/env bash
set -euo pipefail

# Restart a single container within a running pod without touching its
# siblings, by killing its PID 1 and letting kubelet restart just that
# container in place. Requires imagePullPolicy: Always on the target
# container to pick up a freshly-pushed :latest image.
#
# Usage: ./restart-container.sh <namespace> <label-selector> <container>
# Example: ./restart-container.sh audiobookshelf app=audiobookshelf librofm-sync

if [[ $# -ne 3 ]]; then
  echo "usage: $(basename "$0") <namespace> <label-selector> <container>" >&2
  exit 1
fi

namespace="$1"
selector="$2"
container="$3"

pod="$(kubectl get pod -n "$namespace" -l "$selector" -o jsonpath='{.items[0].metadata.name}')"
# `kill` as a standalone binary isn't installed in most minimal images (e.g.
# python:3.12-slim has no procps) -- use the shell's builtin instead. SIGKILL,
# not SIGTERM: the kernel silently drops unhandled signals sent to PID 1
# unless it explicitly traps them, which a plain `sh` entrypoint doesn't.
kubectl exec -n "$namespace" "$pod" -c "$container" -- sh -c 'kill -9 1'
