#!/usr/bin/env python3
"""x6-pureshot-batch: owner-invoked batch reprocessing of 120 MP passthrough (Quad RAW) interval
groups through the vendor PureShot node, run on the camera when idle.

This is the ENTRY POINT + group selection + orchestration. The per-frame processing is delegated to
a pluggable backend (ps2native: runs the vendor PureShot engine on one DNG). With no backend, `run`
plans only and never claims a frame was processed.

Nothing here talks to the camera by itself except through injected callbacks (gate/backend/lister),
so the grouping and selection logic is unit-testable offline. CLI usage:
  x6_pureshot_batch.py groups  [--root DIR] [--list-file F]
  x6_pureshot_batch.py plan    GROUPSPEC [--root DIR]
  x6_pureshot_batch.py run     GROUPSPEC [--root DIR] [--backend CMD] [--out DIR] [--dry-run]
GROUPSPEC: "all" | "1" | "1,3" | "2-4" (1-based group indices from `groups`).
Only passthrough DNGs (WhiteLevel 1023) are listed. The backend defaults to ps2native beside this
script and is run as `CMD IN.dng OUT.dng`; outputs default to <storage>/PureShot/<name>_processed.dng."""
import json, os, re, subprocess, sys, time

MIN_120MP_BYTES = 100_000_000          # a 120 MP level-0 Quad DNG is ~241 MB; filters out thumbnails
NAME_RE = re.compile(r'^IMG_(\d{8})_(\d{6})_(\d+)_(\d+)\.dng$', re.IGNORECASE)

# ---------- enumeration + grouping (pure, testable) ----------
def parse_name(name):
    m = NAME_RE.match(name)
    if not m: return None
    d, t, session, seq = m.groups()
    return dict(name=name, date=d, time=t, session=int(session), seq=int(seq))

