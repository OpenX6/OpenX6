"""Receive a desktop build on the camera's mounted removable SD card."""
import ctypes
import hashlib
import http.client
import json
import os
from pathlib import Path
import socket
import stat
import struct
import subprocess
import sys

MOUNT = '/mnt/sdcard'
FINAL = 'Insta360X6FW.bin'


def check(condition, message):
    if not condition:
        raise RuntimeError(message)


def card_identity(fd):
    mounts = [line.split() for line in Path('/proc/self/mountinfo').read_text().splitlines()
              if line.split()[4] == MOUNT]
    check(len(mounts) == 1, 'SD not mounted or exported / 当前未检测到可用的相机 SD 卡')
    m = mounts[0]
    pos = m.index('-')
    check(m[3] == '/' and 'rw' in m[5].split(',') and m[pos+1] in ('exfat', 'vfat')
          and m[pos+2] in ('/dev/mmcblk1p1', '/dev/mmcblk1'), 'Unsupported SD mount / 无法确认可写外置 SD 卡')
    dev = os.fstat(fd).st_dev
    check(m[2] == f'{os.major(dev)}:{os.minor(dev)}' and
          os.stat(m[pos+2]).st_rdev == dev and os.path.samestat(os.fstat(fd), os.stat(MOUNT)),
          'SD mount changed / SD 卡挂载已改变')
    cid = Path('/sys/class/block/mmcblk1/device/cid').read_text().strip()
    check(len(cid) == 32, 'Cannot identify card / 无法识别 SD 卡')
    return m[0], m[2], cid


def idle():
    check(shared_state('otad')['update_status'] == 'Idle', 'Updater is busy / 升级器非空闲')


def shared_state(kind):
    query = json.dumps({'cmd':'share_state', 'action':'get', 'type':kind})
    result = json.loads(subprocess.check_output(['/usr/bin/ins_bll_cli', query], timeout=5))
    check(result.get('ret') == 'ok', 'Camera state unavailable')
    return result['data']


def scan_sd(fd, original, saved):
    """Replay the mounted card's actual status through the vendor's normal SD scan."""
    idle()
    media = shared_state('media')
    check(media['record']['record_status'] == 'RecordIdle'
          and media['capture']['capture_status'] == 'CaptureIdle', 'Camera is busy')
    storage = shared_state('storaged')
    card = storage['sd']
    check(card['status_code'] == 'Success' and storage['sd_fs_type'] in ('vfat', 'exfat'),
          'SD not ready')
    info = {k: card[k] for k in ('free_bytes', 'used_bytes', 'total_bytes')}
    check(all(type(n) is int and n >= 0 for n in info.values())
          and info['total_bytes'] > 0, 'Invalid card status')
    info.update(status_code=card['status_code'], fs_type=storage['sd_fs_type'], mount_point=MOUNT)
    check(card_identity(fd) == original, 'SD card changed')
    current = os.stat(FINAL, dir_fd=fd, follow_symlinks=False)
    check(os.path.samestat(current, saved) and current.st_size == saved.st_size
          and current.st_mtime_ns == saved.st_mtime_ns, 'Published file changed')
    result = subprocess.check_output(['/usr/bin/ins_ibus_cli', '--run', 'call-publish',
                                      'kCamera', 'kStoraged', 'DeviceStatusChanged', 'Sd',
                                      json.dumps(info, separators=(',', ':'))], timeout=8)
    lines = result.decode(errors='replace').splitlines()
    check(any(line.strip().startswith(('错误码:', 'Error code:'))
              and line.split(':', 1)[1].replace(' ', '') == 'Success(0)(0)' for line in lines),
          'Vendor did not acknowledge the SD notification')


def clean_card(fd):
    for name in os.listdir(fd):
        s = name.casefold()
        if s == '._' + FINAL.casefold() and finder_sidecar(fd, name):
            continue
        check(not s.endswith(('.pkg', '.bin', '.owner-auth')) and
              s not in ('openx6-boot.txt', 'openx6-boot.ready'), 'Remove existing update files first / 请先移走卡根目录已有的升级文件')


