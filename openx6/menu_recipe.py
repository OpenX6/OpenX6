"""Rebuild the initial settings, RAW format and full menu on the verified base GUI."""
import json
import struct

from .common import require, sha
from .compiled_recipes import SOURCES, call, extend_text, payload
from .raw_nr_recipe import branch
from .toolchain import compiler_tools


def menu_code(name, base, tools):
    symbols = json.loads((SOURCES/'single_menu'/(name+'-symbols.json')).read_bytes())
    return payload('single_menu', base, [name+'.S'],
                   {k:int(v,16) for k,v in symbols.items()}, tools, relax=False)


def raw_only_help(gui, tools):
    """Replace the format-7 helper text at construction and on a language change."""
    out = bytearray(gui)
    start = 0x7008740
    sites = ((0x81af2c, 0x4e5090), (0x82f150, 0x57ea60))
    for site, original in sites:
        require(struct.unpack_from('<I', out, site)[0] == (branch(site, original) | 0x80000000),
                'Unexpected RAW format help call')
    require(struct.unpack_from('<I', out, 0x81ae88)[0] == (branch(0x81ae88, 0x7008720) | 0x80000000),
            'RAW Only title required')
    code = payload('single_menu', start, ['raw_help.S'],
                   {'ew_language':0x7341568, 'new_utf8':0x4e4010,
                    'set_option03':0x57ea60, 'set_option04':0x57eab0}, tools, relax=False)
    # The following writable segment begins at this file offset even in GUIs
    # where an extra executable segment precedes it in the program header table.
    require(start + len(code) <= 0x70088b0, 'RAW help exceeds the executable padding')
    extend_text(out, 2, start, code)
    for index, (site, _) in enumerate(sites):
        call(out, site, start + 4*index)
    return bytes(out)


def thermal_touch_site(out, base, code):
    # Unique entry prologue of our touch handler; re-resolve for extension menus.
    signature = bytes.fromhex('5f2403d5e007bca9e20f01a9')
    require(code.count(signature) == 1, 'Missing or ambiguous temperature touch handler')
    struct.pack_into('<I', out, 0x588744, branch(0x588744, base + code.index(signature)))


def feature_menu(gui, tools=None):
    tools = tools or compiler_tools()
    out = bytearray(gui)
    initial = 0x70083f0
    extend_text(out, 2, initial, menu_code('initial', initial, tools))
    struct.pack_into('<I', out, 0x8ddd08, branch(0x8ddd08, initial))
    require(sha(out) == '1edaea24b2b85ad364f67af211fd0dcdae112fe7d6b9ca12bcbd8268f9a36dac',
            'Initial settings menu differs')

    raw_label = 0x7008720
    label = payload('single_menu', raw_label, ['raw_label.S'], {'new_string':0x4e3e80}, tools)
    extend_text(out, 2, raw_label, label)
    struct.pack_into('<I', out, 0x820e5c, 0x52800061)
    struct.pack_into('<I', out, 0x820e9c, 0x52800021)
    call(out, 0x81ae88, raw_label)
    require(sha(out) == 'a9d15b60ca57d73efefbb92ddc564e3f2bd6737e17f1225d6160e86fc20be1f6',
            'RAW Only menu base differs')

    base = 0x7009000
    code = menu_code('menu', base, tools)
    require(len(code) == 22779, 'Unexpected settings menu size')
    offset = (len(out)+4095) & ~4095
    phoff = struct.unpack_from('<Q', out, 32)[0]
    width, count = struct.unpack_from('<HH', out, 54)
    require(width == 56 and count == 12 and offset == 0x715c000, 'Unexpected GUI ELF layout')
    headers = [bytearray(out[phoff+width*i:phoff+width*(i+1)]) for i in range(count)]
    require(struct.unpack_from('<I', headers[6])[0] == 4, 'Expected GUI note header')
    del headers[6]
    rw = headers[3]
    kind, flags, _, va, _, _, memsize, _ = struct.unpack('<II6Q', rw)
    require((kind, flags) == (1,6) and ((va+memsize+15)&~15) == 0xb3500f0,
            'Unexpected GUI writable segment')
    struct.pack_into('<Q', rw, 40, 0xb3500f0-va+4096+16)
    headers.insert(3, struct.pack('<II6Q', 1,5,offset,base,base,len(code),len(code),4096))
    out[phoff:phoff+width*count] = b''.join(headers)
    out.extend(bytes(offset-len(out)))
    out.extend(code)
    for site, target in [(0x8ddd08,base), (0x4e9244,0x700a660), (0x5b0034,0x700a69c),
                         (0x5af704,0x700a6f4), (0x63bbe4,0x700acbc), (0x6a98f4,0x700acd0),
                         (0x5d38f0,0x700c564)]:
        struct.pack_into('<I', out, site, branch(site,target))
    for site, target in [(0x7293a0,0x700ac60), (0x754424,0x700b500), (0x8ae19c,0x700b5f0)]:
        call(out, site, target)
    for site in (0x729338, 0x729364):
        struct.pack_into('<I', out, site, 0x52800081)  # four bitrate choices
    require(out[0x588744:0x588748] == bytes.fromhex("fd7bb7a9"), "Unexpected touch dispatcher")
    thermal_touch_site(out, base, code)
    return raw_only_help(out, tools)


