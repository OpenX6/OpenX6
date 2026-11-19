import struct, subprocess, json, tempfile
from pathlib import Path
from .gui_payload import linked_payload
from .panel_patch import SYMS,SITE
HEADER = struct.Struct('<II6Q')

def check(condition, reason):
    if not condition:
        raise ValueError(reason)

def signed(value, width):
    sign = 1 << (width - 1)
    return (value ^ sign) - sign

def patch(data,HERE,clang,LLD):
    LLVM=Path(clang).parent
    image = bytearray(data)
    def header(number):
        return list(HEADER.unpack_from(image, 64 + number * HEADER.size))
    menu, rw, code = map(header, (3, 4, 6))
    check(menu[:2] == code[:2] == [1, 5] and rw[:2] == [1, 6]
          and code[2] + code[5] == len(data) and code[5] == code[6], 'unknown GUI layout')
    def menu_offset(address):
        check(menu[3] <= address < menu[3] + menu[5], 'invalid menu address')
        return address - menu[3] + menu[2]
    # Locate the storage switch by its title reference; SDK rows can move it.
    signature = bytes.fromhex('800a00f0 00e03c91 010080d2')
    start = menu[2]
    rows = []
    while (match := image.find(signature, start, menu[2]+menu[5])) >= 0:
        start = match+4
        row = match
        row_pc = menu[3]+row-menu[2]
        title_at = row+40
        adr, = struct.unpack_from('<I',image,title_at)
        if adr & 0x9f00001f != 0x10000000:
            continue
        delta = signed(((adr>>29)&3) | (((adr>>5)&0x7ffff)<<2),21)
        title = title_at+delta
        if image[title:title+17] == b'Use External SSD\0':
            rows.append((row,row_pc))
    check(len(rows)==1, 'unknown storage switch constructor')
    row, row_pc = rows[0]
    branch, = struct.unpack_from('<I', image, SITE)
    check(branch >> 26 == 0x25, 'missing timer callback')
    trampoline = SITE + 4 * signed(branch & 0x3ffffff, 26)
    offset = menu_offset(trampoline)
    page, add, jump = struct.unpack_from('<III', image, offset)
    check(page & 0x9f00001f == 0x90000010 and add & 0xffc003ff == 0x91000210
          and jump == 0xd61f0200, 'unknown timer trampoline')
    pages = signed(((page >> 29) & 3) | (((page >> 5) & 0x7ffff) << 2), 21)
    previous = (trampoline & ~4095) + pages * 4096 + ((add >> 10) & 4095)
    check(code[3] <= previous < code[3] + code[5] and b'Show Shutter & ISO' not in data,
          'unexpected previous callback or already patched')
    address = (code[3] + code[5] + 15) & ~15
    state = (rw[3] + rw[6] + 15) & ~15
    check(state + 160 <= code[3] & ~65535, 'no space for UI state')
    source = json.loads((HERE / 'symbols.json').read_bytes())
    aliases = dict(focus_id='stats_focus_holder', focus_class='stats_focus_class',
                   live_class='stats_liveview_class', capture_class='stats_capture_class')
    symbols = SYMS | {s:int(source[s], 16) for s in ('switch_class','switch_title','switch_checked','new_string','new_slot')}
    symbols.update({s:int(source[k],16) for s,k in aliases.items()})
    symbols.update(bar_class=0x7102c18, item=0x712c00, stock_shutter=0x7134a0, stock_iso=0x713610, visible=0x58ac90, is_visible=0x58ac80, state=state, previous=previous, next_row=row_pc+4, app_id=SYMS['global_obj'],
                   app_class=SYMS['global_class'], text_font=int(source['stats_set_font'],16))
    with tempfile.TemporaryDirectory() as temp:
        t = Path(temp)
        (t/'script').write_text('\n'.join(f'{k} = {v};' for k,v in symbols.items()) +
            f'\nSECTIONS {{ . = {address}; .payload : {{ *(.text*) *(.rodata*) }}'
            ' .entries : { LONG(row_bridge) } .bad : { *(.got*) *(.data*) *(.bss*) } ASSERT(SIZEOF(.bad)==0,"unexpected data")'
            ' /DISCARD/ : { *(.note*) *(.eh_frame*) } }')
        flags = ['--target=aarch64-linux-gnu','-Oz','-ffreestanding','-fpie','-fno-builtin',
                 '-fno-stack-protector','-fno-asynchronous-unwind-tables','-fno-jump-tables',
                 '-mgeneral-regs-only','-mbranch-protection=bti+pac-ret']
        for filename in ('view.c','row.S'):
            subprocess.run([LLVM/'clang',*flags,'-c',HERE/filename,'-o',t/(filename+'.o')],check=True)
        subprocess.run([LLD,'-flavor','gnu','--no-relax','--pic-veneer','-T',t/'script','-e','exposure_tick',
                        t/'view.c.o',t/'row.S.o','-o',t/'elf'],check=True)
        target, payload = linked_payload(t/'elf')
        # The linker places the row entry in its own four-byte output section.
        image_elf = (t/'elf').read_bytes()
        sections = struct.unpack_from('<Q', image_elf, 40)[0]
        count, strings = struct.unpack_from('<HH', image_elf, 60)
        rows = [struct.unpack_from('<II4QII2Q',image_elf,sections+i*64) for i in range(count)]
        table = rows[strings]
        names = image_elf[table[4]:table[4]+table[5]]
        entry, = [r for r in rows if names[r[0]:].split(b'\0',1)[0] == b'.entries']
        check(entry[5] == 4, 'invalid row entry table')
        row_entry, = struct.unpack_from('<I',image_elf,entry[4])
        check(address <= row_entry < address+len(payload), 'invalid row entry')
    displacement = (target >> 12) - (trampoline >> 12)
    distance = row_entry - row_pc
    check(0 < displacement < 1<<20 and 0 < distance < 1<<27 and distance%4==0, 'branch range')
    struct.pack_into('<II', image, offset, 0x90000010 | ((displacement&3)<<29) | ((displacement>>2)<<5),
                     0x91000210 | ((target&4095)<<10))
    struct.pack_into('<I', image, row, 0x14000000 | distance//4)
    image.extend(bytes(address - code[3] - code[5]) + payload)
    code[5]=code[6]=len(image)-code[2]
    rw[6]=state+160-rw[3]
    for number,h in ((4,rw),(6,code)):
        HEADER.pack_into(image,64+number*HEADER.size,*h)
    return bytes(image)

