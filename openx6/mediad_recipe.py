"""Rebuild the OpenX6 1.2.36 media service in its original layer order."""
import struct

from .common import require, sha
from .compiled_recipes import extend_text, payload
from .raw_nr_recipe import branch
from .toolchain import compiler_tools

STOCK = 'b41f38a7a4886a4f32cebdd517dddca9e62d214b18ad41b40cae9a916d97324b'
# Stage receipts cover the entire ELF, including code superseded by later stages.
RECEIPTS = {
    'stream bitrate': '96b9a28e38b8a62e0306e3ff7acea8b0c1091c5eb4433147b78503d41a6277ed',
    'record bitrate': '16038064120a789853757462b47bc4f9f7896838e57a5688fb5b0ab879cfa31e',
    'RAW Only': 'fccc1f2898dffac6f24a90882a7db3918a249ac64f17614a4e780c13e83220c1',
    'Max 280': '2ffdd5890cf1ae2b51722a2ba12c001279f1ff3329c833214110e8e673fe5a7d',
    'encoder profile': '8a793d71cbf03256e69547351bf36ff5a0f622047240e447740ed3cefe71ee4f',
    'Dolby bitrate': '1b34f958a986b3fd54b6b62557db3214e415e990a03a6ff815dd90a6dac06d17',
    'color bitrate': '2a9b6019730c486ac24ea8cba64576617c34ffde33e0d1d6223706207d0eeb0d',
    'preview thumbnail': 'a716a7f8b62816c81b27ae62ac8882fccf1ca47ec480841de15c8299e7862098',
    'iLog 260': '7109c2ff953a993d525313821dff877603b9b7c0a67006cbf2ed94342bbddab6',
    'custom bitrate': 'bb0e820e29ad12735bf336a810aa9f7612cbfcc49868925783802675590a1d63',
    'proxy buffer release': 'cfb852bfd6dcc676e1a466fb1f114561cdcd05cb6f16905b75efa14ca6698f31',
    'interval cadence': '7964298ca6b371ddc7dd58aa44de9e10bfb05df6402a2d387f44df8af503d113',
    '16K timelapse': 'e95cb0a4e23c94fca65cf1fadb52861bd3476119efcfaf1a9bdd36dda5d6576a',
    'proxy stop': '6dab975f206cca84d83107f106468cd134e19ad3016ffb68c504f10949282745',
    '16K Fast RAW': '22706f2b1757af667183b9d558bcbb4011d61f36bdc80d1c2a8eebc3563a8730',
}
RX_FILE, RX_VA = 0x8d0000, 0x900000


