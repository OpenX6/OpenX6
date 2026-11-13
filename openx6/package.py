"""Locked research container output. Never executes an updater or signs a release."""
import hashlib,os,struct,zipfile
from pathlib import Path
from .common import require,digest,save
from .official import CHUNK,u32

LOCK=b'abort("OpenX6 OFFLINE CANDIDATE LOCKED: installation is not permitted");\n'
SCRIPTS=['META-INF/com/google/android/updater-script','META-INF/com/google/android/updater-post-install-script']

def pack(official,image,out,release,installer=None,install_allowed=False,byte_progress=None,progress=None):
    require(type(install_allowed) is bool and (not install_allowed or installer is not None),'Explicit guarded release required')
    official=Path(official);out=Path(out);zout=out/('patched-READY.zip' if install_allowed else 'patched-LOCKED.zip')
    # Python defaults to a conservative 2 GiB threshold. The original updater
    # uses ordinary ZIP32 and our uncompressed 3 GiB image still fits uint32.
    zipfile.ZIP64_LIMIT=(1<<32)-1
    n=Path(image).stat().st_size//4096
    replacements={s:LOCK for s in SCRIPTS} if installer is None else dict(installer[0]);replacements['system.transfer.list']=f'3\n{n}\n0\n0\nnew 2,0,{n}\n'.encode()
    members={}
    with zipfile.ZipFile(official/'qcs8625_ota.zip') as src,zipfile.ZipFile(zout,'x',allowZip64=False) as dst:
        for info in src.infolist():
            name=info.filename
            if name=='firmware-update/storsec.mbn':continue
            zi=zipfile.ZipInfo(name,info.date_time)
            for field in ['compress_type','external_attr','internal_attr','create_system','create_version','extract_version','comment']:setattr(zi,field,getattr(info,field))
            zi._compresslevel=6
            zi.file_size=Path(image).stat().st_size if name=='system.new.dat' else len(replacements[name]) if name in replacements else info.file_size
            h=hashlib.sha256()
            with dst.open(zi,'w') as f:
                if name in replacements:b=replacements[name];f.write(b);h.update(b)
                else:
                    stream=Path(image).open('rb') if name=='system.new.dat' else src.open(info)
                    with stream:
                        done = 0
                        last_percent = -1
                        while b := stream.read(CHUNK):
                            f.write(b)
                            h.update(b)
                            if byte_progress and name == 'system.new.dat':
                                done += len(b)
                                percent = 100 * done // zi.file_size
                                if percent != last_percent:
                                    byte_progress('5/6 · Compressing system image / 压缩系统镜像', done, zi.file_size)
                                    last_percent = percent
                        if byte_progress and progress and name == 'system.new.dat':
                            progress('5/6 · Completing and verifying package / 写入并校验封装')
            members[name]=dict(bytes=zi.file_size,sha256=h.hexdigest())
        if installer is not None:
            data=Path(installer[1]).read_bytes();zi=zipfile.ZipInfo('x6-install-guard',(2026,10,28,0,0,0));zi.compress_type=zipfile.ZIP_DEFLATED;zi.external_attr=(0o100755<<16);dst.writestr(zi,data);members['x6-install-guard']=dict(bytes=len(data),sha256=hashlib.sha256(data).hexdigest())
    require(members['system.new.dat']['sha256']==digest(image),'Packed system differs')
    head=bytearray((official/'header.bin').read_bytes()[:4864]);tail=bytearray((official/'tail.bin').read_bytes());module=bytearray((official/'module-header.bin').read_bytes())
    struct.pack_into('<II',head,92,3,0);module[264:328]=digest(zout).encode()
    module_length=576+zout.stat().st_size;cert=len(head)+module_length+64;check=cert+4160
    for i,fields in enumerate([(1,len(head),module_length),(4096,cert,4096),(4097,check,256)]):
        start=4096+i*256;head[start:start+256]=bytes(256);struct.pack_into('<III',head,start,*fields)
    tail[72:328]=bytes(256)
    # Publish ID only, no authorization or issuer signature. Must be signed later
    # by the holder for maintenance, after separate release/installation review.
    tail[768:808]=b'X6REL01\0'+bytes.fromhex(release)
    h=hashlib.sha256();dest=out/('OpenX6-1.2.36-S-UNSIGNED.mwfi.candidate' if install_allowed else 'OpenX6-1.2.36-S-LOCKED-UNSIGNED.mwfi.candidate')
    with dest.open('xb') as f:
        for b in [head,module]:f.write(b);h.update(b)
        with zout.open('rb') as z:
            while b:=z.read(CHUNK):f.write(b);h.update(b)
        f.write(bytes(64));h.update(bytes(64));tail[4232:4296]=h.hexdigest().encode();tail[4296:4360]=h.hexdigest().encode();f.write(tail);f.flush();os.fsync(f.fileno())
    require(dest.stat().st_size==check+320,'Container size differs')
    report=dict(file=dest.name,bytes=dest.stat().st_size,sha256=digest(dest),zip_file=zout.name,zip_sha256=digest(zout),members=members,
                prefix_sha256=h.hexdigest(),release=release,fwcert_offset=cert,fwchk_offset=check,
                signed=False,install_allowed=install_allowed,MCU_included=installer is not None,MCU_delivery=('pinned system file' if installer else 'none'),shared_storsec_included=False,recovery_scripts_abort_only=installer is None,installer_kind="abort-only" if installer is None else "guard-ready" if install_allowed else "guard-locked")
    save(out/'package.json',report);verify(out,allow_install=install_allowed);return report

