"""Attach the native view picker and LCD preview hooks to the verified 1.2.36 ELFs."""
import json
import math
import struct

from .common import require
from .compiled_recipes import SOURCES, payload
from .raw_nr_recipe import branch
from .resolution_labels import patch as resolution_captions

HEADER = struct.Struct('<II6Q')
FLAGS = ['-Oz', '-ffreestanding', '-fpie', '-fvisibility=hidden', '-mgeneral-regs-only',
         '-fno-builtin', '-mbranch-protection=bti', '-march=armv8.3-a']


def linear_preview_grid():
    """Rectilinear 104-degree horizontal view of a centered 180-degree equidistant lens."""
    points = bytearray()
    tangent = math.tan(math.radians(52))
    for row in range(33):
        for col in range(33):
            u = (col / 16 - 1) * tangent
            v = (row / 16 - 1) * tangent
            radius = math.hypot(u, v)
            scale = (1023 / math.pi) * (math.atan(radius) / radius if radius else 1)
            x, y = (round((511.5 + axis * scale) * 16) for axis in (u, v))
            require(0 <= x < 1023*16 and 0 <= y < 1023*16, 'Linear preview grid exceeds lens bounds')
            points.extend(struct.pack('<HH', x, y))
    return bytes(points)


def preview_nodes(stock, name, tools):
    out = bytearray(stock)
    if name == 'fisheye-mesh':
        header = 64
        require(HEADER.unpack_from(stock, header) == (1, 5, 0, 0, 0, 0x1438c, 0x1438c, 0x10000),
                'Unexpected mesh mapping')
        base = 0x14390
        code = payload('single_capture', base, ['fisheye_mesh.S'], {'stock_convert': 0x3760}, tools)
        end = base + len(code)
        require(end < 0x14a58 and stock[0x1438c:end] == bytes(end-0x1438c), 'Mesh hook space occupied')
        for site in (0xcb68, 0xcb9c):
            require(struct.unpack_from('<I', stock, site)[0] == branch(site, 0x3760) | 0x80000000,
                    'Unexpected mesh conversion call')
            struct.pack_into('<I', out, site, branch(site, base) | 0x80000000)
        struct.pack_into('<QQ', out, header+32, end, end)
        out[base:end] = code
    elif name == 'fisheye-render':
        header = 64 + 4*HEADER.size
        require(HEADER.unpack_from(stock, header)[0] == 4 and len(stock) <= 0x10000
                and stock[0x8b84:0x8b88].hex() == 'ff8305d1', 'Unexpected preview renderer entry')
        base = 0x30000
        code = payload('single_capture', base,
                       ['fisheye_render.S', 'fisheye_dispatch.S', 'fisheye_nv12.c',
                        'fisheye_p010.c', 'framed_dispatch.S', 'framed_nv12.c', 'framed_p010.c'],
                       {'original_render': 0x8b88}, tools, inputs={'linear-grid.bin': linear_preview_grid()},
                       defines=FLAGS, relax=False)
        HEADER.pack_into(out, header, 1, 5, 0x10000, base, base, len(code), len(code), 0x10000)
        struct.pack_into('<I', out, 0x8b84, branch(0x8b84, base))
        out.extend(bytes(0x10000-len(out)) + code)
    else:
        raise ValueError('Unknown preview node')
    return bytes(out)


# Successful exit from the final built-in row, after its callback is stored.
# Other exits share MENU_DONE, including allocation failures and extension subpages.
MENU_TAIL = 0x700d3f4
MENU_DONE = 0x70094d4


def check_menu_order(gui, rx, tail_target):
    def word(pc):
        return struct.unpack_from('<I', gui, rx[2] + pc - rx[3])[0]

    require(word(0x70090e0) == (branch(0x70090e0, 0x761eb0) | 0x80000000)
            and word(0x70094d0) == branch(0x70094d0, 0x700d3a0)
            and word(MENU_TAIL - 8) == 0x91130289
            and word(MENU_TAIL - 4) == 0xa9000520
            and word(MENU_TAIL) == branch(MENU_TAIL, tail_target)
            and word(MENU_DONE) == 0xa94153f3,
            'Unexpected Mod Settings row order or constructor exit')


