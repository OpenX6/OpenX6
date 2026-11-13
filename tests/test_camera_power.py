"""Exercise the commands sent to the stock camera against a native-config fixture."""
from contextlib import redirect_stdout
import io
import json
import shlex
import sys
import threading
import unittest
from unittest.mock import Mock, patch

from camera_reader import power, transport


class CameraPowerTests(unittest.TestCase):
    def setUp(self):
        self.stored = {
            'kSystemAutoShutdown': {'auto_power_off_time': 180, 'rotate_shutdown_delay': 'k3Seconds'},
            'kNetdConfig': {'wifi_enable_strategy': 'kAuto', 'another_preference': 'keep'},
        }
        self.writes = []
        self.ignore_wifi = False

    def native(self, argv):
        self.assertEqual(argv[0], '/usr/bin/ins_bll_cli')
        request = json.loads(argv[1])
        key = request['key']
        self.assertEqual(request['cmd'], 'config')
        if request['action'] == 'get_general':
            return json.dumps({'ret': 'ok', 'data': self.stored[key]}).encode()
        self.assertEqual(request['action'], 'set_general')
        self.writes.append(key)
        if not (self.ignore_wifi and key == 'kNetdConfig'):
            self.stored[key] = request['value']
        return b'{"ret":"ok"}'

    def execute(self, session, command):
        wrapped = transport.completed_command(command, 'X6_DONE_' + '0'*24)
        transport.gui_request(wrapped, '192.168.42.254', 65535)
        binary, flag, option, script, *args = shlex.split(command)
        self.assertEqual((binary, flag, option), ('python3', '-E', '-c'))
        out = io.StringIO()
        with patch.object(sys, 'argv', ['-c', *args]), patch('subprocess.check_output', self.native), redirect_stdout(out):
            try:
                exec(script, {})
            except (AssertionError, KeyError) as exc:
                raise RuntimeError('Command failed') from exc
        return out.getvalue().encode()

    def test_persistent_readback_and_idempotent_retry(self):
        for _ in range(2):
            power.prepare(None, self.execute, lambda _: None)
        self.assertEqual(self.writes, list(self.stored))
        self.assertEqual(self.stored['kSystemAutoShutdown'], {'auto_power_off_time': 0, 'rotate_shutdown_delay': 'k3Seconds'})
        self.assertEqual(self.stored['kNetdConfig'], {'wifi_enable_strategy': 'kAlwaysOn', 'another_preference': 'keep'})

    def test_partial_change_and_retry(self):
        self.ignore_wifi = True
        with self.assertRaisesRegex(RuntimeError, 'wifi_enable_strategy'):
            power.prepare(None, self.execute, print)
        self.ignore_wifi = False
        power.prepare(None, self.execute, print)
        self.assertEqual(self.writes, ['kSystemAutoShutdown', 'kNetdConfig', 'kNetdConfig'])

    def test_unexpected_schema_and_disconnect_stop(self):
        self.stored['kSystemAutoShutdown']['auto_power_off_time'] = False
        for execute in (self.execute, Mock(side_effect=EOFError), Mock(return_value=b'')):
            with self.assertRaisesRegex(RuntimeError, 'auto_power_off_time'):
                power.prepare(None, execute, print)
        self.assertEqual(self.writes, [])

    def test_canceled_before_settings(self):
        event = threading.Event()
        event.set()
        execute = Mock()
        with self.assertRaises(InterruptedError):
            power.prepare(None, execute, print, event)
        execute.assert_not_called()

