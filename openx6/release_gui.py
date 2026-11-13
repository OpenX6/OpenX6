"""OpenX6 capture controls, radio preferences and the SD progress panel."""
import json
import struct
from .common import require, sha
from .compiled_recipes import SOURCES, payload
from .raw_nr_recipe import branch
from .camera_info import camera_info_gui
from .wifi_menu import patch as wifi_settings
from .panel_patch import patch as update_panel

from .quick_menu import patch as quick_settings, LABEL as DISPLAY
from .settings_keypads import patch as keypad_settings
CAPTURE = SOURCES/'single_capture'
PH = 64
WIDTH = 56
BASE_GUI = 'da87668122982f01e62f62d36b2f4a6e6b1143187b6eab3d41a15da58b857d43'


def capture_settings(gui, tools):
    """Attach only the recording selectors to a stock-derived feature GUI."""
    data = bytearray(gui)
    offset = PH+3*WIDTH
    rx = list(struct.unpack_from('<II6Q', data, offset))
    rw = struct.unpack_from('<II6Q', data, offset+WIDTH)
    require(rx[:2]==[1,5] and rx[2]+rx[5]==len(data) and rx[5]==rx[6] and rw[:2]==(1,6),
            'Invalid capture GUI mapping')
    recipe = json.loads((CAPTURE/'profile_menu.json').read_bytes())
    # The assembler keeps the original offsets within its independent code section.
    # The load address also accommodates developer modules of different sizes.
    start = (rx[3]+rx[5]+4095)&~4095
    code = payload('single_capture', start, ['profile_menu.S'], recipe['addresses'],
                   tools, relax=False)
    require(start+len(code)<(rw[3]&~4095), 'Capture controls exceed GUI mapping')
    data.extend(bytes(start-rx[3]-rx[5])+code)
    rx[5] = rx[6] = len(data)-rx[2]
    struct.pack_into('<II6Q', data, offset, *rx)
    for site,(expected,relative) in recipe['hooks'].items():
        pc = int(site,16)
        at = rx[2]+pc-rx[3] if pc >= rx[3] else pc
        require(data[at:at+4]==bytes.fromhex(expected), 'Unexpected capture hook at '+site)
        struct.pack_into('<I', data, at, branch(pc, start+int(relative,16)))
    # The original audio menu uses these two resource loads for the single-lens choices.
    for pc,before,after in ((0x88bc90,0x52800021,0x52800001),(0x8a8168,0x90034600,0x900345c0),
                            (0x8a816c,0xf9440400,0xf9426c00)):
        require(struct.unpack_from('<I',data,pc)[0] == before, 'Unexpected menu resource selection')
        struct.pack_into('<I',data,pc,after)
    return bytes(data)


def sd_notice(gui, tools):
    data = bytearray(gui)
    headers = [list(struct.unpack_from('<II6Q',data,PH+i*WIDTH)) for i in range(12)]
    require(data[0x977424:0x977428]==bytes.fromhex('87c7ed97'), 'Unexpected GUI timer call')
    if headers[3][:2] == [1,6]:
        require(headers[6][0] == 4, 'Expected GUI note')
        headers.pop(6)
        headers.insert(3,[1,5,(len(data)+4095)&~4095,0x7009000,0x7009000,0,0,4096])
    rx,rw = headers[3:5]
    require(rx[:2]==[1,5] and rw[:2]==[1,6] and rx[5]==rx[6], 'Unexpected notice mapping')
    base=(rx[3]+rx[5]+15)&~15
    state=(rw[3]+rw[6]+15)&~15
    symbols=dict(last_check=state,timers=0x4e9240,ticks=0x4d8c00,global_obj=0x702d108,
                 global_class=0x70f97c8,get_auto=0x4db7a0,new_utf8=0x4e4010,show_toast=0x55cd10)
    code=payload('single_capture',base,['notice.S'],symbols,tools,relax=False)
    at=rx[2]+base-rx[3]
    require(at>=len(data) and base+len(code)<(rw[3]&~4095),'Notice mapping overflow')
    data.extend(bytes(at-len(data))+code)
    rx[5]=rx[6]=len(data)-rx[2]
    rw[6]=state+16-rw[3]
    for i,h in enumerate(headers):
        struct.pack_into('<II6Q',data,PH+WIDTH*i,*h)
    struct.pack_into('<I',data,0x977424,branch(0x977424,base)|0x80000000)
    return bytes(data)


