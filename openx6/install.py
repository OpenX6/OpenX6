"""Build complete inactive-slot scripts and a source-built, still-locked guard."""
import base64,json,os,re,shutil,subprocess,hashlib
from pathlib import Path
from .common import require,digest,sha,canonical,save
from .official import ZIP_SHA
from .package import SCRIPTS,verify,pack
from .sourcebuild import elf_check
PUBLICS=['ins/etc/x6_owner_authorized_keys','ins/etc/x6_owner_gui_ed25519.pub','ins/etc/x6_owner_factory_ed25519.pub','ins/etc/x6_owner_update_ed25519.pub']

def scripts(original,system_sha1,blocks):
    require(re.fullmatch('[0-9a-f]{40}',system_sha1) and blocks==786432,'System geometry/digest rejected')
    require(sha(original)==json.loads((Path(__file__).parents[1]/'install-guard/platform.json').read_text())['UpdaterScriptSHA256'],'Unknown original updater script')
    text=original.decode();require(text.count('copy_all_source_partitions_except("system,boot,vendor_boot,dtbo")')==1,'Copy exclusions rejected')
    text,n=re.subn(r'\(package_extract_file\("firmware-update/storsec.mbn".*?abort\("Failed to extract firmware-update/storsec.mbn[^\n]+\n','',text,flags=re.S);require(n==1,'Shared-write removal ambiguous')
    start=text.index('if range_sha1(');end=text.index('show_progress(0.050000, 5);',start)
    check=f'assert(range_sha1("/dev/block/bootdevice/by-name/system", "2,0,{blocks}") == "{system_sha1}");\n'
    text=text[:start]+check+text[end:]
    def guard(phase):
        return 'assert(package_extract_file("x6-install-guard", "/tmp/x6-install-guard"));\nset_perm(0, 0, 0755, "/tmp/x6-install-guard");\nassert(run_program("/tmp/x6-install-guard", "'+phase+'") == "0");\n'
    return {SCRIPTS[0]:(guard('pre')+text+'\n'+guard('post')).encode(),SCRIPTS[1]:(guard('pre')+check+guard('post')+'assert(run_program("/sbin/modprobe", "-r", "mtdblock") == "0");\n').encode()}

def feature_sets(manifest,predecessors):
    sets=[]
    for entries in [manifest['Entries'],*[p['Entries'] for p in predecessors]]:
        require(len(entries)==9,'Expected complete feature layout')
        d={'data/x6-owner-features/'+e['Target']:e['SHA256'] for e in entries}
        require(len(d)==9 and all(re.fullmatch('[0-9a-f]{64}',v) for v in d.values()),'Feature digests rejected')
        if d not in sets:sets.append(d)
    return sets

def configuration(build,report,install_allowed=False):
    root=Path(__file__).parents[1]/'install-guard';platform=json.loads((root/'platform.json').read_text());c={k:platform[k] for k in ['AbctlSHA256','Sources','Parts']}
    require(type(install_allowed) is bool,'Invalid explicit install policy');c['InstallAllowed']=install_allowed;c['OwnerTrust']={p:digest(build/'overlay'/p) for p in PUBLICS};c['PublicSHA256']=c['OwnerTrust'][PUBLICS[-1]]
    release=json.loads((build/'release-inputs.json').read_text());seed=json.loads((build/'overlay/usr/share/x6-firstboot-seed/manifest.json').read_text());c['FeatureSets']=feature_sets(seed,release['predecessors'])
    # Older layouts carry only public program hashes. The source installation
    # supplies its own trusted Holder and preferences at maintenance time.
    layouts=json.loads((root.parent/'recipes/single_capture/predecessor-layouts.json').read_bytes())
    from .migration import TARGETS as seed_paths
    for prior in layouts:
        items=prior['entries']
        require(items.keys()==seed_paths and all(re.fullmatch('[0-9a-f]{64}',h) for h in items.values()),'Invalid historic program layout')
        mapping={'data/x6-owner-features/'+n:h for n,h in items.items()}
        if mapping not in c['FeatureSets']:c['FeatureSets'].append(mapping)
    require(len(c['FeatureSets'])<=17,'Too many feature layouts')
    if (build/'overlay/usr/share/x6-holder-layout').is_file():
        c['TargetSeed']=digest(build/'overlay/usr/share/x6-firstboot-seed/manifest.json')
        c['InitialKeys']={name:base64.b64encode((build/'overlay'/name).read_bytes()).decode() for name in [*PUBLICS,'adb_keys']}
    new_bytes=sum(e['Bytes'] for e in seed['Entries']);migration_bytes=(2*new_bytes if 'TargetSeed' in c else new_bytes)+(32<<20)
    for old in release['predecessors']:
        migration_bytes=max(migration_bytes,(32<<20)+new_bytes+sum(e['Bytes'] for e in old['Entries'])+(old['LegacySSH'] or {'Bytes':0})['Bytes']+(build/'overlay/usr/local/sbin/x6-owner-maintenance').stat().st_size)
    for prior in layouts:
        migration_bytes=max(migration_bytes,(32<<20)+new_bytes+prior['program_bytes']+prior['ssh_bytes']+(build/'overlay/usr/local/sbin/x6-owner-maintenance').stat().st_size)
    c['MinimumDataFreeBytes']=migration_bytes+(64<<20)
    updater=digest(build/'overlay/usr/bin/ins_otad');current={'Version':'1.2.36','OTASHA256':updater,'Mode':'maintenance'}
    if current not in c['Sources']:c['Sources'].append(current)
    for p,h in c['OwnerTrust'].items():require(release['owner_public_sha256']['/'+p]==h,'Owner trust differs from source build')
    import zipfile
    by={p['Label']:p for p in c['Parts']};require(len(by)==64,'Platform mapping rejected')
    with zipfile.ZipFile(build/'official/qcs8625_ota.zip') as z:
        original=z.read(SCRIPTS[0]);pairs=re.findall(rb'package_extract_file\("([^"]+)", "/dev/block/bootdevice/by-name/([^"]+)"\)',original);require(len(pairs)==27,'Official write list rejected')
    targets={'system':'system.new.dat'};targets.update({label.decode():name.decode() for name,label in pairs if label!=b'storsec'});require(len(targets)==27,'Payload set rejected')
    c['Targets']=[]
    for label,name in sorted(targets.items()):
        member=report['members'][name];size=by[label+'_a']['Bytes'];require(size==by[label+'_b']['Bytes'] and 0<member['bytes']<=size,'Target bounds rejected')
        c['Targets'].append(dict(Label=label,Device='',Bytes=size,CheckBytes=member['bytes'],SHA256=member['sha256']))
    return c,original

