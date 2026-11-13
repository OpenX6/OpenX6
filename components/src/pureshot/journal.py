#!/usr/bin/env python3
"""Journal records for the PureShot processor.

Two durable records per in-flight frame:
  J   -- the authoritative record on /data (survives card removal): journal/<volkey>__<stem>.json
  .x6j-- an on-volume mirror next to the work copy (survives moving the card to another unit): W/<stem>.x6j

A frame with any J on its volume blocks new work there (recovery must resolve it first). Means are stored in
the string form "<eye>/<colour>" for JSON and converted back to dng's (eye,colour) tuple keys on load."""
import os
import durable
import contract as C

JVERSION = 1


def _safe(s):
    return ''.join(c if (c.isalnum() or c in '-_.') else '_' for c in str(s))


def means_to_json(means):
    return {('%d/%s' % k): v for k, v in means.items()}


def means_from_json(d):
    out = {}
    for k, v in (d or {}).items():
        eye, col = k.split('/', 1)
        out[(int(eye), col)] = v
    return out


# ---- J on /data ----

def j_dir(data_root):
    return os.path.join(data_root, 'journal')


def j_path(data_root, volkey, stem):
    return os.path.join(j_dir(data_root), '%s__%s.json' % (_safe(volkey), _safe(stem)))


def write_j(data_root, rec):
    os.makedirs(j_dir(data_root), exist_ok=True)
    durable.write_json(j_path(data_root, rec['volkey'], rec['stem']), rec)


def read_j(data_root, volkey, stem):
    return durable.read_json(j_path(data_root, volkey, stem))


def delete_j(data_root, volkey, stem):
    p = j_path(data_root, volkey, stem)
    try:
        os.unlink(p)
    except FileNotFoundError:
        pass
    durable.fsync_dir(j_dir(data_root))


def iter_js(data_root, volkey=None):
    """Yield every live J record (skips the stuck/ subdir and corrupt files). Filter by volkey if given."""
    d = j_dir(data_root)
    try:
        names = os.listdir(d)
    except OSError:
        return
    for n in names:
        if not n.endswith('.json'):
            continue
        rec = durable.read_json(os.path.join(d, n))
        if not rec or 'volkey' not in rec or 'stem' not in rec:
            continue
        if volkey is None or rec['volkey'] == volkey:
            yield rec


def has_j(data_root, volkey):
    for _ in iter_js(data_root, volkey):
        return True
    return False


def stuck_dir(data_root):
    return os.path.join(j_dir(data_root), 'stuck')


def move_to_stuck(data_root, rec, reason):
    """Move J into journal/stuck/ with a reason. The frame is then excluded and never overwritten."""
    os.makedirs(stuck_dir(data_root), exist_ok=True)
    rec = dict(rec, state=C.J_STUCK, stuck_reason=reason)
    durable.write_json(os.path.join(stuck_dir(data_root), '%s__%s.json' % (_safe(rec['volkey']), _safe(rec['stem']))), rec)
    delete_j(data_root, rec['volkey'], rec['stem'])


def list_stuck(data_root):
    d = stuck_dir(data_root)
    out = []
    try:
        names = os.listdir(d)
    except OSError:
        return out
    for n in names:
        if n.endswith('.json'):
            rec = durable.read_json(os.path.join(d, n))
            if rec:
                out.append(rec)
    return out


# ---- .x6j on the volume ----

def work_dir(volroot):
    return os.path.join(volroot, C.WORKDIR)


def xj_path(volroot, stem):
    return os.path.join(work_dir(volroot), C.xj_name(stem))


def write_xj(volroot, rec):
    os.makedirs(work_dir(volroot), exist_ok=True)
    # sha_new/mtime_ns/atime_ns are carried so a card moved to another unit recovers a WRITING frame by the
    # exact verified hash (not a cheap structural guess) and restores the real capture time, not 1970.
    sub = {k: rec[k] for k in ('version', 'volkey', 'stem', 'rel', 'size', 'sha_old', 'meta_old',
                               'sha_new', 'mtime_ns', 'atime_ns',
                               'camera_id', 'daemon_version', 'state') if k in rec}
    durable.atomic_write(xj_path(volroot, rec['stem']),
                         __import__('json').dumps(sub, separators=(',', ':'), sort_keys=True))
    durable.fsync_dir(work_dir(volroot))


def read_xj(volroot, stem):
    return durable.read_json(xj_path(volroot, stem))


def delete_xj(volroot, stem):
    try:
        os.unlink(xj_path(volroot, stem))
    except FileNotFoundError:
        pass
    durable.fsync_dir(work_dir(volroot))


def iter_xj(volroot):
    d = work_dir(volroot)
    try:
        names = os.listdir(d)
    except OSError:
        return
    for n in names:
        if n.endswith('.x6j'):
            rec = durable.read_json(os.path.join(d, n))
            if rec and 'stem' in rec:
                yield rec
