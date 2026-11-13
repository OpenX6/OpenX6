#!/bin/sh
set -eu
T=/run/x6-video-nr-v1
exec 8>/run/x6-owner-media.lock
flock -x 8
case "$(/sbin/abctl --boot_slot)" in _a|_b) ;; *) exit 65;; esac
test ! -e /run/x6-owner/vendor-ota-enabled
test ! -e /run/x6-owner/validated-boot
exec 9>$T/lock
flock -x 9
case "${1:-}" in
0) source=$T/native.so; expected=7cf015cc9bc590980ce5ee8f30fc0af144e658efaf936032f50288b29840c607;;
1) source=$T/medium.so; expected=9809ac872770d77567dbed06843220bd680f69e4397f05b358e860bde6213ba2;;
2) source=$T/low.so; expected=02d29ec0fdca45d018a0dfd29c5c767c813adad27984baf49337ae6cdba9abaf;;
3) source=$T/minimal.so; expected=7ef16ab6fc2501c0e6a614e5debbfee55944c7cb445a513508980ccc26d7d379;;
restore) source=; expected=977d6e28c5cc45ceb600e8b01cc4a7b7c967baac68594b009360820cae693d1f;;
*) exit 64;;
esac
state=$(/usr/bin/timeout -k 2 8 /usr/bin/ins_bll_cli '{"cmd":"share_state","action":"get","type":"media"}')
printf '%s' "$state" | grep -Eq '"record_status"[[:space:]]*:[[:space:]]*"RecordIdle"'
printf '%s' "$state" | grep -Eq '"capture_status"[[:space:]]*:[[:space:]]*"CaptureIdle"'
old=$(sha256sum /usr/lib/libinsiqsetting.so | cut -d ' ' -f1)
case "$old" in
977d6e28c5cc45ceb600e8b01cc4a7b7c967baac68594b009360820cae693d1f) ;;
7cf015cc9bc590980ce5ee8f30fc0af144e658efaf936032f50288b29840c607) test $T/native.so -ef /usr/lib/libinsiqsetting.so;;
9809ac872770d77567dbed06843220bd680f69e4397f05b358e860bde6213ba2) test $T/medium.so -ef /usr/lib/libinsiqsetting.so;;
02d29ec0fdca45d018a0dfd29c5c767c813adad27984baf49337ae6cdba9abaf) test $T/low.so -ef /usr/lib/libinsiqsetting.so;;
7ef16ab6fc2501c0e6a614e5debbfee55944c7cb445a513508980ccc26d7d379) test $T/minimal.so -ef /usr/lib/libinsiqsetting.so;;
*) exit 65;;
esac
if test -n "$source"; then test "$(sha256sum "$source" | cut -d ' ' -f1)" = "$expected"; fi
if test "$old" = "$expected"; then exit 0; fi
unmount_ours() {
 if test $T/native.so -ef /usr/lib/libinsiqsetting.so || test $T/medium.so -ef /usr/lib/libinsiqsetting.so || test $T/low.so -ef /usr/lib/libinsiqsetting.so || test $T/minimal.so -ef /usr/lib/libinsiqsetting.so; then umount /usr/lib/libinsiqsetting.so; fi
 test "$(sha256sum /usr/lib/libinsiqsetting.so | cut -d ' ' -f1)" = 977d6e28c5cc45ceb600e8b01cc4a7b7c967baac68594b009360820cae693d1f
}
recover() {
 trap - EXIT
 systemctl stop ins_mediad qmmf-server || true
 unmount_ours
 systemctl start qmmf-server ins_mediad pulseaudio
 if test -d $(dirname /run/x6-video-nr-control/state); then printf '\000' > /run/x6-video-nr-control/state; fi
}
trap recover EXIT
systemctl stop ins_mediad qmmf-server
unmount_ours
if test -n "$source"; then mount --bind "$source" /usr/lib/libinsiqsetting.so; mount -o remount,bind,ro /usr/lib/libinsiqsetting.so; fi
systemctl start qmmf-server ins_mediad pulseaudio
sleep 3
systemctl is-active --quiet qmmf-server ins_mediad pulseaudio
test "$(sha256sum /usr/lib/libinsiqsetting.so | cut -d ' ' -f1)" = "$expected"
trap - EXIT
