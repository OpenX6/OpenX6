"""Third-party declarative modules for locked unified builds."""
import json
from bisect import bisect_right
from pathlib import Path

from sdk import choice, menu_preview, module_plan
from .common import asset, canonical, digest, require, sha

TARGET = '/usr/bin/ins_mediad'


def load_modules(paths, base_sha256, *, stock=False):
    require(len(paths) <= 64, 'Too many selected modules')
    manifests = []
    for path in paths:
        try:
            raw = module_plan.read_bounded(path, 256 << 10)
            doc = json.loads(raw, object_pairs_hook=module_plan.unique_keys)
            require(isinstance(doc, dict), 'Expected a module manifest object')
            schema = doc.get('schema')
            require(schema != 'openx6.menu.preview.v1', 'Preview-only menu; use sdk.menu_preview for this manifest')
            if stock:
                require(schema == 1, 'Menu modules require the full OpenX6 profile')
            manifests.append(doc)
        except (ValueError, OSError, RecursionError) as exc:
            raise ValueError(f'{path}: {exc}') from exc
    if stock:
        require(manifests, 'Select a stock-component module (try stock_debug_label.json).')
        module_plan.selection(TARGET, base_sha256, manifests)
    else:
        split_selection(manifests, base_sha256)
    return manifests


def split_selection(manifests, base_sha256):
    menus, binary = [], []
    for item in manifests:
        require(isinstance(item, dict), 'Invalid module manifest')
        (menus if item.get('schema') in ('openx6.menu.v1', choice.SCHEMA) else binary).append(item)
    stats = [m for m in menus if m['schema'] == 'openx6.menu.v1']
    if stats:
        menu_preview.menu(stats)
    for doc in menus:
        if doc['schema'] == choice.SCHEMA:
            choice.validate(doc)
    require(len({m['title'] for m in menus}) == len(menus), 'Duplicate extension page title')
    menus.sort(key=lambda m: m['id'])
    if binary:
        module_plan.selection(TARGET, base_sha256, binary)
    ids = {m['id'] for m in manifests}
    require(len(ids) == len(manifests), 'Duplicate module ID')
    for item in binary:
        require(not ids.intersection(item['conflicts']), f"{item['id']}: conflicting selection")
    return menus, binary


def validate_selection(manifests, profile_dir, profile):
    action = profile['actions'][TARGET]
    _, edits = module_plan.selection(TARGET, action['sha256'], manifests)
    recipe_path = asset(profile_dir, action['asset'])
    require(digest(recipe_path) == action['asset_sha256'], 'Built-in recipe changed')
    recipe = json.loads(recipe_path.read_bytes())
    # The built-in bundle owns its edited ranges, including its appended payload.
    intervals = [(e['offset'], e['offset'] + e['bytes']) for e in recipe['edits']]
    ends = [end for _, end in intervals]
    for start, end, name, _, _ in edits:
        i = bisect_right(ends, start)
        require(end <= min(recipe['base_bytes'], recipe['target_bytes']), f'{name}: outside shared base')
        require(i == len(ends) or intervals[i][0] >= end, f'{name}: overlaps built-in patch')


def receipt(manifests, report):
    return dict(target=TARGET, modules=report['modules'], edits=report['edits'],
                input_sha256=report['input_sha256'], output_sha256=report['predicted_sha256'],
                manifests={m['id']: sha(canonical(m)) for m in sorted(manifests, key=lambda m: m['id'])},
                planner_sha256=digest(Path(module_plan.__file__)))
