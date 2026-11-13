import json
import unittest

from openx6.build import verify_maintenance_pins
from openx6.common import Rejected, sha


class MaintenancePins(unittest.TestCase):
    def setUp(self):
        self.ssh = '/usr/local/sbin/x6-owner-maintenance'
        self.launcher = '/usr/local/sbin/x6-owner-late-start'
        self.verifier = '/usr/local/sbin/x6-owner-update-check'
        body = b'current maintenance executable'
        checksum = sha(body)
        policy = {'AutomaticEnabled': True, 'Files': {self.ssh: [checksum]}}
        self.files = {
            self.ssh: body,
            self.launcher: f'digest_is {self.ssh} {checksum}\n'.encode(),
            self.verifier: b'\x7fELF' + json.dumps(policy, indent=2).encode() + b'\x00\xff',
        }

    def test_both_consumers_pin_the_installed_program(self):
        verify_maintenance_pins(self.files.__getitem__)

    def test_previous_digest_in_either_consumer_is_rejected(self):
        current = sha(self.files[self.ssh]).encode()
        old = sha(b'previous maintenance executable').encode()
        for path, message in ((self.launcher, 'launcher'), (self.verifier, 'coordinator')):
            with self.subTest(path=path):
                files = self.files.copy()
                files[path] = files[path].replace(current, old)
                with self.assertRaisesRegex(Rejected, message):
                    verify_maintenance_pins(files.__getitem__)

    def test_ambiguous_embedded_policy_is_rejected(self):
        self.files[self.verifier] *= 2
        with self.assertRaisesRegex(Rejected, 'ambiguous'):
            verify_maintenance_pins(self.files.__getitem__)
