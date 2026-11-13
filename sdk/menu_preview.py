"""Offline preview of contributed settings. Contains no camera or firmware operations."""
import argparse
import json
from pathlib import Path

try:
    from . import choice as native_choice
    from .module_plan import ID, check, read_bounded, unique_keys
except ImportError:  # invoked as a standalone script
    import choice as native_choice
    from module_plan import ID, check, read_bounded, unique_keys


def label(value):
    check(isinstance(value, str) and value.strip() == value and 0 < len(value) <= 48
          and value.isprintable(), 'expected a printable label of 1–48 characters')


def menu(manifests):
    modules, rows = {}, {}
    check(1 <= len(manifests) <= 64, 'select 1–64 module menus')
    bindings = set()
    for item in manifests:
        if isinstance(item, dict) and item.get('schema') == native_choice.SCHEMA:
            native_choice.validate(item)
            row = item['setting']
            name = item['id']
            check(name not in modules, 'duplicate module ID')
            modules[name] = item['title']
            rows[name + '.' + row['id']] = dict(row, type='choice',
                                                 choices=[o['id'] for o in row['options']])
            continue
        check(isinstance(item, dict) and set(item) == {'schema', 'id', 'title', 'settings'}
              and item['schema'] in ('openx6.menu.preview.v1', 'openx6.menu.v1'), 'unsupported menu manifest')
        name = item['id']
        check(isinstance(name, str) and ID.fullmatch(name) and name not in modules, 'invalid or repeated module ID')
        label(item['title'])
        if item['schema'] == 'openx6.menu.v1':
            check(item['title'].isascii(), 'native menu requires a printable ASCII title')
        modules[name] = item['title']
        check(isinstance(item['settings'], list) and 1 <= len(item['settings']) <= 16, 'expected 1–16 settings')
        for spec in item['settings']:
            check(isinstance(spec, dict), 'expected a setting')
            kind = spec.get('type')
            check(kind in ('toggle', 'choice'), 'unsupported setting type')
            fields = {'id', 'label', 'default', 'type'}
            if item['schema'] == 'openx6.menu.v1':
                fields.add('binding')
                binding = spec.get('binding')
                check(binding == 'stats.overlay.enabled', 'unsupported runtime binding')
                check(kind == 'toggle' and spec.get('default') is False, 'binding requires a toggle with default false')
                check(binding not in bindings, 'runtime binding already selected')
                bindings.add(binding)
            if kind == 'choice':
                fields.add('choices')
            check(set(spec) == fields, 'unsupported or missing setting field')
            key = spec['id']
            check(isinstance(key, str) and ID.fullmatch(key), 'invalid setting ID')
            path = name + '.' + key
            check(path not in rows, 'duplicate setting: ' + path)
            label(spec['label'])
            if item['schema'] == 'openx6.menu.v1':
                check(spec['label'].isascii(), 'native menu requires a printable ASCII label')
            if kind == 'choice':
                choices = spec['choices']
                check(isinstance(choices, list) and 2 <= len(choices) <= 8, 'expected 2–8 choices')
                for choice in choices:
                    label(choice)
                check(len(set(choices)) == len(choices) and spec['default'] in choices, 'invalid choices or default')
            else:
                check(type(spec['default']) is bool, 'expected a boolean default')
            rows[path] = spec
    return modules, rows


def preview(manifests, edits):
    modules, rows = menu(manifests)
    values = {key: row['default'] for key, row in rows.items()}
    for key, raw in edits:
        check(key in rows, 'unknown setting: ' + key)
        row = rows[key]
        if row['type'] == 'toggle':
            check(raw in ('on', 'off'), 'expected on or off')
            values[key] = raw == 'on'
        else:
            check(raw in row['choices'], 'unknown choice')
            values[key] = raw
    pages = {name: {'title': title, 'settings': []} for name, title in modules.items()}
    for key, row in rows.items():
        name = key.split('.', 1)[0]
        control = {'label': row['label'], 'type': row['type'], 'value': values[key]}
        if row['type'] == 'choice':
            control['choices'] = row['choices']
        if 'options' in row:
            opt = next(o for o in row['options'] if o['id'] == values[key])
            control.update(selected_label=opt['label'], note=opt['note'])
        pages[name]['settings'].append(control)
    return {'page': 'Mod Settings', 'modules': pages, 'values': values}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('manifests', nargs='+', type=Path)
    parser.add_argument('--set', nargs=2, action='append', default=[], metavar=('MODULE.FIELD', 'VALUE'))
    args = parser.parse_args()
    try:
        docs = [json.loads(read_bounded(p, 65536), object_pairs_hook=unique_keys) for p in args.manifests]
        print(json.dumps(preview(docs, args.set), ensure_ascii=False, indent=2))
    except (OSError, ValueError, RecursionError) as exc:
        parser.exit(1, f'Menu preview rejected: {exc}\n')
