import copy
import hashlib
import json
import os
import re
import runpy
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from openx6 import programs
from openx6.build import build
from openx6.modules import load_modules
from sdk.choice import validate


EXAMPLE = Path(__file__).resolve().parents[1]/'sdk/examples/recording_budget/module.json'


class ExecutableModule(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.module = json.loads(EXAMPLE.read_text())
        self.program = EXAMPLE.with_name('budget.py')
        self.worker = runpy.run_path(str(programs.RUNTIME/'worker.py'))
        self.publish = runpy.run_path(str(programs.RUNTIME/'publisher.py'))['publish']

    def test_preflight_pins_without_running(self):
        (self.root/'budget.py').write_text("raise Exception('must not run during build')\n")
        self.module['program']['sha256'] = hashlib.sha256((self.root/'budget.py').read_bytes()).hexdigest()
        path = self.root/'module.json'
        path.write_text(json.dumps(self.module))
        modules = load_modules([path], None)
        inputs = programs.snapshot([path], modules)
        (self.root/'budget.py').write_text('changed')
        files, links = programs.layout(modules, inputs)
        self.assertEqual(files[programs.LIB+'/recording_budget/handler.py'], inputs['recording_budget'])
        self.assertEqual(len(links), 3)
        worker, = [v for k,v in files.items() if k.endswith('.service') and b'worker.py' in v]
        self.assertIn(b'DynamicUser=yes',worker)
        self.assertIn(b'/run/x6-boot/features-allowed',worker)
        with self.assertRaisesRegex(ValueError,'digest'):
            programs.snapshot([path],modules)
        with self.assertRaisesRegex(ValueError,'Missing'):
            programs.layout(modules,{})

    def test_persistent_contexts_match_only_selected_preferences(self):
        with patch.object(programs, 'compile_labeler', return_value=b'fixture') as compile_labeler:
            files, links = programs.layout([self.module], {'recording_budget':self.program.read_bytes()})
        name = 'openx6-choice.recording_budget.rate.pref'
        compile_labeler.assert_called_once_with([name], 'go')
        parent = '/data/x6-owner-features/extensions-v1'
        rules = [line.split() for line in files[programs.CONTEXTS].decode().splitlines()]
        def matches(path, kind):
            return [context for pattern, mode, context in rules
                    if mode == kind and re.fullmatch(pattern, path)]
        state = 'system_u:object_r:initrc_state_t:s0'
        self.assertEqual(matches(parent, '-d'), [state])
        for suffix in ('', '.next'):
            self.assertEqual(matches(parent+'/'+name+suffix, '--'), [state])
        for path in (parent, parent+'/other', parent+'/'+name+'.bak',
                     parent+'/'+name.replace('budget.rate','budgetXrate'), '/data/x6-owner-features/key'):
            self.assertEqual(matches(path, '--'), [])
        self.assertEqual(matches(parent+'/'+name, '-l'), [])
        self.assertEqual(programs.file_attributes(programs.LABELER),
                         ('0700', 'system_u:object_r:setfiles_exec_t:s0'))
        self.assertEqual(set(programs.startup_labels(links).values()), {programs.UNIT_CONTEXT})
        self.assertIn(('Requires='+programs.PREPARE).encode(),files[programs.UNITS+'/'+programs.PUBLISHER])
        self.assertFalse(any('/policy/' in path for path in files))

    def test_menu_without_handler_prepares_only_its_preferences(self):
        menu = dict(self.module)
        del menu['program']
        with patch.object(programs, 'compile_labeler', return_value=b'fixture'):
            files, links = programs.layout([menu], {})
        self.assertEqual(set(links.values()), {'../'+programs.PREPARE})
        self.assertNotIn(programs.LIB+'/worker.py', files)
        self.assertNotIn(programs.LIB+'/publisher.py', files)
        self.assertIn(programs.CONTEXTS, files)

    def test_sources_and_sd_rejected(self):
        for src in ('../secret.py','/tmp/code.py','file.txt','x;evil.py'):
            with self.subTest(src), self.assertRaises(ValueError):
                doc=copy.deepcopy(self.module)
                doc['program']['source']=src
                validate(doc)
        (self.root/'budget.py').symlink_to(self.program)
        with self.assertRaisesRegex(ValueError, 'regular input file'):
            programs.snapshot([self.root/'module.json'],[self.module])
        with self.assertRaisesRegex(ValueError,'built-in modules only'):
            build(None,None,[EXAMPLE],self.root,holder='holder',sd=True)

    def test_preference_forwarding(self):
        feature = self.root/'feature'
        prefs = feature/'extensions-v1'
        prefs.mkdir(parents=True)
        mirror = self.root/'input'
        mirror.mkdir()
        path = prefs/'openx6-choice.recording_budget.rate.pref'
        dest = mirror/'recording_budget'
        def publish():
            self.publish(feature,mirror,self.module)
        publish()
        self.assertEqual(dest.read_bytes(),b'standard')
        path.write_bytes(b'max');path.chmod(0o600)
        publish()
        self.assertEqual(dest.read_bytes(),b'max')
        path.write_bytes(b'unknown')
        publish()
        self.assertFalse(dest.exists())
        path.unlink();path.symlink_to(self.program)
        publish()
        self.assertFalse(dest.exists())

    def test_result_failure_and_retry(self):
        source = self.root/'selection'
        source.write_text('max')
        run = self.worker['reconcile']
        dest = self.root/'result.json'
        self.assertEqual(run(self.program,source,self.root,None),'max')
        self.assertEqual(json.loads(dest.read_text())['result']['mbps'],260)
        bad = self.root/'bad.py'
        for body,error in [('print("no json")',ValueError),('raise SystemExit(3)',ValueError),
                           ('print("x"*9000)',ValueError),('import time; time.sleep(30)',subprocess.TimeoutExpired)]:
            with self.subTest(body=body):
                bad.write_text(body)
                with self.assertRaises(error):
                    run(bad,source,self.root,None,timeout=.3)
                self.assertFalse(dest.exists())
        self.assertEqual(run(self.program,source,self.root,None),'max')
        source.write_text('high')
        # A slow handler finishing after a new selection must not publish its old result.
        with patch.dict(run.__globals__,{'invoke':lambda *args: (source.write_text('standard'),{})[1]}):
            self.assertIsNone(run(self.program,source,self.root,'max'))
        self.assertFalse(dest.exists())
        self.assertEqual(run(self.program,source,self.root,None),'standard')
