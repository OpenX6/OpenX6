"""Apply a single-eye HEVC recording budget only after the sequence header exists."""
import ctypes
import json
import os
import re
import select
import sys
import time
import struct
import subprocess
from fractions import Fraction
from pathlib import Path

LIBC = ctypes.CDLL(None, use_errno=True)
CAPTURE, OUTPUT = 9, 10
CEILING = 245
CUSTOM = Path('/data/x6-owner-features/custom-bitrate-v1/value')


def checked(result):
    if result < 0:
        number = ctypes.get_errno()
        raise OSError(number, os.strerror(number))
    return result


def ioctl(fd, request, size, initial):
    buf = ctypes.create_string_buffer(size)
    ctypes.memmove(buf, initial, len(initial))
    checked(LIBC.ioctl(fd, ctypes.c_ulong(request), ctypes.byref(buf)))
    return buf.raw


def command(*args):
    return subprocess.check_output(args, timeout=4, stderr=subprocess.DEVNULL).decode()


def api(cmd, action, **fields):
    reply = json.loads(command('ins_bll_cli', json.dumps(dict(cmd=cmd, action=action, **fields))))
    if reply.get('ret') != 'ok':
        raise ValueError('camera API unavailable')
    return reply['data']


def control(fd, cid):
    return struct.unpack_from('=i', ioctl(fd, 0xc008561b, 8, struct.pack('=II', cid, 0)), 4)[0]


def period(fd, kind):
    data = ioctl(fd, 0xc0cc5615, 204, struct.pack('=I', kind))
    num, den = struct.unpack_from('=II', data, 12)
    return Fraction(num, den)


def set_period(fd, value):
    buf = bytearray(204)
    struct.pack_into('=I', buf, 0, CAPTURE)
    struct.pack_into('=II', buf, 12, value.numerator, value.denominator)
    ioctl(fd, 0xc0cc5616, 204, bytes(buf))


def apply(change=True):
    mode = api('config', 'get_general', key='kMediaMode')
    if mode['lens_mode'] != 'kSATAngle' or mode['shoot_mode'] not in ('kNormalVideo', 'kFreeFrameVideo'):
        return {'state':'outside-scope'}
    if api('config', 'get_general', key='kMediaVideoBitrateLevel')['bitrate_level'] != 3:
        return {'state':'native'}
    target, = struct.unpack('<I', CUSTOM.read_bytes())
    if not CEILING < target <= 340:
        return {'state':'native'}
    recording = api('share_state', 'get', type='media')['record']
    if recording['record_status'] != 'RecordRunning' or recording['record_data_time_ms'] < 200:
        return {'state':'waiting-header'}
    reply = command('ins_ibus_cli', '--run', 'call-method', 'kCamera', 'kMediad', 'Pipeline', 'PipelineGetStreamInfo', 'StreamTypeMain')
    if 'Success(0)' not in reply or not re.search(r'encode_type:\s*kH265\b', reply):
        return {'state':'no-hevc'}
    def field(name):
        return int(re.search(r'^\s*'+name+r':\s*(\d+)\b', reply, re.M)[1])
    nominal = field('fps')
    if field('bitrate') != min(target,320) * 1000000 or not 1 <= nominal <= 240:
        return {'state':'profile-mismatch'}
    dims = (field('width'), field('height'))
    if target > 320:
        exact = (mode['shoot_mode']=='kFreeFrameVideo' and dims==(3840,3840)
                 and nominal==60 and api('media','get_cam_role')['cam_role'] in ('kCamBack','kCamFront'))
        if not exact:
            target=320
    pid = int(command('pidof','ins_mediad').strip())
    pidfd = checked(LIBC.syscall(434, pid, 0))
    candidates = []
    try:
        for entry in (Path('/proc')/str(pid)/'fd').iterdir():
            try:
                if os.readlink(entry) != '/dev/video33':
                    continue
                fd = checked(LIBC.syscall(438,pidfd,int(entry.name),0))
            except FileNotFoundError:
                continue
            try:
                fmt = ioctl(fd, 0xc0d05604, 208, struct.pack('=I', CAPTURE))
                if struct.unpack_from('=II', fmt, 8) != dims or fmt[16:20] != b'HEVC':
                    continue
                if control(fd,0x9909cf) != CEILING*1000000 or control(fd,0x9909d7) != 1 or control(fd,0x9909ce) != 1:
                    continue
                candidates.append(fd)
                fd = None
            finally:
                if fd is not None:
                    os.close(fd)
        if len(candidates) != 1:
            return {'state':'no-unique-main', 'matched':len(candidates)}
        fd = candidates[0]
        source, current = period(fd, OUTPUT), period(fd, CAPTURE)
        # The source clock is never modified. Derive from it, not from an
        # already scaled capture period, so retries cannot compound.
        if source<=0 or current<=0 or abs(float(1/source)-nominal)>1:
            return {'state':'clock-mismatch','source':str(source),'nominal':nominal}
        # G_PARM on this Qualcomm driver publishes integer floors. Native 29.97
        # reads back as 29, so recover the nominal clock from the main profile.
        native_fps = Fraction(nominal * 1000, 1001) if 1/source == nominal-1 else Fraction(nominal)
        desired = Fraction(target, CEILING) / native_fps
        result = dict(target=target, size=dims, source=str(source), before=str(current), desired=str(desired))
        if current != source and abs(float(1/current)-float(1/desired))<1:
            return dict(result,state='already-applied')
        if current != source and abs(float(current/source)-1)>.02:
            return dict(result,state='unexpected-period')
        if not change:
            return dict(result, state='dry')
        set_period(fd, desired)
        after, input_after = period(fd, CAPTURE), period(fd, OUTPUT)
        if after <= 0 or abs(float(1/after)-float(1/desired)) > 1 or input_after != source:
            set_period(fd, 1/native_fps)
            return dict(result, state='readback-rejected', after=str(after), source_after=str(input_after))
        return dict(result, state='set', after=str(after), source_after=str(input_after))
    finally:
        for fd in candidates:
            os.close(fd)
        os.close(pidfd)


def watch():
    # Device opens are delivered by the kernel, including future open events
    # after mediad restarts. No recorder API polling while idle or recording.
    notify = checked(LIBC.inotify_init1(os.O_CLOEXEC | os.O_NONBLOCK))
    try:
        checked(LIBC.inotify_add_watch(notify, b'/dev/video33', 0x20))  # IN_OPEN
        # Cover a recording already in progress when the service starts.
        deadline = time.monotonic() + 12
        while True:
            remaining = deadline - time.monotonic()
            wait = min(0.35, remaining) if remaining > 0 else None
            if select.select([notify], [], [], wait)[0]:
                os.read(notify, 65536)
                deadline = time.monotonic() + 12
                continue
            if remaining <= 0:
                continue
            try:
                result = apply()
            except (ValueError, KeyError, IndexError, AttributeError, OSError,
                    ZeroDivisionError, struct.error, subprocess.SubprocessError):
                # A service or session may disappear between the getters.
                # Never change another fd on a failed identity check.
                continue
            if result['state'] in ('waiting-header','no-unique-main','no-hevc','profile-mismatch'):
                continue
            if result['state'] in ('set','already-applied'):
                print(json.dumps(result), flush=True)
            deadline = 0
    finally:
        os.close(notify)


if __name__=='__main__':
    if sys.argv[1:] == ['--watch']:
        watch()
    elif not sys.argv[1:]:
        print(json.dumps(apply()))
    else:
        sys.exit(2)
