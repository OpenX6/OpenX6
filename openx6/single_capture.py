"""Rebuild capture profiles, sensor mode 114 and encoder hooks from official inputs."""
import base64
import copy
import json
import struct
import zlib

from .common import require, sha
from .compiled_recipes import SOURCES, payload
from .mediad_recipe import media_service
from .ssd_recipe import ssd_gate

RECIPES = SOURCES / 'single_capture'
GROUP_PATH = '/ins/etc/ins_mediad/ins_media_stream_prd.json'
TUNING = '/usr/lib/camera/instasettings.config'


def video_groups(raw, kind):
    original = json.loads(raw)
    result = copy.deepcopy(original)
    for edit in json.loads((RECIPES/'video_groups.json').read_bytes())[kind]:
        dest, source = result, original
        for key in edit['list']:
            dest, source = dest[key], source[key]
        if 'clone' in edit:
            row = copy.deepcopy(source[edit['clone']])
            for path, value in edit['set']:
                obj = row
                for key in path[:-1]:
                    obj = obj[key]
                obj[path[-1]] = value
        else:
            row = edit['append']
        dest.insert(edit['index'], row)
    return update_capture_table(result, kind)


def apply_table_edits(original, edits):
    updated = copy.deepcopy(original)
    for operation, path, *args in edits:
        dest, source = updated, original
        if operation == 'set':
            for key in path[:-1]:
                dest = dest[key]
            dest[path[-1]] = args[0]
            continue
        require(operation == 'splice', 'Unknown capture table operation')
        for key in path:
            dest, source = dest[key], source[key]
        start, end, items = args
        require(0 <= start <= end <= len(source), 'Invalid capture table range')
        rows = [apply_table_edits(source[item['copy']], item['edits'])
                if 'copy' in item else item['value'] for item in items]
        dest[start:end] = rows
    return updated


def update_capture_table(result, kind):
    spec = json.loads((RECIPES/'capture_tables.json').read_bytes())[kind]
    compact = lambda table: json.dumps(table, separators=(',', ':')).encode()
    require(sha(compact(result)) == spec['base'], 'Unexpected original capture table')
    result = apply_table_edits(result, spec['edits'])
    require(sha(compact(result)) == spec['target'], 'Updated capture table differs')
    return json.dumps(result, separators=(',', ':')).encode()


def admission(raw, kind):
    result = bytearray(raw)
    for at, count, text in reversed(json.loads((RECIPES/('admission-'+kind+'.json')).read_bytes())):
        require(0 <= at <= len(raw)-count, 'Invalid camera admission edit')
        result[at:at+count] = text.encode()
    return bytes(result)


def checked_words(raw, name):
    out = bytearray(raw)
    for at, before, after in json.loads((RECIPES/'instructions.json').read_bytes())[name]:
        require(out[at:at+4] == bytes.fromhex(before), 'Unexpected instruction at '+hex(at))
        out[at:at+4] = bytes.fromhex(after)
    return out


def single_groups(profile):
    stock_groups = json.loads(profile)['stream_profile']['video_stream_groups']
    groups = {g['group_id'] for g in stock_groups
              if g['group_id'].split('_')[:1] in (['Front'], ['Back'], ['SATAngle'])
              and g['group_id'].split('_')[1:2] in (['NormalVideo'], ['FreeFrameVideo'])
              and not g['group_id'].endswith('_lowpower')
              and any(s['type'] == 'main_stream' for s in g['stream_param'])}
    groups.update(('SATAngle_FreeFrameVideo_4K_1x1_100', 'SATAngle_FreeFrameVideo_4K_4x3_120'))
    require(len(groups) == 192, 'Unexpected number of stock stream groups')
    return sorted(groups)


def group_table(names):
    return b''.join(bytes([len(g)])+g.encode('ascii') for g in names)+bytes(1)


