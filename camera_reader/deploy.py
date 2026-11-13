"""Send one GUI-verified build to an owner-confirmed X6 over its hotspot."""
import hashlib
import http.server
import secrets
import sys
import threading
import time
from contextlib import closing
from pathlib import Path

from openx6.common import regular, require
from . import transport

FINAL = 'Insta360X6FW.bin'


def worker_source():
    if getattr(sys, 'frozen', False):
        return (Path(sys.executable).resolve().parents[1]/'Resources/camera-worker/deploy.py').read_bytes()
    return (Path(__file__).parent/'worker/deploy.py').read_bytes()


def bootstrap(host, port, token, worker_hash, size, checksum):
    url = f'http://{host}:{port}/{token}/worker'
    return (f'f=$(mktemp /run/x6sd.XXXXXX) || exit 1; '
            f'if wget -q -T 15 -O "$f" {url} && echo "{worker_hash}  $f" | sha256sum -c - >/dev/null; then '
            f'(python3 -IB "$f" {host} {port} {token} {size} {checksum}; rm -f "$f") </dev/null >/dev/null 2>&1 & '
            'else rm -f "$f"; exit 1; fi')


def transfer(session, source, size, checksum, byte_progress, *, rehearsal=False, request_scan=False):
    token = secrets.token_hex(16)
    worker = worker_source()
    received = threading.Event()
    outcome = []
    offered = copied = approved = False
    published_at = None
    finished = None

    class Handler(http.server.BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def reply(self, data=b'OK'):
            self.send_response(200)
            self.send_header('Content-Length', str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            nonlocal offered, copied
            if self.path == f'/{token}/worker':
                return self.reply(worker)
            if offered or self.path != f'/{token}/card':
                return self.send_error(404)
            offered = True
            self.send_response(200)
            self.send_header('Content-Length', str(size))
            self.end_headers()
            done = 0
            last = -1
            while done < size:
                chunk = source.read(min(1 << 20, size-done))
                if not chunk:
                    return
                self.wfile.write(chunk)
                done += len(chunk)
                if done * 100 // size != last:
                    last = done * 100 // size
                    byte_progress('Sending to camera SD / 正在通过 Wi-Fi 传到相机 SD 卡', done, size)
            copied = True

        def do_POST(self):
            nonlocal approved, published_at, finished
            prefix = f'/{token}/'
            length = self.headers.get('Content-Length','')
            if not self.path.startswith(prefix) or not length.isdigit() or not 0 < int(length) <= 1024:
                return self.send_error(400)
            value = self.rfile.read(int(length)).decode()
            action = self.path[len(prefix):]
            if action == 'readback' and copied and value.isdigit() and 0 < int(value) <= size:
                byte_progress('Verifying camera SD / 正在从相机 SD 卡读回校验', int(value), size)
            elif action == 'verified' and copied and not approved and value == checksum:
                approved = True
                return self.reply(b'DISCARD' if rehearsal else b'PUBLISH')
            elif action == 'published' and approved and not rehearsal and published_at is None and value == checksum:
                published_at = time.monotonic()
                return self.reply(b'SCAN' if request_scan else b'SKIP')
            elif action == 'done' and approved and (
                (rehearsal and value == 'DISCARDED')
                or (published_at is not None and request_scan and value in ('SCAN_REQUESTED', 'SCAN_UNCONFIRMED'))
                or (published_at is not None and not request_scan and value == 'PUBLISHED')
            ):
                finished = value
                outcome.append(None)
                received.set()
            elif action == 'error':
                outcome.append(value)
                received.set()
            else:
                return self.send_error(409)
            self.reply()

    class Server(http.server.HTTPServer):
        def verify_request(self, request, address):
            request.settimeout(45)
            return address[0] == transport.CAMERA

    with Server((session.host, 0), Handler) as server:
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            launch = bootstrap(session.host, server.server_port, token, hashlib.sha256(worker).hexdigest(), size, checksum)
            transport.execute(session, launch)
            deadline = time.monotonic()+1800
            while not received.wait(.15):
                if published_at is not None and time.monotonic() - published_at > 30:
                    raise TimeoutError('Scan receipt timed out')
                if time.monotonic() > deadline:
                    raise TimeoutError('Transfer timed out / Wi-Fi 传输超时')
                session.tick(.05)
            if outcome[0]:
                raise ValueError(outcome[0])
            return finished
        except Exception as exc:
            if published_at is not None:
                return 'SCAN_UNCONFIRMED' if request_scan else 'PUBLISHED'
            if approved and not rehearsal:
                raise RuntimeError('Final SD status uncertain. Check the camera before retrying. / 请先检查相机是否已进入升级，再决定是否重试。') from exc
            raise
        finally:
            server.shutdown()
            thread.join()


def supported_sources():
    root = Path(__file__).resolve().parents[1]
    import json
    return {s['Version']: s['OTASHA256'] for s in json.loads((root/'install-guard/platform.json').read_bytes())['Sources']}


def discover():
    with closing(transport.Session()) as session:
        return session.identity(supported_sources())


def to_card(path, result, identity, progress, byte_progress):
    require(path.name == result.get('candidate') == FINAL and result.get('install_allowed') is True
            and result.get('owner_authorization_embedded') is True, 'Expected a verified signed build')
    size, checksum = result['bytes'], result['sha256']
    with regular(path) as source:
        progress('Checking local build / 正在核对本机成品')
        require(path.stat().st_size == size and hashlib.file_digest(source, 'sha256').hexdigest() == checksum,
                'Local build changed / 本机成品已变化，请重新构建')
        source.seek(0)
        with closing(transport.Session()) as session:
            require(session.identity(supported_sources()) == identity, 'Camera changed / 相机已切换')
            client = 'x6-owner-' + secrets.token_hex(12)
            if not session.authorized(client):
                session.query(0x56)
                progress('Tap Allow on the camera / 请在相机上点允许')
                until = time.monotonic()+60
                while not session.authorized(client):
                    if time.monotonic() > until:
                        raise TimeoutError('Camera approval timed out')
                    time.sleep(.5)
            require(session.identity(supported_sources()) == identity, 'Camera changed / 相机已切换')
            pin = transport.execute(session, 'sha256sum /usr/bin/ins_otad').split()
            expected = supported_sources()[identity[1].lstrip('vV')]
            require(pin[:1] == [expected.encode()], 'Unsupported camera updater / 此升级器不支持部署')
            progress('Checking camera SD / 正在检查机内 SD 卡')
            return transfer(session, source, size, checksum, byte_progress,
                            request_scan=identity[1].lstrip('vV') == '0.8.825')
