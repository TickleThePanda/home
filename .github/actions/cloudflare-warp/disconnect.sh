#!/usr/bin/env bash
# Delete this runner's device registration. Cloudflare keeps registrations
# until deleted, so without this every CI job leaves one behind.

command -v warp-cli >/dev/null || exit 0
sudo warp-cli --accept-tos registration delete \
    || echo "::warning::couldn't delete the WARP registration; remove it under Zero Trust > Devices"