def verify(out,allow_install=False):
    import json
    out=Path(out);r=json.loads((out/'package.json').read_text());candidate=out/r['file'];zpath=out/r['zip_file']
    require(digest(candidate)==r['sha256'] and digest(zpath)==r['zip_sha256'],'Candidate digest rejected')
    require(r['signed'] is False and (r['install_allowed'] is False or (allow_install is True and r['install_allowed'] is True)),'Explicit authorization required to validate unlocked output')
    with candidate.open('rb') as f:
        head=f.read(4864);require(head[:4]==b'MWFI' and u32(head,92)==3 and u32(head,96)==0,'Plaintext container header rejected')
        rows=[struct.unpack_from('<III',head,4096+i*256) for i in range(3)]
        cert=r['fwcert_offset'];check=r['fwchk_offset']
        require(rows==[(1,4864,576+zpath.stat().st_size),(4096,cert,4096),(4097,check,256)],'Module table rejected')
        require(cert==4864+rows[0][2]+64 and check==cert+4160,'Module ranges rejected')
        module=f.read(576);require(module[264:328].decode()==r['zip_sha256'],'Module digest rejected')
        h=hashlib.sha256();left=zpath.stat().st_size
        while left:
            b=f.read(min(left,CHUNK));require(bool(b),'Truncated module');h.update(b);left-=len(b)
        require(h.hexdigest()==r['zip_sha256'] and f.read(64)==bytes(64),'Module payload/boundary rejected')
        tail=f.read();require(len(tail)==4480 and tail[64:68]==b'CFSI' and tail[4224:4228]==b'CWFI','Container tail rejected')
        require(tail[72:328]==bytes(256) and tail[512:712]==bytes(200),'Unexpected signature in locked output')
        require(tail[768:808]==b'X6REL01\0'+bytes.fromhex(r['release']),'Release ID rejected')
        f.seek(0);h=hashlib.sha256();left=cert
        while left:b=f.read(min(left,CHUNK));require(bool(b),'Short prefix');left-=len(b);h.update(b)
        require(tail[4232:4360]==(h.hexdigest()*2).encode(),'Container checksum rejected')
    with zipfile.ZipFile(zpath) as z:
        require(set(z.namelist())==set(r['members']) and len(z.namelist())==len(r['members']),'ZIP file set rejected')
        require('firmware-update/storsec.mbn' not in z.namelist(),'Shared storsec not permitted')
        for name,spec in r['members'].items():
            h=hashlib.sha256();size=0
            with z.open(name) as f:
                while b:=f.read(CHUNK):h.update(b);size+=len(b)
            require(size==spec['bytes'] and h.hexdigest()==spec['sha256'],'ZIP member rejected: '+name)
        if r.get('installer_kind','abort-only')=='abort-only':
            for name in SCRIPTS:require(z.read(name)==LOCK,'Installation lock missing')
        else:
            require(r['installer_kind']==('guard-ready' if r['install_allowed'] else 'guard-locked') and r['recovery_scripts_abort_only'] is False,'Unknown installer policy')
            from .install import verify_installer
            scripts,guard=verify_installer(out,allow_install=allow_install)
            for name,b in scripts.items():require(z.read(name)==b,'Recovery script rejected')
            require(z.read('x6-install-guard')==guard.read_bytes(),'Guard rejected')
    return r
