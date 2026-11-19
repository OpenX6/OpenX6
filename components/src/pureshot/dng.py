#!/usr/bin/env python3
"""DNG inspection/verification for the PureShot processor (DESIGN-v3 §5.1 V0, §5.3 step 6 V1).

Pure stdlib (zlib, hashlib, struct, array) so it runs on the camera's Python 3.10. All heavy scans use
C-level primitives (bytes.translate, array slices + sum) so a full-file pass stays within a few seconds,
and the per-channel mean check samples rows deterministically so V0 (on the original F) and V1 (on the
converted T) measure the identical rows and are directly comparable.

Layout facts (verified on 965/968 and the 16K-fast frames): 15520x7760, 16-bit, Compression 8, 31 strips,
level-0 stored-deflate (each strip is a zlib stream whose first deflate block is stored/BTYPE 0), strips
contiguous from file offset 8, IFD + ~30 KB trailer at the end, WhiteLevel 1023 (passthrough) / 16383
(converted). Only the strip payloads and the 4-byte WhiteLevel value differ between F and its conversion.
"""
import array, hashlib, struct, zlib
import contract as C

TIFF_LE = b'II*\x00'
T_WIDTH, T_HEIGHT, T_BPS, T_COMPRESSION = 256, 257, 258, 259
T_STRIP_OFFSETS, T_ROWS_PER_STRIP, T_STRIP_COUNTS, T_PHOTOMETRIC = 273, 278, 279, 262
T_WHITELEVEL = 50717
# Only these fields are consumed by the 120 MP geometry check.
IFD_COUNTS = {T_WIDTH: 1, T_HEIGHT: 1, T_BPS: 1, T_COMPRESSION: 1,
              T_STRIP_OFFSETS: C.DNG_STRIPS, T_STRIP_COUNTS: C.DNG_STRIPS,
              T_ROWS_PER_STRIP: 1, T_WHITELEVEL: 1}
# Mean-check row sampling: take SAMPLE_RUN consecutive rows every SAMPLE_PERIOD. The run must span a full
# 4-row quad cycle so all four colours (G1,B,R,G2) are measured -- a single stride like y%8 lands on one
# band only and silently drops R/G2. ~970 rows sampled. NOTE: a global mean catches global errors (whole
# frame dark / phase-swapped / constant); it cannot catch a localised defect confined to a few strips
# (that is what per-strip Adler + the atomic-write + redo-from-T recovery cover instead).
SAMPLE_PERIOD, SAMPLE_RUN = 32, 4
def _sampled(y):
    return (y % SAMPLE_PERIOD) < SAMPLE_RUN
PEDESTAL = 64                  # passthrough black pedestal (tag reads 0, real floor ~64)
PEDESTAL_LOW = 32              # a passthrough sample below this is suspicious (torn/converted strip)
EYE_SPLIT = C.DNG_W // 2       # 7760; left half = eye 1, right half = eye 0


class DngError(Exception):
    pass


def _read_ifd0(f):
    """Return (tags, whitelevel_value_file_offset). tags[tag] = (type, count, value_or_offset, raw_bytes)."""
    f.seek(0)
    hdr = f.read(8)
    if len(hdr) != 8 or hdr[:4] != TIFF_LE:
        raise DngError('not little-endian TIFF')
    off = struct.unpack_from('<I', hdr, 4)[0]
    size = f.seek(0, 2)
    if off < 8 or off + 2 > size:
        raise DngError('IFD not yet available')
    f.seek(off)
    n = struct.unpack('<H', f.read(2))[0]
    if off + 2 + 12 * n > size:
        raise DngError('truncated IFD')
    ent = f.read(12 * n)
    if len(ent) != 12 * n:
        raise DngError('truncated IFD')
    tags = {}
    wl_off = None
    for k in range(n):
        tag, typ, cnt, val = struct.unpack_from('<HHII', ent, 12 * k)
        if tag not in IFD_COUNTS:
            continue
        if tag in tags or cnt != IFD_COUNTS[tag] or typ not in (3, 4):
            raise DngError('unexpected geometry field %d' % tag)
        nbytes = cnt * (2 if typ == 3 else 4)
        val_field_off = off + 2 + 12 * k + 8
        if nbytes <= 4:
            raw = ent[12 * k + 8:12 * k + 8 + nbytes]
        else:
            if val < 8 or val + nbytes > size:
                raise DngError('truncated geometry field')
            f.seek(val)
            raw = f.read(nbytes)
            if len(raw) != nbytes:
                raise DngError('truncated geometry field')
        tags[tag] = (typ, cnt, val, raw)
        if tag == T_WHITELEVEL:
            wl_off = val_field_off
    return tags, wl_off


