"""Bind the RAW mode's event loop to the selected native menu layout."""
import hashlib
import struct
from .common import require
from .panel_patch import SITE

HEADER = struct.Struct('<II6Q')

def timer_stub(source, target):
    pages = (target >> 12) - (source >> 12)
    require(0 < pages < 1 << 20, 'RAW timer trampoline out of reach')
    return struct.pack('<II', 0x90000010 | (pages & 3) << 29 | (pages >> 2) << 5,
                       0x91000210 | (target & 4095) << 10)

def bind_recipe(template, gui):
    edits = [item.copy() for item in template['edits']]
    require(len(gui) == template['end'], 'RAW menu code layout differs')
    header_at = 64 + 3 * HEADER.size
    menu = HEADER.unpack_from(gui,header_at)
    require(menu[:2] == (1,5) and menu[5] == menu[6], 'Invalid native menu segment')
    call, = struct.unpack_from('<I',gui,SITE)
    require(call >> 26 == 0x25, 'Missing native timer call')
    pc = SITE + 4 * (call & 0x3ffffff)
    offset = menu[2] + pc - menu[3]
    reference_offset, old, new = edits[0]
    reference_pc = menu[3] + reference_offset - menu[2]
    def destination(stub):
        adrp, add = struct.unpack('<II', bytes.fromhex(stub))
        pages = ((adrp >> 29) & 3) | (((adrp >> 5) & 0x7ffff) << 2)
        addr = (reference_pc & ~4095) + (pages << 12) + ((add >> 10) & 4095)
        require(timer_stub(reference_pc,addr).hex()==stub, 'Invalid RAW timer template')
        return addr
    before = timer_stub(pc,destination(old))
    require(menu[2] <= offset <= menu[2]+menu[5]-12 and gui[offset:offset+12]==before+bytes.fromhex('00021fd6'),
            'RAW timer chain differs')
    edits[0] = [offset,before.hex(),timer_stub(pc,destination(new)).hex()]
    index, = [i for i,(off,_,_) in enumerate(edits) if off==header_at]
    _, before, after = edits[index]
    reference = HEADER.unpack(bytes.fromhex(before))
    require(menu[:5]==reference[:5] and menu[7]==reference[7] and menu[5] <= reference[5],
            'Insufficient space for RAW menu callbacks')
    edits[index] = [header_at,gui[header_at:header_at+HEADER.size].hex(),after]
    return dict(pin=hashlib.sha256(gui).hexdigest(),end=len(gui),edits=edits)
