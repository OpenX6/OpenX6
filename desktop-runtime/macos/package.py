"""Build the standalone Apple Silicon desktop from installed release toolchains."""
import argparse
import hashlib
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from openx6.toolchain import compiler_tools


def output(*args):
    return subprocess.check_output(args,text=True).strip()


def bundle_tools(resources):
    tools=resources/'tools'
    bin=tools/'bin'; libs=tools/'lib'
    bin.mkdir(parents=True); libs.mkdir()
    visited={}
    def macho(source, dest=None):
        source=Path(source).resolve()
        if source in visited:
            return visited[source]
        if dest is None:
            # Two LLVM versions in this toolchain have the same install name.
            tag=hashlib.sha256(source.read_bytes()).hexdigest()[:12]
            dest=libs/(tag+'-'+source.name)
        visited[source]=dest
        shutil.copy2(source,dest)
        dest.chmod(0o755)
        changes=[]
        for row in output('otool','-L',str(source)).splitlines()[1:]:
            path=row.strip().partition(' (compatibility')[0]
            if path.startswith(('/usr/lib/','/System/')) or not path: continue
            if path.startswith('@rpath/'):
                path_on_disk=source.parent/path[7:]
                if not path_on_disk.exists(): path_on_disk=source.parent.parent/'lib'/path[7:]
            else: path_on_disk=Path(path)
            if path_on_disk.resolve()==source:
                changes+=['-id','@loader_path/'+dest.name]
                continue
            dep=macho(path_on_disk)
            changes+=['-change',path,'@loader_path/'+os.path.relpath(dep,dest.parent)]
        if changes:
            subprocess.run(['install_name_tool',*changes,dest],check=True,stderr=subprocess.PIPE)
            subprocess.run(['codesign','--force','--sign','-',dest],check=True,stderr=subprocess.PIPE)
        return dest
    cc=compiler_tools()
    crypto=Path('/opt/homebrew/opt/openssl@3')
    for name,source in {'clang':cc['clang'],'llvm-strip':Path(cc['clang']).with_name('llvm-strip'),
                        'lld':cc['lld'],'zig':cc['zig'],'openssl':crypto/'bin/openssl',
                        **{n:Path('/opt/homebrew/opt/e2fsprogs/sbin')/n for n in ('e2fsck','debugfs')}}.items():
        macho(source,bin/name)
    (tools/'libcrypto').symlink_to(macho(crypto/'lib/libcrypto.3.dylib').relative_to(tools))
    shutil.copytree(Path(cc['clang']).resolve().parents[1]/'lib/clang',libs/'clang',symlinks=False)
    shutil.copytree(Path(cc['zig']).resolve().parents[1]/'lib/zig',tools/'zig-lib',symlinks=False)
    goroot=Path(output(cc['go'],'env','GOROOT'))
    for name in ('src','lib','pkg','bin'): shutil.copytree(goroot/name,tools/'go'/name)
    for name in ('VERSION','go.env'): shutil.copyfile(goroot/name,tools/'go'/name)
    notices=tools/'notices'; notices.mkdir()
    for package in ('go','llvm','llvm@22','zig','lld@22','openssl@3','e2fsprogs','gettext','zstd','xz','z3'):
        directory=Path('/opt/homebrew/opt')/package
        for f in directory.glob('*'):
            if f.is_file() and f.name.upper().startswith(('LICENSE','COPYING','NOTICE','SBOM')):
                shutil.copyfile(f,notices/(package+'-'+f.name))


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('output',type=Path)
    parent=parser.parse_args().output.resolve()
    parent.mkdir(parents=True,exist_ok=True)
    app=parent/'OpenX6.app'
    if app.exists(): raise FileExistsError(app)
    work=parent/'freeze'
    args=[sys.executable,'-m','PyInstaller','--noconfirm','--clean','--windowed','--noupx',
          '--name','OpenX6','--paths',str(ROOT),'--distpath',str(parent),'--workpath',str(work),
          '--specpath',str(work)]
    for rel in ('components','recipes','install-guard','release','desktop-runtime/sign'):
        args+=['--add-data',f'{ROOT/rel}:{rel}']
    from camera_reader.worker.build import build as build_camera_worker
    for rel in ('LICENSE', 'NOTICE.md', 'sdk/LICENSE', 'camera_reader/LICENSE', 'camera_reader/NOTICE.txt'):
        args += ['--add-data', f'{ROOT/rel}:{Path(rel).parent}']
    for f in sorted((ROOT/'openx6').glob('*.py')):
        args+=['--add-data',f'{f}:openx6']
    subprocess.run([*args,str(Path(__file__).with_name('entry.py'))],check=True,cwd=ROOT)
    info = app/'Contents/Info.plist'
    plist = plistlib.loads(info.read_bytes())
    plist['NSLocalNetworkUsageDescription'] = (
        'OpenX6 connects to your X6 Wi-Fi when you choose Unpack on my X6 '
        'to receive verified modules, or when you choose to deploy a signed build to the camera SD card.')

    sys.path.insert(0, str(ROOT/'desktop-runtime/update'))
    from build_identity import stamp
    flags = stamp(app/'Contents/Resources')
    from build_identity import identity
    version = identity()['version']
    plist['CFBundleShortVersionString'] = version.removesuffix('.0') if version.endswith('.0') else version
    plist['CFBundleVersion'] = version
    info.write_bytes(plistlib.dumps(plist))
    subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false', '-ldflags', flags,
                    '-o', app/'Contents/MacOS/openx6-update', './cmd'],
                   cwd=ROOT/'desktop-runtime/update', check=True,
                   env=dict(os.environ, CGO_ENABLED='0', GOOS='darwin', GOARCH='arm64',
                            GOWORK='off', GOTOOLCHAIN='local', GOPROXY='off'))
    subprocess.run(['codesign','--force','--sign','-',app/'Contents/MacOS/openx6-update'],check=True)
    build_camera_worker(app/'Contents/Resources/camera-worker')
    bundle_tools(app/'Contents/Resources')
    subprocess.run(['codesign','--force','--sign','-',app],check=True)
    print(app)
if __name__=='__main__': main()
