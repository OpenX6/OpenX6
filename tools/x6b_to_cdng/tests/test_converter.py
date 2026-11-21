"""Run: python -m unittest discover -s tools/x6b_to_cdng/tests -v"""
import gzip
import hashlib
import json
import struct
import sys
import tempfile
import unittest
from fractions import Fraction
from pathlib import Path
from unittest.mock import patch

import numpy as np
import tifffile

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from decoder import LEVELS, unpack, restore_tail, scan, decode_eye, read_header, raw10_crc
from convert import convert, rate
from dng import write


def encode(values, width, mixed):
    """Independent scalar encoder for small test stripes (not for camera use)."""
    residual = []
    for i, sample in enumerate(values):
        x, y = i % width, i // width
        pred = ((int(values[i-2]) if x >= 2 else 0) +
                (int(values[i-width*2]) if y >= 2 else 0) + 1)//2
        delta = int(sample)-pred
        residual.append(delta*2 if delta >= 0 else -delta*2-1)
    out = bytearray()
    def pack(group, bits):
        lane_bits = len(group)//8*bits
        words = [sum(group[lane+8*j] << (j*bits) for j in range(len(group)//8)) for lane in range(8)]
        full, rem = divmod(lane_bits, 8)
        for k in range(full):
            out.extend((word >> (8*k)) & 255 for word in words)
        if rem:
            remainder = sum((word >> (8*full)) << (lane*rem) for lane, word in enumerate(words))
            out.extend(remainder.to_bytes(rem, 'little'))
    step = 32 if mixed else 64
    half = step//2
    for start in range(0, len(residual), step):
        a, b = residual[start:start+half], residual[start+half:start+step]
        b0, b1 = max(a).bit_length(), max(b).bit_length()
        out.append(b0 | b1 << 4)
        if mixed and b0 == b1:
            pack(a+b, b0)
        else:
            pack(a, b0); pack(b, b1)
    return bytes(out)


class CodecTests(unittest.TestCase):
    def test_scalar_roundtrip_all_widths(self):
        rng = np.random.default_rng(927)
        for mixed in (False, True):
            for bits in range(1, 11):
                values = rng.integers(0, 1 << bits, 64*8, dtype=np.uint16)
                payload = encode(values, 64, mixed)
                actual = unpack(np.frombuffer(payload, np.uint8), 64, 8, mixed, 1024, 11)
                np.testing.assert_array_equal(values, actual)
                with self.assertRaises(ValueError):
                    unpack(np.frombuffer(payload[:-1], np.uint8), 64, 8, mixed, 1024, 11)
                with self.assertRaises(ValueError):
                    unpack(np.frombuffer(payload+b'\0', np.uint8), 64, 8, mixed, 1024, 11)

    def test_bad_symbol_and_width(self):
        for payload in (b'\xff', b'\x0b'+b'\xff'*44):
            with self.assertRaises(ValueError):
                unpack(np.frombuffer(payload, np.uint8), 64, 1, False, 1024, 11)

    def test_lossless_14bit_entire_code_range(self):
        values = np.arange(16384, dtype=np.uint16)
        indices = np.where(values < 1024, values, 1024+(values-1024)//16).astype(np.uint16)
        payload = encode(indices, 128, False)
        decoded = unpack(np.frombuffer(payload, np.uint8), 128, 128, False, 1984, 15)
        groups = len(values)//128
        bitmap = bytearray((groups+7)//8); blocks = bytearray()
        for g in range(groups):
            v = values[g*128:(g+1)*128]
            low = np.where(v >= 1024, v & 15, 0).astype(np.uint8)
            if low.any():
                bitmap[g//8] |= 1 << (g%8)
                blocks.extend((low[::2] | low[1::2] << 4).tobytes())
        actual = restore_tail(decoded, LEVELS[b'L1024___'], bytes(bitmap+blocks))
        np.testing.assert_array_equal(actual, values)
        with self.assertRaises(ValueError):
            restore_tail(decoded, LEVELS[b'L1024___'], bytes(bitmap+blocks[:-1]))

    def test_raw10_crc_layout(self):
        import zlib
        values = np.array([0, 1, 2, 3, 1020, 1021, 1022, 1023], np.uint16)
        self.assertEqual(raw10_crc(values), zlib.crc32(bytes([0,0,0,0,228,255,255,255,255,228])))

    def test_published_synthetic_vectors(self):
        manifest = json.loads((ROOT/'tests/expected.json').read_text())
        with tempfile.TemporaryDirectory() as tmp:
            for name, expected in manifest.items():
                with self.subTest(name=name):
                    path = Path(tmp)/'test.x6b'
                    path.write_bytes(gzip.decompress((ROOT/'tests/fixtures'/name).read_bytes()))
                    record = scan(path)[0]
                    digest = hashlib.sha256()
                    with path.open('rb') as f:
                        for eye in (0, 1):
                            digest.update(decode_eye(f, record, eye).astype('<u2').tobytes())
                    self.assertEqual(digest.hexdigest(), expected['sha256'])


class ContainerTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.data = gzip.decompress((ROOT/'tests/fixtures/synthetic-X6QBLK0B.x6b.gz').read_bytes())

    def test_scan_order_gaps_and_rejections(self):
        later = bytearray(self.data)
        first = read_header(self.data[:4096], 0)
        struct.pack_into('<I', later, 12, first.request+2)
        for at in (32, 40):
            struct.pack_into('<Q', later, at, struct.unpack_from('<Q', later, at)[0]+33366667)
        with tempfile.TemporaryDirectory() as td:
            path = Path(td)/'test.x6b'; path.write_bytes(later+self.data)
            rows = scan(path)
            self.assertEqual([r.request for r in rows], [first.request, first.request+2])
            self.assertEqual(rows[0].offset, len(later))
            for invalid in (self.data[:-1], b'BADMAGIC'+self.data[8:], self.data*2):
                path.write_bytes(invalid)
                with self.assertRaises(ValueError):
                    scan(path)
            bad = bytearray(self.data); struct.pack_into('<I', bad, 48, 0xffffffff)
            path.write_bytes(bad)
            with self.assertRaises(ValueError):
                scan(path)
            bad = bytearray(self.data); struct.pack_into('<I', bad, 16, 5504)
            path.write_bytes(bad)
            with self.assertRaisesRegex(ValueError, 'Unsupported'):
                scan(path)

    def test_cinema_dng_tags_and_pixels(self):
        pixels = np.arange(128*128, dtype=np.uint16).reshape(128, 128)
        with tempfile.TemporaryDirectory() as td:
            path = Path(td)/'test.dng'
            write(path, pixels, {}, Fraction(30000, 1001), 'synthetic frame', 10000000)
            with tifffile.TiffFile(path) as tif:
                page = tif.pages[0]
                np.testing.assert_array_equal(page.asarray(), pixels)
                self.assertEqual(page.tags[258].value, 16)
                self.assertEqual(page.tags[33422].value, bytes([1,2,0,1]))
                self.assertEqual(page.tags[51044].value, (30000,1001))
                self.assertEqual(page.tags[34665].value['ExposureTime'], (1,100))
                self.assertEqual(page.tags[50717].value, 15408)
            with self.assertRaises(FileExistsError):
                write(path, pixels, {}, Fraction(30), 'test')

    def test_conversion_gap_manifest_and_cancellation(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td); source = root/'source'; source.mkdir()
            later = bytearray(self.data)
            request = struct.unpack_from('<I', later, 12)[0]
            struct.pack_into('<I', later, 12, request+2)
            for at in (32,40):
                struct.pack_into('<Q', later, at, struct.unpack_from('<Q', later, at)[0]+33366667)
            (source/'capture.x6b').write_bytes(later+self.data)
            (source/'clip.json').write_text('{"fps_setting":60,"size":[3840,3840]}')
            (source/'audio.aac').write_bytes(b'unchanged audio fixture')
            original = hashlib.sha256((source/'capture.x6b').read_bytes()).hexdigest()
            out = root/'out'
            result = convert(source, out, eye='CAM0', progress=lambda _: None)
            self.assertEqual(result['missing_frames'], 1)
            self.assertEqual(result['written_dngs'], 3)
            self.assertEqual(result['status'], 'complete')
            self.assertEqual((out/'CAM0/frame_00000000.dng').read_bytes(), (out/'CAM0/frame_00000001.dng').read_bytes())
            self.assertEqual((out/'metadata/audio.aac').read_bytes(), b'unchanged audio fixture')
            self.assertIn('True', (out/'frames.csv').read_text())
            self.assertEqual(original, hashlib.sha256((source/'capture.x6b').read_bytes()).hexdigest())
            with self.assertRaises(FileExistsError):
                convert(source, out, progress=lambda _: None)
            with self.assertRaises(ValueError):
                convert(source, source/'bad', progress=lambda _: None)
            with self.assertRaises(ValueError):
                convert(source, root/'gaps-error', gaps='error', progress=lambda _: None)
            with self.assertRaises(InterruptedError):
                convert(source, root/'cancelled', cancelled=lambda: True, progress=lambda _: None)
            with patch('convert.decode_eye', side_effect=ValueError('corrupt stripe')):
                with self.assertRaises(ValueError):
                    convert(source, root/'failed', progress=lambda _: None)
            self.assertEqual(json.loads((root/'failed/conversion.json').read_text())['status'], 'failed')


if __name__ == '__main__':
    unittest.main()
