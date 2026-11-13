"""Two offline build profiles. Both publish only verified, locked candidates."""
import hashlib
import json
import os
import re
import shutil
import subprocess
import tempfile
import zipfile
from pathlib import Path

from sdk import module_plan
from .common import canonical, digest, regular, require, save, sha
from .desktop_keys import read_project_public
from .fsimage import build_image, read_file, tool
from .install import configuration, scripts, verify_installer
from .kit import checked_inventory, TRUST
from .component_sources import SOURCES, assemble
from .modules import load_modules, receipt, TARGET
from .official import import_modules, reconstruct, OFFICIAL_SHA
from .owner import validate_owner
from .package import pack
from . import programs
from .sourcebuild import generate_sources
from .toolchain import compiler_tools


def verify_maintenance_pins(read):
    """Check both runtime consumers against the SSH executable in the finished filesystem."""
    target = '/usr/local/sbin/x6-owner-maintenance'
    expected = sha(read(target))
    launcher = read('/usr/local/sbin/x6-owner-late-start')
    pins = re.findall(rb'^digest_is /usr/local/sbin/x6-owner-maintenance ([0-9a-f]{64})$', launcher, re.M)
    require(pins == [expected.encode()], 'Maintenance launcher pins a different SSH executable')
    verifier = read('/usr/local/sbin/x6-owner-update-check')
    prefix = b'{\n  "AutomaticEnabled":'
    require(verifier.count(prefix) == 1, 'Embedded maintenance policy missing or ambiguous')
    offset = verifier.index(prefix)
    policy, _ = json.JSONDecoder().raw_decode(verifier[offset:].decode('utf-8', errors='replace'))
    require(policy['Files'][target] == [expected], 'Update coordinator pins a different SSH executable')

ROOT = Path(__file__).resolve().parents[1]
STOCK_MEDIA = 'b41f38a7a4886a4f32cebdd517dddca9e62d214b18ad41b40cae9a916d97324b'


def outside_repo(path):
    require(not path.resolve().is_relative_to(ROOT), 'Choose a location outside the OpenX6 repository.')


def preflight_modules(paths, *, full):
    """Check the selection and freeze handler bytes without firmware, keys or compilers."""
    base = STOCK_MEDIA
    if full:
        profile = json.loads((SOURCES/'profile.json').read_bytes())
        base = profile['actions'][TARGET]['sha256']
    mods = load_modules(paths, base, stock=not full)
    return mods, programs.snapshot(paths, mods)


def build_guard(out, image, go, installable):
    with zipfile.ZipFile(out/'official/qcs8625_ota.zip') as z:
        members = {}
        for info in z.infolist():
            with z.open(info) as source:
                members[info.filename] = dict(bytes=info.file_size,sha256=hashlib.file_digest(source,'sha256').hexdigest())
    members['system.new.dat'] = dict(bytes=image['bytes'],sha256=image['sha256'])
    config, original = configuration(out, {'members':members}, install_allowed=installable)
    require(config['InstallAllowed'] is installable, 'Installation guard must remain locked.')
    directory = out/'installer'
    shutil.copytree(ROOT/'install-guard',directory)
    save(directory/'guard-config.json',config)
    (directory/'official-updater-script').write_bytes(original)
    env = dict(os.environ,GO111MODULE='off',CGO_ENABLED='0',GOTOOLCHAIN='local',GOPROXY='off',GOSUMDB='off',GOWORK='off')
    subprocess.run([go,'test','.'],cwd=directory,env=env,check=True)
    guard = directory/'x6-install-guard'
    subprocess.run([go,'build','-trimpath','-ldflags=-s -w -buildid=','-o',guard,'.'],
                   cwd=directory,env=dict(env,GOOS='linux',GOARCH='arm64',GOARM64='v8.0'),check=True)
    ih = digest(out/'system-patched.img','sha1')
    blocks = image['bytes']//4096
    for name,body in scripts(original,ih,blocks).items():
        (directory/Path(name).name).write_bytes(body)
    save(directory/'build.json',dict(config_sha256=digest(directory/'guard-config.json'),
         sources={f.name:digest(f) for f in directory.glob('*.go')},binary_sha256=digest(guard),
         system_sha1=ih,system_blocks=blocks,install_allowed=installable))
    return verify_installer(out, allow_install=installable)


