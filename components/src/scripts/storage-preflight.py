#!/usr/bin/python3
"""Bounded pre-import cleanup. Default is report only; no updater/permit changes."""
import argparse,ctypes,errno,json,os,platform,re,stat,subprocess,time
from pathlib import Path

STREAMS=('kernel','syslog','logcat','event_track','adsp','journal')
class Refused(Exception):pass

def exclusive_lock(fd):
 # The camera's minimal Python has ctypes but no fcntl extension.
 # Keep a nonblocking kernel flock for the lifetime of the open descriptor.
 libc=ctypes.CDLL(None,use_errno=True)
 fn=libc.flock;fn.argtypes=[ctypes.c_int,ctypes.c_int];fn.restype=ctypes.c_int
 if fn(fd,2|4)!=0:
  code=ctypes.get_errno();raise OSError(code,os.strerror(code))

def same_mount_fds(a,b):
 # Overlayfs may expose a different st_dev for regular files and directories.
 # Compare the kernel mount identity of pinned descriptors instead; a foreign
 # bind-mounted file is rejected even if it shares an underlying filesystem.
 if platform.system()!='Linux':return os.fstat(a).st_dev==os.fstat(b).st_dev
 def mount_id(fd):
  ids=[line.split(':',1)[1].strip() for line in Path('/proc/self/fdinfo/'+str(fd)).read_text().splitlines() if line.startswith('mnt_id:')]
  if len(ids)!=1 or not ids[0].isdigit():raise Refused('cannot identify cleanup file mount')
  return int(ids[0])
 return mount_id(a)==mount_id(b)

def fingerprint(s):
 return (s.st_dev,s.st_ino,s.st_mode,s.st_uid,s.st_nlink,s.st_size,s.st_mtime_ns,s.st_ctime_ns)

def references():
 """Fail closed on inaccessible live processes. Include mapped but closed files."""
 used=set()
 for pid in os.listdir('/proc'):
  if not pid.isdigit():continue
  base='/proc/'+pid
  try:
   for fd in os.listdir(base+'/fd'):
    try:s=os.stat(base+'/fd/'+fd);used.add((s.st_dev,s.st_ino))
    except FileNotFoundError:continue
   with open(base+'/maps') as f:
    for line in f:
     fields=line.split(None,5)
     if len(fields)<5:raise Refused('unparseable process maps')
     ino=int(fields[4]);a,b=fields[3].split(':')
     if ino:used.add((os.makedev(int(a,16),int(b,16)),ino))
  except FileNotFoundError:continue
  except (OSError,ValueError) as e:raise Refused('cannot check live references: '+str(e))
 return used

def idle_guard():
 def run(args):
  p=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=8,text=True)
  if p.returncode:raise Refused('readiness query failed')
  return p.stdout
 for path in ('/data/x6-owner-update/install.permit','/run/x6-owner-update/install.permit'):
  if os.path.lexists(path):raise Refused('installation permit present')
 if run(['/bin/systemctl','show','ins_otad.service','-p','MainPID','--value']).strip()!='0':raise Refused('OTA active')
 v=json.loads(run(['/usr/bin/ins_bll_cli','{"cmd":"share_state","action":"get","type":"media"}']))
 d=v.get('data',{})
 if v.get('ret')!='ok' or d.get('record',{}).get('record_status')!='RecordIdle' or d.get('capture',{}).get('capture_status')!='CaptureIdle':raise Refused('camera busy')