def _arr(tags, tag):
    try:
        typ, cnt, val, raw = tags[tag]
    except KeyError:
        raise DngError('missing tag %d' % tag)          # malformed IFD -> decode error, not an uncaught crash
    if typ == 3:
        return list(struct.unpack('<%dH' % cnt, raw[:2 * cnt]))
    if typ == 4:
        return list(struct.unpack('<%dI' % cnt, raw[:4 * cnt]))
    return [val]


def geometry(f):
    """Parse and sanity-check geometry; return a layout dict or raise DngError."""
    tags, wl_off = _read_ifd0(f)
    for tag, want in ((T_WIDTH, C.DNG_W), (T_HEIGHT, C.DNG_H), (T_BPS, C.DNG_BPS), (T_COMPRESSION, C.DNG_COMPRESSION)):
        if _arr(tags, tag)[0] != want:
            raise DngError('tag %d != %d' % (tag, want))
    offs = _arr(tags, T_STRIP_OFFSETS)
    cnts = _arr(tags, T_STRIP_COUNTS)
    rps = _arr(tags, T_ROWS_PER_STRIP)[0]
    if len(offs) != C.DNG_STRIPS or len(offs) != len(cnts):
        raise DngError('strip count %d' % len(offs))
    strips = sorted(zip(offs, cnts))
    f.seek(0, 2); size = f.tell()
    span_lo = strips[0][0]
    span_hi = max(o + c for o, c in strips)
    if span_lo != 8 or span_hi > size:
        raise DngError('unexpected strip span %d..%d of %d' % (span_lo, span_hi, size))
    wl = _arr(tags, T_WHITELEVEL)[0]
    return {'tags': tags, 'wl_off': wl_off, 'whitelevel': wl, 'strips': strips, 'rows_per_strip': rps,
            'size': size, 'span_lo': span_lo, 'span_hi': span_hi}


def meta_hash(f, lay):
    """SHA-256 of every byte outside the strip payload span, with the 4 WhiteLevel value bytes zeroed.
    Identical for a passthrough frame and its correct conversion (only strips + WhiteLevel change)."""
    h = hashlib.sha256()
    f.seek(0)
    head = f.read(lay['span_lo'])          # the 8-byte TIFF header
    h.update(head)
    f.seek(lay['span_hi'])
    tail = bytearray(f.read())             # IFD + metadata + trailer (~30 KB)
    wl = lay['wl_off']
    if wl is not None and lay['span_hi'] <= wl <= lay['size'] - 4:
        i = wl - lay['span_hi']
        tail[i:i + 4] = b'\0\0\0\0'
    h.update(tail)
    return h.hexdigest()


def _drop_dontneed(fd, size):
    import os
    try:
        os.posix_fadvise(fd, 0, 0, os.POSIX_FADV_DONTNEED)
    except (AttributeError, OSError):
        pass


def _strip_rows(lay, idx):
    rps = lay['rows_per_strip']
    first = idx * rps
    return first, min(rps, C.DNG_H - first)


def _eye_color_sums(a, y, quad):
    """Add this decoded row (array('H'), full width) into per-(eye,colour) [sum,count].
    quad=True: F 4x4 layout (colour by (y>>1)&1,(x>>1)&1). quad=False: T 2x2 GBRG (y&1,x&1).
    Returns {(eye,colour): (sum,count)} where colour in G1,B,R,G2."""
    out = {}
    halves = ((1, a[0:EYE_SPLIT]), (0, a[EYE_SPLIT:]))     # eye1 left, eye0 right
    if quad:
        topband = ((y >> 1) & 1) == 0    # rows 0,1 of the 4x4 tile -> G,B ; else R,G
    else:
        topband = (y & 1) == 0           # GBRG: even row -> G,B ; odd -> R,G
    step = 4 if quad else 2
    for eye, e in halves:
        # columns whose block bit (x>>1)&1 (quad) or x&1 (2x2) == 0 are the "left" colour of the pair
        if quad:
            left = e[0::4]; left2 = e[1::4]; right = e[2::4]; right2 = e[3::4]
            s_left = sum(left) + sum(left2); n_left = len(left) + len(left2)
            s_right = sum(right) + sum(right2); n_right = len(right) + len(right2)
        else:
            le = e[0::2]; ri = e[1::2]
            s_left = sum(le); n_left = len(le); s_right = sum(ri); n_right = len(ri)
        if topband:
            cl, cr = 'G1', 'B'
        else:
            cl, cr = 'R', 'G2'
        out[(eye, cl)] = (s_left, n_left)
        out[(eye, cr)] = (s_right, n_right)
    return out


def _accumulate(acc, part):
    for k, (s, n) in part.items():
        a = acc.get(k)
        if a:
            acc[k] = (a[0] + s, a[1] + n)
        else:
            acc[k] = (s, n)


