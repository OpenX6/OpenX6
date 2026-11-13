"""Bootstrap supplied by the native launcher; runs as the existing WSL user."""
import fcntl
import hashlib
import io
import os
from pathlib import Path, PurePosixPath
import stat
import subprocess
import sys
import tempfile
import zipfile


def unpack(stream, expected, dest):
    limit = 32 << 20
    data = stream.read(limit + 1)
    if len(data) > limit or hashlib.sha256(data).hexdigest() != expected:
        raise ValueError('Build program checksum mismatch. Extract the complete release again.')
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        items = archive.infolist()
        if len(items) > 4096 or sum(item.file_size for item in items) > 64 << 20:
            raise ValueError('Invalid program archive size.')
        for item in items:
            name = PurePosixPath(item.filename)
            if (name.is_absolute() or not name.parts or str(name) != item.filename
                    or '..' in name.parts or '\\' in item.filename
                    or not stat.S_ISREG(item.external_attr >> 16)):
                raise ValueError('Invalid program archive member.')
            path = dest.joinpath(*name.parts)
            path.parent.mkdir(parents=True, exist_ok=True)
            with path.open('xb') as out:
                out.write(archive.read(item))


def run(args):
    digest, enrollment, *paths = args
    if os.getuid() == 0 or enrollment not in ('imported', 'existing') or len(paths) != 3:
        raise ValueError('Expected the dedicated unprivileged WSL build account.')
    private = Path.home()/'.local/share/OpenX6'
    private.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd = os.open(private, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        st = os.fstat(fd)
        if st.st_uid != os.getuid() or st.st_mode & 0o077:
            raise ValueError('Unsafe OpenX6 user directory.')
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise ValueError('Another OpenX6 build is in progress.') from None
        if enrollment == 'imported':
            # Written only immediately after a successful first import. Retry after a
            # cancelled/failed first build still permits initial enrollment.
            if not any(os.path.lexists(private/name) for name in
                       ('Holder', 'holder-profile.sha256', 'first-enrollment.pending')):
                with (private/'first-enrollment.pending').open('xb'):
                    pass
        with tempfile.TemporaryDirectory(prefix='app-', dir=private) as tmp:
            app = Path(tmp)
            unpack(sys.stdin.buffer, digest, app)
            env = dict(os.environ, PATH='/opt/toolchain/bin:/opt/toolchain/go/bin:/usr/bin:/usr/sbin:/bin',
                       GOROOT='/opt/toolchain/go', GOTOOLCHAIN='local', GOPROXY='off',
                       GOENV='off', GOSUMDB='off', ZIG_LIB_DIR='/opt/toolchain/zig-lib',
                       OPENX6_LLD='/opt/toolchain/bin/lld')
            result = subprocess.run([sys.executable, '-IB', str(app/'desktop-runtime/windows/bridge.py'),
                            '--upgrade', *paths], stdin=subprocess.DEVNULL,
                           env=env, pass_fds=(fd,))
            if result.returncode:
                # The worker already printed its actionable error. Keep it as the final diagnostic.
                raise SystemExit(result.returncode if result.returncode > 0 else 1)
    finally:
        os.close(fd)


if __name__ == '__main__':
    os.umask(0o077)
    run(sys.argv[1:])
