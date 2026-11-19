from pathlib import Path
import hashlib,json,subprocess as sp,sys,time,os,re,select,signal,shutil
p=Path.cwd()
unit=Path('/run/systemd/system/qmmf-server.service.d/zz-x5-raw-selected.conf')
cfg='/usr/lib/camera/instasettings.config'
so='/usr/lib/com.qti.feature2.rtmcx.so'
T=Path('/sys/kernel/tracing');g='x5post';i=T/'instances'/g
def service(*a):sp.run(['systemctl',*a],check=True,timeout=40)
def cmd(action):
 d=json.loads(sp.check_output(['ins_bll_cli',json.dumps(dict(cmd='media',action=action))],timeout=8))
 assert d['ret']=='ok',d
 return d.get('data',{})
def idle():
 return cmd('get_media_status')['record_status']['status']=='RecordIdle'
def src(f):return Path('/proc')/sp.check_output(['pidof','qmmf-server']).decode().strip()/'root'/f.lstrip('/')
def restart():
 service('stop','pulseaudio','ins_mediad','qmmf-server');service('daemon-reload');service('start','qmmf-server');time.sleep(1);service('start','ins_mediad','pulseaudio')
 for _ in range(35):
  try:
   if idle():return
  except Exception:pass
  time.sleep(1)
 raise RuntimeError('service startup')
medunit=Path('/run/systemd/system/ins_mediad.service.d/zzzz-x5-raw-selected.conf')
configs=['/ins/etc/ins_mediad/ins_media_stream_prd.json','/usr/share/x6-ff-pano-sole-main-v1.json']
def medsrc(f):return Path('/proc')/sp.check_output(['pidof','ins_mediad']).decode().strip()/'root'/f.lstrip('/')
def restore():
 if medunit.exists():medunit.unlink()

 if unit.exists():unit.unlink();restart()
 assert hashlib.sha256(src(so).read_bytes()).hexdigest()=='8e0c12323186185fc6598b3883bb95f2ed688b1c2f42fab46a56d7818cafe16b' and src(cfg).read_bytes()==(p/'orig.cfg').read_bytes() and idle()
 print('restored',flush=True)
if sys.argv[1]=='restore':restore();sys.exit()
root=Path('/mnt/quickreader')
from storage import SSD_REQUIRED, selected_external
def recording_drive():
 try:
  return selected_external(root)
 except ValueError:
  (p/'failure').write_text(SSD_REQUIRED)
  raise
card,device=recording_drive()

from capture_window import RESERVE, MAX_RECORD
assert shutil.disk_usage(root).free > RESERVE + MAX_RECORD
dest=root/('RAWVIDEO-'+time.strftime('%Y%m%d-%H%M%S'))
assert idle() and not unit.exists() and not i.exists()
assert hashlib.sha256(src(so).read_bytes()).hexdigest()=='8e0c12323186185fc6598b3883bb95f2ed688b1c2f42fab46a56d7818cafe16b'
assert hashlib.sha256(Path('/usr/lib/hw/camera.qcom.so').read_bytes()).hexdigest()=='25e3605f33ad201b82a06ea6ee85f6b6f1fa6a3095d144bbef35a8036a9adca3'
orig=src(cfg).read_bytes();(p/'orig.cfg').write_bytes(orig)
test=orig
if True:
 test,n=re.subn(rb'(?m)^(mode\[15\]\s*=\s*0x8007;[^\n#]*;\s*)44(\s*#Pano)',rb'\g<1>14\2',orig);assert n==1
(p/'cfg').write_bytes(test)
for f,to in [(cfg,'cfg'),(so,'rt-tap.so')]:os.setxattr(p/to,'security.selinux',os.getxattr(src(f),'security.selinux'))
bind=[]
for j,f in enumerate(configs):
 raw=medsrc(f).read_bytes()
 (p/('orig'+str(j))).write_bytes(raw)
 obj=json.loads(raw);changed=0
 for group in obj['stream_profile']['video_stream_groups']:
  if group.get('group_param',{}).get('op_mode')=='0x8007':
   for st in group['stream_param']:st['fps']=[50,1]
   changed+=1
 if changed:
  tmp=p/('ram60-'+str(j)+'.json')
  tmp.write_text(json.dumps(obj));os.setxattr(tmp,'security.selinux',os.getxattr(medsrc(f),'security.selinux'))
  bind.append('BindReadOnlyPaths='+str(tmp)+':'+f)
 print(f,changed,flush=True)