def _means(acc):
    return {k: (s / n if n else 0.0) for k, (s, n) in acc.items()}


def v0(path, want_whitelevel=C.WHITE_PASSTHROUGH):
    """Full medium read of a presumed passthrough original. Returns a dict with ok/reason, size, sha256,
    meta, means (per eye+colour), and max/low-pixel stats. Reads from the medium (fsync+fadvise first)."""
    import os
    fd = os.open(path, os.O_RDONLY)
    try:
        os.fsync(fd)
    except OSError:
        pass
    with os.fdopen(fd, 'rb', closefd=True) as f:
        lay = geometry(f)
        res = {'size': lay['size'], 'whitelevel': lay['whitelevel'], 'strips': len(lay['strips'])}
        if lay['whitelevel'] != want_whitelevel:
            return {'ok': False, 'reason': 'whitelevel %d != %d' % (lay['whitelevel'], want_whitelevel), **res}
        res['meta'] = meta_hash(f, lay)
        full = hashlib.sha256()
        acc = {}
        low_count = 0
        over = False
        f.seek(0)
        full.update(f.read(lay['span_lo']))
        gy = 0
        for i, (off, cnt) in enumerate(sorted((o, c) for o, c in zip(*zip(*lay['strips'])))) if False else enumerate(lay['strips']):
            off, cnt = lay['strips'][i]
            f.seek(off)
            comp = f.read(cnt)
            full.update(comp)
            if not (comp[:1] == b'\x78' and ((comp[0] << 8) | comp[1]) % 31 == 0 and (comp[2] >> 1) & 3 == 0):
                return {'ok': False, 'reason': 'strip %d not level-0 stored zlib' % i, **res}
            try:
                raw = zlib.decompress(comp)              # validates Adler-32
            except zlib.error as e:
                return {'ok': False, 'reason': 'strip %d inflate: %s' % (i, e), **res}
            rows = _strip_rows(lay, i)[1]
            if len(raw) != C.DNG_W * (C.DNG_BPS // 8) * rows:
                return {'ok': False, 'reason': 'strip %d length %d' % (i, len(raw)), **res}
            # fast "any sample > 1023" over the whole strip: high byte >= 4
            if bytes(raw[1::2]).translate(None, bytes(range(4))):
                over = True
            # sampled rows for means + low-pixel count
            first_row = i * lay['rows_per_strip']
            for r in range(rows):
                y = first_row + r
                if not _sampled(y):
                    continue
                rowbytes = raw[r * C.DNG_W * 2:(r + 1) * C.DNG_W * 2]
                a = array.array('H'); a.frombytes(rowbytes)
                _accumulate(acc, _eye_color_sums(a, y, quad=True))
                lows = a[1::2]            # high bytes; value<256 needs hi==0
                if 0 in lows:
                    for v in a:
                        if v < PEDESTAL_LOW:
                            low_count += 1
        # finalize full sha over the tail
        f.seek(lay['span_hi']); full.update(f.read())
        res['sha256'] = full.hexdigest()
        res['means'] = _means(acc)
        res['over_1023'] = over
        res['low_below_32_sampled'] = low_count
        if over:
            return {'ok': False, 'reason': 'passthrough has samples > 1023 (torn/converted strips)', **res}
        return {'ok': True, **res}


def v1(path, meta_old, size_old, means_old=None, check_means=False, want_sha=False):
    """Verify a converted T before it may replace the original in place.

    ALWAYS (cheap structural gate, ~0.5 s camera): geometry, size == size_old, WhiteLevel 16383,
    meta == meta_old, every strip is level-0 stored zlib that inflates with a valid Adler-32 to the exact
    length, no sample > 16383, not a constant frame. This is the floor that guards the destructive overwrite.

    check_means=True (optional, for engine acceptance / paranoid spot-checks, needs means_old from v0):
    also accumulate per-eye/colour means and bound each to (mean_F-64)*16383/959 within max(3%, 8 DN).
    Catches a *global* bad conversion (dark/phase-swapped). It does not catch a localised few-strip defect.

    want_sha=True also returns the full sha256 (only used where we need it; adds one 240 MB hash pass)."""
    import os
    fd = os.open(path, os.O_RDONLY)
    try:
        os.fsync(fd)
    except OSError:
        pass
    _drop_dontneed(fd, 0)
    with os.fdopen(fd, 'rb', closefd=True) as f:
        lay = geometry(f)
        res = {'size': lay['size'], 'whitelevel': lay['whitelevel'], 'strips': len(lay['strips'])}
        if lay['size'] != size_old:
            return {'ok': False, 'reason': 'size %d != %d' % (lay['size'], size_old), **res}
        if lay['whitelevel'] != C.WHITE_CONVERTED:
            return {'ok': False, 'reason': 'whitelevel %d != %d' % (lay['whitelevel'], C.WHITE_CONVERTED), **res}
        if meta_hash(f, lay) != meta_old:
            return {'ok': False, 'reason': 'metadata changed outside strips/WhiteLevel', **res}
        full = hashlib.sha256() if want_sha else None
        acc = {} if check_means else None
        constant = True
        first_val = None
        f.seek(0); head = f.read(lay['span_lo'])
        if full: full.update(head)
        for i, (off, cnt) in enumerate(lay['strips']):
            f.seek(off); comp = f.read(cnt)
            if full: full.update(comp)
            if not (comp[:1] == b'\x78' and ((comp[0] << 8) | comp[1]) % 31 == 0 and (comp[2] >> 1) & 3 == 0):
                return {'ok': False, 'reason': 'strip %d not level-0 stored zlib' % i, **res}
            try:
                raw = zlib.decompress(comp)
            except zlib.error as e:
                return {'ok': False, 'reason': 'strip %d inflate: %s' % (i, e), **res}
            rows = _strip_rows(lay, i)[1]
            if len(raw) != C.DNG_W * (C.DNG_BPS // 8) * rows:
                return {'ok': False, 'reason': 'strip %d length %d' % (i, len(raw)), **res}
            if bytes(raw[1::2]).translate(None, bytes(range(0x40))):   # any sample > 16383 -> hi byte >= 0x40
                return {'ok': False, 'reason': 'converted sample > 16383', **res}
            if constant or check_means:
                first_row = i * lay['rows_per_strip']
                for r in range(rows):
                    y = first_row + r
                    if not _sampled(y):
                        continue
                    a = array.array('H'); a.frombytes(raw[r * C.DNG_W * 2:(r + 1) * C.DNG_W * 2])
                    if constant:
                        if first_val is None and len(a):
                            first_val = a[0]
                        if any(v != first_val for v in a):
                            constant = False
                    if check_means:
                        _accumulate(acc, _eye_color_sums(a, y, quad=False))
        if full:
            f.seek(lay['span_hi']); full.update(f.read())
            res['sha256'] = full.hexdigest()
        if constant:
            return {'ok': False, 'reason': 'converted frame is constant', **res}
        if not check_means:
            return {'ok': True, **res}
        res['means'] = _means(acc)
        errs = {}
        worst = 0.0
        for k, mt in res['means'].items():
            mf = means_old.get(k) if means_old else None
            if mf is None:
                return {'ok': False, 'reason': 'missing original mean for %s' % (k,), **res}
            model = (mf - PEDESTAL) * C.WHITE_CONVERTED / (C.WHITE_PASSTHROUGH - PEDESTAL)
            tol = max(0.03 * abs(model), 8.0)
            err = mt - model
            errs['%s:%s' % k] = {'model': round(model, 2), 'measured': round(mt, 2), 'err': round(err, 2), 'tol': round(tol, 2)}
            if abs(err) > tol:
                worst = max(worst, abs(err) - tol)
        res['mean_errors'] = errs
        if worst > 0:
            return {'ok': False, 'reason': 'mean check failed', 'mean_check_failed': True, **res}
        return {'ok': True, **res}


def classify(path):
    """Cheap read-only classify for enumeration (geometry + WhiteLevel only, no full read)."""
    import os
    try:
        with open(path, 'rb') as f:
            lay = geometry(f)
    except (OSError, DngError, struct.error) as e:
        return ('bad', str(e))
    if lay['whitelevel'] == C.WHITE_PASSTHROUGH:
        return ('pending', lay)
    if lay['whitelevel'] == C.WHITE_CONVERTED:
        return ('done', lay)
    return ('other', lay['whitelevel'])


def inspect(path):
    """Cheap enumeration-time read: kind + whitelevel + meta + size, no strip/pixel read (a few KB).
    Used to group frames and decide pending/done without the ~0.5 s full read that v0 does."""
    kind, lay = classify(path)
    if kind == 'bad':
        return {'kind': 'bad', 'reason': lay}
    with open(path, 'rb') as f:
        meta = meta_hash(f, lay)
    return {'kind': kind, 'whitelevel': lay['whitelevel'], 'meta': meta, 'size': lay['size']}


def verify_fast(path, meta_old, size_old):
    """The cheap per-frame destructive-overwrite gate (no mean check). ~0.5 s on the camera. Use this as the
    documented perf fallback on a slow medium; the daemon default is v1(check_means=True) while that stays
    cheap enough (measured so on SSD)."""
    return v1(path, meta_old, size_old, check_means=False)
