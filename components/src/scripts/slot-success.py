#!/usr/bin/python3
"""Mark the current slot on its first boot with a verified root command channel."""
import json,os,re,socket,stat,struct,subprocess,tempfile,time,ctypes,zlib
from pathlib import Path
HEALTH_GIVE_UP=240
POLL=5
SSH_PORTS={0x08AE,0x08AF}  # 2222 regular owner sshd, 2223 start-guard fallback
SUCCESS_BIT=54
BLOCK=4096
class Deferred(RuntimeError):pass

def exclusive_lock(fd):
 # The target Python omits fcntl; use the same kernel flock via libc.
 libc=ctypes.CDLL(None,use_errno=True)
 fn=libc.flock;fn.argtypes=[ctypes.c_int,ctypes.c_int];fn.restype=ctypes.c_int
 if fn(fd,2|4)!=0:
  code=ctypes.get_errno();raise OSError(code,os.strerror(code))

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
 fd,tmp=tempfile.mkstemp(prefix='.slot-',dir=p.parent)
 try:
  if os.write(fd,b)!=len(b):raise OSError('short write')
  os.fsync(fd);os.close(fd);fd=-1;os.replace(tmp,p)
  d=os.open(str(p.parent),os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
  try:os.fsync(d)
  finally:os.close(d)
 finally:
  if fd>=0:os.close(fd)
  if os.path.exists(tmp):os.unlink(tmp)

def listening(text,ports):
 """True when /proc/net/tcp or tcp6 text shows a LISTEN socket on one of the ports."""
 for line in text.splitlines()[1:]:
  f=line.split()
  if len(f)>7 and f[7]=='0' and f[3]=='0A' and int(f[1].rsplit(':',1)[1],16) in ports:return True
 return False

def gpt_attributes(raw,label):
 """Attributes of the named entry from a primary GPT (first six 4 KiB blocks), or None."""
 h=raw[BLOCK:2*BLOCK]
 if h[:8]!=b'EFI PART':return None
 lba,count,size,crc=struct.unpack_from('<QIII',h,72)
 if size<128 or count<1 or count>256 or lba*BLOCK+count*size>len(raw):raise Deferred('GPT entry table out of range')
 table=raw[lba*BLOCK:lba*BLOCK+count*size]
 if zlib.crc32(table)!=crc:raise Deferred('GPT entry table CRC mismatch')
 for i in range(count):
  e=table[i*size:(i+1)*size]
  if not any(e[:16]):continue
  if e[56:128].decode('utf-16-le').split('\0')[0]==label:return struct.unpack_from('<Q',e,48)[0]
 return None

def slot_successful(attrs):return bool(attrs>>SUCCESS_BIT&1)

def fresh(slot):return {'schema':1,'slot':slot,'healthy_boots':[],'marked':None,'last':None}

def decide(ops):
 slot=ops.slot()
 state=ops.load()
 if not isinstance(state,dict) or state.get('schema')!=1 or state.get('slot')!=slot or not isinstance(state.get('healthy_boots'),list):
  state=fresh(slot)
 boot=ops.boot
 if (state.get('last') or {}).get('boot')==boot and (state.get('last') or {}).get('result') in ('marking','marked','already-successful'):
  return 'already-evaluated-this-boot'
 if ops.successful():
  if state.get('marked') is None:state['marked']={'by':'external','boot':boot}
  state['last']={'boot':boot,'result':'already-successful'};ops.save(state);return 'already-successful'
 healthy,detail=ops.health()
 if not healthy:
  state['healthy_boots']=[];state['last']={'boot':boot,'result':'unhealthy','detail':detail};ops.save(state)
  return 'unhealthy; count reset: '+detail
 ops.preflight()
 state['healthy_boots']=[boot]
 # Durable intent before the only write; an interrupted mark is diagnosed, not repeated blindly.
 state['last']={'boot':boot,'result':'marking','count':len(state['healthy_boots'])};ops.save(state)
 ops.set_success()
 if not ops.successful():raise Deferred('slot not reported successful after abctl')
 state['marked']={'by':'x6-slot-success','boot':boot,'after_healthy_boots':list(state['healthy_boots'])}
 state['last']={'boot':boot,'result':'marked','detail':detail};ops.save(state)
 return 'marked'

class Native:
 def __init__(self):
  self.directory=Path('/data/x6-slot-state')
  parent=self.directory.parent.lstat()
  if not stat.S_ISDIR(parent.st_mode) or parent.st_uid or parent.st_mode&0o022:raise Deferred('unsafe data directory')
  self.directory.mkdir(mode=0o700,exist_ok=True)
  s=self.directory.lstat()
  if not stat.S_ISDIR(s.st_mode) or s.st_uid or stat.S_IMODE(s.st_mode)!=0o700:raise Deferred('unsafe state directory')
  self.journal=self.directory/'state.json'
  self.boot=Path('/proc/sys/kernel/random/boot_id').read_text().strip()
  self.fd=os.open(str(self.directory/'lock'),os.O_RDWR|os.O_CREAT|os.O_NOFOLLOW|os.O_NONBLOCK,0o600)
  s=os.fstat(self.fd)
  if not stat.S_ISREG(s.st_mode) or s.st_uid or s.st_nlink!=1 or stat.S_IMODE(s.st_mode)!=0o600:raise Deferred('unsafe lock')
  exclusive_lock(self.fd)
 def run(self,args,seconds=8):
  p=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=seconds,text=True)
  if p.returncode:raise Deferred('native command failed: '+args[0])
  return p.stdout
 def slot(self):
  s=self.run(['/sbin/abctl','--boot_slot']).strip()
  if s not in ('_a','_b'):raise Deferred('unknown slot')
  return s
 def load(self):
  try:return json.loads(safe_read(self.journal,limit=16384))
  except FileNotFoundError:return None
  except (Deferred,ValueError):return None  # unreadable state only ever resets the count
 def save(self,state):
  atomic(self.journal,(json.dumps(state,sort_keys=True)+'\n').encode())
 def gpt(self,label):
  for dev in ['sda','sdb','sdc','sdd','sde','sdf']:
   size=Path('/sys/class/block/'+dev+'/size')
   if not size.exists() or int(size.read_text().strip())<=0:continue
   fd=os.open('/dev/'+dev,os.O_RDONLY|os.O_NOFOLLOW|os.O_CLOEXEC)
   try:raw=os.read(fd,6*BLOCK)
   finally:os.close(fd)
   if len(raw)<6*BLOCK:continue
   attrs=gpt_attributes(raw,label)
   if attrs is not None:return attrs
  raise Deferred('slot entry not found in any GPT')
 def successful(self):
  return slot_successful(self.gpt('boot'+self.slot()))
 def hotspot_root(self):
  # The socket belongs to the running listener, and is removed on unbind.
  try:
   with socket.socket(socket.AF_UNIX,socket.SOCK_STREAM) as conn:
    conn.settimeout(2)
    conn.connect('/run/x6-wifi-root/check.sock')
    pid,uid,_=struct.unpack('3i',conn.getsockopt(socket.SOL_SOCKET,socket.SO_PEERCRED,12))
    if uid!=0 or not os.path.samefile('/proc/%d/exe'%pid,'/usr/local/sbin/x6-wifi-root'):
     return False
    with conn.makefile('rb') as reply:
     return reply.readline(16)==b'uid=0\n'
  except OSError:
   return False
 def probe(self):
  for port in sorted(SSH_PORTS):
   if not any(listening(Path(table).read_text(),{port}) for table in ('/proc/net/tcp','/proc/net/tcp6') if Path(table).exists()):
    continue
   try:
    self.run(['/usr/local/sbin/x6-owner-maintenance','--probe-root','--listen','127.0.0.1:'+str(port)])
   except (Deferred,OSError,subprocess.TimeoutExpired):
    continue
   return True,'root SSH verified on '+str(port)
  if self.hotspot_root():return True,'root verified on wlan1:23456'
  return False,'neither maintenance SSH nor hotspot root available'
 def health(self):
  while True:
   up=float(Path('/proc/uptime').read_text().split()[0])
   ok,detail=self.probe()
   if ok or up>=HEALTH_GIVE_UP:return ok,detail
   time.sleep(POLL)
 def preflight(self):
  if self.run(['/bin/systemctl','show','ins_otad.service','-p','MainPID','--value']).strip()!='0':raise Deferred('Linux OTA active')
  for p in ['/data/x6-owner-update/install.permit','/run/x6-owner-update/install.permit','/run/x6-owner/vendor-ota-enabled','/run/x6-owner/validated-boot']:
   if os.path.lexists(p):raise Deferred('installation marker present: '+p)
  out=json.loads(self.run(['/usr/bin/ins_bll_cli',json.dumps({'cmd':'share_state','action':'get','type':'otad'})]))
  if out.get('ret')!='ok' or out.get('data',{}).get('update_status')!='Idle':raise Deferred('vendor updater not idle')
 def set_success(self):
  # The vendor tool marks only the running slot; its stderr lists absent labels and is ignored.
  self.run(['/sbin/abctl','--set_success'],30)

def main():
 if os.geteuid()!=0:raise SystemExit('root required')
 ops=Native()
 try:
  result=decide(ops);atomic(ops.directory/'status.json',(json.dumps({'result':result,'boot':ops.boot})+'\n').encode())
 except (Deferred,OSError,ValueError,subprocess.TimeoutExpired) as e:
  try:atomic(ops.directory/'status.json',(json.dumps({'deferred':str(e),'boot':ops.boot})+'\n').encode())
  except OSError:pass
  return 1
 return 0
if __name__=='__main__':raise SystemExit(main())
