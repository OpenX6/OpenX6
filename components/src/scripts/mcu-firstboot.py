#!/usr/bin/python3
"""Native MCU application update, once per destination; never force boot or slots.
Ambiguous interruption is deliberately left for diagnosis, not retried blindly.
The Linux firmware installer and its owner-authenticated envelope are unchanged.
"""
import hashlib,json,os,re,stat,subprocess,tempfile,time,ctypes
from pathlib import Path
IMAGE='/usr/share/openx6/mcu/MCUFw-V1.2.0.0.bin'
SHA='9c1f0427e8562b25d64b360c530f1266d10d6cf0f40dcf3ff2dcee33c447991b'
SIZE=130048
OLD={'v0.8.4','v1.0.0'}

class Deferred(RuntimeError):pass

def exclusive_lock(fd):
 # The target Python omits fcntl; use the same kernel flock via libc.
 libc=ctypes.CDLL(None,use_errno=True)
 fn=libc.flock;fn.argtypes=[ctypes.c_int,ctypes.c_int];fn.restype=ctypes.c_int
 if fn(fd,2|4)!=0:
  code=ctypes.get_errno();raise OSError(code,os.strerror(code))

def update(ops):
 version=ops.version()
 if version=='v1.2.0':
  ops.save('complete');return 'already-current'
 if version not in OLD:raise Deferred('unknown MCU version; no downgrade or write')
 if ops.phase():raise Deferred('prior MCU transaction requires diagnosis; no automatic retry')
 ops.preflight()
 ops.verify_image()
 # Commit before crossing the native flash boundary. A failed/ambiguous call
 # cannot be replayed by a service restart or by reinstalling the same package.
 ops.save('writing')
 ops.native('DeviceUpgradeMcu',{'id':'DevIdMainMcu','path':IMAGE,'reboot_type':'DevRebootTypeInt'},120000)
 ops.save('written')
 ops.save('activating')
 ops.native('DeviceUpgradeReboot','DevIdMainMcu',10000)
 ops.save('activated')
 # HAL caches the old version until the next normal system boot. Do not
 # interpret it as an update failure, reflash, or repeatedly reset the MCU.
 if not ops.idle():return 'activated; waiting for a normal idle reboot'
 ops.save('reboot-requested')
 ops.reboot()
 return 'awaiting-next-boot-verification'

def safe_read(p,uid=0,limit=4096):
 fd=os.open(str(p),os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK)
 try:
  s=os.fstat(fd)
  if not stat.S_ISREG(s.st_mode) or s.st_uid!=uid or s.st_nlink!=1 or s.st_mode&0o022 or s.st_size>limit:raise Deferred('unsafe input')
  b=os.read(fd,limit+1)
  if len(b)!=s.st_size:raise Deferred('input changed')
  return b
 finally:os.close(fd)

