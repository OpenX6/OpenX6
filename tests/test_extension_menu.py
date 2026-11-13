import copy
import json
import unittest
from pathlib import Path

from openx6.modules import split_selection
from sdk.menu_preview import preview

EXAMPLES = Path(__file__).resolve().parents[1]/'sdk/examples'


class NativeMenuTests(unittest.TestCase):
    def setUp(self):
        self.menu = json.loads((EXAMPLES/'stats_overlay.json').read_text())
        self.binary = json.loads((EXAMPLES/'media_debug_label.json').read_text())
        self.base = self.binary['base_sha256']

    def test_mixed_selection(self):
        self.assertEqual(split_selection([self.binary, self.menu], self.base),
                         ([self.menu], [self.binary]))
        values = preview([self.menu], [('capture_stats.enabled', 'on')])['values']
        self.assertEqual(values, {'capture_stats.enabled': True})

    def test_conflicts_across_types(self):
        self.binary['conflicts'] = ['capture_stats']
        with self.assertRaisesRegex(ValueError, 'conflicting'):
            split_selection([self.binary, self.menu], self.base)

    def test_duplicate_binding(self):
        with self.assertRaisesRegex(ValueError, 'binding already selected'):
            split_selection([self.menu, dict(self.menu, id='other')], self.base)

    def test_duplicate_id_across_types(self):
        with self.assertRaisesRegex(ValueError, 'Duplicate module ID'):
            split_selection([self.binary, dict(self.menu, id=self.binary['id'])], self.base)

    def test_invalid_fields(self):
        for changes in [{'default': 0}, {'default': True}, {'binding': '/tmp/file'},
                        {'command': 'run'}, {'type':'choice'}, {'label':'X\nY'}, {'label':'📷'}]:
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                doc = copy.deepcopy(self.menu)
                doc['settings'][0].update(changes)
                split_selection([doc], self.base)

    def test_preview_only_rejected(self):
        with self.assertRaises(ValueError):
            split_selection([json.loads((EXAMPLES/'menu_grid.json').read_text())], self.base)