def build(firmware, material, manifests, dest, *, holder=None, progress=print, sd=False, byte_progress=None):
    require(type(sd) is bool and (not sd or (holder and not manifests)), 'SD builds require a complete holder profile and built-in modules only.')
    dest = dest.resolve()
    outside_repo(dest)
    require(dest.parent.is_dir() and not dest.exists(), 'Choose a parent without an existing OpenX6 Build directory.')
    require(shutil.disk_usage(dest.parent).free >= 20 * 1024**3, '20 GiB of local free space required.')
    mods, sources = preflight_modules(manifests, full=bool(holder))
    tool('debugfs')
    tool('e2fsck')
    require(Path(material).is_dir(), 'Select the three-module folder exported by your X6.')
    with tempfile.TemporaryDirectory(prefix='.openx6-build-',dir=dest.parent) as tmp:
        out=Path(tmp)
        if holder:
            tools=compiler_tools()
            validate_owner(holder)
            expected = checked_inventory()
            components=out/'components'
        progress('1/6 · Verifying official firmware / 校验官方固件')
        official=out/'official'
        import_modules(firmware, Path(material), official)
        stock=official/'system.img'
        progress('2/6 · Reconstructing system image / 重建系统镜像')
        reconstruct(official/'qcs8625_ota.zip',stock)
        progress('3/6 · Building components / 编译组件')
        if holder:
            assemble(components, stock, expected, tools=tools)
            profile,entries,release=generate_sources(components,stock,holder,out,**tools,
                project_public=None if sd else read_project_public(TRUST/'preview.pub'),modules=mods,automatic_sd=sd,program_sources=sources)
            links=profile['links']
        else:
            source=read_file(stock,TARGET)
            require(sha(source)==STOCK_MEDIA,'Stock media component digest mismatch.')
            report,patched=module_plan.render(source,TARGET,mods)
            target=out/'overlay'/TARGET.lstrip('/')
            target.parent.mkdir(parents=True)
            target.write_bytes(patched)
            entries={TARGET:dict(sha256=sha(patched),mode='0755',selinux='system_u:object_r:bin_t:s0')}
            links={}
            frozen=dict(profile='stock-example',official_sha256=OFFICIAL_SHA,automatic_sd=False,
                        third_party_modules=receipt(mods,report))
            save(out/'release-inputs.json',frozen)
            release=sha(canonical(frozen))
        progress('4/6 · Building and verifying filesystem / 构建并校验文件系统')
        image=build_image(stock,out/'overlay',entries,links,out/'system-patched.img',out/'audit',
                          startup_labels=profile.get('extension_startup_labels',{}) if holder else {})
        if holder:
            verify_maintenance_pins(lambda path: read_file(out/'system-patched.img',path))
        installer=build_guard(out,image,tools['go'],sd) if holder else None
        progress('5/6 · Packing firmware / 正在封装固件')
        packed=pack(official,out/'system-patched.img',out,release,installer=installer,install_allowed=sd,byte_progress=byte_progress,progress=progress)
        require(packed['install_allowed'] is sd, 'Unexpected install policy')
        result=dict(profile='unified' if holder else 'stock-example',official_sha256=OFFICIAL_SHA,
                    release=release,candidate=packed['file'],sha256=packed['sha256'],bytes=packed['bytes'],
                    installer_kind=packed['installer_kind'],install_allowed=sd,
                    modules=sorted(m['id'] for m in mods))
        progress('6/6 · Signing and verifying / 签名并验签' if sd else '6/6 · Verifying output / 校验输出')
        if sd:
            from .consumer import seal
            final = seal(out, packed, holder, tools['go'])
            result.update(candidate=final.name, sha256=digest(final), bytes=final.stat().st_size,
                          owner_authorization_embedded=True, canonical_sha256=packed['sha256'])
        save(out/'RESULT.json',result)
        if holder:
            shutil.copyfile(out/'installer/guard-config.json',out/'guard-config.json')
        files=[result['candidate'],'RESULT.json','release-inputs.json','package.json']
        if holder: files.append('guard-config.json')
        if (out/'menu-build.json').is_file(): files.append('menu-build.json')
        (out/'SHA256SUMS').write_text(''.join(f'{digest(out/n)}  {n}\n' for n in files))
        dest.mkdir(mode=0o700)
        moved=[]
        try:
            for name in [*files,'SHA256SUMS']:
                (out/name).rename(dest/name)
                moved.append(name)
        except BaseException:
            for name in moved: (dest/name).unlink()
            dest.rmdir()
            raise
        return result
