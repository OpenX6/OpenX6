#!/usr/bin/python3
"""Interval UI preferences, bounded per-series options and a measured storage write
speed. Never starts a capture.

OpenX6 8K and 16K fast interval. Shutter Max is always a cap (Native removed, owner 2026-11-03):
1/30, 1/15, 1/8, 1/4, 1/2, 1, 2 s. The choice is saved as shutter-7.pref; it is created once
from the earlier shutter.pref (Native, 1/8, 1/4, 1/2, 1, 2 s), which is left untouched so an
earlier controller still reads its own value. Native maps to 1/4 s (the vendor still-exposure
maximum is ~200 ms); absent means Native.

16K Fast RAW (owner 2026-11-03, needs in-camera post-processing): fast16k.pref; in 16K scope
only it is published as /run/x6-interval-controls/fast16k {u64 time_ms; i32 enabled; u32 tag
'F16K'}, which mediad reads at series start. Frames are then saved as unprocessed passthrough
DNGs and converted later in the camera by pureshot-batch-v1 (ps2native)."""
import json, math, os, stat, struct, subprocess, time
from pathlib import Path
B=Path('/data/x6-owner-features/interval-controls-v1')
R=Path('/run/x6-interval-controls')
CAPS=(33,67,125,250,500,1000,2000)  # Shutter Max ms: 1/30, 1/15, 1/8, 1/4, 1/2, 1, 2 s
PREFS=[('shutter',6),('timing',2),('tl',1),('fast8k',1),('fast16k',1)]
PREF_FILES={'shutter':'shutter-7.pref'}
OLD_SHUTTER=(3,2,3,4,5,6)  # shutter.pref index (Native, 1/8, 1/4, 1/2, 1, 2 s) -> CAPS index
# Interval Timing is a 3-way radio saved as timing.pref: 0 Native, 1 Minimum, 2 Custom (owner
# 2026-11-04). Custom publishes the owner-typed interval (ms, from custom.value) as the mediad
# period, bypassing the computed safe floor so the owner can go faster at their own risk; mediad
# still clamps to its own hard floor (100 ms fast / 4000 ms non-fast). The screen shows the
# computed Minimum target (published as recommend) as the recommended minimum. minimum.pref is
# still written (1 when timing>=1) so a restored earlier controller reads a valid 0/1.
CUSTOM_FILE=B/'custom.value'
CUSTOM_MIN=100
CUSTOM_MAX=120000
FAST_TAG=0x4b384631
FAST16_TAG=0x4b363146  # b'F16K'
# Camera 4 measurements, 2026-11-01 (work/interval-8k-fast-20261101):
# 8K start-to-start with GrabDone scheduling ~= 345 ms + exposure (+~30 ms while native
# processing runs); the next request needs >= 100 ms after GrabDone.
# 8K pipelined native processing ~= 420-450 ms/frame, unprocessed DNG ~= 200 ms.
# Lossless level-0 DNG: 8K 60.5 MB, 16K 240.9 MB.
# 16K Fast RAW, unit 5 on a Genki SavePoint SSD, 2026-11-03 (work/interval-16k-fast-20261103/live S2/S4):
# start-to-start at a forced 100 ms period ~= 0.46-0.49 s + exposure (median), at most 0.535 s + exposure,
# steady over 60-80 frames at ~520 MB/s; C16 covers the worst cycle so every Minimum frame is on time.
# R16 (automatic exposure above one preview frame period) is still the 8K figure; unprocessed DNG ~= P16 ms/frame.
C16=540
R16=250
P16=400
BYTES={8:60_500_000,16:240_900_000}
STORAGE={'kQuickTransfer':'/mnt/quickreader','kExternalSDCard':'/mnt/sdcard','kInternal':'/ins/media_rw/emulated','kInternalStorage':'/ins/media_rw/emulated'}
SPEED_FILE=B/'storage-speed.json'
SPEED_BYTES=128<<20
SPEED_MAX_AGE=30*86400
SPEED_RETRY=600
SPEED_RESERVE=1<<30
speed_tries={}
def checked_dir(p):
 p.mkdir(mode=0o700,parents=True,exist_ok=True)
 s=p.lstat()
 if not stat.S_ISDIR(s.st_mode) or s.st_uid or s.st_mode&0o022:raise RuntimeError('unsafe directory')
