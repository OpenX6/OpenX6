"""Receive the three stock modules over the owner's X6 hotspot."""
import contextlib
import hashlib
import hmac
import http.server
import io
import os
from pathlib import Path
import secrets
import select
import shutil
import socketserver
import struct
import sys
import tarfile
import tempfile
import threading
import time

from openx6.common import digest, regular, require
from openx6.official import COMPONENTS, inspect_stream
from . import transport
from .worker.profile import UPDATERS, check_updater

CACHE = 'X6-1.2.36-modules'


def discover():
    with contextlib.closing(transport.Session()) as session:
        return session.identity(tuple(UPDATERS))


def verify_modules(path):
    require(not path.is_symlink() and path.is_dir() and
            {p.name for p in path.iterdir()} == set(COMPONENTS), 'Incomplete X6 module set.')
    for name, (length, checksum) in COMPONENTS.items():
        require((path/name).stat().st_size == length and digest(path/name) == checksum,
                'Module verification failed: '+name)
    return path


def make_bundle(remote, host, webport, rangeport, token, version):
    require(version in UPDATERS, 'Unsupported export version')
    url = f'http://{host}:{webport}/{token}/report'
    script = f"""#!/bin/sh
cd {remote} || exit 1
NO_INSTALL=1 /usr/bin/timeout 1000 /usr/bin/python3 -IB guest.py {host} {webport} {rangeport} {token} full {version} > note 2>&1
rc=$?
if [ "$rc" = 0 ]; then result=OK; else result=FAIL; fi
for x in note guest.py profile.py caller.so fusein tool run; do /bin/busybox unlink "$x" || exit 2; done
if [ -d jail ]; then /bin/busybox rmdir jail || exit 2; fi
cd /run && /bin/busybox rmdir {remote} || exit 2
/usr/bin/timeout 10 /usr/bin/wget -q -O /dev/null --post-data="$result" {url}
"""
    buf = io.BytesIO()
    worker = Path(__file__).with_name('worker')
    if getattr(sys, 'frozen', False):
        worker = Path(sys.executable).resolve().parents[1]/'Resources/camera-worker'
    with contextlib.ExitStack() as stack:
        if not (worker/'caller.so').exists() and (worker/'build.py').is_file():
            from .worker.build import build
            worker = Path(stack.enter_context(tempfile.TemporaryDirectory()))
            build(worker)
        archive = stack.enter_context(tarfile.open(fileobj=buf, mode='w:gz'))
        for name in ('guest.py','profile.py','caller.so','fusein','run'):
            data = script.encode() if name == 'run' else (worker/name).read_bytes()
            info = tarfile.TarInfo(name)
            info.mode = {'fusein': 0o755, 'caller.so': 0o444}.get(name, 0o600)
            info.size = len(data)
            archive.addfile(info, io.BytesIO(data))
    return buf.getvalue()


