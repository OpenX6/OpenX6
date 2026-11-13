#!/bin/sh
set -eu
T=/run/x6-stereo-payload
U=/run/x6-stereo-control
case "${1:-}" in 0|1) ;; *) exit 64;; esac
exec 8>/run/x6-owner-media.lock
flock -x 8
case "$(/sbin/abctl --boot_slot)" in _a|_b) ;; *) exit 65;; esac
test ! -e /run/x6-owner/vendor-ota-enabled
test ! -e /run/x6-owner/validated-boot
exec 9>$T/select.lock
flock -x 9
case "$(/sbin/abctl --boot_slot)" in _a|_b) ;; *) exit 65;; esac
test ! -e /run/x6-owner/vendor-ota-enabled
test ! -e /run/x6-owner/validated-boot
media=$(/usr/bin/timeout -k 2 8 /usr/bin/ins_bll_cli '{"cmd":"share_state","action":"get","type":"media"}')
printf '%s' "$media" | grep -Eq '"record_status"[[:space:]]*:[[:space:]]*"RecordIdle"'
printf '%s' "$media" | grep -Eq '"capture_status"[[:space:]]*:[[:space:]]*"CaptureIdle"'
test "$(sha256sum /run/x6-stereo-payload/com.insta.stats.ae_params_imx01f.so.linked | cut -d ' ' -f1)" = 92b13cdbb3221228fb58472a7c67187e57910d94a7955e2719520d3826b6c296
case "$(sha256sum /usr/lib/camera/components/com.insta.stats.ae_params_imx01f.so | cut -d ' ' -f1)" in
cd5833e5a469542a97747e98bd2cc567866448a855925735e2db26f28a69c803) ;;
92b13cdbb3221228fb58472a7c67187e57910d94a7955e2719520d3826b6c296) test /run/x6-stereo-payload/com.insta.stats.ae_params_imx01f.so.linked -ef /usr/lib/camera/components/com.insta.stats.ae_params_imx01f.so ;;
*) exit 65;; esac
test "$(sha256sum /run/x6-stereo-payload/com.insta.stats.ae_params_imx01f_csi2.so.linked | cut -d ' ' -f1)" = ef60421b306b267b779a653eddaa421a643e2bab5d2ba622ce05d0fce56cf68c
case "$(sha256sum /usr/lib/camera/components/com.insta.stats.ae_params_imx01f_csi2.so | cut -d ' ' -f1)" in
29a5b07b7c3fa402b8fa1528ed2d0706d4a63fe298da54c916d2bb5df3b90fc2) ;;
ef60421b306b267b779a653eddaa421a643e2bab5d2ba622ce05d0fce56cf68c) test /run/x6-stereo-payload/com.insta.stats.ae_params_imx01f_csi2.so.linked -ef /usr/lib/camera/components/com.insta.stats.ae_params_imx01f_csi2.so ;;
*) exit 65;; esac
test "$(sha256sum /run/x6-stereo-payload/instasettings.config.linked | cut -d ' ' -f1)" = 89ecd12c1e542678cf27cf58b11a06238cdbf6e99fc2af0dd9291d65a8dc2d8b
case "$(sha256sum /usr/lib/camera/instasettings.config | cut -d ' ' -f1)" in
5a28a4a6b0e0e16e9d26cd98421764b2ed4148237d1cf3b13b3ed211651836d0) ;;
89ecd12c1e542678cf27cf58b11a06238cdbf6e99fc2af0dd9291d65a8dc2d8b) test /run/x6-stereo-payload/instasettings.config.linked -ef /usr/lib/camera/instasettings.config ;;
*) exit 65;; esac
native() {
if test /run/x6-stereo-payload/com.insta.stats.ae_params_imx01f.so.linked -ef /usr/lib/camera/components/com.insta.stats.ae_params_imx01f.so; then umount /usr/lib/camera/components/com.insta.stats.ae_params_imx01f.so; fi
test "$(sha256sum /usr/lib/camera/components/com.insta.stats.ae_params_imx01f.so | cut -d ' ' -f1)" = cd5833e5a469542a97747e98bd2cc567866448a855925735e2db26f28a69c803
if test /run/x6-stereo-payload/com.insta.stats.ae_params_imx01f_csi2.so.linked -ef /usr/lib/camera/components/com.insta.stats.ae_params_imx01f_csi2.so; then umount /usr/lib/camera/components/com.insta.stats.ae_params_imx01f_csi2.so; fi
test "$(sha256sum /usr/lib/camera/components/com.insta.stats.ae_params_imx01f_csi2.so | cut -d ' ' -f1)" = 29a5b07b7c3fa402b8fa1528ed2d0706d4a63fe298da54c916d2bb5df3b90fc2
if test /run/x6-stereo-payload/instasettings.config.linked -ef /usr/lib/camera/instasettings.config; then umount /usr/lib/camera/instasettings.config; fi
test "$(sha256sum /usr/lib/camera/instasettings.config | cut -d ' ' -f1)" = 5a28a4a6b0e0e16e9d26cd98421764b2ed4148237d1cf3b13b3ed211651836d0
}
recover() {
 trap - EXIT
 systemctl stop ins_mediad qmmf-server || true
 native
 systemctl start qmmf-server ins_mediad pulseaudio
 printf '\000' > $U/state
}
trap recover EXIT
systemctl stop ins_mediad qmmf-server
native
if test "$1" = 1; then
mount --bind /run/x6-stereo-payload/com.insta.stats.ae_params_imx01f.so.linked /usr/lib/camera/components/com.insta.stats.ae_params_imx01f.so
mount -o remount,bind,ro /usr/lib/camera/components/com.insta.stats.ae_params_imx01f.so
mount --bind /run/x6-stereo-payload/com.insta.stats.ae_params_imx01f_csi2.so.linked /usr/lib/camera/components/com.insta.stats.ae_params_imx01f_csi2.so
mount -o remount,bind,ro /usr/lib/camera/components/com.insta.stats.ae_params_imx01f_csi2.so
mount --bind /run/x6-stereo-payload/instasettings.config.linked /usr/lib/camera/instasettings.config
mount -o remount,bind,ro /usr/lib/camera/instasettings.config
fi
systemctl start qmmf-server ins_mediad pulseaudio
sleep 3
systemctl is-active --quiet qmmf-server ins_mediad pulseaudio
trap - EXIT
