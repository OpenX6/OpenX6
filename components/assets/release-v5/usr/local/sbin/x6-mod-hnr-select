#!/bin/sh
set -eu
T=/run/x6-hnr-v1
exec 8>/run/x6-owner-media.lock
flock -x 8
case "$(/sbin/abctl --boot_slot)" in _a|_b) ;; *) exit 65;; esac
test ! -e /run/x6-owner/vendor-ota-enabled
test ! -e /run/x6-owner/validated-boot
exec 9>$T/lock
flock -x 9
case "${1:-}" in
0) source=$T/native.so; expected=25e3605f33ad201b82a06ea6ee85f6b6f1fa6a3095d144bbef35a8036a9adca3;;
1) source=$T/off.so; expected=32d68d317c49729d58d89ae7d740f10707e524abbd7330c5b6ac06f35f246f37;;
restore) source=; expected=8386b304b83aab877c469703bae50fd8150ac26a9cf5d730d4c90725bb673a97;;
*) exit 64;;
esac
state=$(/usr/bin/timeout -k 2 8 /usr/bin/ins_bll_cli '{"cmd":"share_state","action":"get","type":"media"}')
printf '%s' "$state" | grep -Eq '"record_status"[[:space:]]*:[[:space:]]*"RecordIdle"'
printf '%s' "$state" | grep -Eq '"capture_status"[[:space:]]*:[[:space:]]*"CaptureIdle"'
old=$(sha256sum /usr/lib/hw/camera.qcom.so | cut -d ' ' -f1)
case "$old" in
8386b304b83aab877c469703bae50fd8150ac26a9cf5d730d4c90725bb673a97) ;;
25e3605f33ad201b82a06ea6ee85f6b6f1fa6a3095d144bbef35a8036a9adca3) test $T/native.so -ef /usr/lib/hw/camera.qcom.so;;
32d68d317c49729d58d89ae7d740f10707e524abbd7330c5b6ac06f35f246f37) test $T/off.so -ef /usr/lib/hw/camera.qcom.so;;
*) exit 65;;
esac
if test -n "$source"; then test "$(sha256sum "$source" | cut -d ' ' -f1)" = "$expected"; fi
if test "$old" = "$expected"; then exit 0; fi
unmount_ours() {
 if test $T/native.so -ef /usr/lib/hw/camera.qcom.so || test $T/off.so -ef /usr/lib/hw/camera.qcom.so; then umount /usr/lib/hw/camera.qcom.so; fi
 test "$(sha256sum /usr/lib/hw/camera.qcom.so | cut -d ' ' -f1)" = 8386b304b83aab877c469703bae50fd8150ac26a9cf5d730d4c90725bb673a97
}
recover() {
 trap - EXIT
 systemctl stop ins_mediad qmmf-server || true
 unmount_ours
 systemctl start qmmf-server ins_mediad pulseaudio
 if test -d $(dirname /run/x6-hnr-control/state); then printf '\000' > /run/x6-hnr-control/state; fi
}
trap recover EXIT
systemctl stop ins_mediad qmmf-server
unmount_ours
if test -n "$source"; then mount --bind "$source" /usr/lib/hw/camera.qcom.so; mount -o remount,bind,ro /usr/lib/hw/camera.qcom.so; fi
systemctl start qmmf-server ins_mediad pulseaudio
sleep 3
systemctl is-active --quiet qmmf-server ins_mediad pulseaudio
test "$(sha256sum /usr/lib/hw/camera.qcom.so | cut -d ' ' -f1)" = "$expected"
trap - EXIT
