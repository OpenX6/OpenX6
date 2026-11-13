"""Package the OpenX6 Windows native desktop using host Python and Go."""
import argparse
import hashlib
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import struct
import subprocess
import sys
import tempfile
import zipfile


UPDATE = Path(__file__).resolve().parents[2] / 'update'
sys.path.insert(0, str(UPDATE))
from build_identity import stamp


def sha256(stream):
    h = hashlib.sha256()
    for block in iter(lambda: stream.read(4 << 20), b''):
        h.update(block)
    return h.hexdigest()


def members(bundle, manifest):
    files = {}
    seen = {'openx6.exe'}
    for line in manifest.read_text(encoding='utf-8').splitlines():
        match = re.fullmatch(r'[0-9a-f]{64}  (.+)', line)
        if not match:
            raise ValueError('Invalid runtime.sha256 entry: ' + line)
        name = match[1]
        path = PurePosixPath(name)
        if (path.is_absolute() or path.as_posix() != name or '..' in path.parts
                or any(c in name for c in '\\:\x00') or name.casefold() in seen):
            raise ValueError('Invalid or duplicate package path: ' + name)
        if name == "desktop-release.json":
            continue  # Always generate the identity from the selected release channel.
        source = bundle.joinpath(*path.parts)
        source.resolve(strict=True).relative_to(bundle)
        if not source.is_file():
            raise ValueError('Missing package file: ' + name)
        files[name] = source
        seen.add(name.casefold())
    required = {'OpenX6.Desktop.exe','Native/build_native.py','Native/identity.py','Native/go-windows.zip',
                'Native/system.hdiff','Native/plan.json','Native/hpatchz.exe','Reader/Python/python.exe'}
    if not required <= files.keys():
        raise ValueError('Native runtime incomplete: ' + ', '.join(sorted(required-files.keys())))
    return files


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', required=True, type=Path, help='Extracted Windows candidate with desired runtime changes')
    parser.add_argument('--launcher', required=True, type=Path, help='Launcher source directory including runtime.sha256')
    parser.add_argument('--out', required=True, type=Path, help='New output ZIP')
    parser.add_argument('--go', default='go', help='Host Go executable')
    args = parser.parse_args()
    bundle = args.bundle.resolve(strict=True)
    files = members(bundle, args.launcher/'runtime.sha256')
    output = args.out.resolve()
    if output.exists() or bundle in output.parents:
        parser.error('Choose a new ZIP outside the input directory.')
    output.parent.mkdir(parents=True, exist_ok=True)
    go = shutil.which(args.go)
    if not go:
        parser.error('Host Go compiler unavailable: ' + args.go)
    go = str(Path(go).resolve())
    with tempfile.TemporaryDirectory(dir=output.parent) as tmp:
        work = Path(tmp)
        gopath = work/'gopath'
        project = gopath/'src/openx6.local/native-launcher'
        project.mkdir(parents=True)
        for source in args.launcher.glob('*.go'):
            shutil.copyfile(source, project/source.name)
        updater = gopath/'src/openx6.local/desktop/update'
        updater.mkdir(parents=True)
        shutil.copyfile(UPDATE/'update.go', updater/'update.go')
        flags = stamp(work)
        files['desktop-release.json'] = work/'desktop-release.json'
        hashes = {}
        for name, file in files.items():
            with file.open('rb') as f:
                hashes[name] = sha256(f)
        (project/'runtime.sha256').write_bytes(''.join(f'{h}  {n}\n' for n,h in hashes.items()).encode())
        env = dict(os.environ, GOOS='windows', GOARCH='amd64', GOAMD64='v1', CGO_ENABLED='0',
                   GOENV='off', GOFLAGS='', GOPATH=str(gopath), GO111MODULE='off', GOTOOLCHAIN='local',
                   GOWORK='off', GOPROXY='off')
        exe = work/'OpenX6.exe'
        subprocess.run([go,'build','-trimpath','-ldflags=-s -w -H=windowsgui -buildid= '+flags,'-o',str(exe),'.'],
                       cwd=project, env=env, check=True)
        with exe.open('rb') as f:
            if f.read(2) != b'MZ':
                raise ValueError('Expected a Windows executable')
            f.seek(0x3c)
            f.seek(struct.unpack('<I', f.read(4))[0])
            if f.read(6) != b'PE\0\0\x64\x86':
                raise ValueError('Expected Windows x64 PE')
            f.seek(0)
            hashes[exe.name] = sha256(f)
        files[exe.name] = exe
        archive = work/'candidate.zip'
        with zipfile.ZipFile(archive, 'x', compression=zipfile.ZIP_DEFLATED, compresslevel=5) as z:
            for name, file in sorted(files.items()):
                z.write(file, name)
        with zipfile.ZipFile(archive) as z:
            if len(z.infolist()) != len(files) or set(z.namelist()) != files.keys():
                raise ValueError('Archive inventory differs')
            for name, expected in hashes.items():
                with z.open(name) as stream:
                    if sha256(stream) != expected:
                        raise ValueError('Archive readback failed: ' + name)
        # Publish only once. Opening exclusively also protects other concurrent releases.
        with archive.open('rb') as src, output.open('xb') as dst:
            try:
                shutil.copyfileobj(src, dst, 4 << 20)
                dst.flush()
                os.fsync(dst.fileno())
            except BaseException:
                dst.close()
                output.unlink()
                raise
    with output.open('rb') as f:
        print(sha256(f) + '  ' + output.name)


if __name__ == '__main__':
    main()
