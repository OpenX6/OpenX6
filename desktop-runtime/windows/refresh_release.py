"""Refresh the Windows app on the qualified, unchanged WSL 2 base."""
import argparse
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import tarfile
import zipfile

from app_bundle import bundle, copy_release_notices
from add_reader import copy_reader_code

HERE = Path(__file__).resolve().parent
BASE_ZIPS = {
    'd664bb50db3ac072c3a376d4e36ea82babe299b94563bd0f4941841be8a06048',
    '71fa52352edf65594c16602706bb281f3477a90113f33e8cb9b46e2390fdb479',
    '9464d103ea5c903dd48642c02f1972dd03e02a2d26897c307ff8d1865ee9c1de',
    '177bd18d1401ee012c4bf380be782f5227788aef0a62abae888e79f27d872604',
    '3f544c505d6944c238ffd179ebe14534c79107ccb39381d0374fabd36e073ea0',
    '4d63ec12c8b339e99d2fe360961845f2f46177eb1e27372ed84236f0cee3119a',
    '933d8d5f08fb135cc3aed58faaff10086f44e81a5d62a16835e63531cbc2dc60',
}
BASE_TAR = 'ee1ab8f3915694f91bde67c674b35c127868fb4dc97582658c5c53179e813581'
BASE_MARKER = '24fe8d0d14c3ef01904969a4efab41b539c435f68ebfbd31ff6c6c74444911e9'


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def launcher(release, runtime, marker, app_hash):
    flags = f'-H=windowsgui -s -w -X main.runtimeHash={runtime} -X main.workerVersion={marker} -X main.appHash={app_hash}'
    sys.path.insert(0, str(HERE.parent/'update'))
    from build_identity import stamp
    flags += ' ' + stamp(release)
    subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false', '-ldflags', flags,
                    '-o', str(release/'OpenX6.exe'), '.'], cwd=HERE, check=True,
                   env=dict(os.environ, GO111MODULE='on', GOWORK='off', GOTOOLCHAIN='local', CGO_ENABLED='0',
                            GOOS='windows', GOARCH='amd64', GOPROXY='off', GOSUMDB='off'))


def refresh(base, output):
    if digest(base) not in BASE_ZIPS:
        raise ValueError('Unrecognized base release.')
    if output.exists():
        raise FileExistsError(output)
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=output.parent) as tmp:
        job = Path(tmp)
        with zipfile.ZipFile(base) as archive:
            archive.extractall(job)
        release = job/'OpenX6-Windows11-x64'
        if digest(release/'OpenX6-runtime.tar') != BASE_TAR:
            raise ValueError('Bundled WSL base mismatch.')
        (release/'OpenX6-app.zip').unlink()
        app = bundle(release/'OpenX6-app.zip')
        # WSL runs the verified app ZIP. Do not distribute a second, stale copy of
        # the application inside an otherwise compatible toolchain filesystem.
        tar = release/'OpenX6-runtime.tar'
        clean = job/'toolchain.tar'
        with tarfile.open(tar,'r|') as old, tarfile.open(clean,'w|',format=tarfile.PAX_FORMAT) as fresh:
            for m in old:
                name = m.name.removeprefix('./')
                if name in ('opt/openx6','opt/openx6-run') or name.startswith('opt/openx6/'):
                    continue
                fresh.addfile(m,old.extractfile(m) if m.isfile() else None)
        clean.replace(tar)
        launcher(release, digest(tar), BASE_MARKER, app)
        shutil.copyfile(HERE/'START-HERE.txt', release/'START-HERE.txt')
        copy_release_notices(release)
        # Start the native helper directory fresh; only retain the embedded interpreter.
        reader = release/'Reader'
        runtime = reader/'Python'
        runtime.rename(job/'python-embed')
        shutil.rmtree(reader)
        reader.mkdir()
        (job/'python-embed').rename(runtime)
        for path in release.glob('*.cmd'):
            path.unlink()
        shutil.copyfile(HERE.parents[1]/'camera_reader/LICENSE',reader/'LICENSE')
        copy_reader_code(release)
        checks = release/'SHA256SUMS'
        files = sorted(p for p in release.rglob('*') if p.is_file() and p != checks)
        checks.write_text(''.join(digest(p)+'  '+p.relative_to(release).as_posix()+'\n' for p in files))
        temporary = job/'release.zip'
        with zipfile.ZipFile(temporary, 'x', compression=zipfile.ZIP_DEFLATED, compresslevel=2) as z:
            for path in files + [checks]:
                z.write(path, path.relative_to(job).as_posix())
        with zipfile.ZipFile(temporary) as z:
            if z.testzip() is not None:
                raise ValueError('Release ZIP readback failed.')
        temporary.rename(output)
    return app


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('base_zip', type=Path)
    p.add_argument('output_zip', type=Path)
    a = p.parse_args()
    print('Build program SHA-256', refresh(a.base_zip, a.output_zip.resolve()))
