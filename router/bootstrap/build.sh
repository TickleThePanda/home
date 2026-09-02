#!/usr/bin/env bash
# Build a bootstrapped vanilla OpenWrt sysupgrade image for the GL-MT6000
# (Flint 2). Local only -- not run by CI. Flash the result by hand; see
# README.md.
set -euo pipefail
cd "$(dirname "$0")"

VERSION="${VERSION:-24.10.5}"
PROFILE="glinet_gl-mt6000"
IMG="${IMAGEBUILDER_IMAGE:-ghcr.io/openwrt/imagebuilder:mediatek-filogic-${VERSION}}"
RUNTIME="${CONTAINER_RUNTIME:-podman}"

for bin in "$RUNTIME" openssl python3; do
  command -v "$bin" >/dev/null || { echo "missing: $bin" >&2; exit 1; }
done

[ -f secrets.env ] || {
  echo "create router/bootstrap/secrets.env from secrets.env.example" >&2; exit 1
}
set -a; . ./secrets.env; set +a
: "${ROUTER_PPPOE_USERNAME:?}" "${ROUTER_PPPOE_PASSWORD:?}" "${ROUTER_WIFI_KEY:?}" \
  "${ROUTER_TS_AUTHKEY:?}"

[ -s ../../deploy_key.pub ] || {
  echo "deploy_key.pub not found in the repo root (from the node bootstrap)" >&2
  exit 1
}

rm -rf build out
mkdir -p build/files/etc/dropbear out

# Carry the current root/LuCI password over. Prefer the existing hash
# straight from the GL backup (ROUTER_ROOT_PASSWORD_HASH); otherwise hash a
# plaintext from secrets.env. Only the hash reaches the image.
if [ -n "${ROUTER_ROOT_PASSWORD_HASH:-}" ]; then
  ROOT_HASH="$ROUTER_ROOT_PASSWORD_HASH"
elif [ -n "${ROUTER_ROOT_PASSWORD:-}" ]; then
  ROOT_HASH="$(openssl passwd -6 "$ROUTER_ROOT_PASSWORD")"
else
  echo "set ROUTER_ROOT_PASSWORD or ROUTER_ROOT_PASSWORD_HASH in secrets.env" >&2
  exit 1
fi
export ROOT_HASH

# templates/ -> build/files/ (Jinja2, see render.py)
python3 render.py

# authorized_keys: the existing CI key, plus any extra interactive keys.
# 0644, not 0600: the Image Builder runs as an unprivileged container user
# that must read it, and dropbear only rejects a *writable* file. Public
# keys -- nothing to hide.
cat ../../deploy_key.pub $([ -f authorized_keys.extra ] && echo authorized_keys.extra) \
  > build/files/etc/dropbear/authorized_keys
chmod 644 build/files/etc/dropbear/authorized_keys

if grep -rn '{{.*}}' build/files; then
  echo "unrendered template expression above" >&2; exit 1
fi

# One space-separated list; strip whole-line and trailing comments.
packages="$(sed 's/#.*//' packages.txt | xargs)"

# The overlay goes in as a read-only bind mount; the built images come back
# out with `<runtime> cp` rather than an output bind mount. Rootless podman
# would otherwise need --userns=keep-id for the container's `buildbot` user
# to write a caller-owned directory, and keep-id + runc trips a
# remount-private failure on some hosts. A named volume caches downloaded
# packages between runs (the runtime owns it, so no uid grief).
cid="$("$RUNTIME" create \
  -v "$PWD/build/files:/builder/user-files:ro" \
  -v router-imagebuilder-dl:/builder/dl \
  "$IMG" \
  sh -c "[ -d ./scripts ] || ./setup.sh; \
    make image PROFILE='$PROFILE' PACKAGES='$packages' FILES=/builder/user-files")"
trap '"$RUNTIME" rm -f "$cid" >/dev/null 2>&1 || true' EXIT

"$RUNTIME" start -a "$cid"
"$RUNTIME" cp "$cid:/builder/bin/targets" out/

image="$(find out -name "*-${PROFILE}-squashfs-sysupgrade.bin" | head -1)"
[ -n "$image" ] || { echo "build produced no sysupgrade image" >&2; exit 1; }
echo
echo "built: router/bootstrap/$image"