def atomic(p,b):
 fd,tmp=tempfile.mkstemp(prefix='.mcu-',dir=p.parent)
 try:
  if os.write(fd,b)!=len(b):raise OSError('short write')
  os.fsync(fd);os.close(fd);fd=-1;os.replace(tmp,p)
  d=os.open(str(p.parent),os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
  try:os.fsync(d)
  finally:os.close(d)
 finally:
  if fd>=0:os.close(fd)
  if os.path.exists(tmp):os.unlink(tmp)

def successful(text):
 return bool(re.search(r'^\s*Error code: Success\(0\) \(0\)\s*$',text,re.M))

class Native:
 def __init__(self):
  self.directory=Path('/data/x6-mcu-state')
  parent=self.directory.parent.lstat()
  if not stat.S_ISDIR(parent.st_mode) or parent.st_uid or parent.st_mode&0o022:raise Deferred('unsafe data directory')
  self.directory.mkdir(mode=0o700,exist_ok=True)
  s=self.directory.lstat()
  if not stat.S_ISDIR(s.st_mode) or s.st_uid or stat.S_IMODE(s.st_mode)!=0o700:raise Deferred('unsafe MCU directory')
  self.journal=self.directory/'state.json'
  self.boot=Path('/proc/sys/kernel/random/boot_id').read_text().strip()
  self.fd=os.open(str(self.directory/'lock'),os.O_RDWR|os.O_CREAT|os.O_NOFOLLOW|os.O_NONBLOCK,0o600)
  s=os.fstat(self.fd)
  if not stat.S_ISREG(s.st_mode) or s.st_uid or s.st_nlink!=1 or stat.S_IMODE(s.st_mode)!=0o600:raise Deferred('unsafe MCU lock')
  exclusive_lock(self.fd)
 def run(self,args,seconds=8):
  p=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=seconds,text=True)
  if p.returncode:raise Deferred('native command failed: '+args[0])
  return p.stdout
 def rpc(self,interface,method,data,milliseconds):
  out=self.run(['/usr/bin/ins_ibus_cli','--run','call-method','kCamera','kHald',interface,method,(json.dumps(data,separators=(',',':')) if isinstance(data,dict) else data),str(milliseconds)],milliseconds/1000+10)
  if not successful(out) or not re.search(r'^\s*id: DevIdMainMcu(?:\(1\))?\s*$',out,re.M):raise Deferred('native RPC refused or failed: '+method)
  return out
 def version(self):
  out=self.rpc('DevInfoApi','DevInfoApiGetDevInfo','DevIdMainMcu',3000)
  values=re.findall(r'^\s*sw_version:\s*(v[0-9]+\.[0-9]+\.[0-9]+)\s*$',out,re.M)
  if len(values)!=1:raise Deferred('ambiguous MCU version')
  return values[0]
 def phase(self):
  try:d=json.loads(safe_read(self.journal))
  except FileNotFoundError:return None
  if set(d)!={'schema','phase','image_sha256','boot'} or d['schema']!=1 or d['image_sha256']!=SHA or d['phase'] not in {'writing','written','activating','activated','reboot-requested','complete'}:raise Deferred('invalid MCU journal')
  return d['phase']
 def save(self,phase):
  # Validate an existing journal even when MCU already reports the target.
  self.phase()
  atomic(self.journal,(json.dumps(dict(schema=1,phase=phase,image_sha256=SHA,boot=self.boot),sort_keys=True)+'\n').encode())
 def preflight(self):
  # Main-system update must already have ended. No forced OTA flags or permits.
  for u in ['x6-owner-maintenance.service','x6-owner-gui-auth.service','ins_gui.service','ins_hald.service']:
   text=self.run(['/bin/systemctl','show',u,'-p','ActiveState','-p','MainPID'])
   d=dict(x.split('=',1) for x in text.splitlines() if '=' in x)
   if d.get('ActiveState')!='active' or not d.get('MainPID','').isdigit() or int(d['MainPID'])<=0:raise Deferred('startup/access service not ready')
   status=Path('/proc')/d['MainPID']/'status'
   if not re.search(r'^Uid:\s+0\s+0\s+0\s+0\s*$',status.read_text(),re.M):raise Deferred('service is not root')
  if self.run(['/bin/systemctl','show','ins_otad.service','-p','MainPID','--value']).strip()!='0':raise Deferred('Linux OTA active')
  for p in ['/data/x6-owner-update/install.permit','/run/x6-owner-update/install.permit']:
   if os.path.lexists(p):raise Deferred('installation permit present')
  # Owner decision 2026-11-02: no power gate of our own. If the whole firmware
  # could be installed, the MCU application can be written too; the remaining
  # checks only keep this write from colliding with an OTA, a recording or an
  # unfinished first boot.
  release=safe_read(Path('/usr/share/x6-firstboot-seed/manifest.json'),limit=1024*1024)
  # Existing bootstrap checks the full manifest; require its completion receipt.
  manifest=json.loads(release)
  receipt=safe_read(Path('/data/x6-owner-features/complete.json'),limit=128).decode().strip()
  if receipt!=hashlib.sha256(release).hexdigest():raise Deferred('firstboot migration not complete')
  for _ in range(3):
   if not self.idle():raise Deferred('camera is busy')
   time.sleep(1)
 def idle(self):
  out=json.loads(self.run(['/usr/bin/ins_bll_cli',json.dumps({'cmd':'share_state','action':'get','type':'media'})]))
  d=out.get('data',{})
  return out.get('ret')=='ok' and d.get('record',{}).get('record_status')=='RecordIdle' and d.get('capture',{}).get('capture_status')=='CaptureIdle'
 def verify_image(self):
  b=safe_read(Path(IMAGE),limit=SIZE)
  if len(b)!=SIZE or hashlib.sha256(b).hexdigest()!=SHA:raise Deferred('MCU image rejected')
 def native(self,method,data,milliseconds):
  out=self.rpc('DeviceUpgrade',method,data,milliseconds)
  # RPC transport success is not the MCU operation's result. Do not activate
  # a failed write merely because the server returned a well-formed response.
  if not re.search(r'^\s*Return value:\s+0 \(0x0\)\s*$',out,re.M):raise Deferred('native MCU operation did not return zero: '+method)
  return out
 def reboot(self):
  self.run(['/bin/systemctl','reboot'],10)

def bounded_startup(ops,tries=18,pause=time.sleep):
 for i in range(tries):
  try:return update(ops)
  except (Deferred,OSError,ValueError,subprocess.TimeoutExpired):
   # Readiness may lag systemd's active state. Only retry before the durable
   # write intent, never a possibly dispatched flash/activation operation.
   if ops.phase() is not None or i+1==tries:raise
   pause(5)

def main():
 if os.geteuid()!=0:raise SystemExit('root required')
 ops=Native()
 try:
  result=bounded_startup(ops);atomic(ops.directory/'status.json',(json.dumps({'result':result,'boot':ops.boot})+'\n').encode())
 except (Deferred,OSError,ValueError,subprocess.TimeoutExpired) as e:
  try:atomic(ops.directory/'status.json',(json.dumps({'deferred':str(e),'boot':ops.boot})+'\n').encode())
  except OSError:pass
  return 1
 return 0
if __name__=='__main__':raise SystemExit(main())