def bind_stats_menu(gui, manifest, tools):
    """Relocate the existing Stats switch into a declarative native subpage."""
    return bind_menus(gui, [manifest], tools)


def bind_menus(gui, manifests, tools, *, legacy_layout=False):
    from .choice_recipe import choice_pages
    from .modules import split_selection
    from sdk.choice import SCHEMA
    menus, binary = split_selection(manifests, None)
    require(menus and not binary, 'Expected native menu manifests')
    stats = next((m for m in menus if m['schema'] == 'openx6.menu.v1'), None)
    choices = [m for m in menus if m['schema'] == SCHEMA]
    base, offset = 0x7009000, 0x715c000
    original = menu_code('menu', base, tools)
    phoff = struct.unpack_from('<Q', gui, 32)[0]
    width, count = struct.unpack_from('<HH', gui, 54)
    header = phoff + 3 * width
    expected = (1, 5, offset, base, base, len(original), len(original), 4096)
    require(width == 56 and count == 12 and struct.unpack_from('<II6Q', gui, header) == expected
            and gui[offset:] == original, 'Unexpected base menu segment')
    symbols = json.loads((SOURCES/'single_menu/menu-symbols.json').read_bytes())
    # Emit UTF-8 as byte literals. Labels cannot introduce assembler directives.
    inputs = {'extension_labels.inc': b''}
    files, defines = ['menu.S'], []
    if legacy_layout:
        defines.append('-DOPENX6_LEGACY_EXTENSION_MENU')
    if stats:
        labels = {'ext_module_title': stats['title'], 'ext_setting_title': stats['settings'][0]['label']}
        data = ''.join(name + ': .byte ' + ','.join(map(str, text.encode() + b'\0')) + '\n'
                       for name, text in labels.items())
        inputs['extension_labels.inc'] = data.encode()
        defines.append('-DOPENX6_STATS_EXTENSION')
    if choices:
        inputs['choice_arrows.inc'], inputs['choice_descriptors.inc'] = choice_pages(choices)
        defines.append('-DOPENX6_CHOICES')
        files.append('choice.c')
    code = payload('single_menu', base, files, {k:int(v,16) for k,v in symbols.items()},
                   tools, inputs=inputs, defines=defines, relax=False)
    following = struct.unpack_from('<Q', gui, header+width+16)[0]
    mapped_end = (base + len(code) + 4095) & ~4095
    require(len(original) < len(code) and mapped_end <= (following & ~4095),
            'Extension exceeds executable memory range')
    out = bytearray(gui[:offset])
    struct.pack_into('<QQ', out, header+32, len(code), len(code))
    out.extend(code)
    thermal_touch_site(out, base, code)
    return bytes(out)