def finder_sidecar(fd, name):
    """Recognize Finder metadata for the install filename on a FAT volume."""
    flags = os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK
    with os.fdopen(os.open(name, flags, dir_fd=fd), 'rb') as src:
        info = os.fstat(src.fileno())
        if not stat.S_ISREG(info.st_mode):
            return False
        header = src.read(26)
        if len(header) != 26 or header[:24] != b'\x00\x05\x16\x07\x00\x02\x00\x00Mac OS X        ':
            return False
        count, = struct.unpack('>H', header[24:])
        if not 1 <= count <= 2:
            return False
        table = src.read(12 * count)
        if len(table) != 12 * count:
            return False
        entries = list(struct.iter_unpack('>III', table))
        ids = {entry[0] for entry in entries}
        return (9 in ids and ids <= {2, 9} and len(ids) == count
                and all(26 + len(table) <= offset <= offset + size <= info.st_size
                        for _, offset, size in entries))


def publish(fd, name):
    libc = ctypes.CDLL(None, use_errno=True)
    if libc.renameat2(fd, name.encode(), fd, FINAL.encode(), 1) != 0:  # RENAME_NOREPLACE
        e = ctypes.get_errno()
        raise OSError(e, os.strerror(e))


def receive(fd, token, size, digest, fetch, report, fingerprint=card_identity, check_idle=idle,
            after_publish=None):
    original = fingerprint(fd)
    check_idle()
    clean_card(fd)
    space = os.fstatvfs(fd)
    check(space.f_bavail * space.f_frsize >= size + (1 << 20), 'SD card full / SD 卡可用空间不足')
    name = '.openx6-' + token + '.partial'
    out = os.open(name, os.O_CREAT | os.O_EXCL | os.O_RDWR | os.O_NOFOLLOW, 0o600, dir_fd=fd)
    published = False
    try:
        with os.fdopen(out, 'w+b') as dst:
            with fetch() as src:
                check(src.status == 200 and src.getheader('Content-Length') == str(size), 'Unexpected transfer size')
                done = 0
                while done < size:
                    chunk = src.read(min(size-done, 1 << 20))
                    check(chunk, 'Incomplete transfer / 传输中断')
                    dst.write(chunk)
                    done += len(chunk)
            dst.flush()
            os.fsync(dst.fileno())
            dst.seek(0)
            h = hashlib.sha256()
            done = 0
            while done < size:
                chunk = dst.read(2 << 20)
                check(chunk, 'Incomplete readback')
                h.update(chunk)
                done += len(chunk)
                if done % (64 << 20) == 0 or done == size:
                    report('readback', str(done))
            check(h.hexdigest() == digest, 'Card digest mismatch / 卡上文件校验失败')
            check(fingerprint(fd) == original, 'Card changed / SD 卡已切换')
            check_idle()
            clean_card(fd)
            check(os.path.samestat(os.fstat(dst.fileno()), os.stat(name, dir_fd=fd, follow_symlinks=False)),
                  'Staged file changed')
            decision = report('verified', digest)
            if decision == b'DISCARD':  # Transfer rehearsal; leave no installation file.
                return False
            check(decision == b'PUBLISH', 'Desktop confirmation missing')
            publish(fd, name)
            published = True
            os.fsync(fd)
            saved = os.fstat(dst.fileno())
        if after_publish is not None:
            after_publish(fd, original, saved)
        return True
    finally:
        if not published:
            os.unlink(name, dir_fd=fd)
            os.fsync(fd)


def main(host, port, token, size, digest):
    prefix = '/' + token + '/'
    def report(action, text):
        con = http.client.HTTPConnection(host, int(port), timeout=20)
        try:
            con.request('POST', prefix + action, text.encode())
            reply = con.getresponse()
            check(reply.status == 200, 'Desktop session closed')
            return reply.read(32)
        finally:
            con.close()
    lock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        try:
            lock.bind('\0openx6-sd-deploy')
        except OSError as exc:
            raise RuntimeError('Another SD transfer is active / 已有写卡任务') from exc
        fd = os.open(MOUNT, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            con = http.client.HTTPConnection(host, int(port), timeout=45)
            def fetch():
                con.request('GET', prefix+'card')
                return con.getresponse()
            finish = 'PUBLISHED'
            def after_publish(fd, original, saved):
                nonlocal finish
                if report('published', digest) == b'SCAN':
                    try:
                        scan_sd(fd, original, saved)
                        finish = 'SCAN_REQUESTED'
                    except (OSError, RuntimeError, ValueError, KeyError, subprocess.SubprocessError):
                        finish = 'SCAN_UNCONFIRMED'
            try:
                published = receive(fd, token, int(size), digest, fetch, report, after_publish=after_publish)
            finally:
                con.close()
        finally:
            os.close(fd)
        report('done', finish if published else 'DISCARDED')
    except Exception as exc:
        report('error', str(exc)[:240])
    finally:
        lock.close()


if __name__ == '__main__':
    main(*sys.argv[1:])
