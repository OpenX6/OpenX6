"""Dependency-ordered, offline source builds and generated integrity manifests."""
import json,os,shutil,subprocess,struct,base64,zlib
from pathlib import Path
from .common import require,sha,digest,canonical,save,asset
from .fsimage import read_file
from .patch import apply_recipe
from .owner import validate_owner,ED,SSH
from .profile import load_profile,TARGETS
from .migration import load_predecessors

GUI='/usr/bin/ins_gui'; SETUP='/usr/local/sbin/x6-owner-gui-setup'
BRIDGE='/usr/lib/libx6-owner-gui-bridge.so'; SEED='/usr/share/x6-firstboot-seed/'
CONVERTER='/usr/lib/x6-pureshot/ps2native'
GUI_CHECKS={
 '/ins/etc/systemd/system/multi-user.target.wants/ins_gui.service':'0644',
 '/lib/ld-linux-aarch64.so.1':'0755',
 '/lib/systemd/system/ins_gui.service':'0644',
 '/lib/systemd/system/ins_gui.service.d/50-owner-gui.conf':'0644',
 '/lib/systemd/system/ins_gui.service.d/42-openx6-runtime.conf':'0644',
 '/lib/systemd/system/x6-owner-gui-auth.service':'0644',
 '/lib/systemd/system/x6-owner-gui-auth.service.d/05-x6-boot-rescue.conf':'0644',
 '/lib/systemd/system/x6-owner-gui-late-start.service':'0644',
 '/lib/systemd/system/x6-owner-gui-late-start.service.d/05-x6-boot-rescue.conf':'0644',
 GUI:'0755','/usr/lib/libins_gui_wrap.so':'0755',BRIDGE:'0755',
 '/usr/local/sbin/x6-owner-gui-auth':'0755'}

def resolve_tool(name,args):
 path=Path(shutil.which(name) or name).resolve();require(path.is_file(),'Missing tool: '+name)
 result=subprocess.run([str(path),*args],capture_output=True,check=True)
 # Clang's later lines contain its installation directory. The version line
 # and executable hash identify the tool independently of the user's path.
 version=result.stdout.decode().splitlines()
 require(bool(version) and version[0].strip(),'Missing tool version: '+path.name)
 return str(path),dict(sha256=digest(path),version=version[0].strip())

def elf_check(b,shared=False):
 require(b[:6]==b'\x7fELF\x02\x01' and len(b)>=64,'Expected 64-bit LE ELF')
 require(struct.unpack_from('<HH',b,16)==(3 if shared else 2,183),'Expected ARM64 ELF type')
 off=struct.unpack_from('<Q',b,32)[0];size,count=struct.unpack_from('<HH',b,54)
 require(size>=56 and count>0 and off+size*count<=len(b),'ELF program header bounds')
 headers=[struct.unpack_from('<II',b,off+size*i) for i in range(count)]
 if not shared:require(not any(t==3 for t,f in headers),'Static helper unexpectedly needs an interpreter')
 require(not any(t==0x6474e551 and f&1 for t,f in headers),'Executable stack rejected')
 return dict(architecture='aarch64',type='shared' if shared else 'static executable')

def elf_check_glibc(b):
 # The converter is the one dynamic helper: it dlopens the vendor PureShot engine, so it needs the camera's own
 # glibc loader -- nothing else (no rpath tricks, no executable stack).
 require(b[:6]==b'\x7fELF\x02\x01' and len(b)>=64,'Expected 64-bit LE ELF')
 require(struct.unpack_from('<HH',b,16)[1]==183 and struct.unpack_from('<H',b,16)[0] in (2,3),'Expected ARM64 executable')
 off=struct.unpack_from('<Q',b,32)[0];size,count=struct.unpack_from('<HH',b,54)
 require(size>=56 and count>0 and off+size*count<=len(b),'ELF program header bounds')
 interp=[]
 for i in range(count):
  t,f=struct.unpack_from('<II',b,off+size*i);o,fs=struct.unpack_from('<Q',b,off+size*i+8)[0],struct.unpack_from('<Q',b,off+size*i+32)[0]
  require(not (t==0x6474e551 and f&1),'Executable stack rejected')
  if t==3:interp.append(b[o:o+fs].rstrip(b'\0'))
 require(interp==[b'/lib/ld-linux-aarch64.so.1'],'Unexpected program interpreter')
 return dict(architecture='aarch64',type='glibc executable')

