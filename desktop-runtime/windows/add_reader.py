"""Add the optional Windows-native Wi-Fi reader to a verified worker release."""
import argparse
import hashlib
import shutil
import zipfile
from pathlib import Path

PYTHON_SHA = '3c01d7cfc666f746a6f064f60fb2156ab0ec464d3d738ab796371a46a9bb24a2'
HERE = Path(__file__).resolve().parent


def add(release, python_embed):
    if not all((release / name).is_file() for name in ('OpenX6.exe', 'OpenX6-runtime.tar')):
        raise ValueError('Missing matched launcher and runtime')
    with python_embed.open('rb') as stream:
        if hashlib.file_digest(stream, 'sha256').hexdigest() != PYTHON_SHA:
            raise ValueError('Unexpected Windows CPython distribution')
        stream.seek(0)
        with zipfile.ZipFile(stream) as archive:
            archive.extractall(release / 'Reader' / 'Python')
    copy_reader_code(release)


def copy_reader_code(release):
    reader = release/'Reader'
    if not (reader/'Python/libcrypto-3.dll').is_file():
        raise ValueError('Missing embedded OpenSSL library')
    root = HERE.parents[1]
    core = reader/'openx6'
    core.mkdir(exist_ok=True)
    for name in ('__init__.py', 'official.py', 'common.py', 'ui_language.py'):
        shutil.copyfile(root/'openx6'/name, core/name)
    shutil.copyfile(root/'camera_reader/NOTICE.txt', reader/'NOTICE.txt')
    shutil.copytree(HERE.parents[1]/'camera_reader', release/'Reader/camera_reader',
                    dirs_exist_ok=True, ignore=shutil.ignore_patterns('__pycache__', '*.pyc'))
    import sys
    sys.path.insert(0, str(root))
    from camera_reader.worker.build import build as build_camera_worker
    build_camera_worker(reader/'camera_reader/worker')
    shutil.copyfile(HERE/'reader/export_modules.py', reader/'export_modules.py')
    shutil.copyfile(HERE/'reader/deploy_sd.py', reader/'deploy_sd.py')
    (reader/'install-guard').mkdir(exist_ok=True)
    shutil.copyfile(root/'install-guard/platform.json', reader/'install-guard/platform.json')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('release', type=Path)
    parser.add_argument('python_embed', type=Path)
    args = parser.parse_args()
    add(args.release, args.python_embed)
