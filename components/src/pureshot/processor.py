#!/usr/bin/env python3
"""x6-pureshot-processor main loop, job model (§5.2) and GUI IPC (§4).

Ties together enumeration/grouping (grouping.py), gates (gates.py) and the safety-critical per-frame
protocol + recovery (frame.py). step() performs one deterministic iteration (handle a request, recover
every present volume before any new work, rescan groups, advance the frame pipeline until one frame
completes, publish status+groups) so the whole orchestration is driveable offline; main() calls step()
in a paced loop. Selection is a set of stable group ids in job.json; recovery runs only when none of the
daemon's own frames is in flight (§5.4).

Frame overlap (2026-11-04, owner: "speed it up, ok with higher risk"): one converter at a time (a single AI
accelerator, ~2 GiB working set), but while it runs the main thread verifies and writes the previous frame and
reads the next one (V0). Each frame keeps its own complete §5.3 protocol and its own J/.x6j/T; see frame.py.
Heat (owner: "we need to process for hours"): the accelerator runs at its top level and frames are PACED --
no new conversion starts while the SoC is at or above PACE_TEMP -- which holds more frames per hour than a
slower level, since the camera's heat budget, not the accelerator, limits a long job (unit 5 measurements)."""
import collections, os, signal, subprocess, sys, threading, time
import contract as C
import durable, journal, grouping, frame, gates as gatesmod, preview

BUSY_GRACE = 3.0    # s: a "camera busy" refusal shorter than this keeps showing "Processing" (probe hiccup)
FRAME_EST_DEFAULT = 16.0   # s per frame before this daemon has timed one (unit 5: SSD ~15 s, internal ~17 s)
FRAME_EST_KEEP = 8         # recent frame intervals kept for the estimate (median)
LIVE_PUBLISH_EVERY = 1.0   # s: status refresh while a frame converts (the page's tick reads it at 1 Hz)
STOP_REASON = 'stop requested'   # the converter-poll abort reason for an owner Stop tap (handled at once)
# Pause reasons that mean the owner is using the camera: the live preview must not be paused then (preview.py).
OWNER_ACTIVE = ('camera busy', 'USB connected', 'file transfer in progress', 'volume gone', 'stopped', STOP_REASON)
RESCAN_IDLE, RESCAN_RUNNING = 5.0, 15.0   # s between group rescans (a job updates its own counts in memory)
CONVERT_TIMEOUT = 300.0    # s: a conversion longer than this is aborted (SD worst case is ~20 s)
# Accelerator level passed to ps2native --dsp (libbva_camera DSPCoreClockLevel; 11 = the top voltage corner, which
# the engine itself requests for its multi-frame shots). Unit 5, 2026-11-04, back-to-back with the overlapped
# daemon's I/O: level 11 = 3.84 s per conversion (4.4 s on average as it warms), level 5 = 5.3 s; both settle near
# the same ~78 degC, so for a long job 11 gets ~20 % more frames through the same heat. The output bytes are
# identical at every level. An older ps2native without --dsp support gets no flag (its fixed level 1).
DSP_LEVEL = 11
PACE_TEMP, PACE_RESUME = 75.0, 74.0   # degC (soc-chip-therm): hold the next conversion at PACE_TEMP until RESUME.
                                      # Keeps hours-long jobs below the 78 degC pause (gates.HEAT_PAUSE); a conversion
                                      # started at 74.9 peaks ~1-1.5 degC higher. Owner chose 75 over 72 (2026-11-04):
                                      # the camera sheds more heat the hotter it runs (~+10 % frames per hour), at the
                                      # price of a smaller margin to the vendor's ~77.8 degC recording stop.


