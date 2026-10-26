#!/usr/bin/env bash
# Build the gateway's sysupgrade image (GL-MT6000) from version.txt and
# packages.txt with the OpenWrt Image Builder, then check the result. Needs no
# secrets: CI runs it to validate a change and before upgrade.sh flashes the
# router. bootstrap/router/build.sh reuses it with its secrets overlay.
#
#   ./router/build.sh                        output in router/out/
#   FILES=<dir> OUT=<dir> ./router/build.sh  add a rootfs overlay, output elsewhere
#
# Output: sysupgrade.bin, sysupgrade.bin.sha256, firmware.manifest (the
# package list the image carries as /etc/firmware.manifest) and the Image
# Builder's own targets/ tree.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"

VERSION="${VERSION:-$(tr -d '[:space:]' <"$here/version.txt")}"
PROFILE=glinet_gl-mt6000
BOARD=glinet,gl-mt6000
IMG="${IMAGEBUILDER_IMAGE:-ghcr.io/openwrt/imagebuilder:mediatek-filogic-v${VERSION}}"
RUNTIME="${CONTAINER_RUNTIME:-podman}"
OUT="${OUT:-$here/out}"
FILES="${FILES:-}"
COMMIT="$(git -C "$here" rev-parse --short HEAD 2>/dev/null || echo unknown)"

fail() { echo "::error::build: $*" >&2; exit 1; }

for bin in "$RUNTIME" jq sha256sum; do
  command -v "$bin" >/dev/null || fail "missing: $bin"
done
[ -z "$FILES" ] || [ -d "$FILES" ] || fail "FILES=$FILES is not a directory"

# One space-separated list; strip whole-line and trailing comments.
packages="$(sed 's/#.*//' "$here/packages.txt" | xargs)"

rm -rf "$OUT"
mkdir -p "$OUT"

# The overlay goes in as a read-only bind mount; the built images come back
# out with `<runtime> cp` rather than an output bind mount. Rootless podman
# would otherwise need --userns=keep-id for the container's `buildbot` user
# to write a caller-owned directory, and keep-id + runc trips a
# remount-private failure on some hosts. A named volume caches downloaded
# packages between runs.
#
# `make manifest` resolves the same package set `make image` installs, so its
# output can go into the image as /etc/firmware.manifest. The router then
# records exactly which package versions it runs, and upgrade.sh compares
# against it. Both runs share one downloaded package index.
mounts=(-v router-imagebuilder-dl:/builder/dl)
[ -n "$FILES" ] && mounts+=(-v "$(cd "$FILES" && pwd):/builder/user-files:ro")

# shellcheck disable=SC2016 # expanded by the container's shell
cid="$("$RUNTIME" create "${mounts[@]}" \
  -e PROFILE="$PROFILE" -e PACKAGES="$packages" \
  -e VERSION="$VERSION" -e COMMIT="$COMMIT" \
  "$IMG" \
  sh -euc '
    [ -d ./scripts ] || ./setup.sh
    mkdir -p /tmp/files/etc
    if [ -d /builder/user-files ]; then cp -r --preserve=mode /builder/user-files/. /tmp/files/; fi
    make -s manifest PROFILE="$PROFILE" PACKAGES="$PACKAGES" >/tmp/manifest
    {
      echo "# OpenWrt $VERSION $PROFILE, built from commit $COMMIT"
      grep -E "^[^ ]+ - " /tmp/manifest
    } >/tmp/files/etc/firmware.manifest
    make image PROFILE="$PROFILE" PACKAGES="$PACKAGES" FILES=/tmp/files
    cp /tmp/files/etc/firmware.manifest /builder/bin/firmware.manifest
  ')"
trap '"$RUNTIME" rm -f "$cid" >/dev/null 2>&1 || true' EXIT

"$RUNTIME" start -a "$cid"
"$RUNTIME" cp "$cid:/builder/bin/targets" "$OUT/"
"$RUNTIME" cp "$cid:/builder/bin/firmware.manifest" "$OUT/firmware.manifest"

target="$OUT/targets/mediatek/filogic"

mapfile -t images < <(find "$target" -name "*-${PROFILE}-squashfs-sysupgrade.bin")
[ "${#images[@]}" -eq 1 ] || fail "expected one sysupgrade image, found ${#images[@]}"
cp "${images[0]}" "$OUT/sysupgrade.bin"
(cd "$OUT" && sha256sum sysupgrade.bin >sysupgrade.bin.sha256)

manifest="$(find "$target" -name "*-${PROFILE}.manifest" | head -1)"
[ -n "$manifest" ] || fail "no package manifest for $PROFILE"

# Every requested package is in the image, and every removed one is not.
for pkg in $packages; do
  case "$pkg" in
    -*) ! grep -q "^${pkg#-} - " "$manifest" || fail "${pkg#-} is still in the image" ;;
    *) grep -q "^$pkg - " "$manifest" || fail "$pkg is not in the image" ;;
  esac
done

# The list baked into the image matches what was actually installed.
diff <(grep -v '^#' "$OUT/firmware.manifest" | sort) <(sort "$manifest") >&2 ||
  fail "/etc/firmware.manifest differs from the build manifest"

# The image is for this device and this release.
profiles="$target/profiles.json"
[ -f "$profiles" ] || fail "no profiles.json"
jq -e --arg b "$BOARD" --arg p "$PROFILE" \
  '.profiles[$p].supported_devices | index($b)' "$profiles" >/dev/null ||
  fail "image does not list $BOARD as a supported device"
built="$(jq -r .version_number "$profiles")"
[ "$built" = "$VERSION" ] || fail "built $built, expected $VERSION"

echo
echo "built: $OUT/sysupgrade.bin (OpenWrt $VERSION, $(grep -vc '^#' "$OUT/firmware.manifest") packages)"
