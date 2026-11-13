"""Resolve the signed OpenX6 inventory from source, firmware, and review inputs."""
import json
import struct
from pathlib import Path

from .common import asset, regular, require, safe_relative, sha
from .fsimage import read_file
from .patch import make_recipe
from .compiled_recipes import base_gui, fast_capture, gui_wrapper, interval_wait, video_nr
from .adb_recipe import owner_adb
from .factory_usb_recipe import factory_api, hardware_service, usb_receiver
from .otad_recipe import maintenance_updater
from .interval_ae_recipe import interval_ae
from .ssd_recipe import ssd_gate, ssd_layout
from .camera_hal_recipe import camera_hal, camera_hal_runtime
from .mediad_recipe import media_service
from .menu_recipe import feature_menu
from .raw_nr_recipe import raw_nr_variant

SOURCES = Path(__file__).resolve().parents[1] / 'components'

USB_GUARD = b'''	# Owner compositions may bind only through the checked transaction.
	if [ -e /tmp/usb_bind_in_progress ] || [ -e /data/usb_bind_in_progress ]; then
		echo "USB transition in progress; refusing unchecked bind" >&2
		exit 1
	fi
	owner_pid=`cat /sys/kernel/config/usb_gadget/g1/idProduct 2>/dev/null`
	case "$owner_pid" in
	  0xc001|0xC001|0x901d|0x901D)
		echo "Use x6-owner-usb switch for owner USB compositions" >&2
		exit 1 ;;
	esac
'''


def inventory(profile, profile_hash):
    files = dict(profile['sources'])
    files['profile.json'] = profile_hash
    files['coordinator-policy-template.json'] = profile['policy_sha256']
    for entry in [*profile['actions'].values(), *profile['seed'].values()]:
        if 'asset' in entry:
            files[entry['asset']] = entry['asset_sha256']
    for entry in profile['mod_runtime']:
        files[entry['recipe']] = entry['recipe_sha256']
    return files


def edit_once(data, search, replacement):
    require(data.count(search) == 1, 'Ambiguous stock script anchor')
    return data.replace(search, replacement)


