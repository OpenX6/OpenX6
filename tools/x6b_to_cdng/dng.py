"""Uncompressed 16-bit CFA CinemaDNG frames, with original integer samples."""
import json
import struct
from fractions import Fraction
from pathlib import Path

REFERENCE = json.loads(Path(__file__).with_name('color-reference.json').read_text())


def write(path, pixels, clip, fps, description, exposure_ns=0):
    fields = {}

    def put(tag, kind, fmt, values):
        fields[tag] = (kind, len(values), struct.pack('<' + fmt * len(values), *values))

    def text(tag, value):
        data = value.encode('ascii', 'replace') + b'\0'
        fields[tag] = (2, len(data), data)

    def rational(tag, values, signed=False):
        values = [Fraction(str(v)).limit_denominator(1000000) for v in values]
        data = b''.join(struct.pack('<ii' if signed else '<II', v.numerator, v.denominator) for v in values)
        fields[tag] = (10 if signed else 5, len(values), data)

    h, w = pixels.shape
    white = int(clip.get('white_level', 1023 if w == 7680 else REFERENCE['white']))
    black = int(clip.get('black_level', REFERENCE['black']))
    cfa = clip.get('cfa_pattern', REFERENCE['cfa'])
    if not 0 <= black < white <= 65535 or cfa != [1, 2, 0, 1]:
        raise ValueError('Unsupported black/white levels or CFA metadata')
    for tag, value in [(254, 0), (256, w), (257, h), (273, 0), (278, h),
                       (279, w*h*2), (50717, white)]:
        put(tag, 4, 'I', [value])
    for tag, value in [(258, 16), (259, 1), (262, 32803), (274, 1),
                       (277, 1), (284, 1), (50778, REFERENCE['illuminant'])]:
        put(tag, 3, 'H', [value])
    put(33421, 3, 'H', [2, 2]); put(33422, 1, 'B', cfa)
    put(50706, 1, 'B', [1, 4, 0, 0]); put(50707, 1, 'B', [1, 1, 0, 0])
    put(50710, 1, 'B', [0, 1, 2]); put(50711, 3, 'H', [1])
    put(50713, 3, 'H', [1, 1]); rational(50714, [black])
    put(50719, 4, 'I', [0, 0]); put(50720, 4, 'I', [w, h])
    put(50829, 4, 'I', [0, 0, h, w])
    text(271, 'Arashi Vision'); text(272, 'Insta360 X6')
    text(305, 'OpenX6 x6b_to_cdng')
    text(50708, f'OpenX6 Bayer {w}')
    text(270, description + '; development reference color matrix and white balance')
    rational(50721, REFERENCE['color'], True)
    rational(50728, REFERENCE['neutral'])
    rational(51044, [fps], True)  # CinemaDNG FrameRate

    # ExposureTime belongs in an EXIF IFD, not the TIFF root IFD.
    if exposure_ns:
        put(34665, 4, 'I', [0])

    def encode():
        start = 8 + 2 + 12*len(fields) + 4
        extra = bytearray()
        header = bytearray(b'II' + struct.pack('<HIH', 42, 8, len(fields)))
        for tag, (kind, count, data) in sorted(fields.items()):
            header += struct.pack('<HHI', tag, kind, count)
            if len(data) <= 4:
                header += data.ljust(4, b'\0')
            else:
                header += struct.pack('<I', start + len(extra))
                extra += data
                extra += bytes(len(extra) % 2)
        return header + bytes(4) + extra

    root_size = len(encode())
    exif = b''
    if exposure_ns:
        f = Fraction(int(exposure_ns), 1000000000).limit_denominator(1000000000)
        put(34665, 4, 'I', [root_size])
        exif = struct.pack('<HHHIII', 1, 33434, 5, 1, root_size+18, 0)
        exif += struct.pack('<II', f.numerator, f.denominator)
    put(273, 4, 'I', [root_size + len(exif)])
    temporary = path.with_suffix('.dng.part')
    if path.exists():
        raise FileExistsError(path)
    try:
        with temporary.open('xb') as f:
            f.write(encode()); f.write(exif)
            f.write(pixels.astype('<u2', copy=False).tobytes())
        temporary.rename(path)
    finally:
        temporary.unlink(missing_ok=True)