class Cleaner:
 def __init__(self,root='/',uid=0,now=None,refs=references,guard=idle_guard,free=None,vendor_uid=None):
  self.root=Path(root);self.uid=uid;self.vendor_uid=vendor_uid;self.now=time.time() if now is None else now;self.refs=refs;self.guard=guard;self.free=free or self.disk_free
 def path(self,p):return self.root/p.lstrip('/')
 def disk_free(self):
  s=os.statvfs(self.path('/data'));return s.f_bavail*s.f_frsize
 def directory(self,p):
  # Pin every parent; do not follow a symlink or traverse writable foreign paths.
  fd=os.open(str(self.root),os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
  try:
   walked=''
   for name in p.strip('/').split('/'):
    new=os.open(name,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=fd);os.close(fd);fd=new
    s=os.fstat(fd)
    walked+='/'+name
    vendor=walked in ['/data/ins','/data/ins/logs','/data/ins/minidump']+['/data/ins/logs/'+x for x in STREAMS]
    if s.st_uid not in ((self.uid,self.vendor_uid) if vendor else (self.uid,)) or (s.st_mode&0o022 and not vendor):raise Refused('unsafe cleanup directory')
   if os.fstat(fd).st_dev!=os.stat(self.path('/data'),follow_symlinks=False).st_dev:raise Refused('cleanup mount differs from data')
   return fd
  except BaseException:os.close(fd);raise
 def candidates(self):
  used=self.refs();out=[];scanned=0
  dirs=[('/data/ins/logs/'+x,x,600,2) for x in STREAMS]+[('/data/ins/minidump',None,86400,0)]
  for folder,stream,age,keep in dirs:
   try:fd=self.directory(folder)
   except FileNotFoundError:continue
   try:
    files=[]
    pattern=(re.escape(stream)+r'_\d+_\d+_\d{8}T\d{6}\.log(?:\.(?:gz|zst))?') if stream else r'rawdump_\d+\.bin'
    for name in os.listdir(fd):
     scanned+=1
     if scanned>8192:raise Refused('cleanup scan limit exceeded')
     if not re.fullmatch(pattern,name):continue
     try:s=os.stat(name,dir_fd=fd,follow_symlinks=False)
     except FileNotFoundError:continue
     if not stat.S_ISREG(s.st_mode) or s.st_uid not in (self.uid,self.vendor_uid) or s.st_nlink!=1 or s.st_mode&0o022:continue
     try:entry=os.open(name,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=fd)
     except FileNotFoundError:continue
     try:
      if fingerprint(os.fstat(entry))!=fingerprint(s) or not same_mount_fds(entry,fd):continue
     finally:os.close(entry)
     files.append((name,s))
    files.sort(key=lambda v:(v[1].st_mtime_ns,v[0]),reverse=True)
    for name,s in files[keep:]:
     if self.now-s.st_mtime<age or (s.st_dev,s.st_ino) in used:continue
     out.append({'directory':folder,'name':name,'fingerprint':fingerprint(s),'bytes':s.st_size,'allocated_bytes':s.st_blocks*512,'mtime':s.st_mtime})
   finally:os.close(fd)
  return sorted(out,key=lambda x:(x['mtime'],x['directory'],x['name']))
 def quarantine_root(self):
  # This path is directly below root-owned /data, never under writable /data/ins.
  data=self.directory('/data')
  try:
   try:os.mkdir('x6-log-quarantine',0o700,dir_fd=data);os.fsync(data)
   except FileExistsError:pass
  finally:os.close(data)
  fd=self.directory('/data/x6-log-quarantine')
  if stat.S_IMODE(os.fstat(fd).st_mode)!=0o700:os.close(fd);raise Refused('unsafe quarantine mode')
  return fd
 def validate_receipt(self,item):
  folder=item.get('directory');name=item.get('name','');fp=item.get('fingerprint',[])
  if folder=='/data/ins/minidump':pattern=r'rawdump_\d+\.bin';age=86400
  elif folder in ['/data/ins/logs/'+x for x in STREAMS]:pattern=re.escape(folder.rsplit('/',1)[1])+r'_\d+_\d+_\d{8}T\d{6}\.log(?:\.(?:gz|zst))?';age=600
  else:raise Refused('quarantine receipt outside allowlist')
  if not re.fullmatch(pattern,name) or len(fp)!=8 or self.now-fp[6]/1e9<age:raise Refused('invalid quarantine receipt')
 def drain_quarantine(self,fd):
  deleted=[]
  for name in os.listdir(fd):
   if not re.fullmatch(r'q-[0-9a-f]{32}',name):raise Refused('foreign quarantine entry')
   sub=os.open(name,os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=fd)
   try:
    st=os.fstat(sub)
    if st.st_uid!=self.uid or stat.S_IMODE(st.st_mode)!=0o700:raise Refused('unsafe quarantine transaction')
    entries=set(os.listdir(sub))
    if not entries:
     os.rmdir(name,dir_fd=fd);os.fsync(fd);continue
    if not entries<= {'receipt.json','payload'} or 'receipt.json' not in entries:raise Refused('incomplete/foreign quarantine transaction')
    rfd=os.open('receipt.json',os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=sub)
    try:
     st=os.fstat(rfd)
     if not stat.S_ISREG(st.st_mode) or st.st_uid!=self.uid or st.st_nlink!=1 or stat.S_IMODE(st.st_mode)!=0o600 or st.st_size>8192:raise Refused('unsafe quarantine receipt')
     raw=os.read(rfd,8193)
    finally:os.close(rfd)
    if 'payload' not in entries:
     # Crash before moving a log, or after deleting it: only our metadata remains.
     os.unlink('receipt.json',dir_fd=sub);os.fsync(sub);os.rmdir(name,dir_fd=fd);os.fsync(fd);continue
    item=json.loads(raw)
    self.validate_receipt(item)
    if 'payload' in entries:
     st=os.stat('payload',dir_fd=sub,follow_symlinks=False)
     # rename changes ctime; all identity/content/owner/link/mode fields must match.
     if fingerprint(st)[:-1]!=tuple(item['fingerprint'])[:-1]:raise Refused('quarantined file changed; preserved for inspection')
     self.guard()
     if (st.st_dev,st.st_ino) in self.refs():raise Refused('quarantined file still referenced; preserved')
     if fingerprint(os.stat('payload',dir_fd=sub,follow_symlinks=False))!=fingerprint(st):raise Refused('quarantine file changed during reference scan')
     os.unlink('payload',dir_fd=sub);os.fsync(sub);deleted.append(item)
    os.unlink('receipt.json',dir_fd=sub);os.fsync(sub)
   finally:os.close(sub)
   os.rmdir(name,dir_fd=fd);os.fsync(fd)
  return deleted
 def quarantine(self,source,item):
  parent=self.quarantine_root();sub=None
  try:
   # Retry recovery first; an unexpected/racing file blocks further deletion.
   recovered=self.drain_quarantine(parent)
   name='q-'+os.urandom(16).hex();os.mkdir(name,0o700,dir_fd=parent);os.fsync(parent)
   sub=os.open(name,os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=parent)
   fd=os.open('receipt.json',os.O_CREAT|os.O_EXCL|os.O_WRONLY|os.O_NOFOLLOW,0o600,dir_fd=sub)
   try:
    raw=json.dumps(item).encode();written=os.write(fd,raw)
    if written!=len(raw):raise Refused('short receipt write')
    os.fsync(fd)
   finally:os.close(fd)
   os.fsync(sub)
   # Atomically take the entry away from the publicly writable log directory.
   # Never unlink it there based on a stale lstat. A replacement fails post-move
   # validation and remains private; no foreign file is automatically deleted.
   os.rename(item['name'],'payload',src_dir_fd=source,dst_dir_fd=sub);os.fsync(source);os.fsync(sub)
   os.close(sub);sub=None
   return recovered+self.drain_quarantine(parent)
  finally:
   if sub is not None:os.close(sub)
   os.close(parent)
 def clean(self,required,apply=False,before_delete=None):
  if required<0 or required>8<<30:raise Refused('invalid space budget')
  before=self.free();deleted=[];skipped=[]
  if apply and self.path('/data/x6-log-quarantine').exists():
   q=self.quarantine_root()
   try:deleted.extend(self.drain_quarantine(q))
   finally:os.close(q)
  items=self.candidates()
  # Quarantine and receipt require a few free metadata blocks. A truly full
  # filesystem fails closed; never claim universal ENOSPC recovery.
  if apply and before<required:
   self.guard()
   for item in items:
    if self.free()>=required:break
    self.guard()
    fd=self.directory(item['directory'])
    try:
     if before_delete:before_delete(item)
     s=os.stat(item['name'],dir_fd=fd,follow_symlinks=False)
     if fingerprint(s)!=tuple(item['fingerprint']) or (s.st_dev,s.st_ino) in self.refs():skipped.append(item['name']);continue
     # Recheck metadata after the process scan. Unlink only this pinned directory.
     if fingerprint(os.stat(item['name'],dir_fd=fd,follow_symlinks=False))!=fingerprint(s):skipped.append(item['name']);continue
     deleted.extend(self.quarantine(fd,item))
    except FileNotFoundError:skipped.append(item['name'])
    finally:os.close(fd)
  after=self.free()
  return {'schema':1,'apply':apply,'required_bytes':required,'free_before':before,'free_after':after,'shortfall_bytes':max(0,required-after),'ready':after>=required,'eligible_allocated_bytes':sum(x['allocated_bytes'] for x in items),'deleted_count':len(deleted),'deleted_logical_bytes':sum(x['bytes'] for x in deleted),'deleted':[(x['directory']+'/'+x['name']) for x in deleted][:32],'skipped_changed_or_open':skipped[:32],'candidate_count':len(items),'candidates':[x['directory']+'/'+x['name'] for x in items][:32],'scope':'closed old allowlisted logs and rawdump only; no recursive cleanup or media/settings/keys/feature deletion'}

def main():
 p=argparse.ArgumentParser(description=__doc__);p.add_argument('--required-bytes',required=True,type=int);p.add_argument('--apply',action='store_true');a=p.parse_args()
 if os.geteuid()!=0 or platform.system()!='Linux' or platform.machine() not in ('aarch64','arm64'):raise Refused('target Linux ARM64 root required')
 # Root-controlled /run file provides exclusion without spending /data space.
 root=os.lstat('/run')
 if not stat.S_ISDIR(root.st_mode) or root.st_uid or root.st_mode&0o022:raise Refused('unsafe run directory')
 fd=os.open('/run/x6-storage-cleanup.lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW|os.O_NONBLOCK,0o600)
 try:
  s=os.fstat(fd)
  if not stat.S_ISREG(s.st_mode) or s.st_uid or s.st_nlink!=1 or stat.S_IMODE(s.st_mode)!=0o600:raise Refused('unsafe cleanup lock')
  exclusive_lock(fd)
  vendor_uid=None
  passwd=os.lstat('/etc/passwd')
  if not stat.S_ISREG(passwd.st_mode) or passwd.st_uid or passwd.st_mode&0o022:raise Refused('unsafe passwd database')
  for line in Path('/etc/passwd').read_text().splitlines():
   fields=line.split(':')
   if fields[0]=='www':
    if vendor_uid is not None:raise Refused('ambiguous vendor UID')
    vendor_uid=int(fields[2])
  result=Cleaner(vendor_uid=vendor_uid).clean(a.required_bytes,a.apply);print(json.dumps(result));return 0 if result['ready'] else 2
 finally:os.close(fd)
if __name__=='__main__':
 try:raise SystemExit(main())
 except (Refused,OSError,ValueError,subprocess.TimeoutExpired) as e:
  print(json.dumps({'ready':False,'error':str(e),'cleanup_may_be_partial':True}));raise SystemExit(3)
