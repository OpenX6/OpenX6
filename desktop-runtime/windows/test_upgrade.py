import hashlib
import io
import fcntl
import subprocess
import sys
import os
from pathlib import Path
import shutil
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import bridge
from app_bundle import bundle
from upgrade import unpack


class HolderUpgrade(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name)
        self.holder = self.root/'Holder'
        inspector = patch.object(bridge, 'inspect')
        inspector.start()
        self.addCleanup(inspector.stop)

    def check(self):
        return bridge.remembered_holder(self.root, None)

    def test_old_and_repeat(self):
        profile = bridge.local_holder(self.holder)
        before = {p.name:p.read_bytes() for p in self.holder.iterdir()}
        self.assertEqual(self.check(), profile)
        self.assertEqual(self.check(), profile)
        self.assertEqual(before, {p.name:p.read_bytes() for p in self.holder.iterdir()})
        self.assertEqual((self.root/'holder-profile.sha256').read_text().strip(), bridge.digest(profile))

    def test_missing_damaged_or_replaced(self):
        with self.assertRaisesRegex(ValueError, 'missing'):
            self.check()
        self.assertFalse(self.holder.exists())
        bridge.local_holder(self.holder)
        self.check()
        self.holder.rename(self.root/'saved')
        with self.assertRaisesRegex(ValueError, 'missing'):
            self.check()
        bridge.local_holder(self.holder)
        with self.assertRaisesRegex(ValueError, 'changed'):
            self.check()
        shutil.copyfile(self.root/'saved/maintenance', self.holder/'maintenance')
        with self.assertRaisesRegex(ValueError, 'identity mismatch'):
            self.check()
        (self.holder/'access').unlink()
        with self.assertRaises(FileNotFoundError):
            self.check()
        self.assertFalse((self.holder/'access').exists())

    def test_new_enrollment_and_retry(self):
        pending = self.root/'first-enrollment.pending'
        pending.touch()
        self.assertTrue(self.check().is_file())
        self.assertFalse(pending.exists())
        self.assertTrue(self.check().is_file())
        self.holder.rename(self.root/'removed')
        pending.touch()
        with self.assertRaisesRegex(ValueError, 'missing'):
            self.check()

    def test_pinned_archive(self):
        archive = self.root/'program.zip'
        expected = bundle(archive)
        with archive.open('rb') as src:
            with self.assertRaisesRegex(ValueError, 'checksum'):
                unpack(src, '0'*64, self.root)
            src.seek(0)
            unpack(src, expected, self.root)
        self.assertTrue((self.root/'openx6/build.py').is_file())
        self.assertFalse(self.holder.exists())
        data = io.BytesIO()
        with zipfile.ZipFile(data,'w') as z:
            z.writestr('../invalid', b'data')
        data.seek(0)
        with self.assertRaisesRegex(ValueError, 'archive member'):
            unpack(data, hashlib.sha256(data.getvalue()).hexdigest(), self.root)

    @unittest.skipIf(os.getuid() == 0, 'bootstrap runs as a normal account')
    def test_bootstrap_stdin_cleanup_and_lock(self):
        archive = io.BytesIO()
        info = zipfile.ZipInfo('desktop-runtime/windows/bridge.py')
        info.external_attr = 0o100644 << 16
        with zipfile.ZipFile(archive, 'w') as z:
            z.writestr(info, "import sys; print('upgraded', *sys.argv[1:])")
        data = archive.getvalue()
        cmd = [sys.executable, '-IB', str(Path(__file__).with_name('upgrade.py')),
               hashlib.sha256(data).hexdigest(), 'imported', 'official', 'material', 'output']
        env = dict(os.environ, HOME=str(self.root))
        done = subprocess.run(cmd, input=data, env=env, capture_output=True, check=True)
        self.assertIn(b'upgraded --upgrade official material output', done.stdout)
        private = self.root/'.local/share/OpenX6'
        self.assertEqual([p.name for p in private.iterdir()], ['first-enrollment.pending'])
        cmd[-4] = 'existing'
        fd = os.open(private, os.O_RDONLY)
        try:
            fcntl.flock(fd, fcntl.LOCK_EX)
            busy = subprocess.run(cmd, input=data, env=env, capture_output=True)
            self.assertNotEqual(busy.returncode, 0)
            self.assertIn(b'build is in progress', busy.stderr)
        finally:
            os.close(fd)
        bad = subprocess.run(cmd, input=b'bad program', env=env, capture_output=True)
        self.assertNotEqual(bad.returncode, 0)
        self.assertIn(b'checksum mismatch', bad.stderr)
        self.assertEqual([p.name for p in private.iterdir()], ['first-enrollment.pending'])


if __name__ == '__main__':
    old = os.umask(0o077)
    try: unittest.main()
    finally: os.umask(old)
