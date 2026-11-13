"""Compile OpenX6 hooks and splice them into their pinned official ELF."""
from pathlib import Path
import struct
import subprocess
import tempfile

from .common import require
from .raw_nr_recipe import branch
from .toolchain import compiler_tools

SOURCES = Path(__file__).resolve().parents[1] / 'recipes'


def payload(folder, base, sources, symbols, tools, *, inputs=None, defines=(), relax=True, merge_constants=True):
    with tempfile.TemporaryDirectory() as directory:
        tmp = Path(directory)
        for name, contents in (inputs or {}).items():
            (tmp/name).write_bytes(contents)
        objects = []
        for filename in sources:
            obj = tmp/(filename + '.o')
            subprocess.run([tools['clang'], '--target=aarch64-linux-gnu', '-Os', '-fPIC',
                            '-fno-stack-protector', '-fno-builtin', '-fno-unwind-tables',
                            '-fno-asynchronous-unwind-tables', '-mbranch-protection=bti+pac-ret',
                            *defines, '-I', str(tmp), '-c', str(SOURCES/folder/filename),
                            '-o', str(obj)], cwd=tmp, check=True)
            objects.append(obj)
        (tmp/'layout.ld').write_text(
            'SECTIONS { . = ' + hex(base) + '; .payload : { *(.text.entry) *(.text*) *(.rodata*) } '
            '/DISCARD/ : { *(.comment) *(.note*) *(.eh_frame*) } }')
        subprocess.run([tools['lld'], '-flavor', 'gnu', '--no-undefined',
                        *([] if relax else ['--no-relax']),
                        *([] if merge_constants else ['-O0']),
                        '--oformat=binary', '-T', 'layout.ld', '-o', 'payload',
                        *objects, *[f'--defsym={k}={hex(v)}' for k,v in symbols.items()]],
                       cwd=tmp, check=True)
        return (tmp/'payload').read_bytes()


def video_nr(stock, gain, tools=None):
    require(gain in (0, 0.33, 0.66, 1), 'Invalid video NR gain')
    site, base = 0x18c04, 0xb6e00
    phoff = struct.unpack_from('<Q', stock, 32)[0]
    require(struct.unpack_from('<QQ', stock, phoff+32) == (0xb6d84, 0xb6d84),
            'Unexpected ISP executable segment')
    code = payload('video_nr', base, ['strength.S'],
                   {'native_resume':site+4, 'native_curves':0xcacd0}, tools or compiler_tools(),
                   inputs={'displaced.bin':stock[site:site+4]},
                   defines=[f'-DNR_GAIN={float(gain)}'], relax=False)
    end = base+len(code)
    require(len(code) == 337 and stock[base:end] == bytes(len(code)), 'Unexpected ISP hook space')
    out = bytearray(stock)
    out[base:end] = code
    struct.pack_into('<I', out, site, branch(site, base))
    struct.pack_into('<QQ', out, phoff+32, end, end)
    return bytes(out)


def fast_capture(stock, tools=None):
    base, offset, site = 0x70000, 0x60000, 0x28afc
    code = payload('fast_capture', base, ['entry.S','passthrough.c'],
                   {'strtol_plt':0x19290}, tools or compiler_tools())
    require(0 < len(code) < 4096 and code[:4] == bytes.fromhex('5f2403d5'),
            'Capture entry must start with BTI')
    return append_hook(stock, code, base, offset, site, alignment=0x10000, link=True)


def append_hook(stock, code, base, offset, site, *, alignment, link=False):
    phoff = struct.unpack_from('<Q', stock, 32)[0]
    size, count = struct.unpack_from('<HH', stock, 54)
    require(size == 56 and count >= 6 and phoff+size*count <= len(stock) <= offset,
            'Unexpected hook layout')
    headers = [stock[phoff+i*size:phoff+(i+1)*size] for i in range(count)]
    require([struct.unpack_from('<I', h)[0] for h in headers[:6]] == [1,1,2,4,4,0x6474e553],
            'Unexpected hook program headers')
    del headers[4]  # Reuse the build-id note header. Keep the original note data.
    headers.insert(2, struct.pack('<IIQQQQQQ', 1,5,offset,base,base,len(code),len(code),alignment))
    out = bytearray(stock)
    out[phoff:phoff+size*count] = b''.join(headers)
    struct.pack_into('<I', out, site, branch(site,base) | (0x80000000 if link else 0))
    return bytes(out).ljust(offset,b'\0') + code