def finish(gui, tools):
    body=camera_info_gui(sd_notice(gui,tools),'1.2.36 O 1.0')
    body=wifi_settings(body, CAPTURE/'wifi-idle-menu.c',tools['clang'],tools['lld'])
    return update_panel(body,CAPTURE/'update-panel.c',tools['clang'],tools['lld'])


def current_settings(gui, tools, feature):
    updated = quick_settings(gui, CAPTURE/'quick-menu.S', tools['clang'], tools['lld'], DISPLAY, feature)
    if feature:
        updated = keypad_settings(updated, tools['clang'], tools['lld'], CAPTURE/'settings-keypads.S')
    return updated


def fallback_gui(base, tools):
    return current_settings(finish(base, tools), tools, False)


def feature_gui(selected, tools, *, extension_menu=False):
    from .fisheye import preview_gui
    body = preview_gui(finish(capture_settings(selected, tools), tools), tools)
    from .video_iq import patch
    body = current_settings(recording_modes(body, tools), tools, True)
    body, _ = patch(body, 'gui', SOURCES.parent/'components/src/native/video-iq', tools['clang'], tools['lld'], extension_menu=extension_menu)
    return body


def consent_gui(gui, tools):
    from .stock_panel import patch
    return patch(gui, SOURCES.parent/'components/src/native/stock-panel.c', tools['clang'], tools['lld'])


def recover_base(fallback):
    """Reverse the fixed fallback hooks for the independent SDK readback."""
    recipe=json.loads((CAPTURE/'fallback-base.json').read_bytes())
    consent = recipe.get('stock_confirmation')
    if consent and sha(fallback) == consent['sha256']:
        restored = bytearray(fallback[:consent['previous_bytes']])
        for offset, encoded in consent['restore']:
            data = bytes.fromhex(encoded)
            restored[offset:offset+len(data)] = data
        fallback = bytes(restored)
    require(sha(fallback)==recipe['fallback_sha256'],'Unrecognized fallback GUI')
    data=bytearray(fallback[:recipe['base_bytes']])
    for at,value in recipe['restore']:
        raw=bytes.fromhex(value)
        data[at:at+len(raw)]=raw
    require(sha(data)==BASE_GUI,'Reconstructed fallback base differs')
    return bytes(data)


def recording_modes(gui, tools):
    """Relocate recording selectors around the selected developer menu payload."""
    out = bytearray(gui)
    recipe = json.loads((CAPTURE/'recording_modes.json').read_bytes())
    def target(pc, word):
        require(word >> 26 == 5, 'Unexpected recording selector branch')
        return pc + 4*((word & 0x3ffffff) - ((word & 0x2000000) << 1))

    anchor = 0x8338d4
    shift = target(anchor, struct.unpack_from('<I', gui, anchor)[0]) - 0x700fb00
    require(shift % 4096 == 0, 'Unexpected capture menu placement')
    header = PH + 3*WIDTH
    rx = list(struct.unpack_from('<II6Q', gui, header))
    rw = struct.unpack_from('<II6Q', gui, header+WIDTH)
    require(rx[:2] == [1,5] and rx[5]==rx[6] and rx[2]+rx[5]==len(gui),
            'Unrecognized recording selector mapping')
    start = (rx[3]+rx[5]+15)&~15
    symbols = dict(recipe['symbols'], capture_base=0x700e000+shift)
    code = payload('single_capture',start,['recording_modes.S'],symbols,tools,relax=False)
    require(start+len(code)<(rw[3]&~4095), 'Recording selectors exceed GUI mapping')
    for site, kind, original, offset in recipe['hooks']:
        pc = site if site<rx[3] else site+shift
        at = pc if pc<rx[3] else pc-rx[3]+rx[2]
        word = struct.unpack_from('<I', gui, at)[0]
        if kind=='capture':
            require(target(pc,word)==original+shift, 'Capture menu target differs')
        else:
            require(kind=='instruction' and word==original, 'Capture menu instruction differs')
        struct.pack_into('<I',out,at,branch(pc,start+offset))
    out.extend(bytes(start-rx[3]-rx[5])+code)
    rx[5]=rx[6]=len(out)-rx[2]
    struct.pack_into('<II6Q',out,header,*rx)
    return bytes(out)
