"""Prepare source-pinned Python handlers for locked full builds."""
import ast
import os
import re
import subprocess
import tempfile
from pathlib import Path

from .common import canonical, require, sha
from .fsimage import read_file
from sdk import choice
from sdk.module_plan import read_bounded

RUNTIME = Path(__file__).resolve().parents[1]/'recipes/module_runtime'
LIB = '/usr/lib/openx6-modules'
UNITS = '/lib/systemd/system'
PUBLISHER = 'openx6-module-input.service'
PREPARE = 'openx6-module-preferences.service'
LABELER = LIB+'/prepare-labels'
CONTEXTS = '/etc/selinux/mls/contexts/files/file_contexts.local'
UNIT_CONTEXT = 'system_u:object_r:systemd_unit_t:s0'
MIME = 'system_u:object_r:{}:s0'


def snapshot(paths, modules):
    require(len(paths) == len(modules), 'Manifest/source association differs')
    sources = {}
    for path, module in zip(paths, modules):
        if 'program' not in module:
            continue
        choice.validate(module)
        spec = module['program']
        handler = Path(path).parent/spec['source']
        source = read_bounded(handler, 65536)
        actual = sha(source)
        require(actual == spec['sha256'],
                f"{module['id']}: {handler.name} digest differs. After reviewing your edits, "
                f"set program.sha256 to {actual}")
        try:
            tree = ast.parse(source.decode('utf-8'), filename=spec['source'], feature_version=(3, 10))
            compile(tree, spec['source'], 'exec')
        except (SyntaxError, UnicodeError) as exc:
            raise ValueError(f"{module['id']}: invalid Python 3.10 source in {handler.name}: {exc}") from exc
        sources[module['id']] = source
    return sources


def file_attributes(path):
    if path == LABELER:
        return '0700', MIME.format('setfiles_exec_t')
    if path == CONTEXTS:
        return '0644', MIME.format('file_context_t')
    return '0644', UNIT_CONTEXT if path.startswith(UNITS+'/') else MIME.format('usr_t')


def startup_labels(links):
    labels = {}
    for path in links:
        labels[path] = labels[str(Path(path).parent)] = UNIT_CONTEXT
    return labels


def receipt(files, links):
    return dict(files={k:sha(v) for k,v in files.items()}, links=links,
                attributes={k:list(file_attributes(k)) for k in files}, startup_labels=startup_labels(links))


def compile_labeler(names, go):
    with tempfile.TemporaryDirectory() as tmp:
        output = Path(tmp)/'prepare-labels'
        env = dict(os.environ, GOOS='linux', GOARCH='arm64', GOARM64='v8.0', CGO_ENABLED='0',
                   GO111MODULE='off', GOWORK='off', GOENV='off', GOFLAGS='', GOTOOLCHAIN='local',
                   GOPROXY='off', GOSUMDB='off')
        subprocess.run([go,'build','-trimpath','-buildvcs=false',
                        '-ldflags=-s -w -buildid= -X main.names='+','.join(names),
                        '-o',str(output),str(RUNTIME/'prepare_labels.go')],env=env,check=True)
        return output.read_bytes()


