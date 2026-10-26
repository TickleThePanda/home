#!/usr/bin/env bash
# Flash the gateway with a router/build.sh image, keeping its config, and check
# it comes back healthy. deploy-infra.yaml runs it; so can anyone who can reach
# 192.168.1.1 with the deploy key.
#
#   ./router/upgrade.sh check  router/out   exit 0 if the router runs this build, 3 if not
#   ./router/upgrade.sh flash  router/out   back up, flash, wait for it, verify
#   ./router/upgrade.sh verify router/out   only the post-flash checks
#
# Environment:
#   ROUTER      SSH target (root@192.168.1.1)
#   SSH_KEY     private key (default: ssh's own choice)
#   BACKUP      where flash writes the age-encrypted config backup
#               (default <out>/backup.tar.gz.age)
#   DRY_RUN=1   flash stops after the backup, before anything changes
#
# Flashing cuts CI's own path to the router: CI reaches the LAN through the
# in-cluster cloudflared, which egresses through this router's WAN. So
# sysupgrade runs detached, and everything after it only polls. There is no
# remote rollback. If the router doesn't come back, recovery is physical:
# bootstrap/router/README.md#recovery.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/.." && pwd)"

ROUTER="${ROUTER:-root@192.168.1.1}"
BOARD=glinet,gl-mt6000
BOOT_TIMEOUT=900

# Files a keep-settings sysupgrade must carry over. Most are OpenWrt's own
# keep list; resolv.conf and the sysctl file are added by
# ansible/router/tasks/sysupgrade.yml. The dropbear host key keeps
# ansible/known_hosts valid, and tailscaled.state keeps the tailnet identity.
KEEP=(
  /etc/config/network /etc/config/wireless /etc/config/firewall
  /etc/config/dhcp /etc/config/dropbear /etc/config/system
  /etc/dropbear/authorized_keys /etc/dropbear/dropbear_ed25519_host_key
  /etc/shadow /etc/tailscale/tailscaled.state
  /etc/resolv.conf /etc/sysctl.d/99-tailscale.conf
)