def verify_installer(out,allow_install=False):
    out=Path(out);d=out/'installer';meta=json.loads((d/'build.json').read_text());c=json.loads((d/'guard-config.json').read_text())
    require(c['InstallAllowed'] is False or (allow_install is True and c['InstallAllowed'] is True),'Installation guard unlocked');require(meta['install_allowed']==c['InstallAllowed'],'Guard policy differs from build record')
    require(digest(d/'guard-config.json')==meta['config_sha256'],'Guard configuration changed')
    for n,h in meta['sources'].items():require(digest(d/n)==h,'Guard source changed')
    require(digest(d/'x6-install-guard')==meta['binary_sha256'],'Guard executable changed');elf_check((d/'x6-install-guard').read_bytes())
    expected=scripts((d/'official-updater-script').read_bytes(),meta['system_sha1'],meta['system_blocks'])
    for name,b in expected.items():require((d/Path(name).name).read_bytes()==b,'Recovery script changed')
    # This exact embedded JSON is consumed by the compiled binary; no env/file policy override.
    require((d/'guard-config.json').read_bytes() in (d/'x6-install-guard').read_bytes(),'Embedded guard policy missing')
    return expected,d/'x6-install-guard'

def prepare(build,out,go='go'):
    import zipfile
    build=Path(build).resolve();out=Path(out).resolve();r=verify(build);require(not out.exists(),'Refuse existing release directory');require(digest(build/'official/qcs8625_ota.zip')==ZIP_SHA,'Official ZIP changed');image=build/'system-patched.img';im=json.loads((build/'image.json').read_text());require(digest(image)==im['sha256']==r['members']['system.new.dat']['sha256'],'Source image changed')
    require(shutil.disk_usage(out.parent).free>10*1024**3,'At least 10 GiB free required');out.mkdir(mode=0o700);root=Path(__file__).parents[1]/'install-guard';d=out/'installer';shutil.copytree(root,d)
    c,original=configuration(build,r);save(d/'guard-config.json',c);(d/'official-updater-script').write_bytes(original)
    env=dict(os.environ,GO111MODULE='off',CGO_ENABLED='0',GOTOOLCHAIN='local',GOPROXY='off',GOSUMDB='off',GOWORK='off')
    go=str(Path(shutil.which(go) or go).resolve())
    def run(cmd,e):
        p=subprocess.run(cmd,cwd=d,env=e,capture_output=True);(d/('native-test-compile.log' if '-c' in cmd else 'tests.log' if 'test' in cmd else 'compile.log')).write_bytes(p.stdout+p.stderr);require(p.returncode==0,'Guard build/test failed; see installer logs')
    run([go,'test','-v','.'],env)
    run([go,'build','-trimpath','-ldflags=-s -w -buildid=','-o','x6-install-guard','.'],dict(env,GOOS='linux',GOARCH='arm64',GOARM64='v8.0'))
    run([go,'test','-c','-o','guard-tests-arm64','.'],dict(env,GOOS='linux',GOARCH='arm64',GOARM64='v8.0'))
    ih=digest(image,'sha1');generated=scripts(original,ih,image.stat().st_size//4096)
    for n,b in generated.items():(d/Path(n).name).write_bytes(b)
    save(d/'build.json',dict(config_sha256=digest(d/'guard-config.json'),sources={p.name:digest(p) for p in d.glob('*.go')},binary_sha256=digest(d/'x6-install-guard'),system_sha1=ih,system_blocks=image.stat().st_size//4096,go_sha256=digest(go),install_allowed=False))
    verify_installer(out)
    # Independent copies; do not hard-link mutable source artifacts.
    shutil.copyfile(image,out/'system-patched.img');save(out/'image.json',im);shutil.copyfile(build/'release-inputs.json',out/'release-inputs.json')
    result=pack(build/'official',out/'system-patched.img',out,r['release'],installer=(generated,d/'x6-install-guard'))
    save(out/'RESULT.json',dict(status='SOURCE_BUILT_INSTALLER_VALIDATED_GUARD_LOCKED',source_build=str(build),candidate=result['file'],sha256=result['sha256'],release=result['release'],install_allowed=False,signed=False,camera_IO=False,original_candidate_unchanged=digest(build/r['file'])==r['sha256'],features=json.loads((build/'RESULT.json').read_text())['features']))
    return result
