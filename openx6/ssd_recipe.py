"""Build the two OpenX6 storage gates using the same layout as the coordinator."""
import json
import struct

from .common import require, sha
from .compiled_recipes import SOURCES, extend_text, payload
from .raw_nr_recipe import branch
from .toolchain import compiler_tools


def ssd_layout():
    return json.loads((SOURCES/'ssd/layout.json').read_bytes())


def ssd_gate(stock, path, tools=None):
    spec = next((s for s in ssd_layout() if path == '/usr/bin/' + s['name']), None)
    require(spec and sha(stock) == spec['old'], 'Official SSD service digest differs')
    site, base, taken, fall = (spec[k] for k in ('site', 'base', 'taken', 'fall'))
    original = 0x54000000 | ((((taken-site)//4) & 0x7ffff) << 5)
    require(fall == site+4 and struct.unpack_from('<I', stock, site)[0] == original,
            'Unexpected original SSD branch')
    code = payload('ssd', base, ['active.S'],
                   dict(taken=taken, fall=fall, active_target=taken if spec['active_taken'] else fall),
                   tools or compiler_tools())
    require(len(code) == spec['bytes'] and spec['offset'] == base, 'Unexpected SSD hook layout')
    out = bytearray(stock)
    extend_text(out, 2, base, code)
    struct.pack_into('<I', out, site, branch(site, base))
    require(sha(out) == spec['new'], 'SSD gate reconstruction differs')
    return bytes(out)

def verify_installed_hooks(read, tools):
    """The startup service checks the final executable, after all capture edits."""
    hooks = json.loads(read('/usr/share/openx6/ssd/hooks.json'))
    require([h['name'] for h in hooks] == ['ins_configd', 'ins_bla'], 'Incomplete SSD hook list')
    for hook in hooks:
        path = '/usr/bin/' + hook['name']
        binary = read(path)
        require(sha(binary) == hook['new'], 'SSD startup digest differs for ' + path)
        site, base, offset = hook['site'], hook['base'], hook['offset']
        require(struct.unpack_from('<I', binary, site)[0] == branch(site, base),
                'SSD startup branch differs for ' + path)
        target = hook['taken'] if hook['active_taken'] else hook['fall']
        code = payload('ssd', base, ['active.S'],
                       dict(taken=hook['taken'], fall=hook['fall'], active_target=target), tools)
        require(len(code) == hook['bytes'] and binary[offset:offset+len(code)] == code,
                'SSD startup hook differs for ' + path)
    require(read('/ins/etc/storaged.json') == read('/usr/share/openx6/ssd/storaged.original.json'),
            'SSD storage configuration differs')