def fmt_left(sec):
    """Owner-facing time-left text, ASCII only (the GUI string loader is not UTF-8)."""
    sec = max(0.0, float(sec))
    r5 = max(5, int(round(sec / 5.0)) * 5)
    if r5 < 60:
        return 'about %d s' % r5
    total = max(60, int(round(sec / 10.0)) * 10)
    if total >= 3600:
        h, rem = divmod(total, 3600)
        return 'about %d h %d min' % (h, rem // 60) if rem // 60 else 'about %d h' % h
    m, s = divmod(total, 60)
    return 'about %d min %d s' % (m, s) if s else 'about %d min' % m


def read_boot_id():
    try:
        with open('/proc/sys/kernel/random/boot_id') as f:
            return f.read().strip()
    except OSError:
        return 'noboot'


def default_list_volumes():
    """Present media volumes as [{volkey, volroot, storage}]. volkey = exFAT UUID (by-uuid) else the
    mountpoint basename. On the camera the UUID comes from /dev/disk/by-uuid; injected in tests."""
    out = []
    byid = {}
    try:
        base = '/dev/disk/by-uuid'
        for u in os.listdir(base):
            byid[os.path.realpath(os.path.join(base, u))] = u
    except OSError:
        pass
    for root, sto in C.STORAGE_ROOTS:
        if not os.path.ismount(root) and not os.path.isdir(os.path.join(root, 'DCIM')):
            continue
        volkey = None
        try:
            dev = os.path.realpath('/dev/block/%d:%d' % (os.stat(root).st_dev >> 8, os.stat(root).st_dev & 0xff))
            volkey = byid.get(dev)
        except OSError:
            pass
        out.append({'volkey': volkey or os.path.basename(root.rstrip('/')), 'volroot': root, 'storage': sto})
    return out


# ---------------------------------------------------------------- conversions ----

class _SyncRun(object):
    """An injected blocking converter(F, T, poll) (offline tests): it runs to completion inside the constructor, with
    poll() = the abort gates + live status + owner Stop, exactly like the sequential protocol."""
    def __init__(self, fn, F, T, poll):
        self.abort_reason = None
        self.tail = ()
        holder = {}
        def p():
            reason = poll()
            if reason:
                holder['reason'] = reason
            return reason
        try:
            self.rc = fn(F, T, p)
        except frame.AbortConvert:
            self.rc = 'abort'
        if self.rc == 'abort':
            self.abort_reason = holder.get('reason')

    def done(self):
        return True

    def wait(self, timeout=None):
        return True

    def request_abort(self, reason):
        pass


class _ConverterRun(object):
    """The production converter: ps2native alone in its own session (so killpg hits exactly it), plus a watchdog
    thread that polls the abort gates every 0.25 s and SIGKILLs it on the first reason -- always safe, F is still
    the untouched original until the critical section, which never overlaps a conversion of the same frame."""
    def __init__(self, argv, check, gates, timeout=CONVERT_TIMEOUT):
        self.rc = None
        self.abort_reason = None
        self.tail = collections.deque(maxlen=12)
        self._cancel = None
        self._wake = threading.Event()
        self._done = threading.Event()
        self._gates, self._check, self._timeout = gates, check, timeout
        try:
            self.p = subprocess.Popen(argv, env={'PATH': '/usr/bin:/bin'}, cwd='/', preexec_fn=os.setsid,
                                      stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        except OSError:
            self.p = None
            self.rc = 3
            self._done.set()
            return
        try:
            os.setpriority(os.PRIO_PROCESS, self.p.pid, 19)
        except OSError:
            pass
        try:
            with open('/proc/%d/oom_score_adj' % self.p.pid, 'w') as f:
                f.write('800')          # under genuine pressure the kernel kills the converter, not capture
        except OSError:
            pass
        try:
            self.pgid = os.getpgid(self.p.pid)
        except OSError:
            self.pgid = self.p.pid
        gates.begin_conversion(self.pgid)     # the reader gate must ignore our own converter's fds
        self._t0 = time.monotonic()
        threading.Thread(target=self._drain, name='pp-conv-stderr', daemon=True).start()
        threading.Thread(target=self._watch, name='pp-conv-watch', daemon=True).start()

    def _drain(self):
        try:
            for line in self.p.stderr:
                self.tail.append(line.decode('utf-8', 'replace').strip()[:160])
        except (OSError, ValueError):
            pass

    def _watch(self):
        try:
            while True:
                rc = self.p.poll()
                if rc is not None:
                    self.rc = rc
                    return
                reason = self._cancel
                if reason is None:
                    try:
                        reason = self._check()
                    except Exception:
                        reason = None
                if reason is None and time.monotonic() - self._t0 > self._timeout:
                    reason = 'converter timeout'
                if reason:
                    self.abort_reason = reason
                    try:
                        os.killpg(self.pgid, signal.SIGKILL)
                    except OSError:
                        pass
                    self.p.wait()
                    self.rc = 'abort'
                    return
                self._wake.wait(0.25)
                self._wake.clear()
        finally:
            self._gates.end_conversion()
            self._done.set()

    def done(self):
        return self._done.is_set()

    def wait(self, timeout=None):
        return self._done.wait(timeout)

    def request_abort(self, reason):
        if self._cancel is None:
            self._cancel = reason
        self._wake.set()


# ---------------------------------------------------------------- processor ----

class Processor(object):
    def __init__(self, data_root=C.DATA, run_dir=C.RUN, gates=None, converter=None,
                 list_volumes=default_list_volumes, clock=time.monotonic, camera_id='x6',
                 daemon_version='pureshot-v1'):
        self.data_root = data_root
        self.run_dir = run_dir
        self.gates = gates or gatesmod.Gates()
        # None = production (ps2native via _ConverterRun). Tests inject a blocking converter(F, T, poll), or an
        # object with start(F, T, check) returning a run (done/wait/request_abort/rc/abort_reason) to test overlap.
        self.converter = converter
        self.list_volumes = list_volumes
        self.clock = clock
        self.camera_id = camera_id
        self.daemon_version = daemon_version
        self.boot_id = read_boot_id()
        self.job = self._load_job()
        self.groups = []            # cached, list of (volkey, volroot, storage, groupdict, gid)
        self._groups_ts = -1e9      # rescan throttle only (NOT the published gen)
        self._gen = 0               # published groups 'gen' = a CONTENT version (see _publish_groups)
        self._gen_sig = None
        self._req_seq = self.job.get('last_req_seq', 0)
        self._last_result = C.RES_OK
        self._current = None        # gid of the frame in flight (converting, else being written/read)
        self._last_frame_result = None   # what the last frame stage actually returned (drives the display)
        self._pause_why = ''             # the real gate decision behind the last R_PAUSED
        self._paused_since = None        # when the current run of R_PAUSED results began (busy-display grace)
        self._frame_started = None       # clock() when the current frame's interval began (live frame %)
        self._durations = []             # recent frame intervals (s) -> per-frame estimate
        self._last_live_publish = -1e9
        # the frame pipeline: at most one frame per stage
        self._prep = None                # frame.Prep: read (V0), not started
        self._run = None                 # (frame.Prep, run): converting
        self._conv = None                # frame.Prep: converted, waiting for V1 + overwrite
        self._retries = {}               # (volkey, rel) -> V1 retries so far (one is allowed)
        self._cooling = False            # thermal pacing: holding the next conversion
        self._caps_key = None
        self._caps = False               # the installed ps2native takes --dsp
        self.crash_at = {}               # fault harness only: stem -> crash labels for that frame's stages
        self._pause_wanted = False       # read by the live-preview pause thread (preview.py); set by publish()
        os.makedirs(self.data_root, exist_ok=True)
        os.makedirs(self.run_dir, exist_ok=True)

    # ---- job.json ----
    def _default_job(self):
        return {'version': 1, 'boot_id': self.boot_id, 'running': False, 'selection': [],
                'excluded': [], 'error': None, 'window_off': 0, 'last_req_seq': 0, 'batch_done': 0}

    def _job_path(self):
        return os.path.join(self.data_root, 'job.json')

    def _load_job(self):
        j = durable.read_json(self._job_path())
        if not j:
            return self._default_job()
        if j.get('boot_id') != self.boot_id:        # resume policy: a job does not auto-run across a reboot
            j['running'] = False
            j['boot_id'] = self.boot_id
        return j

    def _save_job(self):
        self.job['last_req_seq'] = self._req_seq
        durable.write_json(self._job_path(), self.job)

    # ---- request handling (§4) ----
    def _req_path(self):
        return os.path.join(self.run_dir, 'request')

    def _read_request(self):
        try:
            with open(self._req_path(), 'rb') as f:
                b = f.read()
        except OSError:
            return None
        return C.parse_request(b)

    def _stop_pending(self):
        req = self._read_request()
        return req is not None and req['op'] == C.OP_STOP

    def handle_request(self, req):
        if req is None:
            return
        self._req_seq = (self._req_seq + 1) & 0xff
        op, gid = req['op'], req['id']
        sel = set(self.job['selection'])
        self._last_result = C.RES_OK
        if op == C.OP_SELECT:
            sel.add(gid)
        elif op == C.OP_DESELECT:
            sel.discard(gid)
        elif op == C.OP_SELECT_ALL:
            sel = {g[4] for g in self.groups if g[3]['pending'] > 0}
        elif op == C.OP_SELECT_NONE:
            sel = set()
        elif op == C.OP_START:
            if not sel:
                self._last_result = C.RES_NOTHING
            else:
                if not self.job['running']:
                    # a new wave (owner 2026-11-04): progress counts only what this run handles, from 0 %
                    self.job['batch_done'] = 0
                self.job['running'] = True
                self.job['error'] = None
                self.job['boot_id'] = self.boot_id
                self._last_frame_result = None
        elif op == C.OP_STOP:
            self.job['running'] = False
        elif op == C.OP_RESCAN_ACK:
            self.job['error'] = None
            self._groups_ts = -1e9
        elif op == C.OP_PAGE:
            # No shared window any more (owner 2026-11-04): an advanced window was saved in job.json, so the next
            # page open showed only older groups and hid every new one. Older groups are now pages of their own
            # that read their slice of groups-all; an earlier GUI that still sends OP_PAGE gets a no-op.
            self.job['window_off'] = 0
        self.job['selection'] = sorted(sel)
        # clear the request file (consume it)
        try:
            os.unlink(self._req_path())
        except FileNotFoundError:
            pass
        self._save_job()

    # ---- enumeration/grouping ----
    def rescan(self, force=False):
        now = self.clock()
        every = RESCAN_RUNNING if self.job.get('running') else RESCAN_IDLE
        if not force and (now - self._groups_ts) < every:
            return
        self._groups_ts = now
        done = frame.done_set(self.data_root)
        excl = self._excluded_keys()
        # While a USB host may be exporting internal + SD, never open their files (enumerate_volume classifies every
        # DNG by reading its tail -- one open fd at the instant the camera unmounts for export makes that unmount
        # fail). Keep their last known groups instead, so the page's list and gen stay stable through the export.
        hold = self.gates.usb_hold()
        allg = [t for t in self.groups if self.gates.usb_blocks(t[1])] if hold else []
        for v in self.list_volumes():
            if hold and self.gates.usb_blocks(v['volroot']):
                continue
            frames = grouping.enumerate_volume(v['volroot'], v['volkey'], v['storage'])
            for fr in frames:                       # a frame the job gave up on counts as failed, not still to do
                if fr.get('kind') == 'pending' and self._excluded_key(v['volkey'], fr['rel']) in excl:
                    fr['kind'] = 'bad'
            for g in grouping.group_frames(frames):
                # List only what the page can act on or already acted on: a group with a pending passthrough
                # frame, or one holding a frame this daemon converted (done.log). Native DNGs straight from the
                # camera (never passthrough) have nothing to process and filled the page with "complete" groups.
                if not g['pending'] and not any((v['volkey'], m['rel']) in done for m in g['members']):
                    continue
                gid = grouping.group_id(v['volkey'], g['first'])
                allg.append((v['volkey'], v['volroot'], v['storage'], g, gid))
        # newest group first (by last member mtime)
        allg.sort(key=lambda t: t[3]['members'][-1]['mtime'] if t[3]['members'] else 0, reverse=True)
        self.groups = allg
        self.job['window_off'] = 0                          # no shared window (see OP_PAGE)

    def _set_kind(self, fr, kind):
        """Update one member of the cached group list in place (instead of re-reading every DNG on the volume after
        each frame, ~0.6 s on the SSD): the counts the page shows stay right until the next periodic rescan."""
        for volkey, volroot, sto, g, gid in self.groups:
            if volkey != fr['volkey']:
                continue
            for m in g['members']:
                if m.get('rel') == fr['rel']:
                    old = m.get('kind')
                    if old == kind:
                        return
                    m['kind'] = kind
                    for k, d in ((old, -1), (kind, 1)):
                        if k == 'pending':
                            g['pending'] += d
                        elif k == 'done':
                            g['done'] += d
                        elif k in ('bad', 'other'):
                            g['failed'] += d
                    return

    # ---- frame selection ----
    def _excluded_key(self, volkey, rel):
        return '%s\t%s' % (volkey, rel)

    def _excluded_keys(self):
        excl = set(self.job['excluded'])
        excl |= {('%s\t%s' % (s['volkey'], s.get('rel', ''))) for s in journal.list_stuck(self.data_root)}
        return excl

    def _key(self, fr):
        return (fr['volkey'], fr['rel'])

    def _inflight(self):
        """The daemon's own frames in flight, as frame dicts (read, converting, converted)."""
        out = []
        if self._prep is not None:
            out.append(self._prep.frame)
        if self._run is not None:
            out.append(self._run[0].frame)
        if self._conv is not None:
            out.append(self._conv.frame)
        return out

    def pick_next(self):
        """Oldest pending frame of the selected groups, newest group first; skip excluded/blocked/stuck and the
        daemon's own frames already in flight."""
        sel = set(self.job['selection'])
        excl = self._excluded_keys()
        busy = {self._key(f) for f in self._inflight()}
        for volkey, volroot, sto, g, gid in self.groups:
            if gid not in sel:
                continue
            if volroot in self.gates.blocked:
                continue
            for m in g['members']:
                if m.get('kind') != 'pending':
                    continue
                if self._excluded_key(volkey, m['rel']) in excl or (volkey, m['rel']) in busy:
                    continue
                return {'volkey': volkey, 'volroot': volroot, 'rel': m['rel'], 'stem': m['stem'],
                        'path': m['path'], 'gid': gid}
        return None

    # ---- one iteration ----
    def step(self):
        self.handle_request(self._read_request())
        present = self.list_volumes()
        # recovery before any new work, on every present volume with a pending J -- but never while the daemon's own
        # frames are in flight (their J's are live by design), and not on internal/SD while a USB host may be
        # exporting them (usb_hold): recovery reads and may write the volume; it runs after the hold
        if not self._inflight():
            for v in present:
                if self.gates.usb_blocks(v['volroot']):
                    continue
                if journal.has_j(self.data_root, v['volkey']) or list(journal.iter_xj(v['volroot'])):
                    frame.recover_volume(self._frame_ctx(), v['volkey'], v['volroot'])
        self.rescan()
        did = None
        if self.job['running'] and not self.job['error']:
            did = self._advance()
        else:
            self._drain()
        self.publish()
        return did

    def _frame_ctx(self, on_start=None, stem=None):
        ctx = frame.Ctx(self.data_root, self.converter, self.gates, camera_id=self.camera_id,
                        daemon_version=self.daemon_version, on_start=on_start,
                        crash_at=self.crash_at.get(stem, ()))
        ctx.inflight = {(f['volkey'], f['stem']) for f in self._inflight()}
        return ctx

    def _advance(self):
        """Move the frame pipeline on until one frame completes, or nothing more can be done right now.

        Order per pass: collect a finished conversion; start the next one as soon as the converter is free (unless
        pacing for heat); then write the converted frame while that conversion runs; then read the frame after it;
        only then wait for the converter. With a blocking test converter this degenerates to one frame per step."""
        last = None
        launch_refused = False
        while True:
            if not self.job['running'] or self.job['error']:
                self._drain()
                return last or ('idle', None)
            if self._run is not None and self._run[1].done():
                r = self._collect()
                if r is not None:
                    last = r
                continue                               # re-check the job first: a Stop may have just ended it
            if self._run is None and self._prep is not None and not launch_refused:
                if self._pace():
                    launch_refused = True              # too warm: no new conversion now; do the other work
                else:
                    r = self._launch()
                    if r is not None:
                        last = r
                        launch_refused = True
                    continue
            if self._conv is not None:
                return self._finish()
            if last is not None:
                return last
            if self._prep is None:
                nxt = self.pick_next()
                if nxt is not None:
                    r = self._prepare(nxt)
                    if r is not None:
                        return r
                    continue
            if self._run is not None:
                self._wait_run()
                continue
            if self._cooling:
                return ('cooling', None)
            if self._prep is None:
                self._frame_started = None
                self._current = None
                done, total = self._selection_counts()
                if total and done >= total:
                    # every selected frame is converted: the job is complete. End it, so the page's button turns
                    # back to "Start processing", picking another group does not start it without a tap, and the
                    # idle probe stops (a finished-but-"running" job kept polling ins_bll_cli ~1/s forever).
                    self.job['running'] = False
                    self._save_job()
            return ('idle', None)

    def _pace(self):
        """Thermal pacing before a conversion starts: True = hold it (SoC at/above PACE_TEMP until below RESUME)."""
        try:
            t = self.gates._soc()
        except Exception:
            t = None
        if t is None:
            self._cooling = False
        elif self._cooling:
            self._cooling = t >= PACE_RESUME
        else:
            self._cooling = t >= PACE_TEMP
        return self._cooling

    def _dsp_args(self):
        """['--dsp=N'] when the installed ps2native takes it (probed once per binary: an older build would read the
        flag as an override file name and fail every frame)."""
        try:
            st = os.stat(C.PS2NATIVE)
            key = (st.st_ino, st.st_size, st.st_mtime_ns)
        except OSError:
            return []
        if key != self._caps_key:
            self._caps_key = key
            try:
                out = subprocess.run([C.PS2NATIVE, '--caps'], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                     stderr=subprocess.DEVNULL, timeout=10, cwd='/', env={'PATH': '/usr/bin:/bin'})
                self._caps = out.returncode == 0 and b'dsp' in out.stdout
            except (OSError, subprocess.SubprocessError):
                self._caps = False
        return ['--dsp=%d' % DSP_LEVEL] if self._caps else []

    def _thread_check(self, volroot):
        """The converter watchdog's abort test (runs on its thread): the gates, or an owner Stop tap."""
        return self.gates.abort_reason(volroot) or (STOP_REASON if self._stop_pending() else None)

    def _sync_poll(self, volroot):
        def poll():
            return self.gates.abort_reason(volroot) or self._live_publish()
        return poll

    def _start_conversion(self, prep):
        volroot, stem, F = prep.frame['volroot'], prep.frame['stem'], prep.frame['path']
        T = frame._T(volroot, stem)
        if self.converter is None:
            return _ConverterRun([C.PS2NATIVE, F, T] + self._dsp_args(), lambda: self._thread_check(volroot), self.gates)
        if hasattr(self.converter, 'start'):
            return self.converter.start(F, T, lambda: self._thread_check(volroot))
        return _SyncRun(self.converter, F, T, self._sync_poll(volroot))

    def _prepare(self, nxt):
        if self._frame_started is None:
            self._frame_started = self.clock()
        if self._run is None and self._conv is None:
            self._current = nxt['gid']
        ctx = self._frame_ctx(stem=nxt['stem'])
        r, prep = frame.prepare(ctx, nxt, self._retries.get(self._key(nxt), 0))
        if r is None:
            self._prep = prep
            return None
        self._pause_why = ctx.pause_reason
        return self._account(r, nxt)

    def _launch(self):
        prep = self._prep
        self._current = prep.frame['gid']
        ctx = self._frame_ctx(on_start=self.publish, stem=prep.frame['stem'])   # "Processing" shown once gates pass
        r, _ = frame.begin(ctx, prep)
        if r is not None:
            if r != frame.R_PAUSED:
                self._prep = None                      # read again later (F changed) or not at all (error)
            self._pause_why = ctx.pause_reason
            return self._account(r, prep.frame)
        self._prep = None
        self._run = (prep, self._start_conversion(prep))
        return None

    def _collect(self):
        prep, run = self._run
        self._run = None
        ctx = self._frame_ctx(stem=prep.frame['stem'])
        ctx.pause_reason = run.abort_reason or ''
        r, _ = frame.converted(ctx, prep, run.rc)
        if r is None:
            self._conv = prep
            return None
        self._pause_why = ctx.pause_reason
        return self._account(r, prep.frame)

    def _finish(self):
        prep = self._conv
        if self._run is None:
            self._current = prep.frame['gid']
        ctx = self._frame_ctx(stem=prep.frame['stem'])
        try:
            r = frame.finish(ctx, prep)
        finally:
            self._conv = None
        self._pause_why = ctx.pause_reason
        return self._account(r, prep.frame)

    def _wait_run(self):
        """Wait for the running conversion; keep the page live and handle its taps meanwhile. A Stop tap is left for
        the converter watchdog, which aborts the conversion at its next poll (the request is applied right after)."""
        prep, run = self._run
        while not run.wait(0.2):
            now = self.clock()
            if now - self._last_live_publish >= LIVE_PUBLISH_EVERY:
                self._last_live_publish = now
                self.publish()
            req = self._read_request()
            if req is not None and req['op'] != C.OP_STOP:
                self.handle_request(req)
            if not self.job['running'] or self.job['error']:
                run.request_abort('stopped')

    def _drain(self):
        """The job stopped or failed: abort the running conversion (F is still the original), forget the frame that
        was only read, and finish a frame that is already converted -- unless the job failed or its volume is
        blocked, then drop it (recovery would)."""
        self._prep = None
        if self._run is not None:
            self._run[1].request_abort('stopped')
            self._run[1].wait(30.0)
            if self._run[1].done():
                self._collect()
        if self._conv is not None:
            fr = self._conv.frame
            if self.job.get('error') or fr['volroot'] in self.gates.blocked:
                frame._drop(self._frame_ctx(), fr['volroot'], self._conv.rec)
                self._conv = None
            else:
                self._finish()
        if not self._inflight():
            self._current = None
            self._frame_started = None
            self._cooling = False

    def _account(self, r, fr):
        """Bookkeeping for one stage result; returns step()'s (outcome, stem)."""
        key = self._key(fr)
        self._last_frame_result = r
        if r == frame.R_DONE:
            now = self.clock()
            if self._frame_started is not None:
                self._durations = (self._durations + [now - self._frame_started])[-FRAME_EST_KEEP:]
            self._frame_started = now           # frames overlap: the next one's interval starts here
            self._set_kind(fr, 'done')
            self._wave_handled()
            self._retries.pop(key, None)
            self._paused_since = None
        elif r == frame.R_PAUSED:
            if self._pause_why == STOP_REASON:
                # the owner's Stop aborted the conversion: apply that request now, so the page goes straight to
                # Ready / "Start processing" instead of flashing a pause first
                self.handle_request(self._read_request())
                self._last_frame_result = None
                self._paused_since = None
            elif self._paused_since is None:
                self._paused_since = self.clock()
        else:
            self._paused_since = None
            if r == frame.R_RETRY:
                self._retries[key] = self._retries.get(key, 0) + 1
            elif r in (frame.R_STUCK, frame.R_EXCLUDED):
                self.job['excluded'] = sorted(set(self.job['excluded']) | {self._excluded_key(fr['volkey'], fr['rel'])})
                self._retries.pop(key, None)
                self._set_kind(fr, 'bad')           # leaves the wave: never counted as done ("Done 2/2", not 3/3)
            elif r == frame.R_ERRSTOP:
                self.job['running'] = False
                self.job['error'] = 'converter check failed'
            elif r == frame.R_BLOCKED:
                self.job['running'] = False
                self.job['error'] = 'write failed; volume blocked'
            if r != frame.R_PENDING:
                self._save_job()
        if not self._inflight():
            self._current = None                # nothing of ours in flight: no group is "current" on the page
        return (r, fr['stem'])

    def _live_publish(self):
        """Blocking-converter poll hook (~4 Hz). 1) Refresh the status at LIVE_PUBLISH_EVERY so the page's frame % and
        time left move while the converter runs. 2) Return STOP_REASON when the owner tapped Stop (a tap used to wait
        out the whole frame, ~15 s, unit 5 2026-11-04). Aborting a conversion is always safe -- F is still the
        untouched original."""
        now = self.clock()
        if now - self._last_live_publish >= LIVE_PUBLISH_EVERY:
            self._last_live_publish = now
            self.publish()
        req = self._read_request()
        if req is not None and req['op'] == C.OP_STOP:
            return STOP_REASON
        return None

    # ---- publication (§4) ----
    def _wave_handled(self):
        self.job['batch_done'] = int(self.job.get('batch_done', 0) or 0) + 1

    def _batch_counts(self):
        """(done, total) of THIS wave (owner 2026-11-04: "only this wave's work, every wave from 0 %"): frames
        converted since Start, and those plus what is still to do in the selection -- so selecting another group
        mid-run grows the total, frames converted in earlier waves never count, and a frame given up on (STUCK /
        excluded) simply leaves the total instead of being counted as done."""
        left = 0
        sel = set(self.job['selection'])
        for volkey, volroot, sto, g, gid in self.groups:
            if gid in sel:
                left += g['pending']
        done = int(self.job.get('batch_done', 0) or 0)
        return done, done + left

    def _selection_counts(self):
        """(done, total) frames over the selected groups (job completion; the page shows the wave counts)."""
        total = done = 0
        sel = set(self.job['selection'])
        for volkey, volroot, sto, g, gid in self.groups:
            if gid in sel:
                total += g['pending'] + g['done']
                done += g['done']
        return done, total

    def _state_and_pct(self, present):
        """(state, percent, why). `why` is the blocking gate's reason when PAUSED -- shown to the owner, since a
        bare "Paused" gives no hint whether to plug in, cool down, or unplug USB."""
        done, total = self._selection_counts()
        bd, bt = self._batch_counts()
        pct = int(round(bd * 100.0 / bt)) if bt else 0
        if self.job.get('error'):
            return C.ST_ERROR, pct, ''
        if not present and not self.gates.usb_hold():   # exported to a computer is not "No card"
            return C.ST_NOT_READY, pct, ''
        if not self.groups:
            return C.ST_NO_GROUPS, pct, ''
        if self.job['running']:
            if self._last_frame_result == frame.R_PAUSED:
                # Report what the daemon ACTUALLY decided for the last attempt, never a separate passive re-check
                # (that read a cache gone stale during the overwrite and flashed "Paused: camera busy" between
                # frames on the camera). A single refused idle probe (ins_bll_cli slow under load) retried 0.5 s
                # later flashed "Paused: camera busy" (test 6.3): name a busy camera only once it has lasted.
                if (self._pause_why == 'camera busy' and self._paused_since is not None
                        and self.clock() - self._paused_since < BUSY_GRACE):
                    return C.ST_RUNNING, pct, ''
                if self._inflight() or self.pick_next() is not None:
                    return C.ST_PAUSED, pct, self._pause_why
            if self._current is not None or self._inflight():
                return C.ST_RUNNING, pct, ''
            if self.pick_next() is not None:
                return C.ST_RUNNING, pct, ''
            return (C.ST_DONE if total and done >= total else C.ST_READY), pct, ''
        return C.ST_READY, pct, ''

    def _title(self, g):
        """Group row label (the GUI row fits ~19 characters): capture date/time of the first frame so groups can be
        told apart, then the frame count, the progress 'done/total' once some are converted, or 'done' when the
        whole group is (the GUI also greys that row out from RF_COMPLETE)."""
        p = grouping.parse_name(g['first']) or {}
        d, t = p.get('date', ''), p.get('time', '')
        when = ('%s-%s %s:%s' % (d[4:6], d[6:8], t[0:2], t[2:4])) if len(d) == 8 and len(t) == 6 else '?'
        n = g['pending'] + g['done']
        if g['pending'] == 0 and g['done'] > 0:
            tail = '%dx done' % n
        elif g['done'] > 0:
            tail = '%d/%d' % (g['done'], n)
        else:
            tail = '%dx' % n
        return ('%s %s' % (when, tail))[:47]

    def publish(self):
        present = self.list_volumes()
        # Keep the idle cache warm only while a job runs; decisions use a fresh synchronous probe on a stale cache
        # anyway (ok_to_run / idle_fresh) and the in-flight abort poll refreshes it itself. With no job running
        # nothing reads it, and probing every 2 s forever churned ibus programs (one per ins_bll_cli run).
        if self.job.get('running') and (self._inflight() or self.pick_next() is not None):
            self.gates.poll_idle()
        else:
            self.gates.reap_idle()
        state, pct, why = self._state_and_pct(present)
        # the live preview may be paused (screen dark) only while a job really runs or waits on heat/battery, never
        # while the owner is shooting, exporting over USB or downloading
        self._pause_wanted = bool(self.job.get('running') and not self.job.get('error') and
                                  (state == C.ST_RUNNING or (state == C.ST_PAUSED and why not in OWNER_ACTIVE)))
        sev = {C.ST_ERROR: C.SEV_ERROR, C.ST_PAUSED: C.SEV_PAUSED, C.ST_NOT_READY: C.SEV_GREY,
               C.ST_NO_GROUPS: C.SEV_GREY}.get(state, C.SEV_NORMAL)
        text = self._status_text(state, pct, why)
        # two more NUL-separated lines for the page's "Progress" / "Time left" rows (a GUI that predates them reads
        # only the first string, so this stays compatible both ways)
        prog, left = self._progress_lines(state)
        # The page shows these through Embedded Wizard rich text, where '%' is the escape character: a lone '%' is
        # swallowed (unit 5 showed "Total 27, frame 78"), '%%' renders one '%'.
        text = text + '\0' + prog.replace('%', '%%') + '\0' + left.replace('%', '%%')
        durable.atomic_write(os.path.join(self.run_dir, 'status'),
                             C.pack_status(int(time.time()), sev, state, pct, self._req_seq, self._last_result, text),
                             dir_fsync=False)
        self._publish_groups(present)

    def frame_estimate(self):
        """Seconds per frame: median of this daemon's recent frame intervals, else FRAME_EST_DEFAULT."""
        if not self._durations:
            return FRAME_EST_DEFAULT
        d = sorted(self._durations)
        return d[len(d) // 2]

    def _progress_lines(self, state):
        """('Total 23%, frame 45%', 'about 2 min 40 s') -- the page's Progress and Time left values, for THIS wave
        (see _batch_counts). The frame % is the time since the last frame finished / the per-frame estimate, capped
        at 99 % until the next one is done. With no job running they describe the wave a Start would begin now
        (0 % and its time), or the wave that just finished."""
        done, total = self._batch_counts()
        est = self.frame_estimate()
        if not self.job.get('running'):
            left = total - done
            if left > 0:
                return 'Total 0%', fmt_left(left * est)
            if done > 0:
                return 'Total 100%', 'done'
            return '-', '-'
        if not total:
            return '-', '-'
        if state == C.ST_RUNNING and self._frame_started is not None and (self._current is not None or self._inflight()):
            el = max(0.0, self.clock() - self._frame_started)
            ff = min(0.99, el / est) if est > 0 else 0.0
            tp = int((done + ff) * 100.0 / total)
            return 'Total %d%%, frame %d%%' % (tp, int(ff * 100)), fmt_left((total - done) * est - el)
        tp = int(done * 100.0 / total)
        if done >= total:
            return 'Total 100%', 'done'
        if state == C.ST_PAUSED:
            return 'Total %d%%' % tp, 'paused'
        return 'Total %d%%' % tp, fmt_left((total - done) * est)

    def _status_text(self, state, pct, why=''):
        """The page's status value (a label+value row, so keep it short). Progress is frames 'done/total' over
        the selected groups -- a bare percentage read as an unexplained number on the camera. The counts are the
        current wave's (_batch_counts): 'Processing 3/56' even when the groups hold frames from earlier waves."""
        done, total = self._batch_counts()
        if state == C.ST_READY:
            if total > done:
                return 'Ready: %d to do' % (total - done)
            if done:
                return 'Done %d/%d' % (done, done)       # the wave that just finished
            return 'Select groups'
        if state == C.ST_RUNNING and self._cooling and self._run is None:
            return 'Cooling down %d/%d' % (done, total)     # heat pacing: the job runs, the next frame waits
        names = {C.ST_NOT_READY: 'No card', C.ST_NO_GROUPS: 'Nothing to process', C.ST_SCANNING: 'Scanning',
                 C.ST_RUNNING: 'Processing %d/%d' % (done, total),
                 C.ST_PAUSED: ('Paused: %s' % why) if why else 'Paused',
                 C.ST_STOPPING: 'Stopping', C.ST_STOPPED: 'Stopped %d/%d' % (done, total),
                 C.ST_DONE: 'Done %d/%d' % (done, total),
                 C.ST_ERROR: self.job.get('error') or 'Error'}
        return names.get(state, '')

    def _publish_groups(self, present):
        """Two files, one gen: 'groups-all' (every listed group up to GRP_ALL_MAX, newest first) feeds the page and
        its stacked "Show older groups" pages, 8 per page (owner 2026-11-04: more than 8 groups happen). The 544-byte
        'groups' (the first 8, never GF_MORE) stays for a GUI that predates groups-all."""
        sel = set(self.job['selection'])
        flags = 0
        if self.gates.host_attached():
            flags |= C.GF_EXPORT
        if self.gates.blocked:
            flags |= C.GF_BLOCKED
        listed = self.groups[:C.GRP_ALL_MAX]
        # 'gen' is a CONTENT version, not a clock: bump only when the listed groups' ordered id sequence actually
        # changes (groups added/removed/reordered). Per-group count/selection changes do NOT bump it, so an open
        # page keeps reconciling instead of flashing "List changed" every ~6 s the way a clock-based gen did. Every
        # page pins it on open and treats a mismatch as "reopen" (each page shows a slice of this one sequence).
        sig = tuple(gid for _vk, _vr, _sto, _g, gid in listed)
        if sig != self._gen_sig:
            self._gen_sig = sig
            self._gen = (self._gen + 1) & 0xffffffff
        current = {f.get('gid') for f in self._inflight()} | ({self._current} if self._current is not None else set())
        recs = []
        for volkey, volroot, sto, g, gid in listed:
            rf = 0
            if gid in sel:
                rf |= C.RF_SELECTED
            if gid in current:
                rf |= C.RF_CURRENT
            if g['pending'] == 0 and g['done'] > 0:
                rf |= C.RF_COMPLETE
            if g['failed'] > 0:
                rf |= C.RF_FAILURES
            recs.append({'id': gid, 'total': g['pending'] + g['done'], 'done': g['done'],
                         'failed': g['failed'], 'flags': rf, 'storage': sto, 'title': self._title(g)})
        durable.atomic_write(os.path.join(self.run_dir, 'groups-all'),
                             C.pack_groups_all(self._gen, flags, self._req_seq, len(self.groups), recs),
                             dir_fsync=False)
        durable.atomic_write(os.path.join(self.run_dir, 'groups'),
                             C.pack_groups(self._gen, flags, self._req_seq, 0, len(self.groups), 0,
                                           recs[:C.GRP_MAX]), dir_fsync=False)


def precheck_recover(argv, data_root=C.DATA, run_dir=C.RUN, list_volumes=None, converter=None, gates=None):
    """One-shot precheck for a layer apply/restore (hotpatch --precheck), DESIGN §7: take the lock, run §5.4
    recovery on every present volume, then REFUSE (exit 1, printing the names) while any journal/.x6j/STUCK
    remains or any work dir is non-empty -- so a layer change never happens on top of an unresolved or torn
    frame. --ack-lost lets an absent-volume journal be accepted (an SSD gone for good); it never waives a
    STUCK frame or leftover on a PRESENT volume. Returns 0 to allow the layer change, 1 to refuse."""
    ack_lost = '--ack-lost' in argv
    os.makedirs(run_dir, exist_ok=True)
    lock = durable.try_lock_exclusive(os.path.join(run_dir, 'lock'))
    if lock is None:
        sys.stderr.write('x6-pureshot-processor is running; stop it before the layer change\n')
        return 1
    try:
        p = Processor(data_root=data_root, run_dir=run_dir,
                      list_volumes=list_volumes or default_list_volumes,
                      converter=converter, gates=gates)
        present = {v['volkey']: v['volroot'] for v in p.list_volumes()}
        for volkey, volroot in present.items():
            frame.recover_volume(p._frame_ctx(), volkey, volroot)
        refuse, lost_ok = [], []
        for rec in journal.iter_js(data_root):                   # live J left after recovery = unresolved
            line = 'J %s %s (%s)' % (rec['volkey'], rec.get('rel'), rec.get('state'))
            (lost_ok if rec['volkey'] not in present else refuse).append(line)
        for s in journal.list_stuck(data_root):                  # STUCK is never waived
            refuse.append('STUCK %s %s' % (s['volkey'], s.get('rel')))
        for volkey, volroot in present.items():
            wd = journal.work_dir(volroot)
            try:
                left = [n for n in os.listdir(wd) if n != 'quarantine']
            except OSError:
                left = []
            if left:
                refuse.append('W %s not empty: %s' % (volroot, ', '.join(sorted(left))))
    finally:
        os.close(lock)                                           # release the gate; this is a one-shot
    if refuse or (lost_ok and not ack_lost):
        sys.stderr.write('refusing layer change; unresolved work (use --ack-lost only for an absent volume):\n'
                         + '\n'.join(refuse + lost_ok) + '\n')
        return 1
    if lost_ok:
        sys.stderr.write('--ack-lost: accepting pending work on absent volume(s):\n' + '\n'.join(lost_ok) + '\n')
    return 0


def acquire_run_lock(run_dir, lock_path):
    """Take the singleton lock. /run is tmpfs and empty on every boot, so the run dir must be created first --
    otherwise opening the lock file fails with ENOENT and the service crash-loops before doing anything."""
    os.makedirs(run_dir, mode=0o700, exist_ok=True)
    return durable.try_lock_exclusive(lock_path)


def main():
    if '--recover-once' in sys.argv[1:]:
        return precheck_recover(sys.argv[1:])
    lock = acquire_run_lock(C.RUN, C.RUN_LOCK)
    if lock is None:
        sys.stderr.write('another x6-pureshot-processor holds the lock\n')
        return 1
    preview.restore_leftover(C.RUN)          # a preview an earlier instance left paused is started again first
    p = Processor()
    pause = preview.PreviewPause(lambda: p._pause_wanted, C.RUN, usb=p.gates.host_attached,
                                 page=lambda: preview.page_open(C.RUN))
    pause.start()
    while True:
        did = p.step()
        if did and did[0] == frame.R_DONE and p.job.get('running'):
            continue                        # frames overlap: read the next one while the converter is busy
        time.sleep(0.5 if p.job.get('running') else 2.0)


if __name__ == '__main__':
    sys.exit(main())
