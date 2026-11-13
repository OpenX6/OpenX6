import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import factory827 as patcher


class Factory827(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.opt = Path(temp.name)
        self.marker = self.opt/'openx6-worker-version'
        self.marker.write_text(patcher.PREVIOUS+'\n')
        self.policy = self.opt/'openx6/install-guard/platform.json'
        self.policy.parent.mkdir(parents=True)
        # Exercise the frozen migration even if future release policies add other builds.
        import json
        baseline = {'Sources':[]}
        self.base = (json.dumps(baseline,indent=2)+'\n').encode()
        baseline['Sources'].append(patcher.RULE)
        self.new = (json.dumps(baseline,indent=2)+'\n').encode()
        self.policy.write_bytes(self.base)
        for field,value in [('BASE',patcher.sha(self.base)),('PATCHED',patcher.sha(self.new))]:
            override = patch.object(patcher, field, value)
            override.start()
            self.addCleanup(override.stop)

    def test_recover_interrupted_update_and_repeat(self):
        original = patcher.replace
        def interrupt(path,data):
            if path==self.marker: raise OSError('simulated interruption')
            original(path,data)
        with patch.object(patcher,'replace',side_effect=interrupt), self.assertRaises(OSError):
            patcher.migrate(self.opt,patcher.TARGET)
        self.assertEqual(self.marker.read_text(),patcher.PREVIOUS+'\n')
        self.assertEqual(self.policy.read_bytes(),self.new)
        patcher.migrate(self.opt,patcher.TARGET)
        inode = self.marker.stat().st_ino
        patcher.migrate(self.opt,patcher.TARGET)
        self.assertEqual(self.marker.stat().st_ino,inode)
        self.assertEqual(self.marker.read_text(),patcher.TARGET+'\n')

    def test_unknown_runtime_or_policy_is_rejected(self):
        with self.assertRaises(ValueError): patcher.migrate(self.opt,'other')
        self.assertEqual(self.policy.read_bytes(),self.base)
        self.policy.write_bytes(b'unknown')
        with self.assertRaises(ValueError): patcher.migrate(self.opt,patcher.TARGET)
        self.assertEqual(self.marker.read_text(),patcher.PREVIOUS+'\n')
        self.assertEqual(self.policy.read_bytes(),b'unknown')

    def test_link_or_writable_ancestor_is_rejected(self):
        adjacent=self.opt/'unrelated';self.policy.rename(adjacent)
        self.policy.symlink_to(adjacent)
        with self.assertRaises(ValueError): patcher.migrate(self.opt,patcher.TARGET)
        self.assertEqual(adjacent.read_bytes(),self.base)
        self.policy.unlink();os.link(adjacent,self.policy)
        with self.assertRaises(ValueError): patcher.migrate(self.opt,patcher.TARGET)
        self.policy.unlink();adjacent.rename(self.policy)
        self.policy.parent.chmod(0o777)
        with self.assertRaises(ValueError): patcher.migrate(self.opt,patcher.TARGET)
        self.assertEqual(self.marker.read_text(),patcher.PREVIOUS+'\n')