def receive(session, source, size, stage, progress, cancel, version, byte_progress=None):
    """All handlers are joined before the caller can delete or publish staging."""
    token = secrets.token_hex(16)
    remote = '/run/x6exp-'+token[:16]
    closing = threading.Event()
    done = threading.Event()
    accepted = threading.Event()
    received = set()
    camera = transport.CAMERA

    class PeerOnly:
        def verify_request(self, request, address):
            request.settimeout(25)
            return address[0] == camera

    class Range(PeerOnly, socketserver.TCPServer):
        pass

    class Input(socketserver.StreamRequestHandler):
        rbufsize = 0  # select() must see bytes following the token in the same TCP packet.

        def read_exact(self, count):
            data = bytearray()
            while len(data) < count:
                part = self.rfile.read(count-len(data))
                if not part: break
                data.extend(part)
            return bytes(data)

        def handle(self):
            if not hmac.compare_digest(self.read_exact(len(token)), token.encode()):
                return
            while not (closing.is_set() or cancel.is_set()):
                if not select.select([self.connection], [], [], 1)[0]: continue
                req = self.read_exact(16)
                if len(req) != 16: return
                offset, length = struct.unpack('<QQ', req)
                if not 0 < length <= 4 << 20 or offset+length > size: return
                source.seek(offset)
                data = source.read(length)
                if len(data) != length: return
                self.wfile.write(data)

    class Web(PeerOnly, http.server.HTTPServer):
        pass

    class Output(http.server.BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def route(self):
            prefix = '/'+token+'/'
            return self.path[len(prefix):] if self.path.startswith(prefix) else ''

        def do_GET(self):
            if self.route() != 'tool': return self.send_error(404)
            self.send_response(200)
            self.send_header('Content-Length', str(len(bundle)))
            self.end_headers()
            self.wfile.write(bundle)

        def do_POST(self):
            if self.route() != 'report': return self.send_error(404)
            n = self.headers.get('Content-Length')
            if n not in ('2','4'): return self.send_error(400)
            if self.rfile.read(int(n)) == b'OK': accepted.set()
            self.send_response(200)
            self.end_headers()
            done.set()

        def do_PUT(self):
            name = self.route()
            if name not in COMPONENTS or (stage/name).exists(): return self.send_error(403)
            length, checksum = COMPONENTS[name]
            if self.headers.get('Content-Length') != str(length): return self.send_error(400)
            progress('Receiving / 正在接收 '+name)
            remaining = length
            last_percent = -1
            hasher = hashlib.sha256()
            with (stage/name).open('xb') as output:
                while remaining and not (closing.is_set() or cancel.is_set()):
                    block = self.rfile.read(min(1 << 20, remaining))
                    if not block: return
                    output.write(block)
                    hasher.update(block)
                    remaining -= len(block)
                    percent = 100 * (length - remaining) // length
                    if byte_progress and percent != last_percent:
                        byte_progress('Receiving / 正在接收 ' + name, length - remaining, length)
                        last_percent = percent
                if remaining or hasher.hexdigest() != checksum: return self.send_error(409)
                output.flush()
                os.fsync(output.fileno())
            received.add(name)
            self.send_response(201)
            self.end_headers()

    with Range((session.host,0), Input) as ranges, Web((session.host,0), Output) as web:
        bundle = make_bundle(remote, session.host, web.server_port, ranges.server_address[1], token, version)
        h = hashlib.sha256(bundle).hexdigest()
        url = f'http://{session.host}:{web.server_port}/{token}/tool'
        bootstrap = (f'umask 077; mkdir {remote} && cd {remote} || exit 1; '
                     f'if timeout 10 wget -q -O tool {url} && echo "{h}  tool"|sha256sum -c - && tar -xzf tool && chmod 444 caller.so; '
                     'then sh run </dev/null >/dev/null 2>&1 & '
                     'else for x in tool run fusein caller.so guest.py profile.py; do [ ! -f "$x" ] || unlink "$x"; done; '
                     f'cd /run; rmdir {remote}; exit 1; fi')
        threads = [threading.Thread(target=s.serve_forever,daemon=True) for s in (ranges,web)]
        for t in threads: t.start()
        try:
            transport.execute(session, bootstrap)
            deadline = time.monotonic()+1030
            while not done.wait(.15):
                if cancel.is_set(): raise InterruptedError('Export canceled / 已取消导出')
                if time.monotonic() > deadline: raise TimeoutError('Camera export timed out.')
                session.tick(.05)
            require(accepted.is_set() and received == set(COMPONENTS),
                    'Camera export failed. Check camera power, Wi-Fi and firmware.')
        finally:
            closing.set()
            for server in (ranges,web): server.shutdown()
            for thread in threads: thread.join()


def export_to_cache(firmware, parent, serial, progress, cancel=None, byte_progress=None):
    parent = Path(parent)
    parent.mkdir(mode=0o700,parents=True,exist_ok=True)
    target = parent/CACHE
    cancel = cancel or threading.Event()
    if target.exists(): return verify_modules(target)
    require(shutil.disk_usage(parent).free > 2*1024**3,'Camera export needs 2 GiB of free space.')
    with regular(firmware) as source, contextlib.closing(transport.Session()) as session:
        report, _, _ = inspect_stream(source)
        identity = session.identity()
        version = identity[1].lower().removeprefix('v')
        require(identity[0] == serial and version in UPDATERS,
                'On-camera export requires an authorized stock 0.8.825 or 0.8.827 X6.')
        client = 'x6-owner-'+secrets.token_hex(12)
        if not session.authorized(client):
            session.query(0x56)
            progress('Tap Allow on your X6 / 请在相机上点允许')
            deadline = time.monotonic()+60
            while not session.authorized(client):
                if cancel.wait(.5): raise InterruptedError('Export canceled')
                if time.monotonic()>deadline: raise TimeoutError('Camera authorization timed out.')
        require(session.identity()==identity, 'Connected camera changed.')
        pin = transport.execute(session,'sha256sum /usr/bin/ins_otad').split()
        check_updater(version, pin[0].decode('ascii') if pin else '')
        from .power import prepare
        prepare(session, transport.execute, progress, cancel)
        with tempfile.TemporaryDirectory(dir=parent, prefix='.x6-export-') as scratch:
            stage = Path(scratch)/'modules'
            stage.mkdir(mode=0o700)
            progress('Unpacking on camera / 正在机内解包，请保持相机开机和 Wi-Fi 连接')
            receive(session,source,report['bytes'],stage,progress,cancel,version,byte_progress)
            verify_modules(stage)
            if cancel.is_set(): raise InterruptedError('Export canceled / 已取消导出')
            if target.exists(): return verify_modules(target)
            stage.rename(target)
    return target
