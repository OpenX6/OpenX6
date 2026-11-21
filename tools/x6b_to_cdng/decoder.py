"""OpenX6 1.1 Bayer readers. No demosaic, rescaling or new quantization."""
import json
import struct
import zlib
from dataclasses import dataclass
from pathlib import Path

import numpy as np
from numba import njit

LEVELS = {k.encode(): np.array(v, dtype=np.uint16) for k, v in
          json.loads(Path(__file__).with_name('levels.json').read_text()).items()}
SINGLE = (b'X6S8B01\0', b'X6S8B02\0')


@njit(cache=True)
def unpack(src, width, height, mixed, symbols, max_bits):
    out = np.zeros(width * height, np.uint16)
    pos = 0
    words = np.zeros(8, np.uint64)
    step = 32 if mixed else 64
    for y in range(height):
        for x in range(0, width, step):
            if pos >= len(src):
                raise ValueError('Truncated block header')
            header = np.int64(src[pos]); pos += 1
            b0, b1 = header & 15, header >> 4
            if b0 > max_bits or b1 > max_bits:
                raise ValueError('Invalid block width')
            equal = mixed and b0 == b1
            for half in range(1 if equal else 2):
                b = b0 if half == 0 else b1
                count = 32 if not mixed or equal else 16
                size = count * b // 8
                if pos + size > len(src):
                    raise ValueError('Truncated block payload')
                full, rem = divmod((count // 8) * b, 8)
                for lane in range(8):
                    word = np.uint64(0)
                    for k in range(full):
                        word |= np.uint64(src[pos + k * 8 + lane]) << np.uint64(8 * k)
                    if rem:
                        bit = lane * rem
                        value = np.int64(src[pos + full * 8 + bit // 8]) >> (bit % 8)
                        if bit % 8 + rem > 8:
                            value |= np.int64(src[pos + full * 8 + bit // 8 + 1]) << (8 - bit % 8)
                        word |= np.uint64(value & ((1 << rem) - 1)) << np.uint64(8 * full)
                    words[lane] = word
                for j in range(count):
                    z = np.int64((words[j % 8] >> np.uint64(b * (j // 8))) & np.uint64((1 << b) - 1))
                    col = x + half * count + j
                    i = y * width + col
                    left = np.int64(out[i - 2]) if col >= 2 else 0
                    up = np.int64(out[i - 2 * width]) if y >= 2 else 0
                    value = ((left + up + 1) >> 1) + (-(z + 1) // 2 if z & 1 else z // 2)
                    if value < 0 or value >= symbols:
                        raise ValueError('Decoded sample outside permitted range')
                    out[i] = value
                pos += size
    if pos != len(src):
        raise ValueError('Trailing predictor bytes')
    return out


def restore_tail(indices, levels, tail):
    out = levels[indices]
    if tail:
        groups = len(out) // 128
        pos = (groups + 7) // 8
        if len(tail) < pos:
            raise ValueError('Truncated low-bit bitmap')
        for g in range(groups):
            if tail[g // 8] & (1 << (g % 8)):
                if pos + 64 > len(tail):
                    raise ValueError('Truncated low-bit block')
                data = np.frombuffer(tail, np.uint8, 64, pos)
                out[g*128:g*128+128:2] += data & 15
                out[g*128+1:g*128+128:2] += data >> 4
                pos += 64
        if pos != len(tail):
            raise ValueError('Trailing low-bit bytes')
    if np.any(out >= 16384):
        raise ValueError('Invalid 14-bit sample')
    return out


def raw10_crc(values):
    a = values.reshape(-1, 4)
    packed = np.empty((len(a), 5), np.uint8)
    packed[:, :4] = a >> 2
    packed[:, 4] = ((a[:, 0] & 3) | ((a[:, 1] & 3) << 2) |
                    ((a[:, 2] & 3) << 4) | ((a[:, 3] & 3) << 6))
    return zlib.crc32(packed.tobytes())


@dataclass
class Record:
    offset: int
    magic: bytes
    request: int
    width: int
    height: int
    stamps: tuple
    sizes: tuple
    hardware: int = 0
    duration: int = 0
    crcs: tuple = ()
    crc_valid: bool = False

    @property
    def single(self):
        return self.magic in SINGLE

    @property
    def end(self):
        return self.offset + 4096 + sum((n + 4095) & ~4095 for n in self.sizes)


def read_header(header, offset):
    if len(header) != 4096:
        raise ValueError(f'Truncated frame header at byte {offset}')
    magic = header[:8]
    if magic in SINGLE:
        request, w, h, parts = struct.unpack_from('<4I', header, 8)
        if (w, h, parts) != (7680, 7680, 24):
            raise ValueError('Unsupported single-eye dimensions')
        hardware, stamp, duration = struct.unpack_from('<3Q', header, 24)
        sizes = struct.unpack_from('<24I', header, 48)
        crcs = struct.unpack_from('<24I', header, 144)
        flag, = struct.unpack_from('<I', header, 240)
        if flag not in (0, 1):
            raise ValueError('Unknown CRC flag')
        record = Record(offset, magic, request, w, h, (stamp,), sizes,
                        hardware, duration, crcs, bool(flag))
    elif magic in LEVELS:
        size, request, w, h, parts, threshold = struct.unpack_from('<6I', header, 8)
        if (size, w, h, parts, threshold) != (4096, 3840, 3840, 24, 13472):
            raise ValueError('Unsupported dual-eye header')
        record = Record(offset, magic, request, w, h,
                        struct.unpack_from('<2Q', header, 32),
                        struct.unpack_from('<48I', header, 48))
    else:
        raise ValueError(f'Unsupported magic {magic!r} at byte {offset}; only released 1.1 profiles are supported')
    limit = w * (h // 24) * 4
    if any(n <= 4 or n > limit for n in record.sizes):
        raise ValueError(f'Invalid stripe length at byte {offset}')
    return record


def scan(path, cancelled=lambda: False):
    """Headers only; reject incomplete/ambiguous input before writing output."""
    records = []
    length = path.stat().st_size
    with path.open('rb') as f:
        pos = 0
        while pos < length:
            if cancelled():
                raise InterruptedError('Cancelled')
            f.seek(pos)
            record = read_header(f.read(4096), pos)
            if record.end > length:
                raise ValueError(f'Truncated frame {record.request} at byte {pos}')
            records.append(record)
            pos = record.end
    if not records:
        raise ValueError('Empty capture')
    if len({r.magic for r in records}) != 1:
        raise ValueError('Mixed profiles in one capture are not supported')
    if len({r.request for r in records}) != len(records):
        raise ValueError('Duplicate request IDs')
    records.sort(key=lambda r: r.request)
    for before, after in zip(records, records[1:]):
        if any(b <= a for a, b in zip(before.stamps, after.stamps)):
            raise ValueError('Exposure timestamps do not increase with request IDs')
        if before.single and after.hardware <= before.hardware:
            raise ValueError('Hardware frame IDs are not increasing')
    return records


def decode_eye(f, record, eye):
    if eye not in range(1 if record.single else 2):
        raise ValueError('Eye absent from record')
    height = record.height // 24
    result = np.empty((record.height, record.width), np.uint16)
    first = eye * 24
    pos = record.offset + 4096 + sum((n+4095)&~4095 for n in record.sizes[:first])
    for stripe in range(24):
        n = record.sizes[first + stripe]
        f.seek(pos)
        payload = f.read(n)
        if len(payload) != n:
            raise ValueError('Input changed or stripe truncated')
        main, = struct.unpack_from('<I', payload)
        if main > n - 4 or (record.single and main != n - 4):
            raise ValueError('Invalid predictor length')
        levels = None if record.single else LEVELS[record.magic]
        data = unpack(np.frombuffer(payload, np.uint8, main, 4), record.width,
                      height, record.magic == SINGLE[1],
                      1024 if record.single else len(levels),
                      15 if record.magic == b'L1024___' else 11)
        if record.single:
            if record.crc_valid and raw10_crc(data) != record.crcs[stripe]:
                raise ValueError(f'RAW10 CRC mismatch: request {record.request}, stripe {stripe}')
        else:
            data = restore_tail(data, levels, payload[4+main:])
        result[stripe*height:(stripe+1)*height] = data.reshape(height, record.width)
        pos += (n+4095)&~4095
    return result
