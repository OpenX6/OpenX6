"""Reconstruct the interval still-exposure hook for the official AE plugin."""
import struct

from .common import require, sha
from .compiled_recipes import payload, call
from .raw_nr_recipe import branch
from .toolchain import compiler_tools

STOCK = '7c07c5396e0b5d03f617ee148dd1432943f01148c18467937a7644c5a9151095'
TARGET = 'e8c22f20f0a6476c0750914a89908e58d48fa6a5cdc830317b55ab2e5c9c7dac'


def interval_ae(stock, tools=None):
    require(sha(stock) == STOCK, 'Official AE plugin digest differs')
    header = struct.unpack_from('<Q', stock, 32)[0]
    width, count = struct.unpack_from('<HH', stock, 54)
    require((width, count) == (56, 8), 'Unexpected AE program headers')
    headers = [bytearray(stock[header+i*width:header+(i+1)*width]) for i in range(count)]
    require([struct.unpack_from('<II', h) for h in headers[:4]] ==
            [(1, 5), (1, 6), (2, 6), (4, 4)], 'Unexpected AE load segments')
    site, cap1, cap2 = 0x2ded8, 0x2e04c, 0x2e7cc
    require(stock[cap1:cap1+4] == stock[cap2:cap2+4], 'Different AE cap load instructions')
    base, cache = 0x101000, 0xbf1000
    code = payload('interval_ae', base, ['entry.S', 'target.c'],
                   dict(ae_cache=cache, ae_continue=site+4, get_algo=0x16860, get_common=0x17180),
                   tools or compiler_tools(),
                   inputs={'cap.bin': stock[cap1:cap1+4], 'stack.bin': stock[site:site+4]})
    offset = (len(stock) + 4095) & ~4095
    rw = struct.unpack('<II6Q', headers[1])
    require(base+len(code) <= rw[3] and rw[3]+rw[6] <= cache, 'AE code/cache overlap')
    # Allocate the cache in zero-filled BSS; keep the build-id note bytes.
    struct.pack_into('<Q', headers[1], 40, cache + 4096 - rw[3])
    del headers[3]
    headers.insert(1, struct.pack('<II6Q', 1, 5, offset, base, base, len(code), len(code), 4096))
    out = bytearray(stock)
    out[header:header+count*width] = b''.join(headers)
    struct.pack_into('<I', out, site, branch(site, base))
    for address in (cap1, cap2):
        call(out, address, base+192)
    out.extend(bytes(offset - len(out)))
    out.extend(code)
    require(sha(out) == TARGET, 'Interval AE plugin reconstruction differs')
    return bytes(out)
