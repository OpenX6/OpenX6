"""Supply OPENX6_TEST_STOCK_HAL for full HAL builds; no vendor fixtures in checkout."""
import json
import os
from pathlib import Path
import unittest
from unittest.mock import patch

from openx6 import camera_hal_recipe as hal
from openx6.common import sha
from openx6.component_sources import SOURCES, render_stock
from openx6.patch import apply_recipe


class CameraHAL(unittest.TestCase):
    def test_wrong_base_or_variant_is_rejected_before_compilation(self):
        with patch.object(hal, 'payload') as compile_hook:
            with self.assertRaisesRegex(ValueError, 'Official camera HAL digest'):
                hal.camera_hal(b'unknown')
            with self.assertRaisesRegex(ValueError, 'Installed camera HAL digest'):
                hal.camera_hal_runtime(b'unknown', 'native')
            with self.assertRaisesRegex(ValueError, 'Unknown camera HAL variant'):
                hal.camera_hal_runtime(b'unknown', 'other')
            compile_hook.assert_not_called()

    @unittest.skipUnless(os.getenv('OPENX6_TEST_STOCK_HAL'), 'requires official camera HAL')
    def test_signed_recipes_and_stage_rejection(self):
        stock = Path(os.environ['OPENX6_TEST_STOCK_HAL']).read_bytes()
        profile = json.loads((SOURCES/'profile.json').read_bytes())
        rules = json.loads((SOURCES/'stock-rules.json').read_bytes())
        path = '/usr/lib/hw/camera.qcom.so'
        action = profile['actions'][path]
        base_recipe = render_stock(rules[action['asset']], stock)
        self.assertEqual(sha(base_recipe), action['asset_sha256'])
        installed = apply_recipe(stock, json.loads(base_recipe))
        self.assertEqual(sha(installed), action['sha256'])
        for entry in profile['mod_runtime']:
            if entry['base'] != path:
                continue
            rule = rules[entry['recipe']]
            recipe = render_stock(rule, stock)
            self.assertEqual(sha(recipe), entry['recipe_sha256'])
            spec = json.loads(recipe)
            with self.assertRaisesRegex(ValueError, 'base rejected'):
                apply_recipe(stock, spec)
            self.assertEqual(sha(apply_recipe(installed, spec)), hal.RUNTIME[rule['variant']][1])

        compile_hook = hal.payload
        for source, stage in [('bridge.S', 'exposure bridge'), ('switch.S', 'native HNR'),
                              ('budget_entry.S', 'off interval budget')]:
            def damaged(folder, address, sources, *args, **kwargs):
                data = compile_hook(folder, address, sources, *args, **kwargs)
                return data[:-1] + bytes([data[-1] ^ 1]) if source in sources else data

            with self.subTest(stage=stage), patch.object(hal, 'payload', damaged), \
                 self.assertRaisesRegex(ValueError, 'differs at ' + stage):
                if source == 'bridge.S':
                    hal.camera_hal(stock)
                else:
                    hal.camera_hal_runtime(installed, stage.split()[0])