def read(p,n):
 fd=os.open(str(p),os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK|os.O_CLOEXEC)
 try:
  s=os.fstat(fd)
  if not stat.S_ISREG(s.st_mode) or s.st_uid or s.st_nlink!=1 or s.st_mode&0o022 or (n is not None and s.st_size!=n) or s.st_size>65536:raise RuntimeError('unsafe file')
  want=s.st_size if n is None else n
  b=os.read(fd,want+1)
  if len(b)!=want:raise RuntimeError('short file')
  return b
 finally:os.close(fd)
def atom(p,data):
 tmp=p.with_name(p.name+'.next')
 if tmp.exists():
  read(tmp,tmp.lstat().st_size);tmp.unlink()
 fd=os.open(str(tmp),os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 try:
  if os.write(fd,data)!=len(data):raise RuntimeError('short write')
  os.fsync(fd)
 finally:os.close(fd)
 os.replace(tmp,p)
 fd=os.open(str(p.parent),os.O_DIRECTORY|os.O_RDONLY|os.O_NOFOLLOW)
 try:os.fsync(fd)
 finally:os.close(fd)
def command(j):
 p=subprocess.run(['/usr/bin/timeout','-k','1','3','/usr/bin/ins_bll_cli',json.dumps(j)],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=5)
 d=json.loads(p.stdout)
 if p.returncode or d.get('ret')!='ok':raise RuntimeError('configuration unavailable')
 return d.get('data',{})
def cfg(k):return command(dict(cmd='config',action='get_general',key=k))
def idle():
 m=command(dict(cmd='share_state',action='get',type='media'))
 return m.get('record',{}).get('record_status')=='RecordIdle' and m.get('capture',{}).get('capture_status')=='CaptureIdle'
def scope_of(mode,res,fmt):
 if mode!={'lens_mode':'kPanorama','shoot_mode':'kInterval'} or res.get('media_ratio')!='kRatio1x1' or fmt.get('photo_format')!='kDNG':return 0
 return {'kRes16K':16,'kRes8K':8}.get(res.get('media_res'),0)
def pref_path(name):return B/PREF_FILES.get(name,name+'.pref')
def migrate_shutter():
 p=pref_path('shutter')
 if p.exists():return
 old=B/'shutter.pref';v=read(old,1)[0] if old.exists() else 0
 atom(p,bytes([OLD_SHUTTER[v] if v<len(OLD_SHUTTER) else OLD_SHUTTER[0]]))
def migrate_timing():
 """Seed timing.pref from the earlier minimum.pref (1 Minimum / 0 Native)."""
 p=pref_path('timing')
 if p.exists():return
 old=B/'minimum.pref';v=read(old,1)[0] if old.exists() else 0
 atom(p,bytes([1 if v>=1 else 0]))
def read_custom_ms():
 """Owner-typed interval in ms from custom.value, or None if absent/invalid."""
 try:b=read(CUSTOM_FILE,4)
 except (FileNotFoundError,RuntimeError):return None
 v=struct.unpack('<I',b)[0]
 return v if CUSTOM_MIN<=v<=CUSTOM_MAX else None
def write_minimum_compat(timing):
 """Keep minimum.pref consistent (1 when timing>=1) for a restored earlier controller."""
 want=1 if timing>=1 else 0
 p=B/'minimum.pref'
 try:
  if p.exists() and read(p,1)[0]==want:return
 except RuntimeError:pass
 atom(p,bytes([want]))
def budget_ms(index,shutter):
 """Exposure budget in ms: manual shutter as selected, otherwise the Shutter Max cap."""
 cap=CAPS[index]
 if shutter.get('shutter_mode')=='kManual':
  v=shutter['shutter']
  if isinstance(v,bool) or not isinstance(v,(int,float)) or not math.isfinite(v) or not v:raise ValueError('invalid shutter')
  return (float(v) if v>0 else 1e6/abs(v)),True
 return float(cap),False
def estimate(scope,budget,manual,fast,bps):
 if scope==16:
  parts={'capture':C16+budget+(0 if manual or budget<=33 else R16),'processing':P16} if fast else {'base':4500+budget}
 else:
  # Automatic exposure above one preview frame period re-times the sensor once more.
  parts={'capture':345+budget+(0 if fast else 30)+(0 if manual or budget<=33 else 250),'processing':200 if fast else 450}
 if bps:parts['storage']=BYTES[scope]*1000/bps
 return int(math.ceil(max(parts.values()))),parts
def calculate(index,timing,custom_ms,shutter,mode,res,fmt,fast=0,bps=0,fast16=0):
 scope=scope_of(mode,res,fmt);budget,manual=budget_ms(index,shutter)
 period,parts=estimate(scope or 16,budget,manual,fast if scope==8 else fast16 if scope==16 else 0,bps)
 lo=100 if scope==8 or (scope==16 and fast16) else 4500
 if not lo<=period<=120000:raise ValueError('shutter exceeds supported interval')
 cap=CAPS[index] if scope else 0
 # period is the computed safe minimum (the recommended value shown on the Custom screen).
 # Active period published to mediad: Native 0; Minimum the computed floor; Custom the owner's
 # typed value clamped to mediad's hard floor (below the recommendation = the owner's risk).
 if not scope or timing==0:active=0
 elif timing==2 and custom_ms:
  floor=100 if (scope==8 or (scope==16 and fast16)) else 4000
  active=min(max(custom_ms,floor),120000)
 else:active=period
 return (active,cap,period,scope,parts)
def mount_of(path):
 """Topmost mount at exactly `path`: (source, fstype, options, major:minor)."""
 best=None
 for line in Path('/proc/self/mountinfo').read_text().splitlines():
  f=line.split();sep=f.index('-')
  if f[4]==path:best=(f[sep+2],f[sep+1],f[5]+','+f[sep+3],f[2])
 return best
def volume_key(source,devno):
 d=Path('/dev/disk/by-uuid')
 if d.is_dir():
  for e in d.iterdir():
   try:
    if os.path.realpath(str(e))==os.path.realpath(source):return 'uuid:'+e.name
   except OSError:pass
 return 'dev:'+devno
def load_speeds():
 try:
  d=json.loads(read(SPEED_FILE,None))
  return d if isinstance(d,dict) else {}
 except FileNotFoundError:return {}
def measure(path):
 """Write SPEED_BYTES of non-repeating data, fsync, delete. Returns bytes/s."""
 t=os.path.join(path,'.x6-interval-speedtest')
 st=os.statvfs(path)
 if st.f_bavail*st.f_frsize<SPEED_BYTES+SPEED_RESERVE:raise RuntimeError('storage nearly full')
 if os.path.lexists(t):
  s=os.lstat(t)
  if not stat.S_ISREG(s.st_mode):raise RuntimeError('unexpected speed-test entry')
  os.unlink(t)
 chunk=bytearray(os.urandom(4<<20))
 fd=os.open(t,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW|os.O_CLOEXEC,0o600)
 try:
  start=time.monotonic();done=0
  while done<SPEED_BYTES:
   chunk[0:8]=struct.pack('<Q',done)
   n=os.write(fd,chunk)
   if n<=0:raise RuntimeError('short write')
   done+=n
  os.fsync(fd);elapsed=time.monotonic()-start
 finally:
  os.close(fd)
  try:os.unlink(t)
  except FileNotFoundError:pass
  dfd=os.open(path,os.O_DIRECTORY|os.O_RDONLY)
  try:os.fsync(dfd)
  finally:os.close(dfd)
 if elapsed<=0:raise RuntimeError('clock')
 return done/elapsed
def storage_speed(location,want_test):
 """Cached bytes/s for the selected storage; measures once per volume when idle."""
 path=STORAGE.get(location)
 if not path:return 0,None
 m=mount_of(path)
 if not m or m[1] not in ('exfat','vfat','ext4') or 'rw' not in m[2].split(','):return 0,None
 key=location+'|'+volume_key(m[0],m[3]);speeds=load_speeds();e=speeds.get(key)
 now=time.time()
 if isinstance(e,dict) and isinstance(e.get('bps'),(int,float)) and e['bps']>0 and now-e.get('time',0)<SPEED_MAX_AGE:return float(e['bps']),key
 if not want_test or time.monotonic()-speed_tries.get(key,-1e9)<SPEED_RETRY:return 0,key
 speed_tries[key]=time.monotonic()
 if not idle():return 0,key
 bps=measure(path)
 if not idle():return 0,key  # capture started during the test: discard
 speeds[key]={'bps':bps,'time':now,'path':path,'bytes':SPEED_BYTES}
 atom(SPEED_FILE,json.dumps(speeds,sort_keys=True).encode())
 print('STORAGE_SPEED %s %.1f MB/s'%(key,bps/1e6),flush=True)
 return bps,key
def label(display):
 return ('Minimum (%s s target)'%format(display/1000,'.3f').rstrip('0').rstrip('.')).encode()+b'\0'
def rec_text(display):
 """Short recommended-minimum line shown on the Custom input screen (owner accepts the risk)."""
 return ('Min %s s - lower=risk'%format(display/1000,'.3f').rstrip('0').rstrip('.')).encode()+b'\0'
def initialize():
 checked_dir(B);checked_dir(R);migrate_shutter();migrate_timing()
 for name,limit in PREFS:
  p=pref_path(name)
  if not p.exists():atom(p,b'\0')
  v=read(p,1)[0]
  if v>limit:raise RuntimeError('invalid saved preference')
  atom(R/(name+'-state'),bytes([v]));atom(R/(name+'-request'),b'\xff')
 for n in ['tl-options','capture','options','fast8k','fast8k-capture','fast16k']:atom(R/n,bytes(16))
 atom(R/'label',b'Minimum (checking...)\0')
 atom(R/'recommend',struct.pack('<I',0));atom(R/'recommend-text',b'Custom interval\0')
def step():
 vals={}
 for name,limit in PREFS:
  req=R/(name+'-request');v=read(req,1)[0]
  if v<=limit:
   atom(pref_path(name),bytes([v]));atom(req,b'\xff')
  v=read(pref_path(name),1)[0]
  if v>limit:raise RuntimeError('invalid preference')
  vals[name]=v;atom(R/(name+'-state'),bytes([v]))
 timing=vals['timing'];write_minimum_compat(timing);custom_ms=read_custom_ms()
 try:
  mode=cfg('kMediaMode');res=cfg('kMediaResRatio');fmt=cfg('kMediaPhotoFormat')
  tl=mode=={'lens_mode':'kPanorama','shoot_mode':'kTimelapse'} and res.get('media_res')=='kRes16K' and res.get('media_ratio')=='kRatio1x1'
  atom(R/'tl-options',struct.pack('<QII',int(time.monotonic()*1000),vals['tl'] if tl else 0,0x544c3136))
  scope=scope_of(mode,res,fmt);bps=0;key=None
  if scope:
   try:bps,key=storage_speed(cfg('kStorageLocation').get('storage_location'),timing>=1)
   except Exception as e:print('STORAGE_SPEED_ERROR %s: %s'%(type(e).__name__,e),flush=True);bps=0
  p,c,display,scope,parts=calculate(vals['shutter'],timing,custom_ms,cfg('kIqShutterConfig'),mode,res,fmt,vals['fast8k'],bps,fast16=vals['fast16k'])
  now=int(time.monotonic()*1000)
  atom(R/'fast8k',struct.pack('<QII',now,vals['fast8k'] if scope==8 else 0,FAST_TAG))
  atom(R/'fast16k',struct.pack('<QII',now,vals['fast16k'] if scope==16 else 0,FAST16_TAG))
  atom(R/'options',struct.pack('<IIQ',p,c,now))
  atom(R/'label',label(display))
  atom(R/'recommend',struct.pack('<I',display if scope else 0))
  atom(R/'recommend-text',rec_text(display) if scope else b'Custom interval\0')
  atom(R/'estimate.json',json.dumps({'scope':scope,'period_ms':display,'active_period_ms':p,'cap_ms':c,'timing':timing,'custom_ms':custom_ms,'parts_ms':{k:round(v,1) for k,v in parts.items()},'storage_bps':round(bps),'storage_key':key,'fast8k':vals['fast8k'],'fast16k':vals['fast16k']},sort_keys=True).encode())
 except Exception:
  for n in ['tl-options','options','fast8k','fast16k']:atom(R/n,bytes(16))
  atom(R/'label',b'Minimum (not ready)\0');raise
def main():
 initialize()
 while True:
  try:step()
  except Exception as e:print(type(e).__name__+': '+str(e),flush=True)
  time.sleep(1)
if __name__=='__main__':main()
