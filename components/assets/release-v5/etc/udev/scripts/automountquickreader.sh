#!/bin/sh
case "${1:-}" in ''|*[!a-z0-9]*) exit 64;; esac
if test -r /usr/local/sbin/x6-ssd-coordinator; then
 exec /usr/bin/python3 /usr/local/sbin/x6-ssd-coordinator udev "$1"
fi
# During early boot /data may not be mounted yet. Generic SSD events are
# intentionally left for the coordinator; preserve the native vendor route.
p=$(readlink -f "/sys/class/block/$1") || exit 0
while test -n "$p" && test "$p" != /; do
 if test -r "$p/idVendor"; then
  if test "$(cat "$p/idVendor")" = 2e1a; then
   exec /bin/sh /etc/udev/scripts/automountquickreader.openx6-native.sh "$1"
  fi
  exit 0
 fi
 p=${p%/*}
done
exit 0