usage() { sed -n '6,8p' "$0" | sed 's/^# //' >&2; exit 2; }
[ $# -eq 2 ] || usage
cmd="$1"
out="$2"
BACKUP="${BACKUP:-$out/backup.tar.gz.age}"

step() { echo "== $*"; }
fail() { echo "::error::router upgrade: $*" >&2; exit 1; }

ssh_opts=(-o BatchMode=yes -o ConnectTimeout=10
  -o ServerAliveInterval=5 -o ServerAliveCountMax=3
  -o UserKnownHostsFile="$repo/ansible/known_hosts" -o StrictHostKeyChecking=yes)
[ -n "${SSH_KEY:-}" ] && ssh_opts+=(-i "$SSH_KEY")
# shellcheck disable=SC2029 # commands are meant to expand on the router
r() { ssh "${ssh_opts[@]}" "$ROUTER" "$@"; }

if [ ! -f "$out/sysupgrade.bin" ] || [ ! -f "$out/firmware.manifest" ]; then
  fail "$out has no build; run router/build.sh first"
fi
want_version="$(sed -nE '1s/^# OpenWrt ([^ ]+) .*/\1/p' "$out/firmware.manifest")"
packages() { grep -v '^#' | sort; }

# 0 if the router already runs this release with this exact package set.
# Prints what differs otherwise.
matches() {
  local running
  running="$(r 'ubus call system board' | jq -r .release.version)" ||
    fail "cannot read the running release"
  if [ "$running" != "$want_version" ]; then
    echo "release: running $running, build is $want_version"
    return 3
  fi
  # A router flashed before router/build.sh existed has no manifest; that
  # counts as a mismatch, so its first CI flash records one.
  if ! diff <(r 'cat /etc/firmware.manifest 2>/dev/null' | packages) \
    <(packages <"$out/firmware.manifest"); then
    echo "packages differ from the build (< running, > build)"
    return 3
  fi
}

verify() {
  step "verify: release and packages"
  matches || fail "router is not running the new build"

  step "verify: interfaces"
  # Every interface set to start at boot is up: wan (PPPoE), lan, trusted,
  # and each IoT VLAN.
  down="$(r 'ubus call network.interface dump' | jq -r '.interface[]
    | select(.autostart and (.up | not)) | .interface')"
  [ -z "$down" ] || fail "interfaces down: $(tr '\n' ' ' <<<"$down")"
  r 'ifstatus wan' | jq -e '."ipv4-address" | length > 0' >/dev/null ||
    fail "wan has no IPv4 address"

  step "verify: internet and DNS from the router"
  r 'ping -c 3 -W 2 9.9.9.9 >/dev/null' || fail "router cannot reach 9.9.9.9"
  r 'nslookup downloads.openwrt.org >/dev/null' || fail "router cannot resolve names"

  step "verify: Wi-Fi"
  r 'ubus call network.wireless status' | jq -e '
    [.[] | select(.disabled | not)] | length > 0 and all(
      .up and (.interfaces | length > 0) and all(.interfaces[]; .ifname != null))' \
    >/dev/null || fail "a radio or SSID is not up (ubus call network.wireless status)"

  step "verify: Tailscale"
  state="$(r 'tailscale status --json' | jq -r .BackendState)"
  [ "$state" = Running ] || fail "tailscale is $state, not Running"

  step "verify: management access"
  listening="$(r 'netstat -ltn')"
  for port in 22 443; do
    grep -q " 192\.168\.1\.1:$port " <<<"$listening" ||
      fail "nothing listens on 192.168.1.1:$port"
  done

  echo "OK: router runs OpenWrt $want_version and is healthy"
}

flash() {
  step "reachability and identity"
  board="$(r 'ubus call system board')" || fail "cannot reach $ROUTER over SSH"
  [ "$(jq -r .board_name <<<"$board")" = "$BOARD" ] ||
    fail "board is $(jq -r .board_name <<<"$board"), not $BOARD"
  [ "$(jq -r .rootfs_type <<<"$board")" = squashfs ] ||
    fail "rootfs is not squashfs; sysupgrade would not keep settings the usual way"
  echo "running $(jq -r .release.description <<<"$board"), flashing $want_version"

  step "upload"
  size_kb=$(($(stat -c %s "$out/sysupgrade.bin") / 1024))
  free_kb="$(r "df -k /tmp | awk 'NR==2 {print \$4}'")"
  # Room for the image plus sysupgrade's own working copy in RAM.
  [ "$free_kb" -gt $((size_kb * 3)) ] || fail "/tmp has ${free_kb}K free, need $((size_kb * 3))K"
  r 'cat > /tmp/fw.bin' <"$out/sysupgrade.bin"
  want_sum="$(cut -d' ' -f1 "$out/sysupgrade.bin.sha256")"
  [ "$(r 'sha256sum /tmp/fw.bin' | cut -d' ' -f1)" = "$want_sum" ] ||
    fail "uploaded image checksum mismatch"

  step "compatibility"
  r 'sysupgrade -T /tmp/fw.bin' || fail "sysupgrade -T rejects the image"

  step "state that must survive"
  keep="$(r 'sysupgrade -l')"
  for f in "${KEEP[@]}"; do
    grep -qx "$f" <<<"$keep" || fail "$f is not in the sysupgrade keep list"
  done

  step "backup"
  # Streams from the router straight into age, so the plaintext (PPPoE and
  # Wi-Fi keys, /etc/shadow, the Tailscale node key) never touches disk here.
  # Encrypted to the committed SSH keys; decrypt with
  #   age -d -i ~/.ssh/id_ed25519 backup.tar.gz.age > backup.tar.gz
  r 'sysupgrade -b -' | age -R "$repo/ansible/files/ssh-authorized-keys" -o "$BACKUP" ||
    fail "config backup failed"
  [ -s "$BACKUP" ] || fail "backup is empty"
  echo "backup: $BACKUP"

  if [ "${DRY_RUN:-}" = 1 ]; then
    r 'rm -f /tmp/fw.bin'
    echo "DRY_RUN: stopping before the flash"
    return
  fi

  step "flash"
  # Detached: sysupgrade kills dropbear, this session and the CI tunnel with it.
  r 'nohup sysupgrade /tmp/fw.bin >/dev/null 2>&1 </dev/null &'
  started=$(date +%s)

  step "wait for the router (up to $((BOOT_TIMEOUT / 60)) min)"
  # Back once it answers with an uptime shorter than the time since the flash
  # began, so a still-running old system never counts.
  while :; do
    elapsed=$(($(date +%s) - started))
    if [ "$elapsed" -gt "$BOOT_TIMEOUT" ]; then
      fail "router did not come back within $((BOOT_TIMEOUT / 60)) min of the flash.
It may still be booting, or the flash failed. CI cannot see past it. Check from
the LAN or Tailscale; recover per bootstrap/router/README.md#recovery. The
pre-flash config backup is $BACKUP."
    fi
    uptime="$(r 'cut -d. -f1 /proc/uptime' 2>/dev/null)" || uptime=""
    if [ -n "$uptime" ] && [ "$uptime" -lt "$elapsed" ]; then
      echo "back after ${elapsed}s"
      break
    fi
    # sysupgrade stops dropbear within seconds, so the old system answering
    # minutes later means it refused the image and never rebooted.
    if [ -n "$uptime" ] && [ "$elapsed" -gt 180 ]; then
      fail "sysupgrade did not reboot the router; it still runs the old firmware.
Check /tmp/fw.bin and run 'sysupgrade -v /tmp/fw.bin' on the router by hand."
    fi
    sleep 10
  done

  # Interfaces, PPPoE and Tailscale settle after dropbear starts.
  for i in $(seq 1 12); do
    (verify) >/dev/null 2>&1 && break
    [ "$i" -lt 12 ] && sleep 10
  done
  verify
}

case "$cmd" in
  check) if matches; then echo "router already runs this build"; else exit 3; fi ;;
  flash) flash ;;
  verify) verify ;;
  *) usage ;;
esac
