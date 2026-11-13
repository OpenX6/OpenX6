"""Local regular-file IO only; no camera transport or installer."""
from pathlib import Path, PurePosixPath
import hashlib, json, os, stat

class Rejected(ValueError):
    pass

def require(ok, reason):
    if not ok:
        raise Rejected(reason)

def regular(path, secret=False):
    path = Path(path)
    if os.name == 'nt':
        # Only the native reader opens the user-selected official firmware here.
        require(not secret, 'Secret file checks require the build runtime')
        require(stat.S_ISREG(path.lstat().st_mode), 'Input must be a regular file')
        fd = os.open(path, os.O_RDONLY | os.O_BINARY)
    else:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    f = os.fdopen(fd, 'rb')
    s = os.fstat(fd)
    if not stat.S_ISREG(s.st_mode):
        f.close(); raise Rejected('Input must be a regular file')
    if secret and (s.st_uid != os.getuid() or s.st_mode & 0o077 or s.st_nlink != 1):
        f.close(); raise Rejected('Secret input must be privately owned, single-link and mode 0600 or 0400')
    return f

def digest(path, algorithm='sha256'):
    with regular(path) as f:
        return hashlib.file_digest(f, algorithm).hexdigest()

def sha(b):
    return hashlib.sha256(b).hexdigest()

def canonical(obj):
    return json.dumps(obj, sort_keys=True, separators=(',', ':')).encode()

def save(path, obj):
    Path(path).write_bytes(json.dumps(obj, indent=2, ensure_ascii=False).encode() + b'\n')

def safe_relative(value):
    p = PurePosixPath(value)
    require(value and not p.is_absolute() and all(x not in ('', '.', '..') for x in value.split('/')), 'Unsafe relative path')
    require(all(c.isalnum() or c in '/._-' for c in value), 'Unsupported path characters')
    return str(p)

def asset(root, rel):
    rel = safe_relative(rel)
    root = Path(root).resolve()
    p = root / rel
    require(not any(x.is_symlink() for x in [p, *list(p.parents)[:len(Path(rel).parts)-1]]), 'Symlink in component path')
    p.resolve().relative_to(root)
    return p
