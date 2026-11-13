"""Windows offline SD builder for the verified 1.2.36 release."""
import base64
import hashlib
import json
import os
from pathlib import Path
import shutil
import stat
import struct
import subprocess
import sys
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent
SOURCE = HERE/'Source'
sys.path[:0] = [str(HERE),str(HERE/'Lib'),str(SOURCE)]
from dissect.extfs import ExtFS
from openx6.common import canonical, digest, require, save, sha
from openx6 import official, install
from openx6.package import pack
from openx6.build import verify_maintenance_pins
from openx6.owner import ED, PATHS
from identity import identity

RELEASE = '/usr/share/x6-release-id'
VERIFIER = '/usr/local/sbin/x6-owner-update-check'
DOMAIN = b'X6 OWNER MWFI V1\0'

def note(s):
    print(s,flush=True)

def run(command, **kwargs):
    result=subprocess.run(command,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,creationflags=subprocess.CREATE_NO_WINDOW,**kwargs)
    require(result.returncode==0, Path(command[0]).name+' failed: '+result.stdout.decode('utf-8','replace')[-1200:])

def write_slots(image, slots, replacements):
    require(slots.keys()==replacements.keys(), 'Incomplete personalization')
    used=set()
    with image.open('r+b') as stream:
        fs=ExtFS(stream)
        require(fs.block_size==4096, 'Unexpected filesystem block size')
        for path, data in replacements.items():
            spec=slots[path]
            node=fs.get(path)
            require(stat.S_ISREG(node.inode.i_mode) and node.size==len(data)==spec['size']==spec['length'] and spec['offset']==0, 'Size/layout differs: '+path)
            blocks=[]
            for base,count in node.dataruns():
                require(base is not None and base>0,'Unexpected sparse binding')
                blocks.extend(range(base,base+count))
            blocks=blocks[:(node.size+4095)//4096]
            require(blocks==spec['blocks'] and not used.intersection(blocks),'Filesystem allocation differs')
            used.update(blocks)
            require(node.open().read()==bytes([0xa5])*len(data), 'Neutral slot content differs')
            for i,block in enumerate(blocks):
                stream.seek(block*4096)
                stream.write(data[i*4096:(i+1)*4096])
        stream.flush()
        os.fsync(stream.fileno())

def inspect_image(image, plan, replacements, overlay):
    profile=json.loads((SOURCE/'components/profile.json').read_bytes())
    needed={*PATHS,'/usr/share/x6-firstboot-seed/manifest.json','/usr/share/x6-holder-layout','/usr/local/sbin/x6-owner-maintenance','/usr/bin/ins_otad'}
    with image.open('rb') as stream:
        fs=ExtFS(stream)
        def read(path): return fs.get(path).open().read()
        for path,spec in plan['entries'].items():
            node=fs.get(path)
            data=node.open().read()
            require(sha(data)==(sha(replacements[path]) if path in replacements else spec['sha256']), 'Filesystem readback differs: '+path)
            require(node.inode.i_mode==stat.S_IFREG|int(spec['mode'],8) and node.inode.i_uid==node.inode.i_gid==0,'Mode or owner differs: '+path)
            attrs={x.name:x.value for x in node.xattr}
            require(attrs.get('security.selinux')==spec['selinux'].encode()+b'\0','SELinux label differs: '+path)
            if path in needed:
                f=overlay/path.lstrip('/')
                f.parent.mkdir(parents=True,exist_ok=True)
                f.write_bytes(data)
        for path,target in profile['links'].items(): require(fs.get(path).link==target,'Unexpected startup link')
        for path,label in profile.get('extension_startup_labels',{}).items():
            require({x.name:x.value for x in fs.get(path).xattr}.get('security.selinux')==label.encode()+b'\0','Startup label differs')
        verify_maintenance_pins(read)

def cross_compile(out, plan, release):
    root=out/'go'
    with zipfile.ZipFile(HERE/'go-windows.zip') as archive:
        for info in archive.infolist():
            if info.filename.startswith(('go/src/','go/pkg/','go/bin/','go/lib/')) or info.filename in ('go/go.env','go/VERSION'):
                archive.extract(info,out)
    go=root/'bin/go.exe'
    env=dict(os.environ,GOROOT=str(root),GOENV='off',GOOS='linux',GOARCH='arm64',GOARM64='v8.0',CGO_ENABLED='0',GOFLAGS='',GOWORK='off',GOTOOLCHAIN='local',GOPROXY='off',GOSUMDB='off',GOCACHE=str(out/'go-cache'),GO111MODULE='on')
    project=out/'components'
    project.mkdir()
    source=SOURCE/'components/src'
    for directory in ('verifier','holder','vendor'):
        shutil.copytree(source/directory,project/directory)
    for name in ('go.mod','go.sum'): shutil.copyfile(source/name,project/name)
    save(project/'verifier/coordinator-policy.json',dict(plan['coordinator'],Release=release))
    exe=out/'update-check'
    run([go,'build','-mod=vendor','-trimpath','-ldflags=-s -w -buildid=','-o',exe,'./verifier'],cwd=project,env=env)
    data=exe.read_bytes()
    install.elf_check(data)
    require((project/'verifier/coordinator-policy.json').read_bytes() in data,'Compiled update policy missing')
    return go,env,data

def make_guard(out, image, go, env):
    members={}
    with zipfile.ZipFile(out/'official/qcs8625_ota.zip') as z:
        for item in z.infolist():
            with z.open(item) as stream:
                members[item.filename]=dict(bytes=item.file_size,sha256=hashlib.file_digest(stream,'sha256').hexdigest())
    members['system.new.dat']=dict(bytes=image.stat().st_size,sha256=digest(image))
    config,original=install.configuration(out,dict(members=members),install_allowed=True)
    directory=out/'installer'
    shutil.copytree(SOURCE/'install-guard',directory)
    save(directory/'guard-config.json',config)
    (directory/'official-updater-script').write_bytes(original)
    guard=directory/'x6-install-guard'
    run([go,'build','-trimpath','-ldflags=-s -w -buildid=','-o',guard,'.'],cwd=directory,env=dict(env,GO111MODULE='off'))
    h=digest(image,'sha1')
    blocks=image.stat().st_size//4096
    for name,body in install.scripts(original,h,blocks).items(): (directory/Path(name).name).write_bytes(body)
    save(directory/'build.json',dict(config_sha256=digest(directory/'guard-config.json'),sources={f.name:digest(f) for f in directory.glob('*.go')},binary_sha256=digest(guard),system_sha1=h,system_blocks=blocks,install_allowed=True))
    shutil.copyfile(directory/'guard-config.json',out/'guard-config.json')
    return install.verify_installer(out,allow_install=True)

def seal(out, packed, key, roles):
    require(packed['install_allowed'] and packed['installer_kind']=='guard-ready','Guarded SD build required')
    path=out/packed['file']
    off=packed['fwcert_offset']+512
    canonical_hash=bytes.fromhex(packed['sha256'])
    require(digest(path)==packed['sha256'] and roles[ED['update']]==key.public_key().public_bytes_raw(),'Reviewed package or signing key changed')
    with path.open('r+b') as stream:
        header=stream.read(4864)
        project=header[8:24].split(b'\0')[0]
        version=header[24:88].split(b'\0')[0]
        require(project==b'c9' and version==b'1.2.36','Unexpected package target/version')
        message=struct.pack('<8sIIQ32s16s32s32s',b'X6UPD01\0',1,0,packed['bytes'],canonical_hash,project,version,b'1.2.36')
        record=message+key.sign(DOMAIN+message)
        stream.seek(off)
        require(stream.read(200)==bytes(200),'Certificate slot already used')
        stream.seek(off)
        stream.write(record)
        stream.flush()
        os.fsync(stream.fileno())
    h=hashlib.sha256()
    with path.open('rb') as stream:
        left=off
        while left:
            b=stream.read(min(left,4<<20))
            require(bool(b),'Truncated signed package')
            h.update(b)
            left-=len(b)
        observed=stream.read(200)
        require(observed==record,'Signed record changed')
        key.public_key().verify(observed[136:],DOMAIN+observed[:136])
        h.update(bytes(200))
        while b:=stream.read(4<<20): h.update(b)
    require(h.digest()==canonical_hash,'Signed payload differs from reviewed candidate')
    final=out/'Insta360X6FW.bin'
    path.rename(final)
    return final

def build(firmware, modules, parent):
    dest=parent/'OpenX6 SD Build'
    require(parent.is_dir() and not dest.exists(),'Choose an output folder without an existing OpenX6 SD Build.')
    require(shutil.disk_usage(parent).free>=20*1024**3,'20 GiB free space required on output volume.')
    plan=json.loads((HERE/'plan.json').read_bytes())
    with tempfile.TemporaryDirectory(dir=parent,prefix='.ox6-') as temp:
        out=Path(temp)
        note('1/6 · 正在校验官方固件和三个模块')
        official.import_modules(firmware,modules,out/'official')
        note('2/6 · 正在从官方模块重建系统镜像')
        stock=out/'stock.img'
        official.reconstruct(out/'official/qcs8625_ota.zip',stock)
        with stock.open('ab') as f:
            for name in ('MCUFw-V1.2.0.0.bin','cs_press-V84.11.1.8.nfw'): f.write((out/'official'/name).read_bytes())
        image=out/'system.img'
        run([HERE/'hpatchz.exe',stock,HERE/'system.hdiff',image])
        require(digest(image)==plan['neutral_sha256'],'Fixed release image differs from reference')
        stock.unlink()
        note('3/6 · 正在使用本机 Holder 编译相机更新验证器')
        roles,key=identity()
        frozen=dict(plan['release_inputs'],owner_public_sha256={p:sha(v) for p,v in roles.items()},native_build=dict(mode='windows-offline-v1',plan_sha256=digest(HERE/'plan.json'),worker_sha256=digest(__file__),go_archive_sha256=digest(HERE/'go-windows.zip')))
        release=sha(canonical(frozen))
        save(out/'release-inputs.json',frozen)
        go,env,verifier=cross_compile(out,plan,release)
        replacement={**roles,RELEASE:(release+'\n').encode(),VERIFIER:verifier}
        write_slots(image,plan['slots'],replacement)
        note('4/6 · 正在逐文件读回镜像并编译安装保护程序')
        inspect_image(image,plan,replacement,out/'overlay')
        installer=make_guard(out,image,go,env)
        note('5/6 · 正在封装并读回固件')
        packed=pack(out/'official',image,out,release,installer=installer,install_allowed=True,byte_progress=lambda label,n,t: note(f'{label} {100*n//t}%'),progress=note)
        note('6/6 · 正在签名并验证最终固件')
        final=seal(out,packed,key,roles)
        result=dict(profile='unified',official_sha256=official.OFFICIAL_SHA,release=release,candidate=final.name,sha256=digest(final),bytes=final.stat().st_size,installer_kind='guard-ready',install_allowed=True,modules=[],owner_authorization_embedded=True,canonical_sha256=packed['sha256'])
        save(out/'RESULT.json',result)
        ready=out/'published'
        ready.mkdir()
        for name in (final.name,'RESULT.json','guard-config.json','package.json','release-inputs.json'): (out/name).rename(ready/name)
        (ready/'SHA256SUMS').write_bytes(''.join(digest(p)+'  '+p.name+'\n' for p in sorted(ready.iterdir())).encode())
        ready.rename(dest)
    note('OpenX6 verified '+result['sha256'])

if __name__=='__main__':
    try: build(*(Path(os.environ[k]).resolve() for k in ('OPENX6_FW','OPENX6_MODULES','OPENX6_OUTPUT')))
    except Exception as e:
        print(str(e),file=sys.stderr,flush=True)
        raise SystemExit(1)
