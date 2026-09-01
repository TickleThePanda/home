#!/bin/sh
set -e

# Seed each zone's working copy onto the (persistent) working dir only if it
# isn't there yet -- named owns it afterwards, rewriting it plus a .jnl
# journal on every dynamic update.
for f in /seed/*.zone; do
	[ -e "$f" ] || continue
	b=$(basename "$f")
	[ -f "/var/lib/bind/$b" ] || cp "$f" "/var/lib/bind/$b"
done

exec named -g -c /etc/bind/named.conf
