"""Read the single text-and-constants output section and entry from a linked ARM64 ELF."""
import struct
from .common import require


def linked_payload(path):
    image = path.read_bytes()
    require(image[:6] == b'\x7fELF\x02\x01', 'Expected 64-bit little-endian ELF')
    entry, _, shoff = struct.unpack_from('<3Q', image, 24)
    width, count, names_index = struct.unpack_from('<3H', image, 58)
    require(width == 64 and names_index < count and shoff + width*count <= len(image),
            'Invalid section table')
    rows = list(struct.iter_unpack('<II4QII2Q', image[shoff:shoff+width*count]))
    _, _, _, _, pos, size, *_ = rows[names_index]
    names = image[pos:pos+size]
    result = []
    for name, typ, _, addr, pos, size, *_ in rows:
        if names[name:].split(b'\0',1)[0] == b'.payload':
            require(typ == 1 and pos+size <= len(image) and addr <= entry < addr+size,
                    'Invalid linked GUI entry')
            result.append(image[pos:pos+size])
    require(len(result) == 1, 'Expected one linked GUI payload')
    return entry, result[0]


def linked_text(source, base, imports, exports, clang, lld):
    """Assemble code and read its entry addresses using only the bundled linker."""
    from pathlib import Path
    import subprocess
    import tempfile

    with tempfile.TemporaryDirectory() as directory:
        tmp = Path(directory)
        definitions = '\n'.join(f'{name} = {value};' for name, value in imports.items())
        addresses = ' '.join(f'LONG({name})' for name in exports)
        (tmp/'layout.ld').write_text(
            f'{definitions}\nSECTIONS {{ . = {base}; .text : {{ *(.text*) }} '
            f'.entries : SUBALIGN(1) {{ {addresses} }} /DISCARD/ : {{ *(.note*) }} }}')
        subprocess.run([clang, '--target=aarch64-linux-gnu', '-c', source, '-o', 'input.o'],
                       cwd=tmp, check=True)
        subprocess.run([lld, '-flavor', 'gnu', '--no-relax', '--no-undefined', '--oformat=binary',
                        '-T', 'layout.ld', '-o', 'output', 'input.o'], cwd=tmp, check=True)
        image = (tmp/'output').read_bytes()
    split = len(image) - 4*len(exports)
    require(split > 0, 'Empty GUI hook')
    values = struct.unpack_from(f'<{len(exports)}I', image, split)
    require(all(base <= address < base+split and address % 4 == 0 for address in values),
            'GUI entry outside linked text')
    return image[:split], dict(zip(exports, values))
