from unittest.mock import patch
import json
import os
import tempfile
import unittest
from pathlib import Path

from openx6.common import digest, sha
from openx6.component_sources import SOURCES, assemble, generate_rule, inventory, render_stock
from openx6.kit import TRUST
from openx6.patch import apply_recipe


class ComponentSources(unittest.TestCase):
    def test_review_dispositions_cover_signed_inventory_once(self):
        profile = SOURCES/'profile.json'
        pin = json.loads((TRUST/'preview.json').read_bytes())
        self.assertEqual(digest(profile), pin['profile_sha256'])
        expected = inventory(json.loads(profile.read_bytes()), pin['profile_sha256'])
        rules = json.loads((SOURCES/'stock-rules.json').read_bytes())
        self.assertEqual(len(expected), pin['files'])
        local = {str(p.relative_to(SOURCES)): p for p in SOURCES.rglob('*') if p.is_file()}
        del local['stock-rules.json']
        self.assertEqual(len(local) + len(rules), len(expected))
        self.assertEqual(set(local) | set(rules), set(expected))
        for name, path in local.items():
            self.assertEqual(digest(path), expected[name], name)

    def test_tuning_reads_the_pinned_snapshot_descriptor(self):
        import struct
        name = 'src/pureshot/ps2native/stock_tuning.h'
        rule = json.loads((SOURCES/'stock-rules.json').read_bytes())[name]
        with self.assertRaisesRegex(ValueError, 'digest'):
            render_stock(rule, b'wrong ISP library')
        # Independent sentinel on either side catches an offset or count change.
        values = tuple(float(i) / 8 for i in range(41))
        data = bytes(0x40f72c - 4) + struct.pack('<43f', -19, *values, -21)
        header = render_stock(dict(rule, base_sha256=sha(data)), data)
        rows = header.decode().splitlines()[1:-1]
        self.assertEqual(tuple(float.fromhex(r.rstrip(',').removesuffix('f')) for r in rows), values)

    def test_generated_recipe_uses_exact_stock(self):
        stock = b'abcdef'
        rule = dict(op='set-bytes', path='/test', base_sha256=sha(stock), writes=[[2,'00']])
        self.assertEqual(apply_recipe(stock, json.loads(render_stock(rule, stock))), b'ab\0def')
        with self.assertRaisesRegex(ValueError, 'digest'):
            render_stock(rule, b'ABCDEF')
        rule['writes'] = [[6,'ffff']]
        with self.assertRaisesRegex(ValueError, 'span'):
            render_stock(rule, stock)

    def test_seed_menu_delta_requires_installed_gui(self):
        from unittest.mock import patch
        stock, base, final = b'official', b'installed', b'menu'
        rule = dict(op='feature-menu', path='/gui', base_sha256=sha(stock),
                    recipe_base_sha256=sha(base), indent=1)
        with patch('openx6.component_sources.base_gui', return_value=base), \
             patch('openx6.component_sources.feature_menu', return_value=final) as build:
            spec = json.loads(render_stock(rule, stock))
            build.assert_called_once_with(base, None)
            self.assertEqual(apply_recipe(base, spec), final)
            with self.assertRaisesRegex(ValueError, 'base rejected'):
                apply_recipe(stock, spec)
            rule['recipe_base_sha256'] = sha(b'changed')
            with self.assertRaisesRegex(ValueError, 'Installed GUI base differs'):
                render_stock(rule, stock)
            self.assertEqual(build.call_count, 1)

    def test_bad_local_source_cannot_fall_back(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root/'stock-rules.json').write_text('{}')
            (root/'code').write_bytes(b'tampered')
            with self.assertRaisesRegex(ValueError, 'reconstruction differs'):
                assemble(root/'out', None, {'code': sha(b'good')}, sources=root)

    def test_firmware_hooks_reject_modified_inputs_before_patching(self):
        rules = json.loads((SOURCES/'stock-rules.json').read_bytes())
        selected = [rule for rule in rules.values() if rule['op'] in ('fast-capture','video-nr','base-gui','gui-wrapper','interval-wait','feature-menu','media-service','camera-hal','camera-hal-runtime','interval-ae','ssd-gate','owner-adb','hardware-service','usb-receiver','factory-api',
                      'fisheye-render','fisheye-mesh')]
        self.assertEqual(len(selected), 22)
        captures = [r for r in rules.values() if r['op'] == 'single-capture']
        self.assertEqual({r['variant'] for r in captures},
                         {'encoder','buffers','config','sensor','linked','native','fov','caps','streams'})
        for rule in captures:
            with patch('openx6.component_sources.read_file', return_value=b'wrong stock'), \
                 self.assertRaisesRegex(ValueError, 'official capture input'):
                generate_rule(rule, None)
        for rule in selected:
            with self.subTest(rule=rule), self.assertRaisesRegex(ValueError, 'digest'):
                render_stock(rule, b'wrong stock', tools={})

    @unittest.skipUnless(os.getenv('OPENX6_TEST_STOCK_TREE'), 'needs extracted official 1.2.36 files')
    def test_interval_storage_and_usb_match_signed_recipes_and_installed_files(self):
        stock_tree = Path(os.environ['OPENX6_TEST_STOCK_TREE'])
        actions = json.loads((SOURCES/'profile.json').read_bytes())['actions']
        rules = json.loads((SOURCES/'stock-rules.json').read_bytes())
        selected = [rule for rule in rules.values() if rule['op'] in ('interval-ae', 'ssd-gate', 'owner-adb',
                                                               'hardware-service', 'usb-receiver', 'factory-api')]
        self.assertEqual(len(selected), 8)
        for rule in selected:
            path = rule['path']
            with self.subTest(path=path):
                stock = (stock_tree/path.lstrip('/')).read_bytes()
                recipe = render_stock(rule, stock)
                self.assertEqual(sha(recipe), actions[path]['asset_sha256'])
                installed = apply_recipe(stock, json.loads(recipe))
                self.assertEqual(sha(installed), actions[path]['sha256'])

    def test_compiled_output_mismatch_cannot_use_review_copy(self):
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            name = 'recipes/capture.json'
            stock = b'original'
            rule = dict(op='fast-capture', path='/node', base_sha256=sha(stock))
            (root/'stock-rules.json').write_text(json.dumps({name:rule}))
            with patch('openx6.component_sources.read_file', return_value=stock), \
                 patch('openx6.component_sources.fast_capture', return_value=b'wrong target'), \
                 self.assertRaisesRegex(ValueError, 'reconstruction differs'):
                assemble(root/'out', None, {name:sha(b'expected')}, sources=root)
            self.assertFalse((root/'out'/name).exists())
