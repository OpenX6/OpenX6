"""Fast release-boundary checks without firmware, credentials or hardware."""
import json
import tempfile
import unittest
from unittest.mock import patch
from pathlib import Path

from openx6.build import ROOT, STOCK_MEDIA, outside_repo
from openx6.kit import TRUST, checked_inventory
from openx6.release_recipe import verify_recipe
from openx6.modules import TARGET
from openx6 import official
from openx6.common import sha
from sdk.module_plan import selection


class Preview(unittest.TestCase):
    def test_recognition_stops_before_decryption(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root/'renamed.bin'
            source.write_bytes(b'MWFI' + bytes(20) + b'1.3.59')
            with patch.object(official, 'RECOGNIZED_1359_SHA', sha(source.read_bytes())):
                with self.assertRaisesRegex(ValueError, '1.3.59 recognized'):
                    official.import_modules(source, root/'missing-material', root/'output')
                source.write_bytes(source.read_bytes() + b'x')
                with self.assertRaisesRegex(ValueError, 'Unknown official firmware'):
                    official.inspect(source)
            self.assertFalse((root/'output').exists())

    def test_tool_receipt_is_independent_of_install_directory(self):
        from openx6.sourcebuild import resolve_tool
        with tempfile.TemporaryDirectory() as tmp:
            receipts = []
            for name in ('first', 'second'):
                directory = Path(tmp)/name
                directory.mkdir()
                tool = directory/'test-compiler'
                tool.write_text("#!/bin/sh\nprintf 'test compiler 1.0\\nInstalledDir: %s\\n' \"$0\"\n")
                tool.chmod(0o700)
                receipts.append(resolve_tool(str(tool), ['--version'])[1])
            self.assertEqual(receipts[0], receipts[1])
            self.assertEqual(receipts[0]['version'], 'test compiler 1.0')

    def test_example_bases_are_distinct(self):
        examples = ROOT/'sdk/examples'
        stock = json.loads((examples/'stock_debug_label.json').read_bytes())
        unified = json.loads((examples/'media_debug_label.json').read_bytes())
        self.assertEqual(selection(TARGET, STOCK_MEDIA, [stock])[0], [stock['id']])
        with self.assertRaisesRegex(ValueError, 'digest mismatch'):
            selection(TARGET, STOCK_MEDIA, [unified])

    def test_signed_sources_and_separate_profile_signer(self):
        self.assertIn('src/pureshot/ps2native/stock_tuning.h', checked_inventory())
        with tempfile.TemporaryDirectory() as directory:
            edited = Path(directory) / 'profile.json'
            profile = ROOT / 'components/profile.json'
            edited.write_bytes(profile.read_bytes() + b' ')
            for data, signer in [(edited, 'component-preview.pub'), (profile, 'preview.pub')]:
                with self.subTest(signer=signer), self.assertRaisesRegex(ValueError, 'signature'):
                    verify_recipe(data, TRUST/'profile.sig', TRUST/signer)

    def test_output_and_enrollment_exclude_repository(self):
        with self.assertRaisesRegex(ValueError, 'outside'):
            outside_repo(ROOT/'OpenX6 Build')
        with tempfile.TemporaryDirectory() as directory:
            outside_repo(Path(directory)/'OpenX6 Build')
