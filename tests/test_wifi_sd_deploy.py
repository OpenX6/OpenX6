"""A deployment may publish only the GUI's signed build to the confirmed target."""
import hashlib
import os
from pathlib import Path
import struct
import tempfile
import unittest
from unittest.mock import Mock, patch

from camera_reader import deploy, transport


class WiFiSD(unittest.TestCase):
    def test_scan_uses_real_card_status_and_rejects_busy_or_changed_targets(self):
        from camera_reader.worker import deploy as worker
        media = {'record': {'record_status': 'RecordIdle'}, 'capture': {'capture_status': 'CaptureIdle'}}
        sd = {'status_code':'Success', 'free_bytes':40, 'used_bytes':10, 'total_bytes':50}
        state = {'otad':{'update_status':'Idle'}, 'media':media, 'storaged':{'sd_fs_type':'exfat', 'sd':sd}}
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)/worker.FINAL
            path.write_bytes(b'test')
            fd = os.open(tmp, os.O_RDONLY)
            try:
                with (patch.object(worker, 'shared_state', side_effect=state.__getitem__),
                      patch.object(worker, 'card_identity', return_value='card'),
                      patch.object(worker.subprocess, 'check_output', return_value=b'Error code: Success(0) (0)') as notify):
                    saved = path.stat()
                    worker.scan_sd(fd,'card',saved)
                    import json
                    args, = notify.call_args.args
                    self.assertEqual(args[1:-1], ['--run','call-publish','kCamera','kStoraged','DeviceStatusChanged','Sd'])
                    self.assertEqual(json.loads(args[-1]), dict(sd, fs_type='exfat', mount_point='/mnt/sdcard'))
                    for target, key, value in [(state['otad'],'update_status','PreProcess'),
                                               (media['record'],'record_status','Recording'),
                                               (sd,'status_code','NoInsert')]:
                        with patch.dict(target, {key:value}), self.assertRaises(RuntimeError):
                            worker.scan_sd(fd,'card',saved)
                    with self.assertRaises(RuntimeError):
                        worker.scan_sd(fd,'other card',saved)
                    path.write_bytes(b'changed')
                    with self.assertRaises(RuntimeError):
                        worker.scan_sd(fd,'card',saved)
                    notify.assert_called_once()
                    notify.return_value=b'status_code: Success(0)\nError code: Failure(-1)'
                    with self.assertRaisesRegex(RuntimeError, 'acknowledge'):
                        worker.scan_sd(fd,'card',path.stat())
            finally:
                os.close(fd)

    def test_only_valid_finder_metadata_can_coexist_with_a_build(self):
        from camera_reader.worker.deploy import clean_card, FINAL
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            sidecar = root/('._' + FINAL)
            header = struct.pack('>II16sH', 0x51607, 0x20000, b'Mac OS X        ', 1)
            valid = header + struct.pack('>III', 9, 38, 32) + bytes(32)
            fd = os.open(root, os.O_RDONLY)
            try:
                for contents in (b'MWFI', valid[:-1], header, header + struct.pack('>III', 1, 38, 0)):
                    sidecar.write_bytes(contents)
                    with self.assertRaisesRegex(RuntimeError, 'existing update'):
                        clean_card(fd)
                sidecar.write_bytes(valid)
                clean_card(fd)
                (root/FINAL).write_bytes(b'MWFI')
                with self.assertRaisesRegex(RuntimeError, 'existing update'):
                    clean_card(fd)
                self.assertEqual(sidecar.read_bytes(), valid)
            finally:
                os.close(fd)

    def test_bootstrap_fits_stock_protocol(self):
        cmd = deploy.bootstrap('192.168.42.254', 65535, 'a'*32, 'b'*64, 1759144476, 'c'*64)
        encoded = transport.gui_request(transport.completed_command(cmd, 'X6_DONE_'+'d'*24),
                                        '192.168.42.254', 65535)
        self.assertLess(len(encoded), 1000)

    def test_only_verified_local_copy_and_expected_target(self):
        identity = ('EXAMPLE123', 'v0.8.825')
        payload = b'signed desktop output'
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)/deploy.FINAL
            path.write_bytes(payload)
            receipt = dict(bytes=len(payload), sha256=hashlib.sha256(payload).hexdigest(),
                           candidate=path.name, install_allowed=True, owner_authorization_embedded=True)
            session = Mock()
            session.identity.return_value = identity
            good_pin = deploy.supported_sources()['0.8.825'].encode()
            with (patch.object(transport,'Session',return_value=session) as connect,
                  patch.object(transport,'execute',return_value=good_pin+b'  /usr/bin/ins_otad') as execute,
                  patch.object(deploy,'transfer') as send):
                def attempt(r=receipt):
                    deploy.to_card(path,r,identity,Mock(),Mock())
                for changed in ({'sha256':'0'*64},{'install_allowed':False},{'owner_authorization_embedded':False}):
                    with self.assertRaises(ValueError):
                        attempt(receipt|changed)
                connect.assert_not_called()
                session.identity.return_value = ('OTHER', identity[1])
                with self.assertRaises(ValueError): attempt()
                execute.assert_not_called()
                session.identity.return_value = identity
                execute.return_value=b'wrong'
                with self.assertRaises(ValueError): attempt()
                send.assert_not_called()
                execute.return_value=good_pin
                attempt()
                send.assert_called_once()
                self.assertEqual(path.read_bytes(),payload)
