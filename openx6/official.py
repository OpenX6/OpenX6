"""Strict reader for the single pinned official c9 1.2.36 package."""
import os, struct, hashlib, zipfile
from pathlib import Path
from .common import require, regular, digest, sha, save, safe_relative

OFFICIAL_SHA = '2c0b9685f2861455d9f248bf2b8897b24c8fba05dd79662deee0796b0dbd6853'
RECOGNIZED_1359_SHA = 'c6b3601182b52f17b4ca256a0ac036b8fefc50987b735270e225b6c9359f33f4'
ZIP_SHA = '3805d2d958a05ffa3f2cfb47ceaee1d2ab50d2674ba810b938dd26346e845aa5'
SYSTEM_SHA = '537872b094a67405197332c37d9160ee99ed307653f9de77e2383abcec1869f3'
COMPONENTS = {
 'qcs8625_ota.zip': (1733914751, ZIP_SHA),
 'MCUFw-V1.2.0.0.bin': (130048, '9c1f0427e8562b25d64b360c530f1266d10d6cf0f40dcf3ff2dcee33c447991b'),
 'cs_press-V84.11.1.8.nfw': (44160, 'c14400a71f1ea7518f9f05ae953d796c1533a5012133618df8fb4974952f5239'),
}
CHUNK = 4 * 1024 * 1024
u32 = lambda b, p: struct.unpack_from('<I', b, p)[0]
cstr = lambda b: b.split(b'\0', 1)[0].decode('ascii')

def inspect(source):
    with regular(source) as f:
        return inspect_stream(f)


def inspect_stream(f):
    size = f.seek(0, os.SEEK_END)
    f.seek(0)
    head = f.read(5376)
    h = hashlib.sha256(head)
    remaining = max(0, size - len(head) - 4480)
    while remaining:
        block = f.read(min(remaining, CHUNK))
        require(block, 'Truncated official firmware')
        h.update(block)
        remaining -= len(block)
    tail = f.read(4480)
    require(f.tell() == size and not f.read(1), 'Official firmware size changed')
    h.update(tail)
    checksum = h.hexdigest()
    require(checksum != RECOGNIZED_1359_SHA,
            'Official X6 1.3.59 recognized. Select official 1.2.36 to build. / '
            '已识别官方 X6 1.3.59。此版本仅支持从官方 1.2.36 构建，请重新选择。')
    require(checksum == OFFICIAL_SHA, 'Unknown official firmware: whole-file SHA-256 rejected / 未识别的固件，完整文件摘要不匹配')
    require(len(head) == 5376 and len(tail) == 4480, 'Truncated package')
    require(head[:4] == b'MWFI' and u32(head,4) == 1 and cstr(head[24:88]) == '1.2.36', 'Header/version rejected')
    require(cstr(head[8:24]) == 'c9' and (u32(head,92),u32(head,96),u32(head,100)) == (5,1,35), 'Model/key ID/encryption rejected')
    entries = [struct.unpack_from('<III',head,4096+i*256) for i in range(5)]
    require([x[0] for x in entries] == [1,1,1,4096,4097], 'Module types rejected')
    require(tail[64:68] == b'CFSI' and tail[-256:-252] == b'CWFI', 'Tail rejected')
    return dict(sha256=OFFICIAL_SHA, bytes=size, version='1.2.36', model='c9', key_id=35,
                entries=entries, cipher_end=size-4480, plaintext_end=entries[3][1]), head, tail

