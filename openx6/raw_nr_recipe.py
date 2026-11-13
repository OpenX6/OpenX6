"""Generate the OpenX6 RAW tensor gain hooks for the pinned 1.2.36 library."""
import struct

from .common import require


def branch(source, target):
    delta = target - source
    require(delta % 4 == 0 and -(1 << 27) <= delta < (1 << 27), 'Invalid ARM64 branch')
    return 0x14000000 | ((delta // 4) & 0x03ffffff)


def raw_nr_variant(stock, gain):
    require(gain in (0.0, 0.33, 0.66), 'Unsupported RAW NR variant')
    result = bytearray(stock)
    phoff = struct.unpack_from('<Q', stock, 32)[0]
    require(struct.unpack_from('<Q', stock, phoff + 32)[0] == 0xb5deec and
            stock[0xb5df00:0xb5df70] == bytes(0x70), 'Unexpected RAW NR code space')
    # Float and quantized tensor builders use different registers and return sites.
    layouts = [
        (0xb5df00, 25, 23, 22, 0x8b3544, 0x8b3470, (0x8b346c,)),
        (0xb5df40, 21, 15, 19, 0x8b3094, 0x8b2ff0, (0x8b2fec, 0x8b3090)),
    ]
    for start, channels, noise, condition, float_path, quant_path, sites in layouts:
        words = [
            0x7100001f | (8 << 10) | (channels << 5),    # cmp channels, #8
            0x54000061,                                  # b.ne skip_gain
            0x9c000000 | (6 << 5) | 26,                  # ldr q26, gain
            0x6e20dc00 | (26 << 16) | (noise << 5) | noise,  # fmul noise.4s
            0x35000000 | (2 << 5) | condition,           # cbnz condition, quant
            branch(start + 20, float_path),
            branch(start + 24, quant_path),
            0xd503201f,                                  # nop / align gain vector
        ]
        struct.pack_into('<8I4f', result, start, *words, *([gain] * 4))
        for site in sites:
            struct.pack_into('<I', result, site, branch(site, start))
    struct.pack_into('<QQ', result, phoff + 32, 0xb5df70, 0xb5df70)
    return bytes(result)
