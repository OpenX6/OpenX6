"""Rebuild the two maintenance call sites in the official 1.2.36 updater."""
import struct

from .common import require, sha
from .compiled_recipes import call, extend_text, payload
from .raw_nr_recipe import branch
from .toolchain import compiler_tools


STOCK = '07c188c47badd4061eea6d005a526e7244d514d593495d246252fff989b63a4e'
TARGET = '7a66e650cd7c64000b63a3a971f633cd7ea91fd5205460133527aff5103466ec'


def maintenance_updater(stock, tools=None):
    require(sha(stock) == STOCK, 'Official updater digest differs')
    entry, version, base = 0xe2bb4, 0xe7ab8, 0x337a00
    symbols = dict(entry_encrypted=0xe2e20, entry_reject=0xe2bc0, entry_cleanup=0xe2c10,
                   original_version=0x1a15b0, current_got=0x374cc8,
                   integrity_got=0x374f78, hook_got=0x374ad0)
    displaced = stock[entry:entry+4]
    require(struct.unpack('<I', displaced)[0] == 0xb94253e1, 'Unexpected encoding load')
    original_call = branch(version, symbols['original_version']) | 0x80000000
    require(struct.unpack_from('<I', stock, version)[0] == original_call,
            'Unexpected native version call')
    code = payload('otad', base, ['gate.S'], symbols, tools or compiler_tools(),
                   inputs={'load-encoding.bin': displaced}, relax=False)
    out = bytearray(stock)
    extend_text(out, 2, base, code)
    struct.pack_into('<I', out, entry, branch(entry, base))
    call(out, version, base+0x40)
    require(sha(out) == TARGET, 'Maintenance updater reconstruction differs')
    return bytes(out)
