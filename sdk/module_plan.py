"""Plan equal-length OpenX6 module edits against an exact input, offline."""
import argparse
import hashlib
import json
import os
import re
import stat
import sys
from pathlib import Path

TARGETS = ('example', '/usr/bin/ins_mediad')
ID = re.compile(r'[a-z][a-z0-9_]{0,47}')
BASE = re.compile(r'[a-f0-9]{64}')


def check(condition, message):
    if not condition:
        raise ValueError(message)


def unique_keys(pairs):
    result = {}
    for key, value in pairs:
        check(key not in result, f'duplicate field {key}')
        result[key] = value
    return result


def read_bounded(path, maximum):
    path = Path(path)
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or getattr(info, 'st_file_attributes', 0) & stat.FILE_ATTRIBUTE_REPARSE_POINT:
        raise ValueError(f'expected a regular input file: {path.name}')
    flags = os.O_RDONLY | getattr(os, 'O_BINARY', 0) | getattr(os, 'O_NONBLOCK', 0) | getattr(os, 'O_NOFOLLOW', 0)
    with os.fdopen(os.open(path, flags), 'rb') as f:
        check(os.path.samestat(info, os.fstat(f.fileno())), f'input changed while opening: {path.name}')
        data = f.read(maximum + 1)
    check(0 < len(data) <= maximum, f'input size outside limits: {path.name}')
    return data


def identifiers(values):
    check(isinstance(values, list) and len(values) <= 64, 'expected a list of module IDs')
    check(all(isinstance(v, str) and ID.fullmatch(v) for v in values), 'invalid module ID')
    check(len(values) == len(set(values)), 'duplicate module ID')
    return set(values)


def octets(value):
    check(isinstance(value, str) and 0 < len(value) <= 8192
          and re.fullmatch(r'(?:[0-9a-f]{2})+', value), 'invalid hex edit')
    return bytes.fromhex(value)


def selection(target, base, manifests):
    check(target in TARGETS, 'unsupported component')
    check(1 <= len(manifests) <= 64, 'select between 1 and 64 modules')
    modules, edits = {}, []
    for m in manifests:
        check(isinstance(m, dict) and set(m) == {
            'schema', 'id', 'target', 'base_sha256', 'requires', 'conflicts', 'edits'
        }, 'unknown or missing manifest field')
        check(type(m['schema']) is int and m['schema'] == 1 and m['target'] == target, 'schema or target mismatch')
        name = m['id']
        identifiers([name])
        check(name not in modules, 'duplicate selected module')
        check(isinstance(m['base_sha256'], str) and BASE.fullmatch(m['base_sha256'])
              and m['base_sha256'] == base, f'{name}: component digest mismatch')
        modules[name] = identifiers(m['requires']), identifiers(m['conflicts'])
        check(isinstance(m['edits'], list) and 1 <= len(m['edits']) <= 256, 'invalid edit count')
        for edit in m['edits']:
            check(isinstance(edit, dict) and set(edit) == {'offset', 'before', 'after'}, 'invalid edit fields')
            start = edit['offset']
            before, after = octets(edit['before']), octets(edit['after'])
            check(type(start) is int and start >= 0 and len(before) == len(after)
                  and before != after, 'invalid offset, size or empty change')
            end = start + len(before)
            edits.append((start, end, name, before, after))
    for name, (required, conflicts) in modules.items():
        check(required <= modules.keys(), f'{name}: missing dependency')
        check(not conflicts.intersection(modules), f'{name}: conflicting selection')
    order = []
    while len(order) < len(modules):
        ready = sorted(n for n, (deps, _) in modules.items() if n not in order and deps.issubset(order))
        check(ready, 'module dependency cycle')
        order.extend(ready)
    edits.sort()
    cursor = 0
    for start, end, name, _, _ in edits:
        check(start >= cursor, f'{name}: overlapping patch ranges')
        cursor = end
    return order, edits


def render(source, target, manifests):
    base = hashlib.sha256(source).hexdigest()
    order, edits = selection(target, base, manifests)
    result = bytearray(source)
    for start, end, name, before, after in edits:
        check(end <= len(source) and source[start:end] == before, f'{name}: unexpected original bytes')
        result[start:end] = after
    report = dict(schema=1, action='dry-run', target=target, modules=order, edits=len(edits),
                  input_sha256=base, predicted_sha256=hashlib.sha256(result).hexdigest())
    return report, bytes(result)


def plan(source, target, manifests):
    return render(source, target, manifests)[0]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base', required=True, type=Path)
    parser.add_argument('--target', required=True, choices=TARGETS)
    parser.add_argument('manifests', nargs='+', type=Path)
    args = parser.parse_args()
    check(len(args.manifests) <= 64, 'too many modules')
    modules = [json.loads(read_bounded(p, 256 << 10), object_pairs_hook=unique_keys) for p in args.manifests]
    print(json.dumps(plan(read_bounded(args.base, 256 << 20), args.target, modules), indent=2))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, RecursionError) as exc:
        sys.exit(f'OpenX6 module plan rejected: {exc}')
