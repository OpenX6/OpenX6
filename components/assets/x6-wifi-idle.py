#!/usr/bin/python3
"""Close an unattended X6 hotspot after the timeout selected in Wi-Fi Settings."""
import json
import os
import socket
import stat
import subprocess
import time
from pathlib import Path

PREF = Path('/data/x6-wifi-idle/minutes')
INHERITED = Path('/run/x6-wifi-idle/inherited')
CTRL = Path('/tmp/ins/ins_netd/hostapd/wlan1')
MINUTES = (0, 2, 5, 10)
CLOSE = ['/usr/bin/ins_ibus_cli', '--run', 'call-method', 'kCamera', 'kNetd',
         'WifiAp', 'WifiApOff', 'UI', 'Async']


def preference():
    try:
        with PREF.open('rb') as f:
            st = os.fstat(f.fileno())
            b = f.read(2)
    except FileNotFoundError:
        return None
    if not stat.S_ISREG(st.st_mode) or len(b) != 1 or b[0] not in MINUTES:
        raise ValueError('invalid saved timeout')
    return b[0], st.st_ino, st.st_mtime_ns


def wireless_config(action='get_general', value=None):
    msg = {'cmd': 'config', 'action': action, 'key': 'kNetdConfig'}
    if value is not None:
        msg['value'] = value
    p = subprocess.run(['/usr/bin/ins_bll_cli', json.dumps(msg)], check=True,
                       timeout=4, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    answer = json.loads(p.stdout)
    if answer.get('ret') != 'ok':
        raise ValueError('native configuration unavailable')
    return answer.get('data')


def policy():
    cfg = wireless_config()
    if not isinstance(cfg, dict) or cfg.get('wifi_enable_strategy') not in ('kAuto', 'kAlwaysOn'):
        raise ValueError('unknown native Wi-Fi policy')
    return cfg


def ensure_native_always_on():
    # One native preference controls its fixed 120-second countdown. Only after
    # an explicit menu selection do we take ownership of idle timing.
    cfg = policy()
    if cfg['wifi_enable_strategy'] != 'kAlwaysOn':
        cfg['wifi_enable_strategy'] = 'kAlwaysOn'
        wireless_config('set_general', cfg)
        if policy()['wifi_enable_strategy'] != 'kAlwaysOn':
            raise ValueError('native countdown is still active')


def station_count():
    # Confirm the same AP instance before and after STATUS, including restarts.
    first = CTRL.stat()
    if not stat.S_ISSOCK(first.st_mode):
        raise ValueError('control socket unavailable')
    with socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM) as s:
        s.settimeout(0.75)
        s.bind('')  # Linux abstract auto-bind, no filesystem entries.
        s.connect(str(CTRL))
        s.send(b'STATUS')
        raw = s.recv(65536)
    if len(raw) >= 65536 or not raw.endswith(b'\n'):
        raise ValueError('incomplete status')
    fields = {}
    for line in raw.splitlines():
        key, sep, val = line.partition(b'=')
        if key in (b'state', b'bss[0]', b'num_sta[0]'):
            if not sep or key in fields:
                raise ValueError('ambiguous status')
            fields[key] = val
        elif key.startswith(b'bss[') or key.startswith(b'num_sta['):
            raise ValueError('unexpected additional BSS')
    count = fields.get(b'num_sta[0]', b'')
    if fields.get(b'state') != b'ENABLED' or fields.get(b'bss[0]') != b'wlan1' or not count.isdigit():
        raise ValueError('AP state unavailable')
    if not os.path.samestat(first, CTRL.stat()):
        raise ValueError('AP restarted')
    return (first.st_dev, first.st_ino, first.st_ctime_ns), int(count)


class Associations:
    """Remember short associations that begin and end between STATUS polls."""
    def __init__(self):
        self.sock = self.instance = None

    def close(self):
        if self.sock is not None:
            self.sock.close()
        self.sock = self.instance = None

    def changed(self, ap):
        if self.instance != ap:
            self.close()
            s = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
            try:
                s.settimeout(.75)
                s.bind('')
                s.connect(str(CTRL))
                s.send(b'ATTACH')
                if s.recv(64).strip() != b'OK':
                    raise ValueError('AP event subscription unavailable')
                s.setblocking(False)
            except Exception:
                s.close()
                raise
            self.sock, self.instance = s, ap
            return True
        changed = False
        for _ in range(256):
            try:
                packet = self.sock.recv(4096)
            except BlockingIOError:
                return changed
            if not packet:
                raise ValueError('AP monitor disconnected')
            if any(event in packet for event in (b'AP-STA-', b'AP-ENABLED', b'AP-DISABLED', b'TERMINATING')):
                changed = True
        return True  # Continuous events cannot establish uninterrupted idle.


class Timer:
    def __init__(self):
        self.reset()

    def reset(self):
        self.key = self.since = self.last = None

    def due(self, now, pref, ap, clients):
        if not pref or not pref[0] or clients != 0:
            self.reset()
            return False
        key = pref, ap
        if self.key != key or self.last is None or not 0 <= now-self.last <= 6:
            self.key, self.since = key, now
        self.last = now
        return now-self.since >= pref[0]*60


def check_close(pref, ap, events):
    # Re-read both inputs immediately before sending the same native request
    # as the manual switch. In particular, a newly associated client cancels it.
    if preference() != pref or station_count() != (ap, 0) or events.changed(ap):
        return False
    p = subprocess.run(CLOSE, check=True, timeout=6, stdout=subprocess.PIPE,
                       stderr=subprocess.DEVNULL)
    if b'Error code: Success(0)' not in p.stdout:
        raise ValueError('native close request rejected')
    return True


def serve():
    timer = Timer()
    events = Associations()
    applied = None
    inherited_at = float('-inf')
    while True:
        try:
            pref = preference()
            now = time.monotonic()
            if pref is None:
                applied = None
                events.close()
                timer.reset()
                if now-inherited_at >= 30:
                    mode = policy()['wifi_enable_strategy']
                    tmp = INHERITED.with_suffix('.tmp')
                    tmp.write_bytes(bytes([2 if mode == 'kAuto' else 0]))
                    tmp.replace(INHERITED)
                    inherited_at = now
            else:
                if pref != applied:
                    ensure_native_always_on()
                    applied = pref
                    timer.reset()
                if pref[0]:
                    ap, clients = station_count()
                    if events.changed(ap):
                        timer.reset()
                    if timer.due(time.monotonic(), pref, ap, clients):
                        check_close(pref, ap, events)
                        timer.reset()
                else:
                    events.close()
        except (ValueError, OSError, subprocess.SubprocessError):
            events.close()
            timer.reset()
        time.sleep(2)


if __name__ == '__main__':
    serve()
