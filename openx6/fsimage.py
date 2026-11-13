"""Edit regular ext4 image copies with debugfs; never mount or open block devices."""
import os,re,shutil,subprocess
from pathlib import Path
from .common import require,regular,digest,sha,save

def tool(name):
    p=shutil.which(name) or '/opt/homebrew/opt/e2fsprogs/sbin/'+name
    require(Path(p).is_file(),'Missing tool: '+name)
    return p

def quoted(p):
    s=str(p);require(not any(c in s for c in '\n\r"\\'),'Unsupported debugfs path')
    return '"'+s+'"'

def debug(image,command,missing=False):
    with regular(image):pass
    r=subprocess.run([tool('debugfs'),'-R',command,str(image)],capture_output=True)
    low=r.stderr.lower()
    if missing and b'file not found' in low:return None
    require(r.returncode==0 and not any(x in low for x in [b'not found',b'not a directory',b'usage:',b'error',b'could not',b'short read']), 'debugfs command rejected: '+command)
    return r.stdout

def read_file(image,path):
    st=debug(image,'stat '+path,missing=True)
    if st is None:return None
    require(b'Type: regular' in st,'Expected regular filesystem component: '+path)
    return debug(image,'cat '+path)

def build_image(stock,overlay,entries,links,dest,audit,*,startup_labels=None):
    startup_labels = startup_labels or {}
    require(not Path(dest).exists(),'Refuse existing output image')
    with regular(stock):pass
    shutil.copyfile(stock,dest)
    audit=Path(audit);audit.mkdir(exist_ok=True)
    commands=[];made=set()
    def label_file(label):
        require(re.fullmatch(r'system_u:object_r:[a-z0-9_]+:s0',label),'Invalid SELinux label')
        p=audit/('label-'+sha(label.encode())[:16]);p.write_bytes(label.encode()+b'\0');return p
    def directory(path):
        if path in ('/','.') or path in made:return
        directory(str(Path(path).parent));st=debug(stock,'stat '+path,missing=True)
        if st is None:
            label='system_u:object_r:'+('systemd_unit_file_t' if path.startswith('/lib/systemd') else 'usr_t')+':s0'
            label=startup_labels.get(path,label)
            commands.extend(['mkdir '+path,'sif '+path+' mode 040755','sif '+path+' uid 0','sif '+path+' gid 0','ea_set -f '+quoted(label_file(label))+' '+path+' security.selinux'])
        else:require(b'Type: directory' in st,'Non-directory ancestor')
        made.add(path)
    for name,spec in sorted(entries.items()):
        directory(str(Path(name).parent));st=debug(stock,'stat '+name,missing=True)
        if st is not None:
            require(b'Type: regular' in st,'Refuse non-file replacement');commands.append('rm '+name)
        source=Path(overlay)/name.lstrip('/')
        require(digest(source)==spec['sha256'],'Overlay changed')
        require(spec['mode'] in ('0600','0644','0700','0755'),'File mode rejected')
        commands.extend(['write '+quoted(source)+' '+name,'sif '+name+' mode 010'+spec['mode'],'sif '+name+' uid 0','sif '+name+' gid 0','ea_set -f '+quoted(label_file(spec['selinux']))+' '+name+' security.selinux'])
    for link,target in sorted(links.items()):
        require(any(link.startswith('/lib/systemd/system/'+phase+'.target.wants/') for phase in ('multi-user','ins_phase2','ins_phase3')) and re.fullmatch(r'\.\./[a-z0-9-]+\.service',target),'Unexpected service link')
        directory(str(Path(link).parent));require(debug(stock,'stat '+link,missing=True) is None,'Service link already exists')
        commands.append('symlink '+link+' '+target)
        if link in startup_labels:
            commands.append('ea_set -f '+quoted(label_file(startup_labels[link]))+' '+link+' security.selinux')
    cmd=audit/'debugfs.commands';cmd.write_text('\n'.join(commands)+'\n')
    r=subprocess.run([tool('debugfs'),'-w','-f',str(cmd),str(dest)],capture_output=True);(audit/'debugfs.log').write_bytes(r.stdout+r.stderr)
    require(r.returncode==0 and not any(x in r.stderr.lower() for x in [b'not found',b'could not',b'usage:',b'file exists',b'error',b'no space']), 'Image modification failed')
    # Official image has known bitmap-padding inconsistencies: repair only our copy.
    r=subprocess.run([tool('e2fsck'),'-fy',str(dest)],capture_output=True);(audit/'fsck-repair.log').write_bytes(r.stdout+r.stderr);require(r.returncode in (0,1),'Copy filesystem repair failed')
    r=subprocess.run([tool('e2fsck'),'-fn',str(dest)],capture_output=True);(audit/'fsck-final.log').write_bytes(r.stdout+r.stderr);require(r.returncode==0,'Final filesystem check failed')
    for name,spec in entries.items():
        require(sha(read_file(dest,name))==spec['sha256'],'Filesystem readback differs: '+name)
        st=debug(dest,'stat '+name).decode();require('Mode:  '+spec['mode'] in st and re.search(r'User:\s+0\s+Group:\s+0',st),'File permissions differ')
        lp=audit/'label-readback';debug(dest,'ea_get -f '+quoted(lp)+' '+name+' security.selinux')
        require(lp.read_bytes().rstrip(b'\0').decode()==spec['selinux'],'SELinux label differs');lp.unlink()
    for name,target in links.items():require(target in debug(dest,'stat '+name).decode(),'Startup link differs')
    for name,label in startup_labels.items():
        lp=audit/'startup-label-readback'; debug(dest,'ea_get -f '+quoted(lp)+' '+name+' security.selinux')
        require(lp.read_bytes()==label.encode()+b'\0','Startup label differs: '+name); lp.unlink()
    for name in ['/etc/passwd','/etc/shadow','/usr/bin/recovery','/usr/bin/updater','/etc/selinux/config','/etc/init.post_boot.sh']:
        require(read_file(stock,name)==read_file(dest,name),'Protected official component changed')
    return dict(bytes=Path(dest).stat().st_size,sha256=digest(dest),files=entries,links=links,fsck_exit=0)