def group_frames(files):
    """files: list of dict(name, mtime, size[, path]). Returns list of groups (each a dict with
    members sorted by seq, plus count/span/bytes). A group is a maximal run of 120 MP DNGs with the
    same session field and contiguous-ish sequence, not split by a large time gap."""
    frames = []
    for f in files:
        if f.get('size', 0) < MIN_120MP_BYTES: continue
        p = parse_name(f['name'])
        if not p: continue
        p.update(mtime=float(f.get('mtime', 0)), size=int(f['size']), path=f.get('path', f['name']))
        frames.append(p)
    frames.sort(key=lambda x: (x['mtime'], x['seq']))
    groups, cur = [], []
    gaps = []
    def gap_limit():
        if not gaps: return 60.0
        med = sorted(gaps)[len(gaps)//2]
        return max(med * 3.0, 30.0)
    for fr in frames:
        if cur:
            prev = cur[-1]
            dt = fr['mtime'] - prev['mtime']
            contiguous = 0 <= (fr['seq'] - prev['seq']) <= 3
            same = fr['session'] == prev['session']
            if not (same and contiguous and dt <= gap_limit()):
                groups.append(cur); cur = []; gaps = []
            else:
                gaps.append(dt)
        cur.append(fr)
    if cur: groups.append(cur)
    out = []
    for g in groups:
        g.sort(key=lambda x: x['seq'])
        span = g[-1]['mtime'] - g[0]['mtime']
        out.append(dict(session=g[0]['session'], count=len(g), first=g[0]['name'], last=g[-1]['name'],
                        span_s=round(span, 1), bytes=sum(x['size'] for x in g), members=g))
    return out

def select_groups(groups, spec):
    """spec: 'all' | comma/range of 1-based indices. Returns the chosen subset (order preserved)."""
    spec = (spec or '').strip().lower()
    if spec in ('all', '*'):
        return list(range(len(groups)))
    idx = set()
    for part in spec.split(','):
        part = part.strip()
        if not part: continue
        if '-' in part:
            a, b = part.split('-', 1); a, b = int(a), int(b)
            for i in range(min(a, b), max(a, b) + 1): idx.add(i)
        else:
            idx.add(int(part))
    chosen = sorted(i - 1 for i in idx if 1 <= i <= len(groups))
    if not chosen: raise ValueError('no valid group in spec: %r (have 1..%d)' % (spec, len(groups)))
    return chosen

def out_name(name):
    return name[:-4] + '_processed.dng' if name.lower().endswith('.dng') else name + '_processed.dng'

# ---------- on-camera glue (side-effecting; injected in tests) ----------
DEFAULT_ROOTS = ('/mnt/quickreader/DCIM/Camera01', '/mnt/sdcard/DCIM/Camera01', '/ins/media_rw/emulated/DCIM/Camera01')

def ssh_list(root):
    """Passthrough 120 MP DNGs under root (or under every known storage root when root is None)."""
    out = []
    for r in ([root] if root else DEFAULT_ROOTS):
        try: names = os.listdir(r)
        except OSError: continue
        for n in names:
            if not n.lower().endswith('.dng') or n.lower().endswith('_processed.dng'): continue
            p = os.path.join(r, n)
            try: st = os.stat(p)
            except OSError: continue
            if st.st_size < MIN_120MP_BYTES or not is_passthrough(p): continue
            out.append(dict(name=n, mtime=st.st_mtime, size=st.st_size, path=p))
    return out

USB_LUN_GLOB = '/sys/kernel/config/usb_gadget/*/functions/mass_storage.*/lun.*/file'

def storage_exported():
    """True while any USB mass-storage LUN is backed by a device (the host owns that filesystem)."""
    import glob
    for p in glob.glob(USB_LUN_GLOB):
        try:
            if open(p).read().strip(): return True
        except OSError:
            return True
    return False

def on_media_fs(path, mounts='/proc/mounts'):
    """True if path lives on a mounted, writable card/internal-storage filesystem (never rootfs/overlay)."""
    path = os.path.abspath(path); best = None
    try:
        for line in open(mounts):
            dev, mnt, fstype, opts = line.split()[:4]
            mnt = mnt.replace('\\040', ' ')
            if (path == mnt or path.startswith(mnt.rstrip('/') + '/')) and (best is None or len(mnt) > len(best[0])):
                best = (mnt, fstype, opts.split(','))
    except OSError:
        return False
    return bool(best) and best[0] != '/' and best[1] in ('exfat', 'vfat', 'ext4', 'f2fs') and 'rw' in best[2]

def camera_idle():
    """True only if the media service reports CaptureIdle and RecordIdle and no storage is exported
    over USB. Bounded, never hangs; any error or timeout counts as busy (the run then stops cleanly)."""
    if storage_exported(): return False
    try:
        r = subprocess.run(['/usr/bin/timeout', '-k', '1', '25', '/usr/bin/ins_bll_cli',
                            '{"cmd":"share_state","action":"get","type":"media"}'],
                           capture_output=True, text=True, timeout=30)
        s = re.sub(r'\s+', '', r.stdout)
    except Exception:
        return False
    return '"capture_status":"CaptureIdle"' in s and '"record_status":"RecordIdle"' in s

def dng_white_level(path):
    """WhiteLevel of a little-endian TIFF/DNG IFD0 (the IFD sits at the end of X6 files), or None."""
    import struct
    try:
        with open(path, 'rb') as f:
            hdr = f.read(8)
            if hdr[:4] != b'II*\x00': return None
            f.seek(struct.unpack_from('<I', hdr, 4)[0]); n = struct.unpack('<H', f.read(2))[0]
            ent = f.read(12 * n)
        for k in range(n):
            tag, typ, cnt, val = struct.unpack_from('<HHII', ent, 12 * k)
            if tag == 50717: return val & 0xFFFF if typ == 3 else val
    except (OSError, struct.error):
        return None
    return None

def is_passthrough(path):
    return dng_white_level(path) == 1023        # native 120 MP DNGs carry 16383

OUT_SUBDIR = 'PureShot'          # outputs go beside DCIM, never into the camera's DCF folders
SPACE_MARGIN = 512 * 1024 * 1024

def default_outdir(in_path):
    """<storage>/PureShot for <storage>/DCIM/<folder>/<file>; <dir>/PureShot otherwise."""
    parts = os.path.abspath(in_path).split(os.sep)
    if 'DCIM' in parts[:-1]:
        i = len(parts) - 1 - parts[::-1].index('DCIM')
        return os.sep.join(parts[:i] + [OUT_SUBDIR]) or os.sep
    return os.path.join(os.path.dirname(os.path.abspath(in_path)), OUT_SUBDIR)

def free_bytes(path):
    st = os.statvfs(path)
    return st.f_bavail * st.f_frsize

# ---------- run loop ----------
def run(groups, chosen, backend, outdir, gate=camera_idle, dry=False, log=print, sleep=time.sleep,
        space=free_bytes, mkdir=lambda d: os.makedirs(d, exist_ok=True), exists=os.path.exists,
        media=on_media_fs):
    """Process the chosen groups frame by frame. backend: callable(in_path,out_path)->(ok,msg) or None.
    outdir None means default_outdir(input). Honours the idle gate before every frame and stops cleanly
    if it goes busy or the output filesystem runs short. Never modifies or deletes input."""
    plan = [fr for gi in chosen for fr in groups[gi]['members']]
    log('plan: %d group(s), %d frame(s)' % (len(chosen), len(plan)))
    dest = lambda fr: os.path.join(outdir or default_outdir(fr['path']), out_name(fr['name']))
    if dry or backend is None:
        for fr in plan: log('  would process %s -> %s' % (fr['path'], dest(fr)))
        if backend is None and not dry:
            log('NO BACKEND: planning only, nothing processed')
        return dict(planned=len(plan), processed=0, skipped=len(plan))
    done = skipped = failed = 0
    for n, fr in enumerate(plan):
        if not gate():
            log('camera busy; stopping cleanly (processed=%d, remaining=%d)' % (done, len(plan) - n))
            break
        op = dest(fr)
        if exists(op): log('  skip (exists) %s' % op); skipped += 1; continue
        if not media(os.path.dirname(op)):
            log('output %s is not on a mounted, writable card/storage filesystem; stopping' % os.path.dirname(op))
            break
        mkdir(os.path.dirname(op))
        if space(os.path.dirname(op)) < fr['size'] + SPACE_MARGIN:
            log('output filesystem nearly full; stopping cleanly (processed=%d, remaining=%d)' % (done, len(plan) - n))
            break
        t0 = time.time()
        ok, msg = backend(fr['path'], op)
        if ok: done += 1; log('  ok %s (%.1fs)' % (op, time.time() - t0))
        else: failed += 1; log('  FAIL %s: %s' % (fr['name'], msg))
        sleep(0)
    return dict(planned=len(plan), processed=done, skipped=skipped, failed=failed)

# ---------- CLI ----------
def main(argv):
    if not argv: print(__doc__); return 2
    cmd = argv[0]
    def opt(flag, dflt=None):
        return argv[argv.index(flag) + 1] if flag in argv else dflt
    root = opt('--root')
    lf = opt('--list-file')
    files = json.load(open(lf)) if lf else ssh_list(root)
    groups = group_frames(files)
    if cmd == 'groups':
        for i, g in enumerate(groups, 1):
            print('[%2d] session=%d  %d frames  %.0fs  %.2f GB  %s..%s'
                  % (i, g['session'], g['count'], g['span_s'], g['bytes']/1e9,
                     g['first'].split('_')[-1][:-4], g['last'].split('_')[-1][:-4]))
        if not groups: print('(no 120 MP passthrough groups found under %s)' % (root or ', '.join(DEFAULT_ROOTS)))
        return 0
    spec = argv[1] if len(argv) > 1 else 'all'
    chosen = select_groups(groups, spec)
    if cmd == 'plan':
        run(groups, chosen, None, '', dry=True); return 0
    if cmd == 'run':
        outdir = opt('--out')
        here = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'ps2native')
        backcmd = opt('--backend', here if os.access(here, os.X_OK) else None)
        dry = '--dry-run' in argv
        backend = None
        if backcmd and not dry:
            def backend(ip, op, _c=backcmd):
                try:
                    r = subprocess.run([_c, ip, op], capture_output=True, text=True, timeout=600)
                except subprocess.TimeoutExpired:
                    return (False, 'backend timed out after 600 s')
                return (r.returncode == 0, (r.stderr or r.stdout).strip()[-300:])
        res = run(groups, chosen, backend, outdir, dry=dry)
        print(json.dumps(res)); return 0
    print('unknown command: %s' % cmd); return 2

if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
