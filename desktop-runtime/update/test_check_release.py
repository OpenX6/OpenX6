import hashlib
from io import BytesIO
import unittest
from zipfile import ZipFile

from check_release import check_windows, sums


class NativeInventoryTests(unittest.TestCase):
    def setUp(self):
        self.data = b'runtime fixture'
        self.manifest = (hashlib.sha256(self.data).hexdigest() + '  Native/worker.py\n').encode()
        self.contents = {'OpenX6.exe': b'PE' + self.manifest, 'Native/worker.py': self.data}

    def verify(self):
        stream = BytesIO()
        with ZipFile(stream, 'w') as archive:
            for name, data in self.contents.items():
                archive.writestr(name, data)
        with ZipFile(stream) as archive:
            check_windows(archive, self.manifest)

    def test_matching(self):
        self.verify()

    def test_changed_worker(self):
        self.contents['Native/worker.py'] = b'changed'
        with self.assertRaisesRegex(ValueError, 'checksum differs'):
            self.verify()

    def test_extra_file(self):
        self.contents['extra'] = b'added'
        with self.assertRaisesRegex(ValueError, 'inventory differs'):
            self.verify()

    def test_launcher_omits_inventory(self):
        self.contents['OpenX6.exe'] = b'old launcher'
        with self.assertRaisesRegex(ValueError, 'does not embed'):
            self.verify()

    def test_duplicate_checksum(self):
        with self.assertRaisesRegex(ValueError, 'duplicate'):
            sums(self.manifest * 2)
