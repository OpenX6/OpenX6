#!/usr/bin/env python3
"""Enumeration + grouping for the PureShot page (DESIGN-v3 §5.1).

Ported from x6_pureshot_batch.group_frames with the design's fixes: group by same session + time gap only
(no hard contiguous-seq rule) so a burst that crosses the Camera01->Camera02 DCF rollover stays one group,
and seed the gap limit from the whole run's median so a long interval series is not split at its first gap.
Pure functions take a pre-enumerated frame list and are unit-testable offline; enumerate_volume does the
actual per-file classify via dng.inspect (a few-KB tail read each, no full decode)."""
import hashlib, os, re
import dng
import contract as C

MIN_120MP_BYTES = C.MIN_DNG_BYTES
NAME_RE = re.compile(r'^IMG_(\d{8})_(\d{6})_(\d+)_(\d+)\.dng$', re.IGNORECASE)


def parse_name(name):
    m = NAME_RE.match(name)
    if not m:
        return None
    d, t, session, seq = m.groups()
    return {'date': d, 'time': t, 'session': int(session), 'seq': int(seq)}


def group_frames(frames):
    """frames: list of dict(name, rel, path, mtime, size, kind). Returns groups (dicts with members sorted
    by (mtime,seq), id, count, done, pending, failed, storage, span_s, bytes). A group is a maximal run of
    120 MP DNGs of the same session within the gap limit; folder boundaries do not split it."""
    fs = []
    for f in frames:
        if f.get('size', 0) < MIN_120MP_BYTES:
            continue
        p = parse_name(f['name'])
        if not p:
            continue
        p.update(f)
        fs.append(p)
    fs.sort(key=lambda x: (x['mtime'], x['seq']))
    # seed the gap limit from the median gap of the whole run (so the first gap isn't judged against 60 s)
    all_gaps = [fs[i]['mtime'] - fs[i - 1]['mtime'] for i in range(1, len(fs)) if fs[i]['mtime'] >= fs[i - 1]['mtime']]
    med = sorted(all_gaps)[len(all_gaps) // 2] if all_gaps else 0.0
    limit = max(med * 4.0, 30.0)
    groups, cur = [], []
    for fr in fs:
        if cur:
            prev = cur[-1]
            dt = fr['mtime'] - prev['mtime']
            if fr['session'] != prev['session'] or dt > limit:
                groups.append(cur); cur = []
        cur.append(fr)
    if cur:
        groups.append(cur)
    return [_summarize(g) for g in groups]


def _summarize(g):
    g.sort(key=lambda x: (x['mtime'], x['seq']))
    done = sum(1 for x in g if x.get('kind') == 'done')
    pending = sum(1 for x in g if x.get('kind') == 'pending')
    failed = sum(1 for x in g if x.get('kind') in ('bad', 'other'))
    storages = {x.get('storage', C.STO_INTERNAL) for x in g}
    storage = next(iter(storages)) if len(storages) == 1 else g[0].get('storage', C.STO_INTERNAL)
    return {'session': g[0]['session'], 'count': len(g), 'done': done, 'pending': pending, 'failed': failed,
            'first': g[0]['name'], 'last': g[-1]['name'], 'storage': storage,
            'span_s': round(g[-1]['mtime'] - g[0]['mtime'], 1),
            'bytes': sum(x['size'] for x in g), 'members': g}


def group_id(volkey, first_name):
    """Stable 4-byte id = first 4 bytes of sha256(volkey | first member name). Survives re-enumeration and
    rollover as long as the first member is unchanged; used to key GUI selection/clicks (GUI-F2)."""
    h = hashlib.sha256(('%s|%s' % (volkey, first_name)).encode('utf-8')).digest()
    return int.from_bytes(h[:4], 'little')


def enumerate_volume(volroot, volkey, storage, stem_of=lambda n: n[:-4], keep_scanning=None):
    """Scan DCIM/Camera* on a volume, classify each DNG (pending/done/other/bad) via a cheap tail read, and
    return a frame list ready for group_frames. Skips the work dir and dotfiles."""
    if keep_scanning and not keep_scanning():
        return None
    frames = []
    dcim = os.path.join(volroot, 'DCIM')
    try:
        folders = sorted(n for n in os.listdir(dcim) if n.lower().startswith('camera'))
    except OSError:
        return frames
    for folder in folders:
        fdir = os.path.join(dcim, folder)
        try:
            names = os.listdir(fdir)
        except OSError:
            continue
        for name in names:
            if name.startswith('.') or not name.lower().endswith('.dng'):
                continue
            if keep_scanning and not keep_scanning():
                return None
            path = os.path.join(fdir, name)
            try:
                st = os.lstat(path)
            except OSError:
                continue
            if not (st.st_size >= MIN_120MP_BYTES):
                continue
            kind, _ = dng.classify(path)
            frames.append({'name': name, 'rel': os.path.join('DCIM', folder, name), 'path': path,
                           'stem': stem_of(name), 'mtime': st.st_mtime, 'size': st.st_size,
                           'kind': kind, 'storage': storage})
    return frames