def media_service(stock, tools=None):
    require(sha(stock) == STOCK, 'Official media service digest differs')
    tools = tools or compiler_tools()
    out = bytearray(stock)
    phoff = struct.unpack_from('<Q', stock, 32)[0]
    width, count = struct.unpack_from('<HH', stock, 54)
    require(width == 56 and count == 12, 'Unexpected media ELF header layout')
    rx = phoff + 5 * width

    def verify(stage):
        require(sha(out) == RECEIPTS[stage], 'Media reconstruction differs at ' + stage)

    def compile_at(base, sources, symbols, defines=()):
        return payload('mediad', base, sources, symbols, tools, defines=defines)

    def hook(site, target, *, link=False):
        struct.pack_into('<I', out, site, branch(site, target) | (0x80000000 if link else 0))

    def emit(base, sources, symbols, *, defines=(), replace=False):
        code = compile_at(base, sources, symbols, defines)
        offset = RX_FILE + base - RX_VA
        if replace:
            require(RX_FILE <= offset < offset+len(code) <= len(out), 'Media replacement outside extension')
            out[offset:offset+len(code)] = code
        else:
            require(offset >= len(out), 'Media extension overlaps earlier stage')
            out.extend(bytes(offset-len(out)))
            out.extend(code)
            struct.pack_into('<QQ', out, rx+32, len(out)-RX_FILE, len(out)-RX_FILE)

    config = {'config_mgr_got': 0x8d5a10, 'native_get_config': 0x169590}
    record = dict(config, continuation=0x308da4)
    color = dict(record, color_mgr_got=0x8d5a88, native_color=0x2ff4d0)
    interval = {'TIFFSetField': 0x1691a0, 'TIFFNumberOfStrips': 0x16a180,
                'mono_now': 0x16a890, 'x6_interval_state': 0x8f5000}
    proxy = {'native_active': 2521600, 'gst_pad_get_parent_element': 1479472,
             'gst_pad_get_current_caps': 1481872, 'gst_caps_get_structure': 1478576,
             'gst_structure_get_int': 1480128, 'gst_mini_object_unref': 1480096,
             'native_builder': 2240256, 'native_release': 2510432, 'gst_object_unref': 1494784}

    base = 0x8b1db0
    extend_text(out, 2, base, compile_at(base, ['stream.S'], {'advance_stream': 0x304354, 'store_bitrate': 0x304350}))
    struct.pack_into('<I', out, 0x304310, 0x2a0003e5)  # mov w5, w0
    hook(0x30434c, base)
    verify('stream bitrate')

    # Reuse the duplicate property NOTE; the GNU_PROPERTY header still maps it.
    prop = phoff + 8 * width
    require(struct.unpack_from('<I', out, rx)[0] == 4 and
            struct.unpack_from('<I', out, prop)[0] == 0x6474e553 and
            out[rx+4:rx+width] == out[prop+4:prop+width], 'Unexpected property NOTE')
    struct.pack_into('<II6Q', out, rx, 1, 5, RX_FILE, RX_VA, RX_VA, 0, 0, 65536)
    emit(RX_VA, ['record.S'], record, defines=['-DEYE_RATE=160000000'])
    hook(0x308da0, RX_VA)
    verify('record bitrate')

    emit(0x9001a0, ['raw_only.S'], {'mask_continue': 0x2f6750, 'idle_continue': 0x3faf5c, 'idle_epilogue': 0x3fae48})
    hook(0x2f674c, 0x9001a0)
    hook(0x3faf58, 0x9001c0)
    verify('RAW Only')
    emit(RX_VA, ['record.S'], record, defines=['-DEYE_RATE=140000000'], replace=True)
    verify('Max 280')

    profile = {'continuation': 0x31a860}
    emit(0x900200, ['profile.S'], profile)
    hook(0x31a85c, 0x900200)
    verify('encoder profile')

    dolby = [('pano.S', 0x900300, 0x32d5d0), ('aux.S', 0x900500, 0x32dda8)]
    for source, address, site in dolby:
        emit(address, [source], dict(config, continuation=site+4))
        hook(site, address)
    verify('Dolby bitrate')
    emit(0x900800, ['color.S'], color, defines=['-DILOG_RATE=140000000'])
    hook(0x308da0, 0x900800)
    verify('color bitrate')
    emit(0x900a00, ['thumbnail.S'], {'native_callback': 0x2e2da0})
    hook(0x2e1a40, 0x900a00, link=True)
    verify('preview thumbnail')
    emit(0x900800, ['color.S'], color, defines=['-DILOG_RATE=130000000'], replace=True)
    verify('iLog 260')

    for source, address, site in dolby:
        emit(address, [source], dict(config, continuation=site+4), defines=['-DCUSTOM'], replace=True)
    emit(0x901000, ['custom.S'], color)
    hook(0x308da0, 0x901000)
    emit(0x900200, ['profile.S'], profile, defines=['-DCUSTOM'], replace=True)
    verify('custom bitrate')

    emit(0x902000, ['proxy.c', 'proxy_strings.S'], proxy)
    hook(0x2ae670, 0x902000, link=True)
    verify('proxy buffer release')
    emit(0x903000, ['cadence.S', 'fast_dng.c', 'cadence.c', 'cadence_strings.S'], interval)
    for site, address in [(0x4714d0, 0x903000), (0x3ee660, 0x903010), (0x3f0100, 0x903020)]:
        hook(site, address, link=True)
    rw = phoff + 3 * width
    va = struct.unpack_from('<Q', out, rw+16)[0]
    struct.pack_into('<Q', out, rw+40, 0x8f6000-va)  # reserve cadence state in memory only
    verify('interval cadence')
    emit(0x904000, ['timelapse.S', 'timelapse.c'], {})
    hook(0x6adfb0, 0x904000, link=True)
    verify('16K timelapse')
    emit(0x904300, ['presence.S'], {'native_nonnull': 0x29a020, 'native_null': 0x29a028, 'native_inactive': 0x29a150})
    hook(0x29a01c, 0x904300)
    verify('proxy stop')
    emit(0x905000, ['fast_raw.S', 'fast_raw.c', 'fast_raw_strings.S'], dict(interval, capture_continue=0x2c34d8))
    for site, address in [(0x4714d0, 0x905000), (0x3ee660, 0x905010), (0x3f0100, 0x905020),
                          (0x3f0790, 0x9050e0), (0x3f08b4, 0x9051a0)]:
        hook(site, address, link=True)
    hook(0x2c34d4, 0x905260)
    verify('16K Fast RAW')
    return bytes(out)
