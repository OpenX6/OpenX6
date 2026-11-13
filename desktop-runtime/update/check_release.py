"""Check the three desktop release assets against their reviewed source checkout."""
import argparse
import hashlib
import json
from pathlib import Path
import re
from zipfile import ZipFile

ROOT = Path(__file__).resolve().parents[2]
NATIVE = ROOT / 'desktop-runtime/windows/native'
MAC = 'OpenX6-macOS-arm64.zip'
WINDOWS = 'OpenX6-Windows11-x64-native.zip'


def digest(stream):
    return hashlib.file_digest(stream, 'sha256').hexdigest()


def sums(data):
    result = {}
    for line in data.decode('utf-8').splitlines():
        match = re.fullmatch(r'([0-9a-f]{64})  (.+)', line)
        if not match or match[2] in result:
            raise ValueError('Malformed or duplicate checksum entry')
        result[match[2]] = match[1]
    return result


def files(archive):
    names = [entry.filename for entry in archive.infolist() if not entry.is_dir()]
    if len(names) != len(set(names)):
        raise ValueError('Duplicate ZIP member')
    return set(names)


def check_windows(archive, manifest):
    expected = sums(manifest)
    if files(archive) != set(expected) | {'OpenX6.exe'}:
        raise ValueError('Windows runtime inventory differs')
    launcher = archive.read('OpenX6.exe')
    if launcher.count(manifest) != 1:
        raise ValueError('Windows launcher does not embed the reviewed runtime manifest')
    for name, expected_hash in expected.items():
        with archive.open(name) as stream:
            if digest(stream) != expected_hash:
                raise ValueError(f'Windows runtime checksum differs: {name}')


def check(directory):
    accepted = (ROOT / 'release/desktop-SHA256SUMS').read_bytes()
    supplied = (directory / 'SHA256SUMS').read_bytes()
    if supplied != accepted:
        raise ValueError('SHA256SUMS differs from the reviewed release')
    hashes = sums(supplied)
    if set(hashes) != {WINDOWS, MAC}:
        raise ValueError('Expected the native Windows and Apple Silicon ZIPs')
    channel = json.loads((ROOT / 'release/desktop-channel.json').read_bytes())
    for name, expected in hashes.items():
        # Hash and inspect the very same open file.
        with (directory / name).open('rb') as stream:
            if digest(stream) != expected:
                raise ValueError(f'Archive checksum differs: {name}')
            stream.seek(0)
            with ZipFile(stream) as archive:
                if name == WINDOWS:
                    check_windows(archive, (NATIVE / 'Launcher/runtime.sha256').read_bytes())
                    if json.loads(archive.read('desktop-release.json')) != channel:
                        raise ValueError('Windows desktop release identity differs')
                    for worker in ('Native/build_native.py', 'Native/identity.py'):
                        if archive.read(worker) != (NATIVE / worker).read_bytes():
                            raise ValueError(f'Windows source differs: {worker}')
                else:
                    prefix = 'OpenX6.app/Contents/'
                    required = {prefix + name for name in (
                        'MacOS/OpenX6', 'MacOS/openx6-update', 'Resources/desktop-release.json')}
                    if not required <= files(archive) or archive.testzip():
                        raise ValueError('Invalid Mac ZIP or incomplete app')
                    if json.loads(archive.read(prefix + 'Resources/desktop-release.json')) != channel:
                        raise ValueError('Mac desktop release identity differs')
            print(expected, name)
    print(hashlib.sha256(supplied).hexdigest(), 'SHA256SUMS')
    print(channel['repository'], 'v' + channel['version'])


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    check(parser.parse_args().directory)
