#!/usr/bin/env python3
"""Per-frame protocol (DESIGN-v3 §5.3) and recovery (§5.4) for the PureShot processor.

This module holds all the irreversible-overwrite logic and nothing else, so it can be driven by the offline
fault-injection harness with a stub converter and crashes forced at every labelled point. The invariant it
must preserve across any crash/power-loss is: F on the medium is always either the untouched original
(sha_old) or the fully verified conversion (sha_new); if neither can be reached, the frame is STUCK with T
and the journal kept and nothing deleted. It is never left corrupt, truncated, or half-written with no redo
source.

Injection points (ctx.crash_at, a set of labels) call os._exit(137) to simulate an uncatchable SIGKILL --
no finally runs, exactly like a power cut. Production leaves ctx.crash_at empty."""
import os, time, errno, struct, stat as statmod
import durable, journal
import dng
import contract as C

# process_frame / recovery outcomes
R_DONE, R_PENDING, R_EXCLUDED, R_STUCK, R_PAUSED, R_BLOCKED, R_ERRSTOP, R_KEEP, R_NOFRAME, R_RETRY = (
    'done', 'pending', 'excluded', 'stuck', 'paused', 'blocked', 'error-stop', 'keep', 'no-frame', 'retry')

CHUNK = 8 << 20            # 8 MiB in-place overwrite chunk (DESIGN §5.3 step 8)
MEM_MIN = 2300 * 1024 * 1024
V1_RETRIES = 1             # one retry on a non-systematic V1 failure
# Recovery returns R_KEEP (retry next pass) whenever a medium read is indeterminate; there is deliberately no
# attempt cap. A persistently unreadable frame on a PRESENT card keeps its WRITING J (nothing destroyed) and
# precheck_recover then refuses the layer change -- the owner's escape is to remove that failing card and
# redeploy (an ABSENT volume's journals are waived by --ack-lost). Escalating KEEP->STUCK would not help,
# since precheck_recover refuses on STUCK too; only removing the card unblocks, and that is the safe design.


class AbortConvert(Exception):
    pass


class Ctx(object):
    def __init__(self, data_root, converter, gates, camera_id='x6', daemon_version='pureshot-v1',
                 crash_at=(), log=None, on_start=None, on_tick=None):
        self.on_tick = on_tick              # called on every converter poll (~4 Hz): display refresh only
        self.data_root = data_root
        self.converter = converter          # convert(F_abs, T_abs, poll) -> int ; poll() truthy => abort
        self.gates = gates                  # see gates.StubGates / gates.Gates
        self.camera_id = camera_id
        self.daemon_version = daemon_version
        self.crash_at = set(crash_at)
        self.log = log or (lambda *a: None)
        self.on_start = on_start            # called once a frame has passed its gates (status "Processing")
        self.pause_reason = ''              # the REAL gate decision behind an R_PAUSED (shown to the owner)
        self.inflight = None                # pipelined daemon: (volkey, stem) of its own frames in flight (live J's)

    def started(self):
        if self.on_start:
            try:
                self.on_start()             # display only: must never affect the frame
            except Exception:
                pass

    def ticked(self):
        """Converter-poll hook: refreshes the display (live frame %, time left) and returns an abort reason only for a
        stop the owner asked for. Never raises; aborting a conversion is always safe (F is still the original)."""
        if self.on_tick:
            try:
                return self.on_tick()
            except Exception:
                pass
        return None

    def crash(self, label):
        if label in self.crash_at:
            os._exit(137)


def _T(volroot, stem):
    return os.path.join(journal.work_dir(volroot), C.tmp_name(stem))


def _Ttmp(volroot, stem):
    return os.path.join(journal.work_dir(volroot), C.tmp_tmp(stem))


def _done_log(data_root):
    return os.path.join(data_root, 'done.log')


