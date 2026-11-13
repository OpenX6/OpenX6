"""Read back the Stats example from a locked full developer build (macOS/Linux)."""
import argparse
import json
import shutil
import tempfile
import zipfile
from pathlib import Path

from openx6.common import canonical, digest, require, sha
from openx6.fsimage import debug, quoted, read_file
from openx6 import programs
from openx6.menu_recipe import bind_menus, feature_menu
from openx6.toolchain import compiler_tools
from sdk.module_plan import unique_keys

CANDIDATE = 'OpenX6-1.2.36-S-LOCKED-UNSIGNED.mwfi.candidate'
SEED = '/usr/share/x6-firstboot-seed/'


def verify(directory, manifest, check_gui=None, check_programs=None):
    def report(name):
        return json.loads((directory/name).read_bytes(), object_pairs_hook=unique_keys)

    result = report('RESULT.json')
    paths = [manifest] if isinstance(manifest, Path) else manifest
    supplied = [json.loads(p.read_bytes(), object_pairs_hook=unique_keys) for p in paths]
    from openx6.modules import split_selection
    modules, binary = split_selection(supplied, None)
    require(not binary, 'Select only menu manifests for readback')
    sources = programs.snapshot(paths, supplied)
    ids = [m['id'] for m in modules]
    receipt = report('menu-build.json')
    inputs = report('release-inputs.json')
    guard = (directory/'guard-config.json').read_bytes()
    require(result['install_allowed'] is False and result['candidate'] == CANDIDATE
            and json.loads(guard)['InstallAllowed'] is False, 'Expected a locked build')
    recorded = inputs['extension_menu']
    if 'manifest' in recorded:  # First native Stats build.
        matches = (len(modules) == 1 and receipt['module'] == ids[0]
                   and recorded['manifest'] == modules[0]
                   and recorded['manifest_sha256'] == sha(canonical(modules[0])))
    else:
        matches = (receipt['modules'] == ids and recorded['manifests'] == modules
                   and recorded['manifests_sha256'] == sha(canonical(modules)))
    require(receipt['target'] == 'vr180-v1/gui' and matches, 'Selected menu manifests differ')
    package = directory/CANDIDATE
    require(digest(package) == result['sha256'], 'Candidate digest differs')
    with tempfile.TemporaryDirectory(dir=directory.parent) as tmp:
        image = Path(tmp)/'system.img'
        with zipfile.ZipFile(package) as z:
            require(guard in z.read('x6-install-guard'), 'Packed guard configuration differs')
            with z.open('system.new.dat') as src, image.open('xb') as dst:
                shutil.copyfileobj(src, dst, 4 << 20)
        require(digest(image) == report('package.json')['members']['system.new.dat']['sha256'],
                'Filesystem digest differs')
        base = read_file(image, '/usr/bin/ins_gui')
        seed = json.loads(read_file(image, SEED+'manifest.json'))
        blob = read_file(image, SEED+'delta.bin')
        require(sha(blob) == seed['BlobSHA256'] and len(blob) == seed['BlobBytes'], 'Seed delta differs')
        record, = [r for r in seed['Entries'] if r['Target'] == 'vr180-v1/gui']
        require(record['Base'] == 'usr/bin/ins_gui' and sha(base) == record['BaseSHA256']
                and len(base) == record['BaseBytes'], 'Unexpected GUI base')
        tools = compiler_tools()
        from openx6.release_gui import capture_settings, finish, recover_base, current_settings, recording_modes
        from openx6.fisheye import preview_gui
        standard = feature_menu(recover_base(base) if receipt.get('capture_controls') else base, tools)
        require(sha(standard) == receipt['base_sha256'], 'Standard menu differs')
        video_iq = 'video_iq.py' in inputs['engine']
        expected = bind_menus(standard, modules, tools, legacy_layout=not video_iq)
        if video_iq:
            from openx6.release_gui import feature_gui
            expected = feature_gui(expected, tools, extension_menu=True)
            if 'stock_panel.py' in inputs['engine']:
                from openx6.release_gui import consent_gui
                expected = consent_gui(expected, tools)
        elif receipt.get('capture_controls'):
            expected = finish(capture_settings(expected, tools), tools)
            if 'fisheye.py' in inputs['engine']:
                expected = preview_gui(expected, tools)
            if 'settings_keypads.py' in inputs['engine']:
                expected = current_settings(recording_modes(expected, tools), tools, True)
        require(len(expected) == record['Bytes'], 'Invalid GUI size')
        gui = bytearray(base[:len(expected)])
        gui.extend(bytes(len(expected)-len(gui)))
        end = 0
        for chunk in record['Chunks']:
            offset, source, size = (chunk[k] for k in ('Offset','BlobOffset','Length'))
            require(size > 0 and end <= offset <= len(gui)-size and 0 <= source <= len(blob)-size,
                    'Invalid GUI delta range')
            gui[offset:offset+size] = blob[source:source+size]
            end = offset+size
        require(gui == expected and sha(gui) == record['SHA256'] == receipt['output_sha256'],
                'Packed feature GUI differs from rebuilt module')
        files, links = programs.layout(modules, sources, tools['go'])
        actual_files = {}
        if files or 'executable_modules' in inputs:
            expected_programs = programs.receipt(files,links)
            require(expected_programs == inputs.get('executable_modules'), 'Executable module receipt differs')
            for path, expected_file in files.items():
                actual_files[path] = read_file(image, path)
                require(actual_files[path] == expected_file, 'Packed module file differs: '+path)
            labels = programs.startup_labels(links)
            for path in files:
                mode,labels[path] = programs.file_attributes(path)
                require(('Mode:  '+mode).encode() in debug(image,'stat '+path), 'Packed permissions differ: '+path)
            for path,label in labels.items():
                attr = Path(tmp)/'label'
                debug(image,'ea_get -f '+quoted(attr)+' '+path+' security.selinux')
                require(attr.read_bytes() == label.encode()+b'\0', 'Packed SELinux label differs: '+path)
                attr.unlink()
            for path, target in links.items():
                stat = debug(image, 'stat '+path)
                require(b'Type: symlink' in stat and ('"'+target+'"').encode() in stat,
                        'Packed module startup link differs: '+path)
        if check_programs:
            check_programs(bytes(gui), actual_files)
        if check_gui:
            check_gui(bytes(gui))
        if 'third_party_modules' in inputs:
            binary = inputs['third_party_modules']
            require(sha(read_file(image, '/usr/bin/ins_mediad')) == binary['output_sha256'],
                    'Packed media module differs')
    return dict(modules=ids, gui_sha256=sha(expected), candidate_sha256=result['sha256'],
                install_allowed=False)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('build', type=Path)
    parser.add_argument('manifest', nargs='+', type=Path)
    args = parser.parse_args()
    print(json.dumps(verify(args.build, args.manifest), indent=2))
