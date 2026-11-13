import copy
import hashlib
import json
import unittest
from pathlib import Path
from module_plan import plan, unique_keys

EX = Path(__file__).resolve().parent / 'examples'


class ModulePlan(unittest.TestCase):
    def setUp(self):
        self.source = (EX/'sample.txt').read_bytes()
        self.first = json.loads((EX/'demo.json').read_bytes())
        self.second = {**self.first, 'id':'other', 'requires':[self.first['id']],
                       'edits':[{'offset':0,'before':'4f','after':'6f'}]}

    def test_deterministic_order_and_digest(self):
        actual = plan(self.source,'example',[self.second,self.first])
        self.assertEqual(actual,plan(self.source,'example',[self.first,self.second]))
        self.assertEqual(actual['modules'],['demo_revision','other'])
        self.assertEqual(actual['predicted_sha256'], hashlib.sha256(b'openX6 demo v2\n').hexdigest())

    def test_refusals(self):
        for case in ['input','target','protected','hash','overlap','cycle','missing','conflict','unknown','duplicate','size','original','bool']:
            with self.subTest(case=case), self.assertRaises(ValueError):
                a,b = copy.deepcopy([self.first,self.second])
                data,target = self.source,'example'
                if case=='input': data+=b'x'
                if case=='target': target='/usr/bin/ins_mediad'
                if case=='protected': target=a['target']=b['target']='/adb_keys'
                if case=='hash': a['base_sha256']='f'*64
                if case=='overlap': b['edits']=a['edits']
                if case=='cycle': a['requires']=['other']
                if case=='missing': b['requires']=['missing']
                if case=='conflict': a['conflicts']=['other']
                if case=='unknown': b['run']='program'
                if case=='duplicate': b['id']=a['id']
                if case=='size': b['edits'][0]['after']='1234'
                if case=='original': b['edits'][0]['before']='ff'
                if case=='bool': b['edits'][0]['offset']=False
                plan(data,target,[b,a])

    def test_duplicate_json_field(self):
        with self.assertRaises(ValueError):
            json.loads('{"id":"first","id":"second"}',object_pairs_hook=unique_keys)
