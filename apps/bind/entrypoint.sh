#!/bin/sh
set -e

# Seed a zone file onto the (persistent) working dir only if it isn't there
# yet -- named owns it afterwards, rewriting it plus a .jnl journal on every
# dynamic update.
for z in home.arpa 1.168.192.in-addr.arpa; do
	[ -f "/var/lib/bind/$z.zone" ] || cp "/seed/$z.zone" "/var/lib/bind/$z.zone"
done

exec named -g -c /etc/bind/named.conf
