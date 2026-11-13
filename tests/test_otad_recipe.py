"""Supply the stock 1.2.36 updater to verify the signed maintenance delta."""
import json
import os
from pathlib import Path
import unittest
from unittest.mock import patch

from openx6 import otad_recipe
from openx6.common import sha
from openx6.component_sources import SOURCES, render_stock
from openx6.patch import apply_recipe


class MaintenanceUpdater(unittest.TestCase):
    def test_unknown_updater_rejected_before_compilation(self):
        with patch.object(otad_recipe, 'payload') as compile_gate:
            with self.assertRaisesRegex(ValueError, 'Official updater digest differs'):
                otad_recipe.maintenance_updater(b'wrong firmware')
            compile_gate.assert_not_called()

    @unittest.skipUnless(os.getenv('OPENX6_TEST_STOCK_OTAD'), 'requires official updater')
    def test_signed_delta_and_damaged_gate(self):
        stock = Path(os.environ['OPENX6_TEST_STOCK_OTAD']).read_bytes()
        profile = json.loads((SOURCES/'profile.json').read_bytes())
        action = profile['actions']['/usr/bin/ins_otad']
        rule = json.loads((SOURCES/'stock-rules.json').read_bytes())[action['asset']]
        recipe = render_stock(rule, stock)
        self.assertEqual(sha(recipe), action['asset_sha256'])
        self.assertEqual(sha(apply_recipe(stock, json.loads(recipe))), action['sha256'])
        with patch.object(otad_recipe, 'payload', return_value=bytes(662)):
            with self.assertRaisesRegex(ValueError, 'updater reconstruction differs'):
                render_stock(rule, stock)
