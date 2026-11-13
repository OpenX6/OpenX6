"""Exercise the contributor workflow without firmware, a display or a camera."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from openx6.build import preflight_modules
from sdk.__main__ import create_module

ROOT = Path(__file__).resolve().parents[1]


class ContributorWorkflow(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.directory = Path(tmp.name)/'field_budget'
        self.manifest = create_module(self.directory)

    def cli(self, *args):
        return subprocess.run([sys.executable, '-B', '-m', 'sdk', *map(str, args)],
                              cwd=ROOT, capture_output=True, text=True)

    def test_init_and_combined_check(self):
        self.assertEqual((self.directory/'LICENSE').read_bytes(), (ROOT/'sdk/LICENSE').read_bytes())
        original = self.manifest.read_bytes()
        self.assertNotEqual(self.cli('init', self.directory).returncode, 0)
        self.assertEqual(self.manifest.read_bytes(), original)
        result = self.cli('check', self.manifest, ROOT/'sdk/examples/stats_overlay.json')
        self.assertEqual(result.returncode, 0, result.stderr)
        summary = json.loads(result.stdout)
        self.assertEqual(summary['modules'], ['field_budget', 'capture_stats'])
        self.assertEqual(summary['checked_handlers'], ['field_budget'])
        self.assertIn('Duplicate', self.cli('check', self.manifest, self.manifest).stderr)
        self.assertIn('full OpenX6', self.cli('check', '--profile', 'stock', self.manifest).stderr)
        preview = self.cli('check', ROOT/'sdk/examples/menu_grid.json')
        self.assertNotEqual(preview.returncode, 0)
        self.assertIn('sdk.menu_preview', preview.stderr)

    def test_edits_require_review_and_handlers_never_run(self):
        doc = json.loads(self.manifest.read_text())
        handler = self.directory/doc['program']['source']
        for text, error in [
                ("raise SystemExit('unexpected execution')\n", None),
                ("try:\n pass\nexcept* Exception:\n pass\n", 'Python 3.10'),
                ("return 42\n", 'Python 3.10')]:
            with self.subTest(text=text):
                handler.write_text(text, encoding='utf-8')
                actual = hashlib.sha256(handler.read_bytes()).hexdigest()
                before = self.manifest.read_bytes()
                result = self.cli('check', self.manifest)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(actual, result.stderr)
                self.assertEqual(self.manifest.read_bytes(), before)
                doc['program']['sha256'] = actual
                self.manifest.write_text(json.dumps(doc))
                if error:
                    with self.assertRaisesRegex(ValueError, error):
                        preflight_modules([self.manifest], full=True)
                else:
                    _, sources = preflight_modules([self.manifest], full=True)
                    self.assertEqual(sources['field_budget'], handler.read_bytes())

    def test_invalid_template_and_manifest(self):
        bad = self.directory.parent/'not a module id'
        self.assertNotEqual(self.cli('init', bad).returncode, 0)
        self.assertFalse(bad.exists())
        self.manifest.write_text('{"schema":1,"schema":2}')
        result = self.cli('check', self.manifest)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('module.json', result.stderr)
        self.assertIn('duplicate field', result.stderr)
