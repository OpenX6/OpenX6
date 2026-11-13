import copy
import json
import tempfile
import unittest
from pathlib import Path

from openx6.choice_recipe import choice_pages
from openx6.modules import split_selection
from sdk.choice import read_choice, validate
from sdk.menu_preview import preview

EXAMPLE = Path(__file__).resolve().parents[1]/'sdk/examples/field_checklist.json'


class ChoiceModuleTests(unittest.TestCase):
    def setUp(self):
        self.doc = json.loads(EXAMPLE.read_bytes())

    def test_manifest_drives_feedback(self):
        p = preview([self.doc], [('field_checklist.rig', 'vehicle')])
        row, = p['modules']['field_checklist']['settings']
        self.assertEqual(row['selected_label'], 'Vehicle')
        self.assertIn('tether', row['note'])

    def test_unknown_action_and_invalid_identifiers(self):
        for changes in [{'id': '../x'}, {'label':'a\nb'}, {'command':'true'}]:
            with self.subTest(changes), self.assertRaises(ValueError):
                doc=copy.deepcopy(self.doc)
                doc['setting']['options'][0].update(changes)
                validate(doc)
        self.doc['setting']['default'] = 'gone'
        with self.assertRaises(ValueError):
            validate(self.doc)

    def test_duplicate_choices_and_duplicate_page(self):
        other=copy.deepcopy(self.doc)
        other['setting']['options'][1]['id']='tripod'
        with self.assertRaisesRegex(ValueError,'duplicate option'):
            validate(other)
        with self.assertRaisesRegex(ValueError,'Duplicate extension page'):
            split_selection([self.doc, dict(self.doc,id='another')],None)

    def test_owned_preference_and_reordering(self):
        with tempfile.TemporaryDirectory() as tmp:
            directory=Path(tmp)
            path=directory/'openx6-choice.field_checklist.rig.pref'
            self.assertEqual(read_choice(directory,self.doc),'tripod')
            path.write_bytes(b'vehicle')
            path.chmod(0o600)
            self.doc['setting']['options'].reverse()
            self.assertEqual(read_choice(directory,self.doc),'vehicle')
            path.write_bytes(b'retired')
            with self.assertRaisesRegex(ValueError,'unrecognized'):
                read_choice(directory,self.doc)
            self.assertEqual(path.read_bytes(),b'retired')
            path.chmod(0o666)
            with self.assertRaisesRegex(ValueError,'unsafe'):
                read_choice(directory,self.doc)
            path.unlink()
            path.symlink_to('absent')
            with self.assertRaises(OSError):
                read_choice(directory,self.doc)

    def test_two_independent_choices_and_assembly_text(self):
        second=dict(self.doc,id='second', title='Second')
        menus,_=split_selection([second,self.doc],None)
        self.assertEqual([m['id'] for m in menus],['field_checklist','second'])
        second['setting']=copy.deepcopy(second['setting'])
        second['setting']['options'][0]['note']='"; .word 0xdeadbeef'
        validate(second)
        arrows,definitions=choice_pages(menus)
        self.assertEqual(arrows.count(b'bl ext_add_arrow'),2)
        self.assertNotIn(b'.word 0xdeadbeef',definitions)
