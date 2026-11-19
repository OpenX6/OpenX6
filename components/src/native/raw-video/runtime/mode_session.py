"""Own only the stock geometry temporarily required by RAW acquisition."""
import copy,json,os,time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
MODE='kMediaMode';RES='kMediaResRatio';FPS='kMediaVideoFps'
GEOMETRY=(MODE,RES,FPS)
CONTROLS=('kMediaPanoCamRole','kIq3AAutoSwitch','kIq3AExpMode','kIqShutterConfig','kIqIsoConfig','kAudioAlgoMode','kIq3AEvBias','kIqWbConfig','kIqMeterMode')
TARGET={MODE:{'lens_mode':'kPanorama','shoot_mode':'kNormalVideo'},RES:{'media_res':'kRes8K','media_ratio':'kRatio1x1','sub_media_res':'kRes4K'},FPS:{'video_fps':'k30'}}
class ModeSession:
 def __init__(self,api,path=Path('/data/x6-raw-mode/prepare-state.json'),sleep=time.sleep):
  self.api,self.path,self.sleep=api,Path(path),sleep;self.active=False
 def read(self,key):return self.api('config','get_general',key=key)
 def read_many(self,keys):
  with ThreadPoolExecutor(max_workers=4) as pool:return dict(zip(keys,pool.map(self.read,keys)))
 def idle(self):
  s=self.api('media','get_media_status')
  if s['record_status']['status']!='RecordIdle' or s['capture_status']['status']!='CaptureIdle':raise RuntimeError('Camera busy - RAW preparation deferred')
 def save(self,state):
  self.path.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
  tmp=self.path.with_suffix('.tmp')
  with tmp.open('w') as f:json.dump(state,f);f.flush();os.fsync(f.fileno())
  os.replace(tmp,self.path);fd=os.open(self.path.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 def load(self):
  if not self.path.exists():return None
  s=json.loads(self.path.read_text());assert s['version']==1 and set(s['geometry'])==set(GEOMETRY)
  return s
 def clear(self):
  self.path.unlink(missing_ok=True)
  if self.path.parent.exists():
   fd=os.open(self.path.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 def settings(self):return self.read_many(CONTROLS)
 def set(self,key,value):
  if self.read(key)!=value:self.api('config','set_general',key=key,value=copy.deepcopy(value))
 def apply_geometry(self,expected):
  current=self.read_many(GEOMETRY)
  for k in GEOMETRY:
   if current[k]!=expected[k]:
    self.api('config','set_general',key=k,value=copy.deepcopy(expected[k]))
    if k==MODE:current=self.read_many(GEOMETRY)
 def wait_geometry(self,expected):
  for _ in range(35):
   if self.read_many(GEOMETRY)==expected:return
   self.sleep(.1)
  raise RuntimeError('RAW camera preparation did not settle')
 def keep(self,settings):
  # Mode changes may reset 3A. Apply modes first and per-control Auto flags last.
  for attempt in range(2):
   current=self.settings()
   if current==settings:return
   for k in CONTROLS:
    if current[k]!=settings[k]:self.api('config','set_general',key=k,value=copy.deepcopy(settings[k]))
   self.sleep(.15)
  if self.settings()!=settings:raise RuntimeError('Could not preserve RAW exposure / lens / audio')
 def begin(self):
  self.idle()
  if self.active and self.read_many(GEOMETRY)==TARGET:return
  if not self.active and self.path.exists():
   self.leave()
   if self.path.exists():self.clear() # explicit new RAW entry supersedes deferred recovery
  controls=self.settings()
  state=self.load() if self.active else None
  if state is None:
   state={'version':1,'geometry':self.read_many(GEOMETRY),'controls':controls,'stage':'preparing'}
  state['controls']=controls;state['stage']='preparing';self.save(state)
  try:
   self.apply_geometry(TARGET)
   self.wait_geometry(TARGET);self.keep(controls)
   state['stage']='ready';self.save(state);self.active=True
  except BaseException:
   self.active=False
   try:self.leave(failed=True)
   except Exception:pass
   raise
 def leave(self,failed=False):
  self.active=False;state=self.load()
  if state is None:return
  self.idle();current=self.read(MODE);prior=state['geometry']
  if current not in (TARGET[MODE],prior[MODE]):
   # Do not pull the user out of a newly chosen photo/other video mode.
   state['stage']='pending';self.save(state);return
  if state['stage']=='pending' and current!=prior[MODE]:return
  geometry=self.read_many(GEOMETRY)
  if not failed and state['stage']!='preparing' and any(geometry[k] not in (TARGET[k],prior[k]) for k in GEOMETRY):
   self.clear();return # an explicit later choice takes precedence
  controls=state['controls'] if failed or state['stage']=='preparing' else self.settings()
  self.apply_geometry(prior)
  self.wait_geometry(prior);self.keep(controls);self.clear()
 def recover(self):
  if not self.active and self.path.exists():self.leave()
