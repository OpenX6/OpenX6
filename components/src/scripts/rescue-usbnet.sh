#!/bin/sh
# OpenX6 rescue level: make the owner's SSH reachable over USB (CDC-NCM) as a
# second transport beside the vendor Wi-Fi AP.
#
# Runs only on a boot that x6-boot-guard put at level 2 (/run/x6-boot/rescue
# exists). For this boot it stops usbd (an explicit stop, so its Restart=always
# does not bring it back), adds the kernel's built-in ncm function to the vendor
# gadget "g1" next to whatever the vendor linked (composite gadget; the vendor
# links are never removed so ins_hald's USB mode state stays consistent), binds
# the UDC again and gives the new interface a fixed link-local address
# (169.254.44.1/16) plus 192.168.44.1/24. No DHCP server of our own: the vendor
# dnsmasq for the Wi-Fi AP holds port 67 on every interface, and a host that gets
# no DHCP answer self-assigns 169.254.x.x and reaches 169.254.44.1 directly
# (macOS verified). The owner sshd already listens on every interface, so nothing
# about SSH, keys or authentication changes: same channel, second transport.
#
# ins_hald rewrites the gadget links when the USB mode changes (UDisk / phone
# control); it may drop our link. The unit's main process therefore keeps
# watching and puts ncm.0 back (unbind, link, bind) whenever it is missing, at
# most once per WATCH_MIN seconds. Nothing persistent is written; the next boot
# (level 0) runs the vendor USB stack untouched because the unit's condition is
# not met.
set -u
SYSFS=${X6_RESCUE_USBNET_SYSFS:-/sys}
RUN=${X6_RESCUE_USBNET_RUN:-/run/x6-rescue-usbnet}
MARKER=${X6_RESCUE_USBNET_MARKER:-/run/x6-boot/rescue}
WAIT=${X6_RESCUE_USBNET_WAIT:-10}
WATCH_INTERVAL=${X6_RESCUE_USBNET_WATCH_INTERVAL:-3}
WATCH_MIN=${X6_RESCUE_USBNET_WATCH_MIN:-10}
WATCH_ROUNDS=${X6_RESCUE_USBNET_WATCH_ROUNDS:-0}
PATH=${X6_RESCUE_USBNET_PATH:-/sbin:/usr/sbin:/bin:/usr/bin}
export PATH
ADDR=192.168.44.1
PREFIX=24
LINKLOCAL=169.254.44.1
LLPREFIX=16
G=$SYSFS/kernel/config/usb_gadget/g1
FUNC=$G/functions/ncm.0
CFG=$G/configs/c.1
LINK=$CFG/ncm.0

