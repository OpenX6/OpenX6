#!/usr/bin/env python3
"""Durable-I/O primitives for the PureShot processor. Pure stdlib + ctypes (the camera's Python 3.10 has no
fcntl module, so flock goes through libc). Everything here is about making a write survive a power cut and
making a read come from the medium, not the page cache -- the two properties the overwrite safety rests on."""
import ctypes, ctypes.util, errno, hashlib, json, os, struct

_libc = ctypes.CDLL(ctypes.util.find_library('c') or 'libc.so.6', use_errno=True)
LOCK_EX, LOCK_NB, LOCK_UN = 2, 4, 8


def flock(fd, op):
    if _libc.flock(int(fd), int(op)) != 0:
        e = ctypes.get_errno()
        raise OSError(e, os.strerror(e))


def try_lock_exclusive(path):
    """Open/create path and take a non-blocking exclusive flock. Returns the fd (keep it open to hold the
    lock) or None if another instance holds it."""
    fd = os.open(path, os.O_RDWR | os.O_CREAT | os.O_CLOEXEC, 0o600)
    try:
        flock(fd, LOCK_EX | LOCK_NB)
    except OSError as e:
        os.close(fd)
        if e.errno in (errno.EWOULDBLOCK, errno.EAGAIN, errno.EACCES):
            return None
        raise
    return fd


def fsync_dir(path):
    try:
        fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
    except OSError:
        return
    try:
        os.fsync(fd)
    except OSError:
        pass
    finally:
        os.close(fd)


def atomic_write(path, data, mode=0o600, dir_fsync=True):
    """Write bytes durably: temp sibling -> fsync -> rename over path -> fsync dir. Atomic replace."""
    if isinstance(data, str):
        data = data.encode('utf-8')
    d = os.path.dirname(path) or '.'
    tmp = path + '.next'
    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_CLOEXEC, mode)
    try:
        os.write(fd, data)
        os.fsync(fd)
    finally:
        os.close(fd)
    os.rename(tmp, path)
    if dir_fsync:
        fsync_dir(d)


def write_json(path, obj, mode=0o600):
    atomic_write(path, json.dumps(obj, separators=(',', ':'), sort_keys=True), mode)


def read_json(path):
    """Return the parsed object, or None if absent/empty/corrupt (callers treat corrupt == absent, never crash)."""
    try:
        with open(path, 'rb') as f:
            b = f.read()
    except OSError:
        return None
    if not b:
        return None
    try:
        return json.loads(b.decode('utf-8'))
    except (ValueError, UnicodeDecodeError):
        return None


def fadvise_dontneed(fd):
    try:
        os.posix_fadvise(fd, 0, 0, os.POSIX_FADV_DONTNEED)
    except (AttributeError, OSError):
        pass


def sha256_medium(path, chunk=1 << 22):
    """SHA-256 read from the MEDIUM: fsync the fd first (flush pages a killed daemon left dirty -- fadvise
    alone can't evict dirty pages), drop the cache, then read sequentially. Raises OSError on any I/O error;
    callers in recovery treat that as 'no decision', never as a mismatch."""
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        try:
            os.fsync(fd)
        except OSError:
            pass
        fadvise_dontneed(fd)
        h = hashlib.sha256()
        while True:
            b = os.read(fd, chunk)
            if not b:
                break
            h.update(b)
        return h.hexdigest()
    finally:
        os.close(fd)


def pwrite_copy(src_fd, dst_fd, size, chunk=1 << 23):
    """Copy exactly `size` bytes from src_fd@0 to dst_fd@0 with pread/pwrite (in place, no truncate). Returns
    bytes written. Raises on a short write so the caller can treat it as a failed critical section."""
    off = 0
    while off < size:
        want = min(chunk, size - off)
        b = os.pread(src_fd, want, off)
        if not b:
            raise OSError(errno.EIO, 'short read from source at %d' % off)
        n = os.pwrite(dst_fd, b, off)
        if n != len(b):
            raise OSError(errno.EIO, 'short pwrite at %d (%d of %d)' % (off, n, len(b)))
        off += n
    return off
