"""Exercise the camera transport and fail-closed publication without actual hardware."""
import hashlib
import http.client
import io
from itertools import product
from pathlib import Path
import re
import socket
import struct
import tarfile
import tempfile
import threading
import unittest
from unittest.mock import Mock, patch

from camera_reader import export
from openx6 import official


class ExportTests(unittest.TestCase):
    def test_wire_and_cleanup(self):
        for version, failure in product(export.UPDATERS, ('', 'wrong-bytes', 'missing', 'camera-error', 'wrong-peer-token')):
            with self.subTest(version=version, failure=failure), tempfile.TemporaryDirectory() as tmp:
                parent=Path(tmp)
                identity=('EXAMPLE', 'v'+version)
                target=Mock(host='127.0.0.1')
                target.identity.return_value=identity
                target.authorized.side_effect=[False, True]
                source=parent/'stock.bin'
                source.write_bytes(b'official')
                payloads={'a':b'hello','b':b'world','c':b'!'}
                specs={k:(len(v),hashlib.sha256(v).hexdigest()) for k,v in payloads.items()}
                peers=[]

                def execute(session, cmd):
                    if cmd.startswith('python3 -E -c'):
                        return b'OK\n'
                    if cmd.startswith('sha256sum'):
                        return export.UPDATERS[version].encode()+b'  /usr/bin/ins_otad\n'
                    # Exercise the exact frame-length restriction of the real GUI.
                    export.transport.gui_request(export.transport.completed_command(cmd,'X6_DONE_'+'0'*24),
                                                 '192.168.42.254',65535)
                    url=re.search(r'http://127.0.0.1:(\d+)/(\w+)/tool',cmd)
                    port,token=int(url[1]),url[2]

                    def action(method,path,body=None):
                        con=http.client.HTTPConnection('127.0.0.1',port,timeout=5)
                        con.request(method,'/'+path,body=body)
                        reply=con.getresponse()
                        result=reply.status,reply.read()
                        con.close()
                        return result

                    def camera():
                        try:
                            code,bundle=action('GET',token+'/tool')
                            self.assertEqual(code,200)
                            with tarfile.open(fileobj=io.BytesIO(bundle)) as tar:
                                run=tar.extractfile('run').read().decode()
                                self.assertEqual(tar.getmember('caller.so').mode, 0o444)
                                self.assertIn('full '+version+' >', run)
                                self.assertIn('chmod 444 caller.so', cmd)
                                self.assertEqual(set(tar.getnames()),{'run','guest.py','profile.py','caller.so','fusein'})
                            rangeport=int(re.search(r'guest.py 127.0.0.1 \d+ (\d+)',run)[1])
                            with socket.create_connection(('127.0.0.1',rangeport)) as stream:
                                stream.sendall(token.encode()+struct.pack('<QQ',2,4))
                                self.assertEqual(stream.makefile('rb').read(4),b'fici')
                            if failure=='wrong-peer-token':
                                self.assertEqual(action('PUT','wrong/a',b'hello')[0],403)
                            for name,data in payloads.items():
                                if name=='c' and failure=='missing':continue
                                if name=='a' and failure=='wrong-bytes':data=b'wrong'
                                code,_=action('PUT',token+'/'+name,data)
                                self.assertEqual(code,409 if data==b'wrong' else 201)
                            action('POST',token+'/report',b'FAIL' if failure in ('camera-error','wrong-peer-token') else b'OK')
                        except BaseException as exc:
                            peers.append(exc)
                            action('POST',token+'/report',b'FAIL')
                    t=threading.Thread(target=camera);t.start()
                    self.addCleanup(t.join)
                    return b'OK'
                local=parent/'cache'
                with (patch.object(export.transport,'CAMERA','127.0.0.1'),
                      patch.object(export.transport,'Session',return_value=target),
                      patch.object(export.transport,'execute',side_effect=execute),
                      patch.object(export,'inspect_stream',return_value=({'bytes':8},None,None)),
                      patch.object(export,'COMPONENTS',specs),
                      patch.object(export,'__file__',str(parent/'export.py'))):
                    worker=parent/'worker';worker.mkdir()
                    for name in ('guest.py','profile.py','caller.so','fusein'): (worker/name).write_bytes(b'test')
                    if failure:
                        with self.assertRaises(ValueError): export.export_to_cache(source,local,identity[0],lambda _:None)
                        self.assertEqual(list(local.iterdir()),[])
                    else:
                        cached=export.export_to_cache(source,local,identity[0],lambda _:None)
                        self.assertEqual(cached,export.verify_modules(cached))
                        with patch.object(export.transport,'Session',side_effect=AssertionError):
                            self.assertEqual(cached,export.export_to_cache(source,local,identity[0],print))
                    self.assertFalse(peers,peers)
                    target.query.assert_called_once_with(0x56)

    def test_cancel_and_disconnect_discard_an_incomplete_transfer(self):
        for why in ('cancel','disconnect'):
            with self.subTest(why=why), tempfile.TemporaryDirectory() as d:
                parent=Path(d)
                source=parent/'official.bin'
                source.write_bytes(b'encrypted')
                worker=parent/'worker'
                worker.mkdir()
                for name in ('guest.py','profile.py','caller.so','fusein'): (worker/name).touch()
                session=Mock(host='127.0.0.1')
                session.identity.return_value=('EXAMPLE','v0.8.825')
                session.authorized.return_value=True
                event=threading.Event()
                session.tick.side_effect=EOFError('camera disconnected')

                def execute(session,cmd):
                    if cmd.startswith('python3 -E -c'):
                        return b'OK\n'
                    if cmd.startswith('sha256sum'):return export.UPDATERS['0.8.825'].encode()
                    url=re.search(r'http://127.0.0.1:(\d+)/(\w+)/tool',cmd)
                    con=http.client.HTTPConnection('127.0.0.1',int(url[1]),timeout=5)
                    con.request('PUT','/'+url[2]+'/a',body=b'hello')
                    self.assertEqual(con.getresponse().status,201)
                    con.close()
                    if why=='cancel':event.set()
                    return b'OK'

                with (patch.object(export.transport,'Session',return_value=session),
                      patch.object(export.transport,'execute',side_effect=execute),
                      patch.object(export.transport,'CAMERA','127.0.0.1'),
                      patch.object(export,'inspect_stream',return_value=({'bytes':9},None,None)),
                      patch.object(export,'COMPONENTS',{'a':(5,hashlib.sha256(b'hello').hexdigest())}),
                      patch.object(export,'__file__',str(parent/'export.py'))):
                    with self.assertRaises(InterruptedError if why=='cancel' else EOFError):
                        export.export_to_cache(source,parent/'cache','EXAMPLE',print,event)
                    self.assertEqual([],list((parent/'cache').iterdir()))

    def test_unsupported_camera_rejected_before_command(self):
        with tempfile.TemporaryDirectory() as d:
            path=Path(d)/'official';path.touch()
            session=Mock();session.identity.return_value=('X','v0.8.829')
            with (patch.object(export.transport,'Session',return_value=session),
                  patch.object(export,'inspect_stream',return_value=({},None,None)),
                  patch.object(export.transport,'execute') as execute):
                with self.assertRaisesRegex(ValueError,'0.8.825'):
                    export.export_to_cache(path,Path(d)/'cache','X',print)
                execute.assert_not_called()
                session.authorized.assert_not_called()

    def test_rejects_mismatched_updater_before_power_or_transfer(self):
        for version, other in [('0.8.825', '0.8.827'), ('0.8.827', '0.8.825')]:
            for checksum in (export.UPDATERS[other], '0' * 64, ''):
                with self.subTest(version=version, checksum=checksum), tempfile.TemporaryDirectory() as tmp:
                    root = Path(tmp)
                    official = root/'official'
                    official.touch()
                    session = Mock()
                    session.identity.return_value = ('EXAMPLE', 'v' + version)
                    session.authorized.return_value = True
                    with (patch.object(export.transport, 'Session', return_value=session),
                          patch.object(export, 'inspect_stream', return_value=({}, None, None)),
                          patch.object(export.transport, 'execute', return_value=checksum.encode()) as command,
                          patch.object(export, 'receive') as receive):
                        with self.assertRaisesRegex(ValueError, 'updater combination'):
                            export.export_to_cache(official, root/'cache', 'EXAMPLE', lambda _: None)
                        command.assert_called_once_with(session, 'sha256sum /usr/bin/ins_otad')
                        receive.assert_not_called()
                        self.assertEqual(list((root/'cache').iterdir()), [])

    def test_refused_authorization_or_changed_camera_never_runs_commands(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            official = root/'official'
            official.touch()
            expected = ('EXAMPLE', 'v0.8.827')
            for scenario in ('denied', 'wrong-serial', 'changed-serial', 'changed-version'):
                with self.subTest(scenario=scenario):
                    session = Mock()
                    cancel = threading.Event()
                    if scenario == 'denied':
                        cancel.set()
                    session.authorized.return_value = scenario != 'denied'
                    session.identity.side_effect = {
                        'denied': [expected], 'wrong-serial': [('OTHER', expected[1])],
                        'changed-serial': [expected, ('OTHER', expected[1])],
                        'changed-version': [expected, (expected[0], 'v0.8.825')],
                    }[scenario]
                    with (patch.object(export.transport, 'Session', return_value=session),
                          patch.object(export, 'inspect_stream', return_value=({}, None, None)),
                          patch.object(export.transport, 'execute') as command):
                        with self.assertRaises(InterruptedError if scenario == 'denied' else ValueError):
                            export.export_to_cache(official, root/'cache', expected[0], lambda _: None, cancel)
                        command.assert_not_called()
                        if scenario == 'denied':
                            session.query.assert_called_once_with(0x56)
                        self.assertEqual(list((root/'cache').iterdir()), [])

    def test_power_readback_failure_stops_before_transfer(self):
        with tempfile.TemporaryDirectory() as d:
            parent = Path(d)
            source = parent/'stock.bin'
            source.touch()
            session = Mock()
            session.identity.return_value = ('EXAMPLE', 'v0.8.825')
            session.authorized.return_value = True
            with (patch.object(export.transport, 'Session', return_value=session),
                  patch.object(export, 'inspect_stream', return_value=({'bytes': 0}, None, None)),
                  patch.object(export.transport, 'execute', side_effect=[export.UPDATERS['0.8.825'].encode(), b'OK', b'']),
                  patch.object(export, 'receive') as transfer):
                with self.assertRaisesRegex(RuntimeError, 'wifi_enable_strategy'):
                    export.export_to_cache(source, parent/'cache', 'EXAMPLE', lambda s: None)
                transfer.assert_not_called()
                self.assertEqual(list((parent/'cache').iterdir()), [])