def preview_gui(gui, tools):
    out = bytearray(gui)
    header = 64 + 3*HEADER.size
    rx = list(HEADER.unpack_from(out, header))
    rw = HEADER.unpack_from(out, header+HEADER.size)
    require(rx[:2] == [1, 5] and rx[2]+rx[5] == len(gui) and rx[5] == rx[6]
            and rw[:2] == (1, 6), 'Unexpected view picker mapping')
    base = (rx[3]+rx[5]+15) & ~15
    recipe = json.loads((SOURCES/'single_capture/fisheye_picker.json').read_bytes())
    code = payload('single_capture', base, ['fisheye_picker.S', 'fisheye_picker.c'],
                   dict(recipe['symbols'], stock_audio_set=0x6364f0, load_resource=0x4df4d0,
                        page_done=MENU_DONE, group_add=0x58b350, switch_title=0x964030, set_checked=0x9640f0, new_slot=0x4e1830), tools, defines=FLAGS, relax=False)
    require(base+len(code) < rw[3] & ~4095, 'View picker exceeds executable mapping')
    for index, (site_hex, expected) in enumerate(recipe['hooks']):
        site = int(site_hex, 16)
        require(gui[site:site+4] == bytes.fromhex(expected), 'Unexpected view picker hook at '+site_hex)
        struct.pack_into('<I', out, site, branch(site, base+4*index))
    # Branch through the picker table so SDK menus may move the added code.
    for index, (site, context) in enumerate(((0x7361a4, '21008052d300fc97d1ffff17'),
                                            (0x74328c, '2100805299ccfb9786ffff17'),
                                            (0x749894, '2100805217b3fb975dffff17'))):
        require(gui[site-4:site+8] == bytes.fromhex(context), 'Unexpected native audio switch')
        struct.pack_into('<I', out, site, branch(site, base+4*(6+index)) | 0x80000000)
    shortcut_site = 0x6e112c
    native = branch(shortcut_site, recipe['symbols']['set_frame']) | 0x80000000
    require(struct.unpack_from('<I', gui, shortcut_site)[0] == native,
            'Unexpected native view shortcut')
    struct.pack_into('<I', out, shortcut_site, branch(shortcut_site, base+9*4) | 0x80000000)
    check_menu_order(gui, rx, MENU_DONE)
    at = rx[2] + MENU_TAIL - rx[3]
    struct.pack_into('<I', out, at, branch(MENU_TAIL, base + 40))
    out.extend(bytes(base-rx[3]-rx[5]) + code)
    rx[5] = rx[6] = len(out)-rx[2]
    HEADER.pack_into(out, header, *rx)
    return resolution_captions(bytes(out))


def reader_signature(gui):
    """Verify the native model-reader hook and its relocated picker table."""
    site = 0x5dc488
    signature = gui[site:site+16]
    require(len(signature)==16 and signature[4:].hex()=='bf04007140030054e10314aa',
            'Unexpected GUI mode reader context')
    word = struct.unpack_from('<I', signature)[0]
    require(word >> 26 == 5, 'Missing GUI mode reader branch')
    table = site + 4*((word & 0x3ffffff) - ((word & 0x2000000) << 1)) - 12
    rx = HEADER.unpack_from(gui, 64 + 3*HEADER.size)
    require(rx[:2]==(1,5) and rx[3]<=table<=rx[3]+rx[5]-40, 'Picker table outside executable mapping')
    # Each of the six native call sites must reach its own slot in the same table.
    recipe = json.loads((SOURCES/'single_capture/fisheye_picker.json').read_bytes())
    for index, (location, _) in enumerate(recipe['hooks']):
        pc = int(location, 16)
        require(struct.unpack_from('<I', gui, pc)[0]==branch(pc,table+4*index),
                'Inconsistent GUI mode reader table')
    shortcut_site = 0x6e112c
    require(struct.unpack_from('<I',gui,shortcut_site)[0] == (branch(shortcut_site,table+36) | 0x80000000),
            'Missing live view shortcut icon hook')
    check_menu_order(gui, rx, table + 40)
    return signature.hex()
