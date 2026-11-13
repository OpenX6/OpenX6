"""Checked local recipes; every added executable is built from pinned source."""
import json
from pathlib import Path
from .common import require,digest,asset,safe_relative,regular
from .official import OFFICIAL_SHA,COMPONENTS
from .proxy_profile import PROFILE_PATH
from .owner import validate_owner,PATHS

TARGETS = {'/usr/lib/libx6-owner-gui-bridge.so':'gui-bridge',
 '/usr/local/sbin/x6-firstboot':'bootstrap','/usr/local/sbin/x6-owner-gui-auth':'gui-auth',
 '/usr/local/sbin/x6-owner-gui-setup':'gui-setup','/usr/local/sbin/x6-owner-maintenance':'ssh',
 '/usr/local/sbin/x6-owner-usb':'usb','/usr/local/sbin/x6-raw-nr-control':'nr-control',
 '/usr/local/sbin/x6-vr180-control':'vr180-control','/usr/local/sbin/x6-owner-update-check':'verifier'}
TARGETS.update({'/usr/local/sbin/x6-mod-'+k+'-control':'mod-'+k for k in ['stereo','hnr','vnr']})
TARGETS.update({'/usr/local/sbin/x6-proxy-control':'proxy-control','/usr/local/sbin/x6-mod-controls-prepare':'mod-prepare'})
TARGETS.update({'/usr/local/sbin/x6-statsd':'stats','/usr/local/sbin/x6-preview-control':'preview'})
TARGETS.update({'/usr/local/sbin/x6-boot-guard':'boot-guard'})
# 16K Fast RAW Processing converter: C against the vendor engine (glibc, dlopen), compiled with zig and stripped
TARGETS.update({'/usr/lib/x6-pureshot/ps2native':'pureshot/ps2native'})
TARGETS['/usr/local/sbin/x6-wifi-root']='wifi-root'
TARGETS['/usr/lib/x6-freeframe-square8-pool.so']='square8'
GENERATED={'/etc/nginx/conf.d/fileops.conf',PROFILE_PATH,'/usr/local/sbin/x6-owner-late-start','/ins/etc/x6_owner_patch_manifest.json','/usr/share/x6-firstboot-seed/delta.bin','/usr/share/x6-firstboot-seed/manifest.json'}

def load_profile(root):
 root=Path(root)
 with regular(root/'profile.json') as f:p=json.load(f)
 require(p['schema']==2 and p['model']=='c9' and p['official_sha256']==OFFICIAL_SHA,'Profile base rejected')
 require(p['installation_allowed'] is False,'Only locked profiles supported')
 for path,s in p['actions'].items():
  require(path.startswith('/'),'Absolute camera path required');safe_relative(path[1:])
  require(not path.startswith(('/data/','/persist/','/dev/','/proc/','/sys/')),'Device data rejected')
  require(s['kind'] in ('owner-public','release','compile','patch','asset','generated','official-component'),'Unknown operation')
  if s['kind']=='official-component':require(path=='/usr/share/openx6/mcu/MCUFw-V1.2.0.0.bin' and s['component']=='MCUFw-V1.2.0.0.bin' and s['sha256']==COMPONENTS[s['component']][1],'Unsupported official component')
  if s['kind']=='compile':require(TARGETS.get(path)==s['source'].removeprefix('src/'),'Unknown source target')
  if s['kind']=='generated':require(path in GENERATED,'Unknown generated target')
  if s['kind'] in ('patch','asset'):
   f=asset(root,s['asset']);require(digest(f)==s['asset_sha256'],'Component asset changed')
   if s['kind']=='asset':require(not f.read_bytes().startswith(b'\x7fELF'),'Precompiled helper rejected')
 require({k for k,s in p['actions'].items() if s['kind']=='compile'}==set(TARGETS),'Missing source target')
 require({k for k,s in p['actions'].items() if s['kind']=='owner-public'}==PATHS,'Holder key roles differ')
 for name,s in p['seed'].items():
  safe_relative(name);safe_relative(s['base'])
  require(s['kind'] in ('copy','patch','compile'),'Unknown seed operation')
  if s['kind']=='patch':require(digest(asset(root,s['asset']))==s['asset_sha256'],'Seed recipe changed')
  if s['kind']=='official-component':require(path=='/usr/share/openx6/mcu/MCUFw-V1.2.0.0.bin' and s['component']=='MCUFw-V1.2.0.0.bin' and s['sha256']==COMPONENTS[s['component']][1],'Unsupported official component')
  if s['kind']=='compile':require(name=='vr180-v1/setup','Unknown seed compilation')
 for v in p.get('mod_runtime',[]):require(digest(asset(root,v['recipe']))==v['recipe_sha256'],'Mod recipe changed')
 for rel,h in p['sources'].items():require(digest(asset(root,rel))==h,'Component source changed: '+rel)
 actual={str(x.relative_to(root)) for x in (root/'src').rglob('*') if x.is_file()}
 require(actual==set(p['sources']),'Unexpected component source file')
 require(digest(root/'coordinator-policy-template.json')==p['policy_sha256'],'Policy changed')
 policy=json.loads((root/'coordinator-policy-template.json').read_text())
 require(policy['AutomaticEnabled'] is False and policy['OwnerTrust']=={},'Locked owner-neutral policy required')
 return p,policy

def generate(root,stock,owner_path,out,go='go',clang='clang',lld='ld.lld',predecessors=(),zig='zig',modules=()):
 from .sourcebuild import generate_sources
 return generate_sources(Path(root),Path(stock),owner_path,Path(out),go,clang,lld,predecessors,zig=zig,modules=modules)