def extended_media(raw, profile, tools):
    table = group_table(single_groups(profile))
    symbols = dict(config_mgr_got=0x8d5a10, native_get_config=0x169590, continuation=0x308da4,
                   color_mgr_got=0x8d5a88, native_color=0x2ff4d0)
    code = payload('single_capture', 0x906000, ['single_rate.S'], symbols, tools,
                   inputs={'groups.table':table}, relax=False)
    base = checked_words(media_service(raw, tools), '/usr/bin/ins_mediad')
    return bytes(base).ljust(0x8d6000, b'\0') + code


def recording_route(media, stock_profile, tools):
    from . import proxy_profile
    from .compiled_recipes import call

    require(sha(media) == 'a386e1c189a39325e1cba71120be7ad5db23e553b6f624e83c005c11bf5f3ad7',
            'Unexpected recording service before proxy routing')
    alternate = proxy_profile.build(video_groups(stock_profile, 'streams'))
    groups = json.loads(alternate)['stream_profile']['video_stream_groups']
    names = [group['group_id'] for group in groups]
    require(len(names)==89, 'Unexpected proxy routes')
    table = group_table(names[:77])
    site, entry = 0x33d768, 0x90ca40
    code = payload('single_capture', entry, ['proxy_route.S'], {'native_open_json': 0x33b840},
                   tools, inputs={'recording_groups.table': table}, relax=False)
    out = bytearray(media)
    phoff = struct.unpack_from('<Q', out, 32)[0]
    phsize = struct.unpack_from('<H', out, 54)[0]
    header = phoff + 5 * phsize
    kind, flags, offset, va, _, filesz, memsz, _ = struct.unpack_from('<II6Q', out, header)
    require((kind, flags, filesz) == (1, 5, memsz) and offset + filesz == len(out)
            and entry == va + ((filesz + 15) & ~15)
            and out[site:site+4] == bytes.fromhex('36f8ff97'), 'Unexpected recording hook layout')
    out.extend(bytes(entry - va - filesz))
    out.extend(code)
    struct.pack_into('<QQ', out, header + 32, len(out) - offset, len(out) - offset)
    call(out, site, entry)
    return capture_rate_routes(out, header, stock_profile, names, tools)


def capture_rate_routes(media, header, profile, groups, tools):
    require(sha(media) == '95eca63e20c111e49160bdef896d85bfe7939726af997e0ebc70ba4761cd8f9a', 'Unexpected encoder before capture mode extension')
    front_back = [f'{lens}_FreeFrameVideo_{size}_1x1_30'
                  for size in ('3K', '5K', '8K') for lens in ('Front', 'Back')]
    single = front_back + single_groups(profile)
    aliases = [f'SATAngle_FreeFrameVideo_1080P_1x1_{fps}' for fps in (30,25,24)]
    inputs = {f'{n}.table': group_table(groups[:n]) for n in (85,86,87,88,89)}
    inputs.update({
        'legacy-single.table': group_table(single + aliases + groups[83:85]),
        'legacy-proxy.table': group_table(groups[:77] + aliases + groups[83:85]),
        'capture-single.table': group_table(single + groups[77:85]),
    })
    start = 0x90d4e0
    symbols = dict(format_continue=0x31248c, format_previous=0x909548, get_color=0x2ff4d0,
                   color_continue=0x2c3e30, media_got=0x8d5000, store_rate=0x9061e8,
                   regular_rate=0x906164, max_rate=0x906198)
    code = payload('single_capture', start, ['capture_rate.S'], symbols, tools,
                   inputs=inputs, relax=False)
    media.extend(bytes(start-0x30000-len(media)) + code)
    struct.pack_into('<QQ', media, header+32, len(media)-0x8d0000, len(media)-0x8d0000)
    # Three routing branches and the two group-table references are pinned to the input above.
    for site, instruction in ((0x2c3e2c,0x14193799),(0x312480,0x1417f4f8),
                              (0x8d608c,0x1004bf4d),(0x8d6160,0x14003434),
                              (0x8dca4c,0x3003994c)):
        struct.pack_into('<I', media, site, instruction)
    require(sha(media) == 'dbc3ef7b694bd45719a3eb750586b762f794f55aa8d379a4786bba56c8848ea5', 'Rebuilt recording routes differ')
    return bytes(media)


