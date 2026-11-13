#!/usr/bin/python3
"""Queue an explicit boot mode from the physical SD card."""
import glob
import os
import re
import secrets
import stat
import subprocess
import sys
import time
from pathlib import Path

CARD = '/mnt/sdcard'
REQUEST = 'OPENX6-BOOT.TXT'
READY = 'OPENX6-BOOT.READY'
PENDING = re.compile(r'\.openx6-boot-([0-9a-f]{32})\.pending')
LEVELS = {b'NORMAL': '0', b'LEVEL1': '1', b'LEVEL2': '2'}
GUARD = '/usr/local/sbin/x6-boot-guard'


def is_card(mountinfo, mount_id):
    rows = [line.split() for line in mountinfo.splitlines()]
    matches = [r for r in rows if len(r) > 6 and r[4] == CARD]
    if len(matches) != 1:
        return False
    row = matches[0]
    split = row.index('-')
    return (row[0] == mount_id and row[2].startswith('179:') and row[3] == '/'
            and 'rw' in row[5].split(',') and row[split+1] in ('vfat', 'exfat')
            and re.fullmatch(r'/dev/mmcblk[0-9]+(?:p[0-9]+)?', row[split+2]) is not None)


def exported():
    for pattern in ('/sys/kernel/config/usb_gadget/*/functions/mass_storage.*/lun.*/file',
                    '/sys/class/android_usb/android0/f_mass_storage/lun*/file'):
        if any(Path(p).read_bytes().strip() for p in glob.glob(pattern)):
            return True
    return False


def read_level(directory, name):
    fd = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=directory)
    with os.fdopen(fd, 'rb') as f:
        s = os.fstat(f.fileno())
        if not stat.S_ISREG(s.st_mode) or s.st_size > 32 or s.st_nlink != 1:
            return None
        return LEVELS.get(f.read(33).strip())


def consume(directory, arm):
    # All card operations use this pinned directory, including after removal.
    entries = os.listdir(directory)
    pending = [name for name in entries if PENDING.fullmatch(name)]
    if len(pending) > 1:
        return
    if pending:
        name = pending[0]
    elif REQUEST.lower() in (name.lower() for name in entries):
        if read_level(directory, REQUEST) is None:
            return
        name = '.openx6-boot-' + secrets.token_hex(16) + '.pending'
        os.rename(REQUEST, name, src_dir_fd=directory, dst_dir_fd=directory)
    else:
        return
    os.fsync(directory)
    level = read_level(directory, name)
    if level is None:
        return
    arm(PENDING.fullmatch(name)[1], level)
    os.replace(name, READY, src_dir_fd=directory, dst_dir_fd=directory)
    os.fsync(directory)
    subprocess.run([GUARD,'ack-card',PENDING.fullmatch(name)[1]], check=True, timeout=8)


def arm(token, level):
    subprocess.run([GUARD,'arm-card',token,level], check=True, timeout=8)


def scan():
    fd = os.open(CARD, os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        info = Path('/proc/self/fdinfo/'+str(fd)).read_text()
        mount = re.search(r'^mnt_id:\s*(\d+)$', info, re.M)
        if mount and is_card(Path('/proc/self/mountinfo').read_text(), mount[1]) and not exported():
            consume(fd, arm)
    finally:
        os.close(fd)


if __name__ == '__main__':
    while True:
        try:
            scan()
        except (OSError, ValueError, subprocess.SubprocessError):
            pass  # Missing, exported or removed SD; an unfinished request retries.
        if sys.argv[1:] == ['--once']:
            break
        time.sleep(3)