def generate_sources(root,stock,owner_path,out,go,clang,lld,predecessors=(),automatic_sd=False,zig='zig',project_public=None,modules=(),program_sources=None):
 require(type(automatic_sd) is bool,"Invalid explicit release policy")
 p,policy=load_profile(root);owner=validate_owner(owner_path)
 previous=load_predecessors(predecessors,owner,layout_only=True)
 go,gov=resolve_tool(go,['version']);clang,ccv=resolve_tool(clang,['--version']);lld,ldv=resolve_tool(lld,['-flavor','gnu','--version'])
 zig,zigv=resolve_tool(zig,['version']);strip,stripv=resolve_tool(str(Path(clang).with_name('llvm-strip')),['--version'])
 frozen=dict(automatic_sd=automatic_sd,predecessors=previous,profile_sha256=sha(canonical(p)),engine={f.name:digest(f) for f in sorted(Path(__file__).parent.glob('*.py'))},
             tools={'go':gov,'clang':ccv,'lld':ldv,'zig':zigv,'llvm-strip':stripv},owner_public_sha256={k:sha(v) for k,v in sorted(owner.items())})
 if project_public is not None:
  from .owner import ed25519_public
  project_public=ed25519_public(project_public)
  require(project_public not in owner.values() and project_public != base64.b64decode(owner[SSH].split()[1])[-32:], 'Project and holder keys must be distinct')
  frozen['project_update_public']=project_public.hex()
  p['features']=[*p['features'], 'Optional-project-update-signature']
 menus,binary=[],[]
 if modules:
  from . import modules as extension
  require(not automatic_sd,'Third-party modules require a locked candidate')
  menus,binary=extension.split_selection(modules,p['actions'][extension.TARGET]['sha256'])
 if menus:
  from .compiled_recipes import SOURCES
  from sdk import menu_preview,choice
  source_names=['extension_pages.inc','menu.S','menu-symbols.json']
  if any(m['schema']==choice.SCHEMA for m in menus):source_names.append('choice.c')
  frozen['extension_menu'] = dict(manifests=menus,manifests_sha256=sha(canonical(menus)),
       source_sha256={name:digest(SOURCES/'single_menu'/name) for name in source_names},
       validators={Path(m.__file__).name:digest(Path(m.__file__)) for m in (menu_preview,choice)})
 if binary:
  from sdk.module_plan import render
  extension.validate_selection(binary,root,p)
  spec=p['actions'][extension.TARGET]
  recipe=json.loads(asset(root,spec['asset']).read_bytes())
  core=apply_recipe(read_file(stock,extension.TARGET),recipe)
  require(sha(core)==spec['sha256'],'Built-in module base rejected')
  report,extended=render(core,extension.TARGET,binary)
  frozen['third_party_modules']=extension.receipt(binary,report)
 from . import programs
 program_files,program_links=programs.layout(menus,program_sources or {},go)
 if program_files:
  frozen['executable_modules']=programs.receipt(program_files,program_links)
 release=sha(canonical(frozen));save(out/'release-inputs.json',frozen)
 overlay=out/'overlay';overlay.mkdir();build=out/'component-source';shutil.copytree(root/'src',build)
 if project_public is not None:
  key_bytes=','.join(str(x) for x in project_public)
  (build/'verifier/project_build.go').write_text('package main\nfunc init() { projectUpdatePublic = []byte{'+key_bytes+'} }\n')
 env=dict(os.environ,GO111MODULE='on',GOPROXY='off',GOSUMDB='off',GOWORK='off',GOTOOLCHAIN='local',
          GOOS='linux',GOARCH='arm64',GOARM64='v8.0',CGO_ENABLED='0',GOFLAGS='')
 entries={};compilations=[];seed_files={};cache={}
 def base(name):
  if name not in cache:
   f=overlay/name.lstrip('/');cache[name]=f.read_bytes() if f.is_file() else read_file(stock,name)
  require(cache[name] is not None,'Missing base '+name);return cache[name]
 def put(name,b):
  spec=p['actions'][name];f=overlay/name.lstrip('/');f.parent.mkdir(parents=True,exist_ok=True);f.write_bytes(b);f.chmod(int(spec['mode'],8))
  entries[name]=dict(sha256=sha(b),mode=spec['mode'],selinux=spec['selinux']);cache[name]=b
 def run(command):
  r=subprocess.run(command,cwd=build,env=env,capture_output=True)
  with (out/'component-build.log').open('ab') as f:f.write((' '.join(command)+'\n').encode()+r.stdout+r.stderr)
  require(r.returncode==0,'Source compilation failed; see component-build.log')
 def compile_go(target,package,extra_ldflags=""):
  f=out/'compiled'/target;f.parent.mkdir(parents=True,exist_ok=True)
  run([go,'build','-mod=vendor','-trimpath','-ldflags=-s -w -buildid='+extra_ldflags,'-o',str(f.resolve()),'./'+package])
  b=f.read_bytes();meta=elf_check(b);compilations.append(dict(target=target,source=package,sha256=sha(b),**meta));return b
 for name,s in p['actions'].items():
  kind=s['kind']
  if kind=='owner-public':put(name,owner[name])
  elif kind=='release':put(name,(release+'\n').encode())
  elif kind=='official-component':
   from .official import COMPONENTS
   b=(stock.parent/s['component']).read_bytes();n,want=COMPONENTS[s['component']]
   require(len(b)==n and sha(b)==want==s['sha256'],'Official component rejected');put(name,b)
  elif kind in ('patch','asset'):
   if binary and name==extension.TARGET:
    put(name,extended)
   else:
    b=apply_recipe(base(name),json.loads(asset(root,s['asset']).read_text())) if kind=='patch' else asset(root,s['asset']).read_bytes()
    require(sha(b)==s['sha256'],'Static recipe output differs');put(name,b)
 from .ssd_recipe import verify_installed_hooks
 verify_installed_hooks(base, dict(clang=clang, lld=lld))
 from . import phone_route, proxy_profile
 route=phone_route.build(base(phone_route.PATH),(build/'native/mobile-upload.nginx').read_bytes())
 require(sha(route)==p['actions'][phone_route.PATH]['sha256'],'Mobile upload route digest differs')
 put(phone_route.PATH,route)
 proxy_data=proxy_profile.build(base(proxy_profile.ORIGINAL_PATH))
 require(sha(proxy_data)==p['actions'][proxy_profile.PROFILE_PATH]['sha256'],'Recording profile digest mismatch')
 put(proxy_profile.PROFILE_PATH,proxy_data)
 # Reconstruct media variants independently from stock-derived files. Embed only
 # sparse edits in a source-built preparer, rather than shipping library copies.
 variants=[]
 for v in p.get('mod_runtime',[]):
  recipe=json.loads(asset(root,v['recipe']).read_text());target=apply_recipe(base(v['base']),recipe)
  variants.append(dict(Base=v['base'],Runtime=v['runtime'],BaseSHA=recipe['base_sha256'],SHA=sha(target),Size=len(target),Edits=[dict(Offset=e['offset'],Before=e['before'],Data=base64.b64encode(zlib.decompress(base64.b64decode(e['data']))).decode()) for e in recipe['edits']]))
 save(build/'mod-prepare/manifest.json',variants)
 fixtures=out/'mod-runtime-inputs'
 for v in variants:
  f=fixtures/v['Base'].lstrip('/');f.parent.mkdir(parents=True,exist_ok=True);f.write_bytes(base(v['Base']))
 hostenv=dict(os.environ,GOTOOLCHAIN='local',GOWORK='off',GOPROXY='off',GOSUMDB='off',CGO_ENABLED='0',X6_MOD_BASE_DIR=str(fixtures.resolve()))
 for k in ('GOOS','GOARCH','GOARM64','GOFLAGS'):hostenv.pop(k,None)
 check=subprocess.run([go,'test','-mod=vendor','-count=1','-v','./mod-prepare'],cwd=build,env=hostenv,capture_output=True)
 (out/'mod-runtime-tests.log').write_bytes(check.stdout+check.stderr)
 require(check.returncode==0,'Generated runtime reconstruction/lifecycle rejected; see mod-runtime-tests.log')

 for package in ['mod-stereo','mod-hnr','mod-vnr','proxy-control','mod-prepare','stats','zebra']:
  name=next(k for k,v in TARGETS.items() if v==package);put(name,compile_go(name.lstrip('/'),package))
 save(out/'mod-controls-build.json',dict(variants=variants,gui_single_seed=True,preferences='preserve other settings; retire hnr/spatial choices to native=0; fresh stereo=0 proxy_disabled=1'))
 # No libc/sysroot downloads: this bridge uses declared vendor/process imports
 # and direct syscalls. Original vendor loader resolves them at runtime.
 pool=(out/'compiled/pool.so').resolve()
 run([zig,'cc','-target','aarch64-linux-gnu.2.35','-O2','-s','-fPIC','-mbranch-protection=standard','-shared','-Wl,-z,relro,-z,now','-o',str(pool),'square8/pool.c','square8/input.c','-ldl'])
 pool_path='/usr/lib/x6-freeframe-square8-pool.so'
 run([strip,'--remove-section=.comment',str(pool)])
 pb=pool.read_bytes()
 elf_check(pb,True)
 require(sha(pb)==p['actions'][pool_path]['sha256'],'Square HEVC buffer helper differs from tested build')
 put(pool_path,pb)
 obj=(out/'bridge.o').resolve();lib=(out/'compiled/gui-bridge.so').resolve();lib.parent.mkdir(exist_ok=True)
 run([clang,'--target=aarch64-linux-gnu','-Os','-ffreestanding','-fno-stack-protector','-fno-unwind-tables','-fno-asynchronous-unwind-tables','-fPIC','-mbranch-protection=bti+pac-ret','-c','gui-bridge/bridge.c','-o',str(obj)])
 run([lld,'-flavor','gnu','-shared','-z','relro','-z','now','-z','noexecstack','--soname=libx6-owner-gui-bridge.so','-o',str(lib),str(obj)])
 b=lib.read_bytes();compilations.append(dict(target=BRIDGE,source='gui-bridge/bridge.c + gate.c',sha256=sha(b),**elf_check(b,True)));put(BRIDGE,b)
 for package in ('ssh','usb','gui-auth','nr-control','boot-guard','wifi-root'):
  name=next(k for k,v in TARGETS.items() if v==package);put(name,compile_go(name.lstrip('/'),package))
 # The converter: zig cross-compiles against its bundled glibc 2.35 stubs (the camera has 2.35), then every symbol
 # and debug section is stripped, so the bytes do not depend on where it was built. They must equal the pinned
 # hash of this profile’s converter with the official snapshot tuning.
 conv=(out/'compiled/ps2native').resolve();conv.parent.mkdir(parents=True,exist_ok=True)
 zenv=dict(env,ZIG_GLOBAL_CACHE_DIR=str((out/'zig-cache/global').resolve()),ZIG_LOCAL_CACHE_DIR=str((out/'zig-cache/local').resolve()))
 for command in ([zig,'cc','-target','aarch64-linux-gnu.2.35','-O2','pureshot/ps2native/ps2native.c','-ldl','-lm','-o',str(conv)],[strip,'--strip-all',str(conv)]):
  r=subprocess.run(command,cwd=build,env=zenv,capture_output=True)
  with (out/'component-build.log').open('ab') as f:f.write((' '.join(command)+'\n').encode()+r.stdout+r.stderr)
  require(r.returncode==0,'Converter compilation failed; see component-build.log')
 b=conv.read_bytes();require(sha(b)==p['actions'][CONVERTER]['sha256'],'Converter build differs from the pinned stripped bytes')
 compilations.append(dict(target=CONVERTER,source='pureshot/ps2native/ps2native.c',sha256=sha(b),**elf_check_glibc(b)));put(CONVERTER,b)
 script=(build/'scripts/ssh-late-start.sh.in').read_text()
 script=script.replace('@SSH_PUBLIC_SHA256@',sha(owner[SSH])).replace('@SSH_EXECUTABLE_SHA256@',entries['/usr/local/sbin/x6-owner-maintenance']['sha256'])
 require('@' not in script,'Unresolved startup template')
 put('/usr/local/sbin/x6-owner-late-start',script.encode())
 put('/ins/etc/x6_owner_patch_manifest.json',json.dumps(dict(files={n:entries[n]['sha256'] for n in p['access_manifest_files']}),indent=2).encode()+b'\n')
 # Rebuild feature vendor deltas before both startup executables. No cached seed.
 for name,s in p['seed'].items():
  if name.endswith('/setup'):continue
  b=base('/'+s['base'])
  if s['kind']=='patch':b=apply_recipe(b,json.loads(asset(root,s['asset']).read_text()));require(sha(b)==s['sha256'],'Feature recipe differs')
  seed_files[name]=b
 from .menu_recipe import bind_menus
 from .release_gui import fallback_gui, feature_gui, consent_gui, DISPLAY
 require(p['display_version']==DISPLAY, 'Unsupported display version')
 gui_tools={'clang':clang,'lld':lld}
 name='vr180-v1/gui'
 standard=seed_files[name]
 selected=bind_menus(standard,menus,gui_tools) if menus else standard
 fallback=consent_gui(fallback_gui(base(GUI),gui_tools),gui_tools)
 require(sha(fallback)==p['gui_digests']['fallback'] and sha(standard)==p['gui_digests']['feature'],'OpenX6 GUI inputs differ')
 put(GUI,fallback)
 seed_files['vr180-v1/backup/gui']=base(GUI)
 seed_files[name]=consent_gui(feature_gui(selected,gui_tools,extension_menu=bool(menus)),gui_tools)
 if not menus:require(sha(seed_files[name])==p['gui_digests']['finished'],'OpenX6 menu output differs')
 # Bind the running-model reader to the actual picker in this menu layout.
 from .fisheye import reader_signature
 signature=reader_signature(seed_files[name])
 preview=next(k for k,v in TARGETS.items() if v=='preview')
 put(preview,compile_go(preview.lstrip('/'),'preview',' -X main.modeSignature='+signature))
 if menus:
  save(out/'menu-build.json',dict(target=name,base_sha256=sha(standard),output_sha256=sha(seed_files[name]),
       modules=[m['id'] for m in menus],capture_controls=True))
 from .live_exposure import patch as show_exposure
 from .zebra import patch as highlight_warning
 exposed = show_exposure(seed_files[name],build/'native/live-exposure',clang,lld)
 seed_files[name] = highlight_warning(exposed,build/'native/zebra',clang,lld,sha(exposed))
 def setup_source(gui):
  values={name:dict(SHA=sha(gui if name==GUI else base(name)),Mode=mode) for name,mode in GUI_CHECKS.items()}
  text='package main\n\nvar expectedFiles = map[string]fileSpec{\n'
  text+=''.join(json.dumps(n)+': {SHA:'+json.dumps(v['SHA'])+',Mode:'+v['Mode']+'},\n' for n,v in sorted(values.items()))+'}\n'
  text+='var stockGUIUnit = fileSpec{SHA:'+json.dumps(sha(base('/lib/systemd/system/ins_gui.service')))+',Mode:0644}\n'
  (build/'gui-setup/expected.go').write_text(text)
  return values
 checks_base=setup_source(base(GUI));put(SETUP,compile_go(SETUP.lstrip('/'),'gui-setup'));seed_files['vr180-v1/backup/setup']=base(SETUP)
 (out/'startup-stock').mkdir()
 (out/'startup-stock/ins_gui.service').write_bytes(base('/lib/systemd/system/ins_gui.service'))
 (out/'startup-stock/loader').write_bytes(base('/lib/ld-linux-aarch64.so.1'))
 # Compile the feature variant from the SAME module path, changing only pinned
 # GUI bytes. This keeps the seed compact and does not encode a build location.
 checks_feature=setup_source(seed_files['vr180-v1/gui']);seed_files['vr180-v1/setup']=compile_go('feature/gui-setup','gui-setup')
 import runpy
 rawsrc=root/'src/native/raw-video'
 runtime=runpy.run_path(str(rawsrc/'build_runtime.py'))['build'](rawsrc,zig,strip,base('/usr/lib/com.qti.feature2.rtmcx.so'))
 path='/usr/share/x5-raw-video/'
 put(path+'runtime.tar',runtime)
 recipe=json.loads(base(path+'ui-recipe.json'))
 inputgui=seed_files['vr180-v1/gui']
 from .raw_menu import bind_recipe
 recipe = bind_recipe(recipe,inputgui)
 put(path+'ui-recipe.json',canonical(recipe))
 require(sha(inputgui)==recipe['pin'] and len(inputgui)==recipe['end'],'RAW GUI base mismatch')
 rawgui=bytearray(inputgui)
 for off,was,now in recipe['edits']:
  before=bytes.fromhex(was);require(rawgui[off:off+len(before)]==before,'RAW GUI hook mismatch')
  rawgui[off:off+len(before)]=bytes.fromhex(now)
 setup=seed_files['vr180-v1/setup']; require(setup.count(recipe['pin'].encode())==1,'RAW startup GUI pin missing')
 pins=dict(gui=sha(rawgui),base_setup=sha(setup),setup=sha(setup.replace(recipe['pin'].encode(),sha(rawgui).encode())),recipe=sha(base(path+'ui-recipe.json')),runtime=sha(runtime))
 put(path+'pins.json',canonical(pins))
 shutil.copyfile(build/'gui-setup/expected.go',out/'gui-setup-feature-expected.go');setup_source(base(GUI))
 expected='package main\nvar payloads = []payload{\n'
 for key,path,label,mode in [('gui',GUI,'bin_t','0755'),('setup',SETUP,'bin_t','0755'),('sync.so','/usr/lib/hw/com.qti.chi.override.so','lib_t','0644')]:
  expected+='{'+','.join(json.dumps(v) for v in [key,path,sha(base(path)),sha(seed_files['vr180-v1/'+key]),label])+','+mode+'},\n'
 (build/'vr180-control/expected.go').write_text(expected+'}\n')
 put('/usr/local/sbin/x6-vr180-control',compile_go('usr/local/sbin/x6-vr180-control','vr180-control'))
 blob=bytearray();records=[]
 for name,s in sorted(p['seed'].items()):
  source=base('/'+s['base']);target=seed_files[name];chunks=[]
  f=out/'seed-targets'/name;f.parent.mkdir(parents=True,exist_ok=True);f.write_bytes(target);f.chmod(s['mode'])
  for offset in range(0,len(target),4096):
   chunk=target[offset:offset+4096]
   if chunk!=source[offset:offset+4096]:chunks.append(dict(Offset=offset,BlobOffset=len(blob),Length=len(chunk)));blob.extend(chunk)
  records.append(dict(Target=name,Base=s['base'],BaseSHA256=sha(source),BaseBytes=len(source),SHA256=sha(target),Bytes=len(target),Mode=s['mode'],Chunks=chunks))
 manifest=dict(Version=1,BlobSHA256=sha(blob),BlobBytes=len(blob),Entries=records)
 raw=(json.dumps(manifest,separators=(',',':'))+'\n').encode();put(SEED+'manifest.json',raw);put(SEED+'delta.bin',blob)
 require(len(blob)<16*1024**2,'Seed exceeds target bound')
 save(build/'bootstrap/migration-policy.json',dict(LocalHolder=True,Owners={},Previous=[]))
 (build/'bootstrap/expected.go').write_text('package main\nconst productionSeed = '+json.dumps(sha(raw))+'\n')
 put('/usr/local/sbin/x6-firstboot',compile_go('usr/local/sbin/x6-firstboot','bootstrap'))
 policy['AutomaticEnabled']=automatic_sd;policy['Release']=release;policy['OwnerTrust']={}
 for name in ['/usr/local/sbin/x6-owner-maintenance','/usr/local/sbin/x6-owner-gui-auth',BRIDGE]:policy['Files'][name]=[entries[name]['sha256']]
 policy['Files'][GUI]=[entries[GUI]['sha256'],sha(seed_files['vr180-v1/gui'])]
 save(build/'verifier/coordinator-policy.json',policy)
 put('/usr/local/sbin/x6-owner-update-check',compile_go('usr/local/sbin/x6-owner-update-check','verifier'))
 require(set(entries)==set(p['actions']),'Incomplete build graph')
 if program_files:
  programs.install(program_files,program_links,stock,overlay,entries,p)
 # Independent reconstruction from generated records, including the compiled
 # feature startup. This checks emitted seed offsets, not just target hashes.
 for e in records:
  source=base('/'+e['Base']);b=bytearray(source[:e['Bytes']]);b.extend(bytes(e['Bytes']-len(b)));end=0
  for c in e['Chunks']:
   a,n,v=c['Offset'],c['Length'],c['BlobOffset'];require(a>=end and n>0 and a+n<=len(b) and 0<=v<=len(blob)-n,'Seed bounds')
   b[a:a+n]=blob[v:v+n];end=a+n
  require(bytes(b)==seed_files[e['Target']] and sha(b)==e['SHA256'],'Seed readback differs')
 save(out/'seed-checks.json',[dict(target=e['Target'],sha256=e['SHA256']) for e in records])
 save(out/'source-build.json',dict(compilations=compilations,gui_base_checks=checks_base,gui_feature_checks=checks_feature,coordinator_policy=policy,seed_manifest_sha256=sha(raw),seed_bytes=len(raw)+len(blob),no_precompiled_helpers=True))
 return p,entries,release