def import_modules(source, directory, dest):
    """Reassemble and verify the official plaintext digest from exported modules."""
    report, head, tail = inspect(source)
    directory = Path(directory)
    require(directory.is_dir() and not directory.is_symlink(), 'Select the verified X6 module folder.')
    names = set(p.name for p in directory.iterdir())
    missing = sorted(set(COMPONENTS) - names)
    require(not missing, 'Missing required X6 module / 缺少必需模块: ' + ', '.join(missing))
    require(names == set(COMPONENTS), 'Expected only the three verified X6 modules.')
    layout = [('qcs8625_ota.zip', 4, 'host_8625', '1.2.36'),
              ('MCUFw-V1.2.0.0.bin', 5, 'slave_main_mcu', '1.2.0'),
              ('cs_press-V84.11.1.8.nfw', 2, 'accessory_sensor_press', '84.11.1.8')]
    dest.mkdir(mode=0o700)
    prefix = hashlib.sha256(head)
    cursor = len(head)
    for index, (name, kind, method, version) in enumerate(layout, 1):
        size, expected = COMPONENTS[name]
        require(tuple(report['entries'][index-1]) == (1, cursor, size+576), 'Module layout mismatch.')
        hdr = bytearray(576)
        struct.pack_into('<I', hdr, 64, index)
        struct.pack_into('<I', hdr, 132, kind)
        for offset, value in ((68, name), (136, 'full'), (152, method), (184, version), (264, expected)):
            hdr[offset:offset+len(value)] = value.encode('ascii')
        prefix.update(hdr)
        h = hashlib.sha256()
        with regular(directory/name) as src, (dest/name).open('xb') as dst:
            require(os.fstat(src.fileno()).st_size == size, 'Wrong module size: '+name)
            while block := src.read(CHUNK):
                dst.write(block)
                prefix.update(block)
                h.update(block)
        require(h.hexdigest() == expected, 'Wrong module digest: '+name)
        if index == 1:
            (dest/'module-header.bin').write_bytes(hdr)
        cursor += size + len(hdr)
    require(report['plaintext_end'] == cursor+64, 'Wrong plaintext length.')
    prefix.update(bytes(64))
    require(prefix.hexdigest().encode() == tail[4232:4296], 'Official plaintext digest mismatch.')
    (dest/'header.bin').write_bytes(head)
    (dest/'tail.bin').write_bytes(tail)
    report.update(plaintext_sha256=prefix.hexdigest(), all_module_hashes_match=True,
                  MCU_emitted=True, key_material_emitted=False)
    save(dest/'official.json', report)
    return report


def parse_ranges(text, maximum=786432):
    a=list(map(int,text.split(',')))
    require(a and a[0]==len(a)-1 and a[0]%2==0,'Invalid transfer range')
    pairs=list(zip(a[1::2],a[2::2]))
    require(all(0<=l<r<=maximum for l,r in pairs),'Out-of-bounds transfer range')
    return pairs

def reconstruct(zip_path, output):
    require(digest(zip_path)==ZIP_SHA,'Official component ZIP changed')
    with zipfile.ZipFile(zip_path) as z:
        names=z.namelist();require(len(names)==len(set(names))==34,'ZIP member set rejected')
        for name in names:safe_relative(name)
        lines=z.read('system.transfer.list').decode().splitlines()
        require(lines[0]=='3' and lines[2:4]==['0','0'],'Unsupported transfer format')
        ops=[]; used=bytearray(786432);counts=dict(new=0,zero=0,erase=0)
        for line in lines[4:]:
            op, arg=line.split();require(op in counts,'Non-full-OTA operation rejected')
            ranges=parse_ranges(arg)
            for l,r in ranges:
                require(not any(used[l:r]),'Overlapping transfer operation');used[l:r]=b'\1'*(r-l);counts[op]+=r-l
            ops.append((op,ranges))
        require(counts['new']*4096==z.getinfo('system.new.dat').file_size,'new.dat length rejected')
        require(counts['new']+counts['zero']==int(lines[1]),'Transfer totals rejected')
        with Path(output).open('xb') as dst,z.open('system.new.dat') as src:
            dst.truncate(786432*4096)
            for op,ranges in ops:
                if op!='new':continue
                for l,r in ranges:
                    dst.seek(l*4096);left=(r-l)*4096
                    while left:
                        b=src.read(min(left,CHUNK));require(bool(b),'Truncated new.dat');left-=len(b);dst.write(b)
            require(not src.read(1),'Trailing block data')
    require(digest(output)==SYSTEM_SHA,'Reconstructed official filesystem digest rejected')
    return dict(bytes=Path(output).stat().st_size,sha256=SYSTEM_SHA,block_counts=counts)
