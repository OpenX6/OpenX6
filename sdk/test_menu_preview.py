import json
from pathlib import Path
import unittest

from menu_preview import preview


class PreviewTests(unittest.TestCase):
    def setUp(self):
        self.menu = json.loads((Path(__file__).parent/'examples/menu_grid.json').read_text())

    def test_defaults_and_edits(self):
        self.assertIs(preview([self.menu], [])['values']['grid_guide.enabled'], False)
        result = preview([self.menu], [('grid_guide.enabled', 'on'), ('grid_guide.style', 'Center')])
        self.assertEqual(result['modules']['grid_guide']['settings'][1],
                         {'label': 'Grid style', 'type': 'choice', 'value': 'Center', 'choices': ['Thirds','Center']})
        self.assertEqual(result['values'], {'grid_guide.enabled': True, 'grid_guide.style': 'Center'})

    def test_namespaces(self):
        other = dict(self.menu, id='another')
        values = preview([other, self.menu], [('another.enabled', 'on')])['values']
        self.assertTrue(values['another.enabled'])
        self.assertFalse(values['grid_guide.enabled'])

    def test_duplicate_module_and_setting(self):
        with self.assertRaises(ValueError):
            preview([self.menu, self.menu], [])
        self.menu['settings'] *= 2
        with self.assertRaises(ValueError):
            preview([self.menu], [])

    def test_unknown_values_actions_and_target(self):
        for edits in [[('grid_guide.style', 'Other')], [('grid_guide.enabled', '1')], [('other.x', 'on')]]:
            with self.assertRaises(ValueError):
                preview([self.menu], edits)
        self.menu['settings'][0]['command'] = 'touch example'
        with self.assertRaises(ValueError):
            preview([self.menu], [])
