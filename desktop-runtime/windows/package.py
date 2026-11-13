"""Build the WSL distribution offline; smoke-test it before emitting a matched Windows launcher."""
import argparse
import hashlib
import os
import shutil
import subprocess
import tempfile
import uuid

from app_bundle import bundle, copy_release_notices
from refresh_release import launcher
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]


def call(*args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def main():
    p=argparse.ArgumentParser()
    p.add_argument('--toolchain-image',required=True,help='Digest of a locally loaded, clean Linux/amd64 toolchain image')
    p.add_argument('--firmware',type=Path,required=True)
    p.add_argument('--modules',type=Path,required=True)
    p.add_argument('--output',type=Path,required=True)
    args=p.parse_args()
    output=args.output.resolve()
    if output.exists(): raise FileExistsError(output)
    image_id=subprocess.check_output(['docker','image','inspect','--format','{{.Id}}',args.toolchain_image],text=True).strip()
    if image_id!=args.toolchain_image or not image_id.startswith('sha256:'):
        raise ValueError('Pin an already-loaded local toolchain by its image ID.')
    output.parent.mkdir(parents=True,exist_ok=True)
    with tempfile.TemporaryDirectory(dir=output.parent) as temporary:
        job=Path(temporary);context=job/'context';context.mkdir()
        # Closed allowlist: never send the workspace, a selected secret, or firmware as build context.
        for name in ('openx6','components','recipes','install-guard','release','sdk','desktop-runtime/sign'):
            shutil.copytree(ROOT/name,context/'app'/name,ignore=shutil.ignore_patterns('__pycache__','*.pyc'))
        for name in ('LICENSE', 'NOTICE.md'):
            shutil.copyfile(ROOT/name, context/'app'/name)
        bridge=context/'app/desktop-runtime/windows'
        bridge.mkdir()
        for name in ('bridge.py','worker.sh'):
            shutil.copyfile(Path(__file__).with_name(name),bridge/name)
        version=hashlib.sha256(image_id.encode())
        version.update(Path(__file__).with_name('Containerfile').read_bytes())
        for f in sorted((context/'app').rglob('*')):
            if f.is_file():
                version.update(str(f.relative_to(context)).encode()+b'\0')
                version.update(hashlib.sha256(f.read_bytes()).digest())
        worker=version.hexdigest()
        token=uuid.uuid4().hex
        base='openx6-local-toolchain:'+token
        image='openx6-worker-test:'+token
        call('docker','tag',image_id,base)
        try:
            call('docker','build','--network=none','--pull=false','--platform=linux/amd64','--build-arg',
             'TOOLCHAIN_IMAGE='+base,'--build-arg','WORKER_VERSION='+worker,'-t',image,'-f',str(Path(__file__).with_name('Containerfile')),str(context))
        finally: call('docker','image','rm',base)
        try:
            # Real build in an ephemeral container. No mount for private holder or output.
            # Exercise the exact bridge used by OpenX6.exe, including the DrvFS-style
            # paths, unprivileged worker and final output readback. All writes are ephemeral.
            app_path = job/'OpenX6-app.zip'
            app = bundle(app_path)
            script = """set -eu
mkdir -p '/mnt/c/OpenX6 测试'
chown openx6 '/mnt/c/OpenX6 测试'
cd '/mnt/c/OpenX6 测试'
umask 077
runuser -u openx6 -- /usr/bin/python3 -IB /qa/upgrade.py "$1" imported 'C:/input/official' 'C:/input/modules' 'C:/OpenX6 测试' < /qa/app.zip
cd 'OpenX6 SD Build'
sha256sum -c SHA256SUMS
"""
            call('docker','run','--rm','-i','--network=none','--platform=linux/amd64',
                 '--mount','type=bind,readonly,src='+str(args.firmware.resolve())+',dst=/mnt/c/input/official',
                 '--mount','type=bind,readonly,src='+str(Path(__file__).resolve().with_name('upgrade.py'))+',dst=/qa/upgrade.py',
                 '--mount','type=bind,readonly,src='+str(app_path)+',dst=/qa/app.zip',
                 '--mount','type=bind,readonly,src='+str(args.modules.resolve())+',dst=/mnt/c/input/modules',
                 image,'/bin/sh','-c',script,'smoke',app)
            release=job/'release';release.mkdir()
            tar=release/'OpenX6-runtime.tar'
            # Export an unused container, never the one which received user inputs.
            cid=subprocess.check_output(['docker','create',image,'/bin/true'],text=True).strip()
            try: call('docker','export','-o',str(tar),cid)
            finally: call('docker','rm',cid)
            with tar.open('rb') as fh: checksum=hashlib.file_digest(fh,'sha256').hexdigest()
            app_path.rename(release/'OpenX6-app.zip')
            launcher(release, checksum, worker, app)
            copy_release_notices(release)
            names = ('OpenX6.exe', 'desktop-release.json', 'LICENSE', 'sdk/LICENSE', 'OpenX6-NOTICE.md', 'RELEASE_NOTES.md')
            extra = ''.join(hashlib.sha256((release/n).read_bytes()).hexdigest()+'  '+n+'\n' for n in names)
            (release/'SHA256SUMS').write_text(checksum+'  OpenX6-runtime.tar\n'+app+'  OpenX6-app.zip\n'+extra)
            release.rename(output)
        finally: call('docker','image','rm',image)

if __name__=='__main__':main()