mkdir -p -m 700 "$RUN" 2>/dev/null || true
log() {
	printf '%s X6_RESCUE_USBNET: %s\n' "$(cut -d' ' -f1 /proc/uptime 2>/dev/null)" "$*" >>"$RUN/log" 2>/dev/null
	echo "X6_RESCUE_USBNET: $*" >&2
}
udc_name() {
	n=$(ls "$SYSFS/class/udc" 2>/dev/null | wc -l | tr -d ' ')
	[ "$n" = 1 ] || return 1
	ls "$SYSFS/class/udc"
}
bound() {
	[ -n "$(cat "$G/UDC" 2>/dev/null || true)" ]
}
unbind() {
	bound && echo "" >"$G/UDC"
	return 0
}
bind() {
	echo "$1" >"$G/UDC"
}
has_links() {
	for p in "$CFG"/*; do
		[ -L "$p" ] && return 0
	done
	return 1
}
# ensure_ncm: make sure ncm.0 exists, is linked into the configuration and the
# UDC is bound. Vendor links are left exactly as they are. Returns 0 when the
# link is in place (whether it was already there or was put back), 1 on failure.
ensure_ncm() {
	udc=$(udc_name) || { log "UDC count is not exactly one; refusing"; return 1; }
	if [ -L "$LINK" ] && bound; then
		return 0
	fi
	[ -d "$FUNC" ] || mkdir "$FUNC" || { log "cannot create ncm function"; return 1; }
	# Function links cannot change while the gadget is bound.
	unbind
	if [ ! -L "$LINK" ]; then
		ln -s "$FUNC" "$LINK" || { log "cannot link ncm function"; has_links && bind "$udc"; return 1; }
		log "ncm.0 linked into the gadget configuration"
	fi
	bind "$udc" || { log "cannot bind UDC $udc"; return 1; }
	return 0
}
ifname_ready() {
	ifname=$(cat "$FUNC/ifname" 2>/dev/null || true)
	case "$ifname" in
	"" | "(unnamed net_device)") return 1 ;;
	esac
	[ -d "$SYSFS/class/net/$ifname" ]
}
configure_if() {
	ip link set dev "$ifname" up || { log "cannot bring $ifname up"; return 1; }
	ip addr add "$LINKLOCAL/$LLPREFIX" dev "$ifname" 2>/dev/null || true
	ip addr add "$ADDR/$PREFIX" dev "$ifname" 2>/dev/null || true
	umask 077
	printf 'interface=%s\nlinklocal=%s/%s\naddress=%s/%s\nudc=%s\nssh=%s:2222\n' "$ifname" "$LINKLOCAL" "$LLPREFIX" "$ADDR" "$PREFIX" "$udc" "$LINKLOCAL" >"$RUN/status"
	return 0
}
teardown() {
	unbind
	rm -f "$LINK"
	udc=$(udc_name 2>/dev/null || true)
	[ -n "$udc" ] && has_links && bind "$udc" 2>/dev/null
	if [ -s "$RUN/status" ]; then
		ifname=$(sed -n 's/^interface=//p' "$RUN/status")
		if [ -n "$ifname" ]; then
			ip addr del "$ADDR/$PREFIX" dev "$ifname" 2>/dev/null
			ip addr del "$LINKLOCAL/$LLPREFIX" dev "$ifname" 2>/dev/null
		fi
	fi
	rm -f "$RUN/status"
	if command -v systemctl >/dev/null 2>&1; then systemctl start usbd.service >/dev/null 2>&1 || true; fi
	return 0
}
# watch: keep the link alive. Runs forever (WATCH_ROUNDS=0) as the unit's main
# process; a bounded number of rounds is for tests.
watch() {
	last=0
	round=0
	while [ "$WATCH_ROUNDS" -eq 0 ] || [ "$round" -lt "$WATCH_ROUNDS" ]; do
		round=$((round + 1))
		if [ ! -L "$LINK" ] || ! bound; then
			now=$(cut -d' ' -f1 /proc/uptime 2>/dev/null | cut -d. -f1)
			now=${now:-0}
			if [ $((now - last)) -ge "$WATCH_MIN" ]; then
				last=$now
				log "ncm.0 link or UDC binding went away (vendor USB mode change?); putting it back"
				if ensure_ncm && ifname_ready; then
					configure_if || true
				fi
			fi
		fi
		[ "$WATCH_ROUNDS" -eq 0 ] || [ "$round" -lt "$WATCH_ROUNDS" ] || break
		sleep "$WATCH_INTERVAL"
	done
	return 0
}

case "${1:-}" in
start)
	if [ ! -e "$MARKER" ]; then
		log "no rescue marker; vendor USB stack left alone"
		exit 0
	fi
	# Root is needed on the real system; a redirected tree (tests) has no such requirement.
	if [ "$SYSFS" = /sys ] && [ "$(id -u)" != 0 ]; then log "root required"; exit 1; fi
	[ -d "$G" ] && [ -d "$CFG" ] || { log "vendor gadget g1 not present; nothing to take over"; exit 1; }
	udc_name >/dev/null || { log "UDC count is not exactly one; refusing"; exit 1; }
	if command -v systemctl >/dev/null 2>&1; then systemctl stop usbd.service >/dev/null 2>&1 || log "usbd stop returned non-zero (continuing)"; fi
	ensure_ncm || { teardown; exit 1; }
	i=0
	while [ "$i" -lt "$WAIT" ]; do
		ifname_ready && break
		sleep 1
		i=$((i + 1))
	done
	ifname_ready || { log "ncm interface did not appear within ${WAIT}s"; teardown; exit 1; }
	configure_if || { teardown; exit 1; }
	log "owner SSH reachable over USB: $ifname $LINKLOCAL/$LLPREFIX and $ADDR/$PREFIX (udc $udc)"
	# Stay as the unit's main process (ExecStopPost restores the vendor gadget on stop).
	watch
	exit 0
	;;
ensure)
	# One watch round without waiting: used by tests and for manual checks.
	WATCH_ROUNDS=1 WATCH_MIN=0 watch
	exit 0
	;;
stop)
	teardown
	log "ncm.0 removed; vendor gadget configuration left as the vendor set it"
	exit 0
	;;
*)
	echo "usage: $0 start|ensure|stop" >&2
	exit 64
	;;
esac