def layout(modules, sources, go='go'):
    programs = sorted((m for m in modules if 'program' in m), key=lambda m:m['id'])
    require(set(sources) == {m['id'] for m in programs}, 'Missing or unexpected executable source')
    choices = [m for m in modules if m['schema'] == choice.SCHEMA]
    if not choices:
        require(not programs, 'Executable modules require a validated menu choice')
        return {}, {}
    files = {}
    # Stock initrc_state_t already permits the native GUI to save private service state.
    # Scope local restore rules to the directory and the exact approved menu filenames.
    for item in choices:
        choice.validate(item)
    names = sorted('openx6-choice.'+m['id']+'.'+m['setting']['id']+'.pref' for m in choices)
    prefdir = '/data/x6-owner-features/extensions-v1'
    state = MIME.format('initrc_state_t')
    contexts = [f'{prefdir} -d {state}', f'{LABELER} -- {MIME.format("setfiles_exec_t")}']
    for name in names:
        escaped = re.escape(name)
        contexts.append(f'{prefdir}/{escaped}(\\.next)? -- {state}')
    files[CONTEXTS] = ('\n'.join(contexts)+'\n').encode()
    files[LABELER] = compile_labeler(names, go)
    files[f'{LIB}/create_directory.py'] = (RUNTIME/'create_directory.py').read_bytes()
    links = {}
    gate = ('[Unit]\nWants=x6-boot-guard.service\nAfter=x6-boot-guard.service\n'
            'ConditionPathExists=/run/x6-boot/features-allowed\n')
    def service(name, body):
        files[f'{UNITS}/{name}'] = (gate+body).encode()
        links[f'{UNITS}/ins_phase3.target.wants/{name}'] = '../'+name
    service(PREPARE, f'''Requires=x6-runtime-prepare.service
After=x6-runtime-prepare.service
Before=ins_gui.service
[Service]
Type=oneshot
RemainAfterExit=yes
ExecStartPre=/usr/bin/python3 -I -B {LIB}/create_directory.py
ExecStart={LABELER}
CapabilityBoundingSet=
ProtectSystem=strict
ProtectHome=yes
PrivateNetwork=yes
PrivateDevices=yes
ReadWritePaths=/data/x6-owner-features
''')
    if not programs:
        return files, links
    files[f'{LIB}/catalog.json'] = canonical(programs)
    for script in ('publisher.py','worker.py'):
        files[f'{LIB}/{script}'] = (RUNTIME/script).read_bytes()
    service(PUBLISHER, f'''Requires={PREPARE}
After={PREPARE}
Before=ins_gui.service
[Service]
ExecStart=/usr/bin/python3 -I -B {LIB}/publisher.py {LIB}/catalog.json
RuntimeDirectory=openx6-module-input
RuntimeDirectoryMode=0755
Restart=on-failure
RestartSec=5
NoNewPrivileges=yes
CapabilityBoundingSet=
''')
    for module in programs:
        name = module['id']
        source = sources[name]
        require(sha(source) == module['program']['sha256'], 'Handler source changed')
        path = f'{LIB}/{name}/handler.py'
        files[path] = source
        # A digest gives valid short system accounts/units even for long IDs with underscores.
        ident = 'ox6m'+sha(name.encode())[:16]
        out = 'openx6-result-'+name
        service(ident+'.service', f'''Requires={PUBLISHER}
After={PUBLISHER}
[Service]
DynamicUser=yes
User={ident}
RuntimeDirectory={out}
RuntimeDirectoryMode=0700
ExecStart=/usr/bin/python3 -I -B {LIB}/worker.py {path} /run/openx6-module-input/{name} /run/{out}
Restart=on-failure
RestartSec=5
NoNewPrivileges=yes
CapabilityBoundingSet=
PrivateNetwork=yes
PrivateDevices=yes
ProtectSystem=strict
ProtectHome=yes
InaccessiblePaths=-/data -/mnt -/media -/run/dbus -/run/systemd/private
SystemCallFilter=~@network-io
RestrictAddressFamilies=AF_UNIX
RestrictSUIDSGID=yes
UMask=0077
TasksMax=8
MemoryMax=64M
CPUQuota=20%
LimitFSIZE=16384
KillMode=control-group
''')
    return files, links


def install(files, links, stock, overlay, entries, profile):
    for path, data in files.items():
        require(path not in entries and read_file(stock,path) is None, 'Extension path collision: '+path)
        dst = overlay/path.lstrip('/')
        dst.parent.mkdir(parents=True,exist_ok=True)
        dst.write_bytes(data)
        mode, label = file_attributes(path)
        dst.chmod(int(mode,8))
        entries[path] = dict(sha256=sha(data),mode=mode,selinux=label)
    require(not links.keys() & profile['links'].keys(), 'Extension startup link collision')
    profile['links'].update(links)
    profile['extension_startup_labels'] = startup_labels(links)
