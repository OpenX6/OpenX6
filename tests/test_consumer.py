import os
import shutil
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from openx6.build import build
from openx6.consumer import local_holder, seal


class Consumer(unittest.TestCase):
    def test_unreviewed_modules_and_missing_identity_rejected(self):
        with self.assertRaisesRegex(ValueError, 'built-in modules only'):
            build(None,None,['example'],None,holder='owner',sd=True)
        with self.assertRaisesRegex(ValueError, 'complete holder'):
            build(None,None,[],None,sd=True)
        with self.assertRaisesRegex(ValueError, 'locked'):
            seal(None,{'installer_kind':'guard-locked','install_allowed':False},None,None)

    def test_partial_identity_is_never_replaced(self):
        with tempfile.TemporaryDirectory() as root:
            key=Path(root)/'access'
            key.write_bytes(b'do not replace')
            with self.assertRaises(FileNotFoundError):
                local_holder(Path(root))
            self.assertEqual(key.read_bytes(),b'do not replace')

    @unittest.skipUnless(shutil.which('openssl') and shutil.which('ssh-keygen'),'needs enrollment tools')
    def test_reuse_and_refuse_mixed_keys(self):
        with tempfile.TemporaryDirectory() as root:
            first=Path(root)/'first'; second=Path(root)/'second'
            profile=local_holder(first)
            before=profile.read_bytes()
            self.assertEqual(local_holder(first).read_bytes(),before)
            local_holder(second)
            shutil.copyfile(second/'access',first/'access')
            with self.assertRaisesRegex(ValueError,'identity mismatch'):
                local_holder(first)
            self.assertEqual(profile.read_bytes(),before)