assert bind
test=test.replace(b'0x8007;        30;',b'0x8007;        50;')
(p/'cfg').write_bytes(test)
exec((p/'taps7.py').read_text())
def regk(s):
 fd=os.open(T/'kprobe_events',os.O_WRONLY|os.O_APPEND)
 try:os.write(fd,(s+'\n').encode())
 finally:os.close(fd)
try:
 unit.parent.mkdir(exist_ok=True)
 unit.write_text('[Service]\nBindReadOnlyPaths='+str(p/'rt-tap.so')+':'+so+'\nBindReadOnlyPaths='+str(p/'cfg')+':'+cfg+'\n')
 medunit.parent.mkdir(exist_ok=True);medunit.write_text("[Service]\n"+"\n".join(bind)+"\n")
 restart();assert os.path.samefile(src(so),p/'rt-tap.so')
 pid=sp.check_output(['pidof','qmmf-server']).decode().strip()
 i.mkdir();(i/'trace_clock').write_text('mono');(i/'buffer_size_kb').write_text('2048')
 for k in ['primary','fence']:
  off,fmt=probe_defs[k]
  event='p:'+g+'/'+k+' '+cam+':0x'+off+' '
  try:up(event+fmt)
  except OSError as exc:
   import errno
   if k!='primary' or exc.errno not in (errno.EINVAL,errno.EOPNOTSUPP):raise
   up(event+fmt.split(' applied_ptr=')[0])
 (i/'events'/g/'fence/filter').write_text('port==9')
 regk('p:'+g+'/sof camera:__cam_req_mgr_process_req link=+0($arg1):s32 frame=+8($arg2):s64 trigger=+16($arg2):u32 sof=+24($arg2):u64');(i/'events'/g/'sof/filter').write_text('trigger==1')
 def measure(label):
  d=p/label;d.mkdir();
  if recording_drive() != (card,device):
   (p/'failure').write_text('External SSD changed. Try again.')
   raise RuntimeError('External drive changed')
  dest.mkdir()
  (d/'bench.log').symlink_to(dest/'.writer.log')
  (p/'destination').write_text(str(dest))
  from deferred_meta import recording
  with recording(dest,50,audio=True), (d/'bench.log').open('wb') as logfile:
   worker=sp.Popen([str(p/sys.argv[1]),pid,*([str(dest),device] if sys.argv[1]=='card' else [])],stdin=sp.PIPE,stdout=logfile,stderr=logfile,bufsize=0)
   try:
    for _ in range(150):
     if (d/'bench.log').read_bytes().startswith(b'ready'):break
     assert worker.poll() is None,worker.returncode
     time.sleep(.1)
    else:raise RuntimeError('bench start')
    assert sp.check_output(['pidof','qmmf-server']).decode().strip()==pid
    (i/'trace').write_text('');(i/'events'/g/'enable').write_text('1')
    from exposure_sidecar import ExposureOutput
    from capture_window import feed
    with (i/'trace_pipe').open('rb',buffering=0) as pipe, ExposureOutput(dest/'exposure.part') as durations:
     first,last,sent,period=feed(pipe,worker.stdin,p,root,durations)
    (i/'events'/g/'enable').write_text('0');worker.stdin.close()
    assert worker.wait(timeout=90)==0
    (d/'window').write_text(json.dumps([first,last,sent,durations.error]))

    print(label,first,last,sent,flush=True)
   finally:
    if worker.poll() is None:worker.kill();worker.wait()
  from take_status import finalize_paired
  finalize_paired(dest, p, 50)
  return period
 for binary in sys.argv[1].split(','):
  sys.argv[1]=binary
  period=measure(binary+'-data')
  print('period ns',period,flush=True)
  assert 19800000.0<period<20200000.0

finally:
 if i.exists():
  (i/'events/enable').write_text('0');i.rmdir()
 for k in ('primary','fence','sof'):
  if (T/'events'/g/k).exists():(regk if k=='sof' else up)('-:'+g+'/'+k)
 restore()

