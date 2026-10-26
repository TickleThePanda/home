#!/usr/bin/env bash
# Build a bootstrapped OpenWrt sysupgrade image for the GL-MT6000 (Flint 2):
# the production firmware (../../router/build.sh, same release and packages)
# plus a first-boot config overlay with secrets baked in. Local only -- not
# run by CI. Flash the result by hand; see README.md.
set -euo pipefail
cd "$(dirname "$0")"

for bin in openssl python3; do
  command -v "$bin" >/dev/null || { echo "missing: $bin" >&2; exit 1; }
done

[ -f secrets.env ] || {
  echo "create bootstrap/router/secrets.env from secrets.env.example" >&2; exit 1
}
set -a; . ./secrets.env; set +a
: "${ROUTER_PPPOE_USERNAME:?}" "${ROUTER_PPPOE_PASSWORD:?}" "${ROUTER_WIFI_KEY:?}" \
  "${ROUTER_TS_AUTHKEY:?}"

[ -s ../../ansible/files/ssh-authorized-keys ] || {
  echo "ansible/files/ssh-authorized-keys not found" >&2
  exit 1
}

rm -rf build
mkdir -p build/files/etc/dropbear

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

# authorized_keys: the repo's committed key set, plus any extra interactive
# keys someone doesn't want committed. 0644, not 0600: the Image Builder
# runs as an unprivileged container user that must read it, and dropbear
# only rejects a *writable* file. Public keys -- nothing to hide.
cat ../../ansible/files/ssh-authorized-keys $([ -f authorized_keys.extra ] && echo authorized_keys.extra) \
  > build/files/etc/dropbear/authorized_keys
chmod 644 build/files/etc/dropbear/authorized_keys

if grep -rn '{{.*}}' build/files; then
  echo "unrendered template expression above" >&2; exit 1
fi

# The image carries the secrets above, so it stays in the gitignored out/.
FILES="$PWD/build/files" OUT="$PWD/out" ../../router/build.sh