def append_done(data_root, rec, sha_new):
    line = '%s\t%s\t%d\t%s\t%.3f\n' % (rec['volkey'], rec['rel'], rec['size'], sha_new, time.time())
    fd = os.open(_done_log(data_root), os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    try:
        os.write(fd, line.encode('utf-8'))
        os.fsync(fd)
    finally:
        os.close(fd)


def done_set(data_root):
    """Every (volkey, rel) this daemon has converted, read once (the group list filter uses it per rescan)."""
    out = set()
    try:
        with open(_done_log(data_root), 'r') as f:
            for ln in f:
                p = ln.rstrip('\n').split('\t')
                if len(p) >= 2:
                    out.add((p[0], p[1]))
    except OSError:
        pass
    return out


def done_has(data_root, volkey, rel):
    try:
        with open(_done_log(data_root), 'r') as f:
            for ln in f:
                p = ln.rstrip('\n').split('\t')
                if len(p) >= 2 and p[0] == volkey and p[1] == rel:
                    return True
    except OSError:
        pass
    return False


# ---------------------------------------------------------------- overwrite + finish (shared) ----

def do_overwrite(ctx, F, T, size, sha_new, mtime_ns, atime_ns):
    """Critical section (step 8) / recovery redo. In-place pwrite T->F over the existing clusters, fsync,
    read back from the medium, restore mtime. Returns True on success. On any I/O failure returns False --
    the caller keeps J=WRITING, keeps T, blocks the volume: never the abort path that deletes T."""
    try:
        sfd = os.open(T, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC)
    except OSError:
        return False
    try:
        try:
            dfd = os.open(F, os.O_WRONLY | os.O_NOFOLLOW | os.O_CLOEXEC)   # no create, no truncate
        except OSError:
            return False
        try:
            st = os.fstat(dfd)
            if st.st_size != size:
                return False
            off = 0
            first = True
            while off < size:
                want = min(CHUNK, size - off)
                b = os.pread(sfd, want, off)
                if len(b) != want:
                    return False
                n = os.pwrite(dfd, b, off)
                if n != len(b):
                    return False
                off += n
                if first:
                    ctx.crash('pwrite_partial')      # torn F: first chunk new, rest old
                    first = False
            ctx.crash('after_pwrite')
            os.fsync(dfd)
            ctx.crash('after_fsync_f')
        finally:
            os.close(dfd)
    finally:
        os.close(sfd)
    # read back from the medium
    try:
        if durable.sha256_medium(F) != sha_new:
            return False
    except OSError:
        return False
    ctx.crash('after_readback')
    try:
        os.utime(F, ns=(atime_ns, mtime_ns))
        durable.fsync_dir(os.path.dirname(F))
    except OSError:
        return False
    ctx.crash('after_utime')
    return True


def finish_frame(ctx, volroot, rec, sha_new):
    """Step 9: unlink T + .x6j inside W, fsync W, append done.log, remove J. Idempotent."""
    stem, volkey = rec['stem'], rec['volkey']
    for p in (_T(volroot, stem), _Ttmp(volroot, stem)):
        try:
            os.unlink(p)
        except FileNotFoundError:
            pass
    journal.delete_xj(volroot, stem)
    durable.fsync_dir(journal.work_dir(volroot))
    if not done_has(ctx.data_root, volkey, rec['rel']):
        append_done(ctx.data_root, rec, sha_new)
    journal.delete_j(ctx.data_root, volkey, stem)
    return R_DONE


# ---------------------------------------------------------------- forward per-frame ----
#
# The protocol runs in four stages so the daemon can overlap frames (2026-11-04: per frame the AI step is ~4-5 s of
# accelerator time, while V0/V1/overwrite are ~3 s of the daemon's own CPU+I/O that used to wait in line):
#   prepare  gates, lstat, V0 (F only READ; nothing written)            -- may run while another frame converts
#   begin    launch gates on a fresh idle answer, F unchanged since V0, J + .x6j durable BEFORE the converter opens F
#   converted  the converter's outcome (abort / exit code / done)
#   finish   V1, J VERIFIED, critical-section gates, in-place overwrite, done.log  -- may run while the NEXT converts
# process_frame() runs them in order with a blocking converter (the fault harness and any sequential caller).
# Overlap never changes a frame's own protocol: two frames in flight are different files with different J/.x6j/T,
# so recovery resolves each record on its own (CONVERTING -> drop, WRITING -> the rows of _recover_writing).

class Prep(object):
    """A frame checked and read by V0. `rec` is its journal record; nothing is durable until begin()."""
    __slots__ = ('frame', 'rec', 'ino', 'attempt')

    def __init__(self, frame, rec, ino, attempt=0):
        self.frame, self.rec, self.ino, self.attempt = frame, rec, ino, attempt


def prepare(ctx, frame, attempt=0):
    """Steps 1-2. Returns (R_*, None) to stop here, or (None, Prep). Writes nothing; F is only read."""
    volroot, volkey, stem, F = frame['volroot'], frame['volkey'], frame['stem'], frame['path']
    ok, why = ctx.gates.ok_to_run(volroot)
    if not ok:
        ctx.pause_reason = why
        return (R_PAUSED if why != 'blocked' else R_BLOCKED), None
    if not ctx.gates.space_ok(volroot, 0):
        ctx.pause_reason = 'low space'
        return R_PAUSED, None
    if ctx.inflight is not None:
        # DESIGN §5.4: no new frame on a volume with an unresolved J. With frames overlapped the daemon's OWN
        # in-flight frames have live J's by design; anything else (a record recovery could not resolve yet) blocks.
        for j in journal.iter_js(ctx.data_root, volkey):
            if (volkey, j.get('stem')) not in ctx.inflight:
                ctx.pause_reason = 'recovering'
                return R_PAUSED, None
        if (volkey, stem) in ctx.inflight:
            return R_PENDING, None                          # never two frames under one stem (one J/T name)

    # step 2: lstat + V0(F) from the medium
    try:
        st = os.lstat(F)
    except OSError:
        return R_PENDING, None
    if not (statmod.S_ISREG(st.st_mode) and st.st_nlink == 1):
        return R_EXCLUDED, None
    if not ctx.gates.space_ok(volroot, 2 * st.st_size):
        ctx.pause_reason = 'low space'                     # this frame's T plus the previous frame's T, overlapped
        return R_PAUSED, None
    try:
        r0 = dng.v0(F)
    except OSError:
        return R_PENDING, None                             # transient medium read error; F untouched -> retry
    except (dng.DngError, struct.error):
        ctx.log('exclude', '%s: malformed original (v0 raised)' % stem)
        return R_EXCLUDED, None
    if not r0.get('ok'):
        ctx.log('exclude', '%s: %s' % (stem, r0.get('reason')))
        return R_EXCLUDED, None
    rec = {'version': journal.JVERSION, 'volkey': volkey, 'stem': stem, 'rel': frame['rel'],
           'size': st.st_size, 'mtime_ns': st.st_mtime_ns, 'atime_ns': st.st_atime_ns, 'sha_old': r0['sha256'],
           'meta_old': r0['meta'], 'means': journal.means_to_json(r0['means']), 'camera_id': ctx.camera_id,
           'daemon_version': ctx.daemon_version, 'state': C.J_CONVERTING}
    return None, Prep(frame, rec, st.st_ino, attempt)


def begin(ctx, prep):
    """Launch decision + step 3. Returns (R_*, None) or (None, prep) with J/.x6j durable.

    V0 read the whole ~240 MB original (0.3-2.5 s) with no idle probe of its own, so the launch is decided on a
    FRESH idle answer (test 6.3: a stale one made the converter's first abort polls kill it at once), then on all
    the gates again (a USB host may have attached). F must still be exactly what V0 read: with frames overlapped
    V0 may have run seconds earlier. All of this happens before J, so a refusal leaves nothing to undo."""
    fr, rec = prep.frame, prep.rec
    volroot, volkey, stem, F = fr['volroot'], fr['volkey'], fr['stem'], fr['path']
    if not ctx.gates.idle_fresh(volroot):
        ctx.pause_reason = 'camera busy'
        return R_PAUSED, None
    ok, why = ctx.gates.ok_to_run(volroot)          # the idle cache is fresh now, so this adds no second probe
    if not ok:
        ctx.pause_reason = why
        return (R_PAUSED if why != 'blocked' else R_BLOCKED), None
    try:
        st = os.lstat(F)
    except OSError:
        return R_PENDING, None
    if st.st_size != rec['size'] or st.st_mtime_ns != rec['mtime_ns'] or st.st_ino != prep.ino:
        return R_PENDING, None                             # owner changed F after V0: read it again later

    ctx.started()                                          # gates passed: the frame really starts now

    # step 3: durable J + .x6j BEFORE F is ever opened by the converter
    T, Ttmp = _T(volroot, stem), _Ttmp(volroot, stem)
    os.makedirs(journal.work_dir(volroot), exist_ok=True)
    for p in (T, Ttmp):
        try:
            os.unlink(p)
        except FileNotFoundError:
            pass
    try:
        journal.write_xj(volroot, rec)
        journal.write_j(ctx.data_root, rec)
    except OSError as e:
        ctx.log('error', 'journal write failed: %s' % e)
        return R_ERRSTOP, None
    ctx.crash('after_j_converting')
    return None, prep


def converted(ctx, prep, rc):
    """Step 4/5 outcome. Abort or a failing exit code drops T/.x6j/J (F was only read). Returns (R_*, None) or
    (None, prep) with T in place."""
    if rc == 'abort' or rc != 0:
        _drop(ctx, prep.frame['volroot'], prep.rec)
        if rc == 'abort':
            return R_PAUSED, None
        # exit-code map (DESIGN §5.3 / §11). 3 after V0 passed => engine unavailable => stop job.
        if rc in (9, 20, 22, 25, 26, 27, 28):
            return R_EXCLUDED, None
        if rc == 3:
            return R_ERRSTOP, None
        return R_EXCLUDED, None
    ctx.crash('after_convert')
    return None, prep


def finish(ctx, prep):
    """Steps 6-9 for a converted frame. Returns R_DONE, or why not. R_RETRY = a non-systematic V1 failure on the
    first attempt: T/J are dropped and the caller converts the frame once more (a second failure is STUCK)."""
    fr, rec = prep.frame, prep.rec
    volroot, volkey, stem, F = fr['volroot'], fr['volkey'], fr['stem'], fr['path']
    T = _T(volroot, stem)
    size, mtime_ns, atime_ns = rec['size'], rec['mtime_ns'], rec['atime_ns']

    # step 6: V1(T) full, with mean check. A raise here must never crash the daemon: a transient read
    # error means F is still the untouched original (nothing written) -> drop and retry; a structurally
    # broken T (ps2native returned 0 but produced garbage) routes into the V1-fail retry/STUCK path.
    try:
        r1 = dng.v1(T, rec['meta_old'], size, journal.means_from_json(rec['means']), check_means=True, want_sha=True)
    except OSError:
        _drop(ctx, volroot, rec)
        return R_PENDING
    except (dng.DngError, struct.error) as e:
        r1 = {'ok': False, 'reason': 'v1 raised: %s' % e}
    if not r1.get('ok'):
        if r1.get('mean_check_failed'):
            ctx.log('error', '%s: converter mean check failed' % stem)
            return R_ERRSTOP                                   # systematic: stop, don't destroy more frames
        ctx.log('warn', '%s: V1 failed (%s), attempt %d' % (stem, r1.get('reason'), prep.attempt + 1))
        if prep.attempt >= V1_RETRIES:
            journal.move_to_stuck(ctx.data_root, rec, 'V1 failed: %s' % r1.get('reason'))
            _quarantine(volroot, stem)                         # keep T in quarantine
            return R_STUCK
        _drop(ctx, volroot, rec)
        return R_RETRY
    sha_new = r1['sha256']

    rec['state'] = C.J_VERIFIED
    rec['sha_new'] = sha_new
    journal.write_j(ctx.data_root, rec)
    ctx.crash('after_verified')

    # step 7: critical-section gates (idle; no USB host about to export internal/SD) + F unchanged (same size,
    # mtime, ino on this mount). A refusal drops the conversion -- exactly what recovery does for VERIFIED -- so no
    # stray J is left beside the daemon's in-flight frames.
    if not ctx.gates.idle_fresh(volroot):
        ctx.pause_reason = 'camera busy'
        _drop(ctx, volroot, rec)
        return R_PAUSED
    usb_blocks = getattr(ctx.gates, 'usb_blocks', None)
    if usb_blocks is not None and usb_blocks(volroot):
        ctx.pause_reason = 'USB connected'
        _drop(ctx, volroot, rec)
        return R_PAUSED
    try:
        st2 = os.lstat(F)
    except OSError:
        _drop(ctx, volroot, rec); return R_PENDING
    if st2.st_size != size or st2.st_mtime_ns != mtime_ns or st2.st_ino != prep.ino:
        _drop(ctx, volroot, rec)                              # owner changed/replaced F
        return R_PENDING

    # step 8: critical section
    rec['state'] = C.J_WRITING
    journal.write_j(ctx.data_root, rec)
    journal.write_xj(volroot, rec)
    ctx.crash('after_j_writing')
    if not do_overwrite(ctx, F, T, size, sha_new, mtime_ns, atime_ns):
        ctx.gates.block_volume(volroot)
        ctx.log('error', '%s: overwrite failed, volume blocked' % stem)
        return R_BLOCKED
    rec['state'] = C.J_WRITTEN
    journal.write_j(ctx.data_root, rec)
    ctx.crash('after_j_written')

    # step 9
    finish_frame(ctx, volroot, rec, sha_new)
    ctx.crash('after_donelog')
    return R_DONE


def run_blocking(ctx, prep):
    """Step 4 with a blocking converter(F, T, poll): poll() returns the first abort reason (gates, owner stop)."""
    volroot = prep.frame['volroot']
    def poll():
        reason = ctx.gates.abort_reason(volroot) or ctx.ticked()
        if reason:
            ctx.pause_reason = reason                  # why the conversion was aborted
        return reason
    try:
        return ctx.converter(prep.frame['path'], _T(volroot, prep.frame['stem']), poll)
    except AbortConvert:
        return 'abort'


def process_frame(ctx, frame):
    """frame: dict(volkey, volroot, rel, stem, path). Returns an R_* outcome (never R_RETRY: the one V1 retry
    happens here). Preconditions: gates hold and there is no pending J for this frame (caller runs recovery
    first, DESIGN §5.4)."""
    r, prep = prepare(ctx, frame)
    if r is not None:
        return r
    while True:
        r, _ = begin(ctx, prep)
        if r is not None:
            return r
        r, _ = converted(ctx, prep, run_blocking(ctx, prep))
        if r is not None:
            return r
        r = finish(ctx, prep)
        if r != R_RETRY:
            return r
        prep.attempt += 1


def _drop(ctx, volroot, rec):
    for p in (_T(volroot, rec['stem']), _Ttmp(volroot, rec['stem'])):
        try:
            os.unlink(p)
        except FileNotFoundError:
            pass
    journal.delete_xj(volroot, rec['stem'])
    journal.delete_j(ctx.data_root, rec['volkey'], rec['stem'])


def _quarantine(volroot, stem):
    q = os.path.join(volroot, C.QUARANTINE)
    os.makedirs(q, exist_ok=True)
    for name in (C.tmp_name(stem), C.tmp_tmp(stem)):
        src = os.path.join(journal.work_dir(volroot), name)
        if os.path.exists(src):
            try:
                os.replace(src, os.path.join(q, name))
            except OSError:
                pass
    durable.fsync_dir(q)


# ---------------------------------------------------------------- recovery (§5.4) ----

def _medium_sha(path):
    try:
        return durable.sha256_medium(path)
    except OSError:
        return None


def _is_original(F, size, meta_old):
    """F is the untouched passthrough original? (regular, size S, WhiteLevel 1023, meta==meta_old)."""
    try:
        with open(F, 'rb') as f:
            lay = dng.geometry(f)
            if lay['size'] != size or lay['whitelevel'] != C.WHITE_PASSTHROUGH:
                return False
            return dng.meta_hash(f, lay) == meta_old
    except Exception:
        return None                 # unreadable/undecidable -> no decision


def recover_frame(ctx, volroot, rec):
    """Resolve one J record per the §5.4 table (top-to-bottom, first match). Medium reads only; any OSError
    => R_KEEP (retry later). Returns an R_* outcome."""
    stem, volkey, size = rec['stem'], rec['volkey'], rec['size']
    F = os.path.join(volroot, rec['rel'])
    T, Ttmp = _T(volroot, stem), _Ttmp(volroot, stem)
    meta_old = rec['meta_old']
    sha_new = rec.get('sha_new')
    state = rec.get('state')

    if state in (C.J_CONVERTING, C.J_VERIFIED):
        # F should be the original; nothing was written. Clean the work copy, frame pending again.
        _drop(ctx, volroot, rec)
        return R_PENDING

    if state == C.J_WRITTEN:
        return finish_frame(ctx, volroot, rec, sha_new)

    if state == C.J_WRITING:
        return _recover_writing(ctx, volroot, rec, F, T, Ttmp, size, meta_old, sha_new)

    # unknown state -> treat conservatively
    return R_KEEP


def _recover_writing(ctx, volroot, rec, F, T, Ttmp, size, meta_old, sha_new):
    stem, volkey = rec['stem'], rec['volkey']
    sha_old = rec.get('sha_old')
    t_exists = os.path.exists(T)
    ttmp_exists = os.path.exists(Ttmp)
    t_unreadable = False   # a PRESENT T/Ttmp we could not hash right now (maybe transient): do not decide
                           # from it, but prefer KEEP over STUCK if F also turns out indeterminate.

    # row 1: redo step 8 from a T whose medium SHA == the recorded verified hash (sha_new). We ONLY redo the
    # destructive overwrite from a hash-confirmed T; a T that merely passed the cheap structural gate is never
    # a redo source -- verify_fast cannot catch a globally wrong conversion (dark/phase-swapped) and the redo
    # overwrites the only original. A read error on T is "unknown": it must NOT short-circuit recovery (an F
    # that already holds the verified conversion must still be finished by row 2) and must NOT be read as
    # "T invalid" (durable.sha256_medium's recovery contract). sha_new now travels in the WRITING .x6j, so a
    # card moved to another unit still takes this exact-hash path; a truly foreign/old orphan without sha_new
    # falls through to row 3 (reprocess from the original) or row 4 (STUCK), never overwritten from an
    # unverified T.
    if sha_new is not None:
        try:
            if t_exists:
                tsha = _medium_sha(T)
                if tsha is None:
                    t_unreadable = True                    # fall through to the F-based rows
                elif tsha == sha_new:
                    return _redo_from_T(ctx, volroot, rec, F, T, size, sha_new)
            elif ttmp_exists:
                ttsha = _medium_sha(Ttmp)
                if ttsha is None:
                    t_unreadable = True
                elif ttsha == sha_new:
                    os.replace(Ttmp, T)
                    durable.fsync_dir(journal.work_dir(volroot))
                    return _redo_from_T(ctx, volroot, rec, F, T, size, sha_new)
        except OSError:
            return R_KEEP

        # row 2: F already carries the verified conversion -> just finish (restore mtime, done.log, drop J).
        # This runs even when T was unreadable: an already-written F must be finished, not stranded forever.
        fsha = _medium_sha(F)
        if fsha is None:
            return R_KEEP
        if fsha == sha_new:
            try:
                os.utime(F, ns=(rec.get('atime_ns', rec['mtime_ns']), rec['mtime_ns']))
                durable.fsync_dir(os.path.dirname(F))
            except OSError:
                return R_KEEP
            return finish_frame(ctx, volroot, rec, sha_new)

    # row 3: F is byte-for-byte the untouched original -> nothing durable landed; drop J/T, pending again.
    # This decision requires the FULL-file medium SHA == sha_old, never the strip-excluding meta check: a
    # torn overwrite leaves the trailer (WhiteLevel 1023, meta == meta_old) intact while the leading strips
    # are already converted, and a meta-only test would wrongly accept that corrupt file and _drop it,
    # destroying the only original. _is_original stays only as a cheap pre-filter so the 240 MB hash is
    # skipped when F plainly is not the original.
    pre = _is_original(F, size, meta_old)
    if pre is None:
        return R_KEEP                                      # F unreadable now -> retry, decide nothing
    if pre and sha_old is not None:
        fsha = _medium_sha(F)
        if fsha is None:
            return R_KEEP
        if fsha == sha_old:
            _drop(ctx, volroot, rec)
            return R_PENDING

    # F is torn/corrupt and not a redo-able conversion. If a PRESENT T/Ttmp was merely unreadable just now
    # (maybe a transient medium glitch), KEEP and retry -- it may become readable and let us redo -- rather
    # than a premature STUCK. Only STUCK when there is no viable redo source (T readable-but-wrong, or absent).
    if t_unreadable:
        return R_KEEP

    # row 4: torn F, or cannot prove original/conversion -> STUCK (keep T, .x6j, J; block volume; delete none)
    journal.move_to_stuck(ctx.data_root, rec, 'WRITING unresolved on recovery')
    _quarantine(volroot, stem)
    ctx.gates.block_volume(volroot)
    return R_STUCK


def _redo_from_T(ctx, volroot, rec, F, T, size, sha_new):
    if do_overwrite(ctx, F, T, size, sha_new, rec['mtime_ns'], rec.get('atime_ns', rec['mtime_ns'])):
        rec = dict(rec, state=C.J_WRITTEN, sha_new=sha_new)
        journal.write_j(ctx.data_root, rec)
        return finish_frame(ctx, volroot, rec, sha_new)
    ctx.gates.block_volume(volroot)
    return R_BLOCKED


def recover_volume(ctx, volkey, volroot):
    """Resolve every J on this volume, then every orphan .x6j, before any new work may start (§5.4)."""
    results = []
    for rec in list(journal.iter_js(ctx.data_root, volkey)):
        results.append((rec['stem'], recover_frame(ctx, volroot, rec)))
    # orphans: .x6j present with no live J
    for xj in list(journal.iter_xj(volroot)):
        if journal.read_j(ctx.data_root, volkey, xj['stem']) is not None:
            continue
        rec = dict(xj)
        if 'mtime_ns' not in rec:                     # old .x6j without timestamps: avoid a 1970 stamp
            try:
                rec['mtime_ns'] = os.lstat(os.path.join(volroot, rec['rel'])).st_mtime_ns
            except OSError:
                rec['mtime_ns'] = 0
        rec.setdefault('state', C.J_WRITING)
        results.append(('orphan:' + xj['stem'], recover_frame(ctx, volroot, rec)))
    return results
