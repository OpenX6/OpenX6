"""Migrate the two public files of the original Windows acceptance runtime."""
import fcntl
import hashlib
import json
import os
import stat
import sys
import tempfile
from pathlib import Path

PREVIOUS = 'e59263c6a0f673fa68196825fd5ad0685cc37a11731dca6b0cc4d66d82bfb0b1'
BASE = '7bc79aac20ebf59c78158a850756f223d4c6b1a1b11eed89a92005ddc7f10352'
PATCHED = '692692ee764df787758c45f0d4856424dcabe23380fffd4be2fb5a929b3bc3d0'
RULE = dict(Version='0.8.827', OTASHA256='d2ef51479725950986b9ad45a82ed2a7c60a8accafeef58a46f795b8f770531b', Mode='first-install')


def sha(data):
    return hashlib.sha256(data).hexdigest()


TARGET = sha((PREVIOUS+'\n'+PATCHED+'\n').encode())


def replace(path, data):
    with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as tmp:
        name = Path(tmp.name)
        try:
            tmp.write(data)
            tmp.flush()
            os.fchmod(tmp.fileno(), 0o644)
            os.fsync(tmp.fileno())
            os.replace(name, path)
            directory = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY)
            try:
                os.fsync(directory)
            finally:
                os.close(directory)
        finally:
            name.unlink(missing_ok=True)


def migrate(opt, expected):
    marker = opt/'openx6-worker-version'
    policy = opt/'openx6/install-guard/platform.json'
    for entry in (opt, opt/'openx6', policy.parent, marker, policy):
        info = entry.lstat()
        if info.st_uid != os.geteuid() or info.st_mode & 0o022:
            raise ValueError('unexpected worker file owner or mode')
        if entry in (marker, policy):
            if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or info.st_size > 8192:
                raise ValueError('unsafe worker file')
        elif not stat.S_ISDIR(info.st_mode):
            raise ValueError('unsafe worker directory')
    directory = os.open(opt, os.O_RDONLY | os.O_DIRECTORY)
    try:
        fcntl.flock(directory, fcntl.LOCK_EX)
        # Shipped /opt and its subdirectories are owned and writable only by root.
        previous = marker.read_text()
        before = policy.read_bytes()
        checksum = sha(before)
        if expected != TARGET or previous not in (PREVIOUS+'\n', TARGET+'\n'):
            raise ValueError('unrecognized worker revision')
        if checksum not in (BASE, PATCHED) or (previous == TARGET+'\n' and checksum != PATCHED):
            raise ValueError('unexpected existing factory compatibility policy')
        if checksum == BASE:
            config = json.loads(before)
            config['Sources'].append(RULE)
            payload = (json.dumps(config, indent=2)+'\n').encode()
            if sha(payload) != PATCHED:
                raise ValueError('updated factory compatibility policy failed verification')
            replace(policy, payload)
        # A retry after the policy rename can complete the version marker.
        if previous != TARGET+'\n':
            replace(marker, (TARGET+'\n').encode())
    finally:
        os.close(directory)


if __name__ == '__main__':
    if os.geteuid() != 0 or len(sys.argv) != 2:
        raise SystemExit('expected the verified WSL compatibility update')
    migrate(Path('/opt'), sys.argv[1])
