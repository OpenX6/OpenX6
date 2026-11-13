"""Rebuild the OpenX6 HAL and its two runtime variants for firmware 1.2.36."""
import struct

from .common import require, sha
from .compiled_recipes import call, payload
from .raw_nr_recipe import branch
from .toolchain import compiler_tools

STOCK = 'd0b00bdd8e5bc11786c69cab7531920072852084c078b7d9c3b5bc7662968225'
INSTALLED = '8386b304b83aab877c469703bae50fd8150ac26a9cf5d730d4c90725bb673a97'
# Each pair pins the full HAL after the HNR hook and after the interval budget.
RUNTIME = {
    'native': ('2989c4ccc841464444af518ada5791a093f0b46ac4042f4fce39e242c6e1d7ef',
               '0175413c97265a30c43ac2cfc91e38ff94d483ac9858dbcd6d60055200f08c39'),
    'off': ('3d2528d38bc452aa88ddc82562face52ea7cf2440a657417ad3e46cb14420744',
            '55e285e8bac53504007dfba4f08773f3dd67be2462739e9d21a4597bee2f6d6f'),
}
RX_FILE, RX_VA = 0x1d60000, 0x29c0000


def extension_header(data):
    phoff = struct.unpack_from('<Q', data, 32)[0]
    width, count = struct.unpack_from('<HH', data, 54)
    require((width, count) == (56, 10), 'Unexpected camera HAL program headers')
    return phoff + 3 * width


def camera_hal(stock, tools=None):
    require(sha(stock) == STOCK, 'Official camera HAL digest differs')
    header = extension_header(stock)
    prop = header + 3 * 56
    require(struct.unpack_from('<I', stock, header)[0] == 4 and
            struct.unpack_from('<I', stock, prop)[0] == 0x6474e553 and
            stock[header+4:header+56] == stock[prop+4:prop+56], 'Unexpected HAL property note')
    # Preserve the object-order constants and unrelaxed addressing of the first build.
    native_fill = 0x13ac4a0
    code = payload('camera_hal', RX_VA, ['bridge.S', 'exposure.c'],
                   {'native_fill': native_fill, 'owner_log': 0x391a70}, tools or compiler_tools(),
                   defines=['-O2', '-ffreestanding', '-fno-pic', '-mbranch-protection=standard'],
                   relax=False, merge_constants=False)
    out = bytearray(stock)
    for site in (0x1092bf0, 0x1092c44, 0x1092d24):
        require(struct.unpack_from('<I', out, site)[0] == (branch(site, native_fill) | 0x80000000),
                'Camera HAL exposure call differs')
        call(out, site, RX_VA)
    out.extend(bytes(RX_FILE - len(out)))
    out.extend(code)
    struct.pack_into('<II6Q', out, header, 1, 5, RX_FILE, RX_VA, RX_VA, len(code), len(code), 65536)
    require(sha(out) == INSTALLED, 'Camera HAL reconstruction differs at exposure bridge')
    return bytes(out)


def camera_hal_runtime(installed, variant, tools=None):
    require(variant in RUNTIME, 'Unknown camera HAL variant')
    require(sha(installed) == INSTALLED, 'Installed camera HAL digest differs')
    tools = tools or compiler_tools()
    out = bytearray(installed)
    header = extension_header(out)

    def extend(site, delta, sources, continuation, expected, stage, defines=()):
        base, offset = RX_VA + delta, RX_FILE + delta
        extent = len(out) - RX_FILE
        require(struct.unpack_from('<II6Q', out, header) ==
                (1, 5, RX_FILE, RX_VA, RX_VA, extent, extent, 65536) and len(out) <= offset,
                'Unexpected camera HAL extension layout')
        code = payload('camera_hal', base, sources, {continuation: site+4}, tools,
                       inputs={'displaced.bin': out[site:site+4]}, defines=defines)
        out.extend(bytes(offset - len(out)))
        out.extend(code)
        struct.pack_into('<I', out, site, branch(site, base))
        struct.pack_into('<QQ', out, header+32, delta+len(code), delta+len(code))
        require(sha(out) == expected, 'Camera HAL reconstruction differs at ' + variant + ' ' + stage)

    hnr_sha, interval_sha = RUNTIME[variant]
    extend(0xb4ef88, 0x5b0, ['switch.S'], 'native_resume', hnr_sha, 'HNR',
           defines=['-DHNR_OFF=' + ('1' if variant == 'off' else '0')])
    extend(0x1458534, 0x1000, ['budget_entry.S', 'budget.c'], 'budget_continue',
           interval_sha, 'interval budget')
    # Use the pipeline prefix from this firmware's own HAL type-name table.
    start, site = 0x1cd0bb0, 0x9427a0
    code = payload('camera_hal', start, ['crop_main.S'], {'hal_continue':site+4}, tools,
                   inputs={'path-prefix.bin':installed[2364568:2364578]}, relax=False)
    require(len(code)==232 and out[start:start+len(code)]==bytes(len(code)),
            'Unrecognized HAL code alignment area')
    require(struct.unpack_from('<QQ',out,96)==(start,start), 'Unexpected initial HAL segment size')
    out[start:start+len(code)] = code
    struct.pack_into('<QQ',out,96,start+len(code),start+len(code))
    struct.pack_into('<I',out,site,branch(site,start))
    return bytes(out)