def interval_wait(stock, tools=None):
    site, base = 0x39530, 0x110000
    code = payload('interval_wait', base, ['wait_entry.S', 'wait.c'],
                   {'wait_continue': site + 4}, tools or compiler_tools(),
                   inputs={'displaced.bin': stock[site:site+4]})
    require(len(code) == 622, 'Unexpected interval acquisition hook size')
    return append_hook(stock, code, base, (len(stock)+4095) & ~4095, site, alignment=4096)


def extend_text(out, header_index, start, code):
    phoff = struct.unpack_from('<Q', out, 32)[0]
    phsize = struct.unpack_from('<H', out, 54)[0]
    header = phoff + phsize*header_index
    kind, flags, offset, va, _, fs, ms, _ = struct.unpack_from('<II6Q', out, header)
    end = start + len(code)
    next_offset = struct.unpack_from('<Q', out, header+phsize+8)[0]
    require((kind, flags, offset, va) == (1,5,0,0) and fs == ms and
            fs <= start < end <= next_offset and not any(out[fs:end]),
            'Expected padding after executable segment')
    out[start:end] = code
    struct.pack_into('<QQ', out, header+32, end, end)


def call(out, site, target):
    struct.pack_into('<I', out, site, branch(site, target) | 0x80000000)


def base_gui(stock, tools=None):
    tools = tools or compiler_tools()
    out = bytearray(stock)
    shim = payload('base_gui', 0x70082f0, ['start.S'],
                   {'original_print':0x977750, 'original_dlsym':0x4831f0, 'original_done':0x9772a0}, tools)
    require(len(shim) == 162 and shim[72:76] == shim[:4] == bytes.fromhex('5f2403d5'), 'Unexpected GUI lifecycle entry')
    extend_text(out, 2, 0x70082f0, shim)
    call(out, 0x484620, 0x70082f0)
    call(out, 0x4846b8, 0x7008338)

    labels = {'EwNewStringAnsi':0x4e3e80, 'WidgetsSetEntryDualText_OnSetStringDesc':0x963080}
    display = payload('base_gui', 0x87d460, ['display_version.S'], labels, tools)
    require(len(display) == 72 and display[60:69] == b'1.2.36 S\0', 'Unexpected version label')
    init = payload('base_gui', 0x8cd0a0, ['display_init.S'], dict(labels, version_label=0x87d460+60), tools)
    require(len(init) == 28, 'Unexpected display initializer size')
    out[0x87d460:0x87d460+72] = display
    out[0x8cd0a0:0x8cd0a0+28] = init

    menu = payload('base_gui', 0x70083a0, ['max_menu.S'],
                   {'CommClassStringOptionList05_OnSetOption01':0x57e830,
                    'CommClassIntOptionList05_OnSetOption02':0x57e480,
                    'CommClassStringOptionList05_OnSetOption02':0x57e880,
                    'EwNewStringAnsi':labels['EwNewStringAnsi']}, tools)
    extend_text(out, 2, 0x70083a0, menu)
    for site in (0x729338, 0x729364):
        struct.pack_into('<I', out, site, 0x52800061)  # mov w1, #3 (Std, High, Max)
    call(out, 0x7293a0, 0x70083a0)
    return bytes(out)


def gui_wrapper(stock, tools=None):
    base, site = 0x2e9c30, 0x112dd0
    out = bytearray(stock)
    gate = payload('base_gui', base, ['owner_gate.c'], {'owner_notify':0x8a420},
                   tools or compiler_tools(), defines=['-ffreestanding'])
    require(len(gate) == 1312 and gate[:4] == bytes.fromhex('3f2303d5'),
            'Unexpected owner GUI broker entry')
    extend_text(out, 0, base, gate)
    struct.pack_into('<II', out, site, 0xd503245f, branch(site+4,base))
    for site in (0x177540,0x20cbb8):
        struct.pack_into('<I', out, site, 0x71000c1f)  # cmp w0, #3 (through Custom)
    struct.pack_into('<I', out, 0x20cbbc, 0x2a0003f4)  # mov w20,w0
    out[0x20cbc0] = (stock[0x20cbc0] & 0xf0) | 9  # unsigned lower-or-same
    return bytes(out)
