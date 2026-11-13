"""Root side: only read validated choice IDs and publish a read-only selection."""
import json
import os
from pathlib import Path
import stat
import sys
import tempfile
import time


def selection(root, module):
    if not root.is_dir() or root.is_symlink():
        raise ValueError('feature namespace unavailable')
    row = module['setting']
    directory = root/'extensions-v1'
    if directory.is_symlink():
        raise ValueError('unsafe preference directory')
    path = directory / f"openx6-choice.{module['id']}.{row['id']}.pref"
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
    except FileNotFoundError:
        return row['default']
    with os.fdopen(fd, 'rb') as f:
        st = os.fstat(f.fileno())
        if (not stat.S_ISREG(st.st_mode) or stat.S_IMODE(st.st_mode) != 0o600
                or st.st_uid != os.geteuid() or st.st_nlink != 1):
            raise ValueError('unsafe preference')
        value = f.read(49)
    for option in row['options']:
        if option['id'].encode() == value:
            return option['id']
    raise ValueError('unknown choice')


def publish(root, directory, module):
    dest = directory/module['id']
    try:
        value = selection(root, module).encode()
    except (OSError, ValueError):
        dest.unlink(missing_ok=True)
        return
    if dest.exists() and dest.read_bytes() == value:
        return
    fd, name = tempfile.mkstemp(dir=directory)
    try:
        with os.fdopen(fd, 'wb') as f:
            f.write(value)
            os.fchmod(f.fileno(), 0o644)
        os.replace(name, dest)
    finally:
        Path(name).unlink(missing_ok=True)


if __name__ == '__main__':
    modules = json.loads(Path(sys.argv[1]).read_bytes())
    while True:
        for item in modules:
            publish(Path('/data/x6-owner-features'), Path('/run/openx6-module-input'), item)
        time.sleep(0.5)