def render_stock(rule, stock, tools=None):
    require(stock is not None and sha(stock) == rule['base_sha256'],
            'Official component digest differs: ' + rule['path'])
    op = rule['op']
    if op == 'snapshot-tuning':
        # 1.2.36 IMX01F dual-eye 120 MP snapshot descriptor, ELF file offset.
        values = struct.unpack_from('<41f', stock, 0x40f72c)
        return ('static const float raw_tune[] = {\n' +
                ',\n'.join(v.hex() + 'f' for v in values) + '\n};\n').encode()
    if op == 'copy':
        return stock
    if op == 'log-limits':
        config = json.loads(stock)
        for group in config['tags']:
            group['file_size'], group['max_files'] = (16384, 8) if group['name'] == 'logcat' else (4096, 4)
        return (json.dumps(config, indent=4) + '\n').encode()
    if op == 'start-usb':
        result = stock
        for index in (0, 1):
            anchor = f'if [ -d /sys/class/android_usb/android{index} ]'.encode()
            prefix = b"# boot hsusb composition:\n\t" if index == 0 else b"# boot hsic composition:\n\t"
            result = edit_once(result, prefix + anchor, prefix + b'if [ "${X6_OWNER_USB_INIT_ONLY:-0}" != "1" ] && ' + anchor[3:])
        start = b'  start)\n'
        result = edit_once(result, start, start + USB_GUARD)
    elif op == 'launcher':
        action = {'/sbin/launch_adbd': 'run-adbd', '/sbin/usb/compositions/901D': 'switch 901D',
                  '/sbin/usb/compositions/C001': 'switch C001'}[rule['path']]
        result = f'#!/bin/sh\nexec /usr/local/sbin/x6-owner-usb {action}\n'.encode()
    elif op == 'raw-nr':
        result = raw_nr_variant(stock, rule['gain'])
    elif op == 'maintenance-updater':
        result = maintenance_updater(stock, tools)
    elif op == 'hardware-service':
        result = hardware_service(stock)
    elif op == 'usb-receiver':
        result = usb_receiver(stock, tools)
    elif op == 'factory-api':
        result = factory_api(stock, tools)
    elif op == 'owner-adb':
        result = owner_adb(stock, rule['path'])
    elif op == 'interval-ae':
        result = interval_ae(stock, tools)
    elif op == 'ssd-gate':
        result = ssd_gate(stock, rule['path'], tools)
    elif op in ('fisheye-render', 'fisheye-mesh'):
        from .fisheye import preview_nodes
        result = preview_nodes(stock, op, tools)
    elif op == 'camera-hal':
        result = camera_hal(stock, tools)
    elif op == 'camera-hal-runtime':
        stock = camera_hal(stock, tools)
        result = camera_hal_runtime(stock, rule['variant'], tools)
    elif op == 'media-service':
        result = media_service(stock, tools)
    elif op == 'base-gui':
        result = base_gui(stock, tools)
    elif op == 'gui-wrapper':
        result = gui_wrapper(stock, tools)
    elif op == 'interval-wait':
        result = interval_wait(stock, tools)
    elif op == 'feature-menu':
        # The first-boot menu recipe is applied to the installed base GUI.
        stock = base_gui(stock, tools)
        require(sha(stock) == rule['recipe_base_sha256'], 'Installed GUI base differs')
        result = feature_menu(stock, tools)
    elif op == 'fast-capture':
        result = fast_capture(stock, tools)
    elif op == 'video-nr':
        result = video_nr(stock, rule['gain'], tools)
    elif op in ('set-bytes', 'installed-set-bytes'):
        if op == 'installed-set-bytes':
            from .patch import apply_recipe
            stock = apply_recipe(stock, json.loads(render_stock(rule['installed'], stock, tools)))
            require(sha(stock)==rule['installed_sha256'], 'Camera timing base differs')
        result = bytearray(stock)
        end = 0
        for offset, value in rule['writes']:
            data = bytes.fromhex(value)
            require(data and end <= offset <= len(stock) - len(data), 'Invalid replacement span')
            end = offset + len(data)
            result[offset:end] = data
    else:
        raise ValueError('Unknown stock transformation')
    if 'video_iq' in rule:
        from .toolchain import compiler_tools
        from .video_iq import patch
        tool = tools or compiler_tools()
        result, _ = patch(result, rule['video_iq'], SOURCES/'src/native/video-iq', tool['clang'], tool['lld'])
    fmt = {'separators': (',', ':')} if rule.get('compact') else {'indent': rule.get('indent', 2)}
    return (json.dumps(make_recipe(stock, bytes(result)), **fmt) + '\n').encode()


def generate_rule(rule, image, tools=None, *, installed_actions=None):
    if rule['op'] == 'single-capture':
        from .single_capture import reconstruct
        raw = read_file(image, rule['path'])
        require(raw is not None and sha(raw) == rule['base_sha256'], 'Unexpected official capture input')
        return reconstruct(rule, raw, lambda path:read_file(image,path), tools)
    if rule['op'] != 'ssd-hooks':
        return render_stock(rule, read_file(image, rule['path']), tools)
    hooks = []
    for spec in ssd_layout():
        require(spec['name'] in ('ins_configd', 'ins_bla'), 'Unknown SSD hook')
        base = read_file(image, '/usr/bin/' + spec['name'])
        require(base is not None and sha(base) == spec['old'], 'SSD hook base mismatch')
        site = spec['site']
        require(0 <= site <= len(base) - 4, 'SSD instruction outside file')
        path = '/usr/bin/' + spec['name']
        require(installed_actions and path in installed_actions, 'Missing installed SSD service')
        hooks.append(dict(spec, new=installed_actions[path]['sha256'],
                          original_instruction=base[site:site + 4].hex()))
    return json.dumps(hooks).encode()


def assemble(dest, stock_image, expected, *, sources=SOURCES, tools=None):
    rules = json.loads((sources/'stock-rules.json').read_bytes())
    require(set(rules) <= expected.keys(), 'Inconsistent component audit')
    dest.mkdir(mode=0o700)
    for name, checksum in expected.items():
        safe_relative(name)
        if name in rules:
            rule = rules[name]
            actions = None
            if rule['op'] == 'ssd-hooks':
                actions = json.loads((sources/'profile.json').read_bytes())['actions']
            data = generate_rule(rule, stock_image, tools, installed_actions=actions)
        else:
            with regular(asset(sources, name)) as source:
                data = source.read()
        require(sha(data) == checksum, 'Component reconstruction differs: ' + name)
        target = dest/name
        target.parent.mkdir(parents=True, exist_ok=True)
        with target.open('xb') as out:
            out.write(data)
