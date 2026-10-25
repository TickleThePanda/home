#!/usr/bin/env bash
# Install cloudflare-mesh, enroll it with the service token in INPUT_* and
# wait for it to connect.
#
# cloudflare-mesh is the WARP client without the GUI. cloudflare-warp drags in
# ~70 GUI packages from the Ubuntu mirror, which can take minutes. It is
# published only in the staging repo's `nightly` component, so this installs
# whatever nightly is current; the log shows which.

set -Eeuo pipefail

repo=https://pkg-staging.cloudflareclient.com
keyring=/usr/share/keyrings/cloudflare-mesh-archive-keyring.gpg
timeout=60

fail() {
    echo "::error::$*"
    exit 1
}

# warp-svc reads this at start and registers with the service token, so it
# must exist before the package installs and starts the service.
sudo install -d -m 755 /var/lib/cloudflare-warp
sudo sh -c 'umask 077; cat > /var/lib/cloudflare-warp/mdm.xml' <<EOF
<dict>
  <key>organization</key><string>${INPUT_ORGANIZATION}</string>
  <key>auth_client_id</key><string>${INPUT_AUTH_CLIENT_ID}</string>
  <key>auth_client_secret</key><string>${INPUT_AUTH_CLIENT_SECRET}</string>
  <key>service_mode</key><string>warp</string>
  <key>auto_connect</key><integer>1</integer>
  <key>onboarding</key><false/>
</dict>
EOF

curl -fsSL "$repo/pubkey.gpg" | sudo gpg --yes --dearmor -o "$keyring" \
    || fail "couldn't fetch the signing key from $repo"
echo "deb [signed-by=$keyring] $repo/ $(lsb_release -cs) nightly" \
    | sudo tee /etc/apt/sources.list.d/cloudflare-mesh.list >/dev/null

sudo apt-get -o DPkg::Lock::Timeout=-1 update -q \
    || fail "apt-get update failed"
sudo apt-get -o DPkg::Lock::Timeout=-1 install -y -q --no-install-recommends cloudflare-mesh \
    || fail "couldn't install cloudflare-mesh from $repo (nightly)"
echo "Installed cloudflare-mesh $(dpkg-query -W -f='${Version}' cloudflare-mesh)"
sudo systemctl enable --now warp-svc

warp() { sudo warp-cli --accept-tos "$@"; }

# Until the MDM registration lands, status reports "Registration Missing";
# once registered, auto_connect connects, and `connect` covers the case where
# it doesn't.
start=$SECONDS
status=
while (( SECONDS - start < timeout )); do
    status=$(warp status 2>&1 || true)
    case $status in
        *"Status update: Connected"*) echo "Connected in $((SECONDS - start))s"; exit 0 ;;
        *"Status update: Disconnected"*) warp connect >/dev/null 2>&1 || true ;;
    esac
    sleep 1
done

echo "Last status: $status"
warp registration show || true
sudo journalctl -u warp-svc -n 50 --no-pager || true
if [[ $status == *"Registration Missing"* ]]; then
    fail "WARP never registered -- check the service token (gha2 in terraform/cloudflare-zero-trust/service_tokens.tf)"
fi
fail "WARP didn't connect within ${timeout}s"