def local_recipe(original, target):
    # Generate the page payloads only on the builder holding the official input.
    edits = []
    for at in range(0, len(target), 4096):
        data = target[at:at+4096]
        old = original[at:at+len(data)]
        if old != data:
            edits.append(dict(offset=at, bytes=len(data), before=sha(old),
                              data=base64.b64encode(zlib.compress(data,9)).decode()))
    spec = dict(version=1, base_sha256=sha(original), base_bytes=len(original),
                target_sha256=sha(target), target_bytes=len(target), edits=edits)
    return (json.dumps(spec, indent=2)+'\n').encode()


def reconstruct(rule, original, read, tools):
    kind = rule['variant']
    if kind in ('streams', 'caps', 'fov'):
        data = video_groups(original, kind)
    elif kind in ('linked', 'native'):
        return admission(original, kind)
    elif kind == 'sensor':
        data = sensor_mode(original)
    elif kind == 'encoder':
        profile = read(GROUP_PATH)
        require(sha(profile) == rule['groups_sha256'], 'Stock stream profile differs')
        data = recording_route(extended_media(original, profile, tools), profile, tools)
        # Keep untouched bytes available to independent binary modules, even
        # on a page containing another built-in change.
        from .patch import make_recipe
        return (json.dumps(make_recipe(original, data), indent=2)+'\n').encode()
    else:
        require(kind in ('config', 'buffers'), 'Unknown capture variant')
        base = ssd_gate(original, rule['path'], tools) if kind == 'config' else original
        data = checked_words(base, rule['path'])
    return local_recipe(original, bytes(data))


def sensor_mode(original):
    FLL = 4167
    def u32(data, offset): return struct.unpack_from('<I', data, offset)[0]
    header=bytearray(original[:216])
    sections={}
    for i in range(4):
        k,o,n=struct.unpack_from('<III',header,168+12*i)
        sections[k]=bytearray(original[o:o+n])
    assert u32(header,28)==len(original)-4 and zlib.crc32(original[:-4])==u32(original,len(original)-4)
    data=sections[1]; table=list(struct.iter_unpack('<IIII',sections[3]))
    def sym(i):
        index,off,n,kind=table[i-1]
        assert index==i
        return bytes(data[off:off+n])
    def add(blob):
        i=len(table)+1;table.append((i,len(data),len(blob),i));data.extend(blob);return i
    old_modes=sym(26)
    assert len(old_modes)==114*332
    mode=bytearray(old_modes[15*332:16*332])
    old_fll=u32(mode,48);old_hz=struct.unpack_from('<d',mode,72)[0]
    new_hz=round(old_hz*old_fll/FLL,6)
    struct.pack_into('<I',mode,48,FLL)
    struct.pack_into('<d',mode,72,new_hz)
    assert u32(mode,116)==179
    regs=bytearray(sym(u32(mode,120)))
    changed=[]
    for pos in range(0,len(regs),40):
        addr,n,ref=struct.unpack_from('<III',regs,pos+8)
        if addr in (0x340,0x341):
            shift=8 if addr==0x340 else 0
            assert n==1 and u32(sym(ref),0)==(old_fll>>shift)&255
            struct.pack_into('<I',regs,pos+16,add(struct.pack('<I',(FLL>>shift)&255)))
            changed.append(addr)
    assert sorted(changed)==[0x340,0x341]
    struct.pack_into('<I',mode,120,add(regs))
    array=old_modes+mode
    countref=struct.pack('<II',114,26)
    assert data.count(countref)==1
    struct.pack_into('<I',data,data.index(countref),115)
    index,_,_,kind=table[25];table[25]=(index,len(data),len(array),kind);data.extend(array)
    sections[3]=b''.join(struct.pack('<4I',*s) for s in table)
    contents=bytearray()
    for i,k in enumerate((0,3,2,1)):
        struct.pack_into('<III',header,168+12*i,k,len(header)+len(contents),len(sections[k]))
        contents.extend(sections[k])
    struct.pack_into('<I',header,28,len(header)+len(contents))
    image=header+contents;image.extend(struct.pack('<I',zlib.crc32(image)))
    return bytes(image)
