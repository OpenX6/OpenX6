"""Public predecessor metadata only; no images, keys or device backup imports."""
import json,re
from pathlib import Path
from .common import require,regular,save,sha,canonical,digest
from .owner import validate_owner,PATHS,unique_object,SSH,ADB,ssh_public,adb_public,ed25519_public

TARGETS={'vr180-v1/gui','vr180-v1/setup','vr180-v1/sync.so','vr180-v1/backup/gui','vr180-v1/backup/setup','vr180-v1/backup/sync.so','raw-nr-v1/medium.so','raw-nr-v1/low.so','raw-nr-v1/minimal.so'}
def hash_ok(s):return isinstance(s,str) and re.fullmatch('[0-9a-f]{64}',s) is not None

def load_predecessors(paths,owner,*,layout_only=False):
 result=[];seen=set();want={k.lstrip('/'):sha(v) for k,v in owner.items()}
 for path in paths:
  with regular(path) as f:
   b=f.read(131073);require(len(b)<=131072,'Predecessor metadata too large');p=json.loads(b,object_pairs_hook=unique_object)
  require(set(p)=={'Version','Seed','Owners','Entries','LegacySSH','Receiptless'} and p['Version']==1,'Predecessor schema')
  require(hash_ok(p['Seed']) and p['Seed'] not in seen,'Duplicate/invalid predecessor')
  require(isinstance(p['Owners'],dict) and p['Owners'].keys()==want.keys() and all(hash_ok(h) for h in p['Owners'].values()), 'Invalid predecessor owner hashes')
  if not layout_only:require(p['Owners']==want,'Predecessor holder differs; key rotation is not migration')
  require(type(p['Receiptless']) is bool and len(p['Entries'])==len(TARGETS),'Predecessor bounds')
  names=set()
  for e in p['Entries']:
   require(set(e)=={'Target','SHA256','Bytes','Mode'} and e['Target'] in TARGETS and e['Target'] not in names,'Predecessor entry')
   require(hash_ok(e['SHA256']) and type(e['Bytes']) is int and 0<e['Bytes']<=160*1024**2,'Predecessor size/hash')
   require(e['Mode'] in (384,420,493),'Predecessor file mode');names.add(e['Target'])
  if p['LegacySSH'] is not None:
   e=p['LegacySSH'];require(set(e)=={'SHA256','Bytes'} and hash_ok(e['SHA256']) and type(e['Bytes']) is int and 0<e['Bytes']<=32*1024**2,'Legacy SSH record')
  seen.add(p['Seed']);result.append(p)
 require(len(result)<=16,'Too many predecessors')
 return sorted(result,key=lambda p:p['Seed'])

def export_predecessor(build,dest):
 build=Path(build);dest=Path(dest);require(not dest.exists(),'Refuse existing predecessor descriptor')
 raw=(build/'overlay/usr/share/x6-firstboot-seed/manifest.json').read_bytes();m=json.loads(raw)
 image=json.loads((build/'image.json').read_text())['files']
 require(image['/usr/share/x6-firstboot-seed/manifest.json']['sha256']==sha(raw),'Prior seed manifest changed')
 owners={}
 for name in PATHS:
  f=build/'overlay'/name.lstrip('/');b=f.read_bytes();require(sha(b)==image[name]['sha256'],'Prior public file changed')
  normalized=ssh_public(b) if name==SSH else adb_public(b) if name==ADB else ed25519_public(b)
  owners[name]=sha(normalized)
 helper=build/'overlay/usr/local/sbin/x6-owner-maintenance'
 require(digest(helper)==image['/usr/local/sbin/x6-owner-maintenance']['sha256'],'Prior SSH helper changed')
 p=dict(Version=1,Seed=sha(raw),Owners={k.lstrip('/'):v for k,v in owners.items()},Entries=[{k:e[k] for k in ['Target','SHA256','Bytes','Mode']} for e in m['Entries']],LegacySSH=dict(SHA256=digest(helper),Bytes=helper.stat().st_size),Receiptless=True)
 # Export contains hashes only. This is an explicitly trusted local compatibility
 # declaration, not an authenticated recovery certificate or live inventory.
 save(dest,p);dest.chmod(0o600);return p
