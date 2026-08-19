#!/bin/sh
set -eu

export PYTHONUNBUFFERED=1
SLEEP_TIME="${SLEEP_TIME:-6h}"

while true; do
  librofm-download /audiobooks
  sleep "$SLEEP_TIME"
done
