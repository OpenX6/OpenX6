#!/usr/bin/env python3
"""Gates and interruption handling (DESIGN-v3 §5.5).

None of this destroys data (frame.py owns that); gates only decide when it is safe to run ps2native and when
to pause/abort it. All camera-specific probes are injectable so the decision logic is unit-testable offline;
on the camera the defaults read sysfs/proc and run one freshness-bounded `ins_bll_cli share_state media`
probe. abort_reason() is polled every 0.25 s by the converter; ok_to_run()/idle_fresh() gate launch and the
critical section. SSD frames keep running across a USB host attach (UDisk exports only internal + SD).

Threads (pipelined daemon, 2026-11-04): a converter watchdog thread polls abort_reason() while the main thread
verifies/overwrites the previous frame and reads the next one. The idle-probe state machine is the only shared
state that needs it: every touch of it holds _plock, and no lock is held while sleeping or scanning /proc. The
rest is single-assignment state (latches, last-seen times) that is safe under the GIL."""
import os, re, subprocess, threading, time
import contract as C

IDLE_MAX_AGE = 1.5          # newest completed idle probe must be this fresh (s); > on-camera p99 (§10.2)
IDLE_FRESH_LAUNCH = 0.5     # require a <=0.5 s old idle answer right before launch / critical section
PROBE_MIN_INTERVAL = 0.5    # don't launch a new probe more often than this
IDLE_ABORT_AGE = 3.0        # in-conversion abort tolerance: a probe slowed by ps2native's CPU load must not
                            # spuriously abort; a camera that really became busy is still caught within ~3 s
IDLE_PROBE_WAIT = 5.0       # bounded wait for ONE synchronous probe (> the probe's own `timeout -k 1 3`)
USB_DETACH_HOLD = 12.0      # internal/SD stay untouched this long after the USB host is gone (DESIGN §5.5 F9: >= 10 s,
                            # so the owner's export retry succeeds). Measured from the last time the daemon SAW the
                            # host, and the idle loop samples every 2 s -> 12 s guarantees >= 10 s after the real
                            # unplug (unit 5 live check: a 10 s hold resumed 9.3 s after the cable came out). Also rides
                            # out the ~0.3 s 'not attached' blip the UDC shows while the camera switches USB modes.
# Freshness is always stamped at a probe's LAUNCH time (its answer describes the camera at about that moment),
# never at the time we happened to collect it -- a probe reaped late must not look fresh.
MEM_MIN = 2300 * 1024 * 1024                 # launch floor: headroom for ps2native's ~2 GiB working set
# During-conversion abort floor: MUST stay well below MEM_MIN - ps2native's peak RSS, or the abort gate
# SIGKILLs ps2native by its OWN allocation. At a floor launch MemAvailable ~= 2300-2048 = ~252 MiB during the
# run, so 256 would self-abort; 128 MiB keeps clear margin. It is only a last-ditch near-OOM backstop
# (ps2native also gets oom_score_adj=800 so the kernel kills it first under true pressure), and a false abort
# is safe (F stays the untouched original). Phase 8 (8.5) calibrates this against the real measured peak.
MEM_ABORT = 128 * 1024 * 1024
SPACE_RESERVE = (100 + 512) * 1024 * 1024    # 100 MiB DCF reserve + 512 MiB margin
IDLE_TOKENS = ('CaptureIdle', 'RecordIdle', 'FileRepairIdle')
BATT_START, BATT_PAUSE = 30, 20              # §5.5: start >=30% or powered; pause <20% (hysteresis)
HEAT_PAUSE, HEAT_RESUME = 78.0, 70.0         # soc-chip-therm degC; vendor stopped a recording ~77.8 (INT-8)


def _default_udc_states():
    base = '/sys/class/udc'
    out = []
    try:
        for d in os.listdir(base):
            try:
                with open(os.path.join(base, d, 'state')) as f:
                    out.append(f.read().strip())
            except OSError:
                pass
    except OSError:
        pass
    return out


def _default_mem_available():
    try:
        with open('/proc/meminfo') as f:
            for ln in f:
                if ln.startswith('MemAvailable:'):
                    return int(ln.split()[1]) * 1024
    except OSError:
        pass
    return 1 << 62          # unknown -> don't block on memory


def _default_battery():
    """(capacity_pct or -1 if unknown, externally_powered). Camera-confirmed paths (unit 5 2026-11-03):
    /sys/class/power_supply/battery/capacity; powered = any non-Battery supply with online==1
    (usb / USB_PD / ucsi), since battery 'status' can read Discharging while a charger is attached."""
    cap = -1
    try:
        with open('/sys/class/power_supply/battery/capacity') as f:
            cap = int(f.read().strip())
    except (OSError, ValueError):
        pass
    powered = False
    try:
        for d in os.listdir('/sys/class/power_supply'):
            if d == 'battery':
                continue
            try:
                with open('/sys/class/power_supply/%s/online' % d) as f:
                    if f.read().strip() == '1':
                        powered = True
                        break
            except OSError:
                pass
    except OSError:
        pass
    return cap, powered


def _default_soc_temp():
    """soc-chip-therm in degC, scanned by type (zone index varies: it was thermal_zone57 on unit 5). None
    if not found -> heat gate is then a no-op (don't block on a missing sensor)."""
    base = '/sys/class/thermal'
    try:
        for z in os.listdir(base):
            if not z.startswith('thermal_zone'):
                continue
            try:
                with open('%s/%s/type' % (base, z)) as f:
                    if f.read().strip() != 'soc-chip-therm':
                        continue
                with open('%s/%s/temp' % (base, z)) as f:
                    return int(f.read().strip()) / 1000.0
            except (OSError, ValueError):
                pass
    except OSError:
        pass
    return None


def _default_free_bytes(volroot):
    try:
        s = os.statvfs(volroot)
        return s.f_bavail * s.f_frsize
    except OSError:
        return 0


def _default_present(volroot):
    # presence by the mountpoint's device, not by mount text (design: judge by device, not mount string)
    try:
        os.stat(volroot)
        return True
    except OSError:
        return False


def _default_reader_active(volroot, exclude_pgid=None):
    """A Wi-Fi download / USB file-transfer reader holding a DCIM file open under this volume. Best-effort
    scan of /proc/*/fd; absence of /proc (host tests) -> False.

    The daemon's OWN converter MUST be excluded or this gate trips on itself: ps2native holds the original DNG
    (which lives under DCIM/) open for reading for the whole ~6 s conversion, so without the exclusion the
    0.25 s abort poll SIGKILLs it immediately and no frame ever converts. exclude_pgid skips every fd of the
    converter's own process group -- ps2native is alone in its setsid session, so this is exactly our own
    converter and nothing else; a genuine external reader of the same file is in a different group and still
    pauses us (we must NOT skip it by path, or a real USB/Wi-Fi reader would be missed)."""
    try:
        pids = [p for p in os.listdir('/proc') if p.isdigit()]
    except OSError:
        return False
    root = os.path.realpath(volroot)
    me = str(os.getpid())       # the daemon itself: with frames overlapped it reads/overwrites DCIM files while the
                                # converter runs, and must not abort its own conversion as a "file transfer"
    for p in pids:
        if p == me:
            continue
        if exclude_pgid is not None:
            try:
                if os.getpgid(int(p)) == exclude_pgid:
                    continue
            except (OSError, ValueError):
                continue
        fdd = '/proc/%s/fd' % p
        try:
            for fd in os.listdir(fdd):
                try:
                    tgt = os.readlink(os.path.join(fdd, fd))
                except OSError:
                    continue
                if tgt.startswith(root + '/') and '/DCIM/' in tgt:
                    return True
        except OSError:
            continue
    return False


class Gates(object):
    def __init__(self, udc_states=_default_udc_states, mem_available=_default_mem_available,
                 free_bytes=_default_free_bytes, present=_default_present,
                 reader_active=_default_reader_active, run_idle_probe=None, clock=time.monotonic,
                 battery=_default_battery, soc_temp=_default_soc_temp):
        self._udc = udc_states
        self._mem = mem_available
        self._free = free_bytes
        self._present = present
        self._reader = reader_active
        self._clock = clock
        self._battery = battery
        self._soc = soc_temp
        self._hot = False               # heat hysteresis latch
        self._low_batt = True           # start latched low -> first run needs >=BATT_START% or power (§5.5)
        self._run_probe = run_idle_probe or self._spawn_idle_probe
        self._plock = threading.Lock()  # guards the idle-probe state below (watchdog thread + main thread)
        self._proc = None
        self._proc_launched = -1e9      # launch time of the in-flight probe (its answer is stamped with this)
        self._last_idle_ts = -1e9
        self._last_result = None
        self._last_done_launch = -1e9   # launch time of the newest COLLECTED probe (idle or not)
        self._last_probe_launch = -1e9
        self._last_attached = -1e9      # last time a USB host was seen attached (drives the detach hold)
        self.stop = False
        self.blocked = set()
        self._active_pgid = None        # converter's process group, excluded from the reader gate

    # ---- battery / heat (camera-confirmed paths) ----
    def _heat_block(self):
        t = self._soc()
        if t is None:
            return False
        if t >= HEAT_PAUSE:
            self._hot = True
        elif t < HEAT_RESUME:
            self._hot = False
        return self._hot

    def _battery_block(self):
        cap, powered = self._battery()
        if powered:
            self._low_batt = False
            return None
        if cap <= 0:                    # capacity 0/unknown + no charger -> require power (AGENTS 2026-10-31)
            return 'battery unknown'
        if cap < BATT_PAUSE:
            self._low_batt = True
        elif cap >= BATT_START:
            self._low_batt = False
        return 'low battery' if self._low_batt else None

    # ---- idle freshness ----
    # Camera-confirmed 2026-11-03 (unit 5): ins_bll_cli is a JSON-command shell, not `verb arg`. The media
    # state query is this exact JSON argv; its output contains "CaptureIdle"/"RecordIdle"/"FileRepairIdle"
    # (data.capture.capture_status, data.idle_analysis.record_status, data.file_repair.repair_state) when idle.
    # `ins_bll_cli share_state media` parse-errors ("invalid literal") -> would read as never-idle, so the
    # daemon would never run; that is the form to NOT use.
    IDLE_PROBE_CMD = ('timeout', '-k', '1', '3', 'ins_bll_cli',
                      '{"cmd":"share_state","action":"get","type":"media"}')

    def _spawn_idle_probe(self):
        try:
            return subprocess.Popen(list(self.IDLE_PROBE_CMD), stdin=subprocess.DEVNULL,
                                    stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
        except OSError:
            return None

    def _launch(self, now):
        self._last_probe_launch = now
        self._proc_launched = now
        self._proc = self._run_probe()
        if self._proc is None:
            self._last_result = False

    def _reap(self):
        """Collect a finished probe; an idle answer is stamped with that probe's LAUNCH time."""
        if self._proc is None:
            return
        rc = self._proc.poll()
        if rc is None:
            return
        out = b''
        try:
            out = self._proc.stdout.read() or b''
        except Exception:
            pass
        launched = self._proc_launched
        self._proc = None
        text = out.decode('utf-8', 'replace')
        idle = rc == 0 and all(tok in text for tok in IDLE_TOKENS)
        self._last_result = idle
        self._last_done_launch = max(self._last_done_launch, launched)
        if idle:
            self._last_idle_ts = max(self._last_idle_ts, launched)

    def poll_idle(self):
        """Background freshness, non-blocking. Reap a finished probe FIRST, then keep one in flight -- the old
        launch-then-reap order left a reaped slot empty until the following call, so in the 2 s idle loop the
        newest answer was ~2 s old half the time and an idle camera read as busy."""
        with self._plock:
            now = self._clock()
            self._reap()
            if self._proc is None and now - self._last_probe_launch >= PROBE_MIN_INTERVAL:
                self._launch(now)
                self._reap()                              # an instant probe completes within the same call

    def reap_idle(self):
        """Collect a finished probe without launching a new one (the daemon calls this while no job runs, so an
        idle camera is not probed at all: every ins_bll_cli run registers an ibus program, and the GUI then
        spammed erpc errors trying to reach the exited ones)."""
        with self._plock:
            self._reap()

    def check_idle_now(self, wait=IDLE_PROBE_WAIT):
        """Synchronous, bounded idle check for a DECISION point (frame launch on a stale cache, and the critical
        section): run a FRESH probe and decide on its own answer, never on a possibly stale cached one. Fails
        closed (False) if no fresh idle answer arrives within `wait`. The lock is held only for each state
        touch, never across a sleep: the converter watchdog keeps polling meanwhile, and if it reaps OUR probe
        the answer is still ours (_last_done_launch >= our launch)."""
        tries = int(wait / 0.05) + 1
        launched = None
        for _ in range(tries):                            # let an older in-flight probe finish (it is time-bounded)
            with self._plock:
                self._reap()
                if self._proc is None:
                    launched = self._clock()
                    self._launch(launched)
                    self._reap()
                    break
            time.sleep(0.05)
        if launched is None:
            return False
        for _ in range(tries):
            with self._plock:
                self._reap()
                if self._last_done_launch >= launched:
                    return self._last_idle_ts >= launched
            time.sleep(0.05)
        return False

    def idle_fresh(self, volroot=None):
        """Decision gate right before launching ps2native and before the in-place overwrite. An idle answer whose
        probe was launched <= IDLE_MAX_AGE ago is used as is: with frames overlapped the converter watchdog keeps
        one probe in flight all the time, so the answer is ~0.6-1.2 s old, and a second synchronous probe would add
        ~0.6 s per decision (measured on unit 5) for ~1 s more certainty. Otherwise a fresh synchronous probe."""
        if self._idle_ok(IDLE_MAX_AGE):
            return True
        return self.check_idle_now()

    def _idle_ok(self, max_age=IDLE_MAX_AGE):
        return (self._clock() - self._last_idle_ts) <= max_age

    # ---- USB host ----
    def host_attached(self):
        attached = any(s and s != 'not attached' for s in self._udc())
        if attached:
            self._last_attached = self._clock()
        return attached

    def usb_hold(self):
        """A USB host is attached, or left less than USB_DETACH_HOLD ago. While this holds, internal and SD must
        not be touched at all -- no rescan (it opens every DNG), no recovery, no new frame -- because the camera
        may be unmounting them for export at any instant, and one open fd makes that unmount fail (EBUSY).
        Test 6.3 (2026-11-04) caught the 5 s rescan opening internal DNGs while the owner chose a USB mode."""
        if self.host_attached():
            return True
        return (self._clock() - self._last_attached) < USB_DETACH_HOLD

    def usb_blocks(self, volroot):
        """usb_hold() applies only to internal + SD: UDisk exports exactly those two (INT-5); SSD frames go on."""
        return self.storage(volroot) != C.STO_SSD and self.usb_hold()

    # ---- storage ----
    def storage(self, volroot):
        for root, sto in C.STORAGE_ROOTS:
            if os.path.realpath(volroot) == os.path.realpath(root):
                return sto
        return C.STO_INTERNAL

    # ---- composite gates used by frame.py ----
    def ok_to_run(self, volroot, fresh=True):
        """fresh=True (frame launch): a stale cached idle answer is re-checked with one synchronous probe, so an
        idle camera is not refused just because the background answer aged. fresh=False (status display only):
        never blocks, decides on the cached answer."""
        if volroot in self.blocked:
            return (False, 'blocked')
        if self.stop:
            return (False, 'stopped')
        if self.usb_blocks(volroot):            # before presence: an exported volume reads "USB connected"
            return (False, 'USB connected')
        if not self._present(volroot):
            return (False, 'volume gone')
        # The launch floor guards ps2native's ~2 GiB working set. While OUR conversion runs, its own RSS is already
        # out of MemAvailable, so reading the next frame (V0) would wrongly refuse; the floor is checked again at
        # that frame's launch, after this conversion has exited. A genuine near-OOM still aborts via MEM_ABORT.
        if self._active_pgid is None and self._mem() < MEM_MIN:
            return (False, 'low memory')
        b = self._battery_block()
        if b:
            return (False, b)
        if self._heat_block():
            return (False, 'too hot')
        self.poll_idle()
        if not self._idle_ok() and not (fresh and self.check_idle_now()):
            return (False, 'camera busy')
        if self._reader_blocks(volroot):
            return (False, 'file transfer in progress')
        return (True, '')

    def space_ok(self, volroot, need):
        return self._free(volroot) >= need + SPACE_RESERVE

    def abort_reason(self, volroot):
        """Polled every 0.25 s while ps2native runs; return the first reason to SIGKILL it, else None."""
        if self.stop:
            return 'stopped'
        if volroot in self.blocked:
            return 'blocked'
        if not self._present(volroot):
            return 'volume gone'
        self.poll_idle()
        if not self._idle_ok(IDLE_ABORT_AGE):            # tolerant: a slow probe under load must not abort
            return 'camera busy'
        if self.storage(volroot) != C.STO_SSD and self.host_attached():
            return 'USB connected'
        if self._reader_blocks(volroot):
            return 'file transfer in progress'
        if self._mem() < MEM_ABORT:
            return 'low memory'
        if self._heat_block():
            return 'too hot'
        b = self._battery_block()
        if b:
            return b
        return None

    # ---- reader gate, aware of our own converter ----
    def begin_conversion(self, pgid):
        """The converter calls this around ps2native so the reader gate ignores our own converter's fds."""
        self._active_pgid = pgid

    def end_conversion(self):
        self._active_pgid = None

    def _reader_blocks(self, volroot):
        return self._reader(volroot, self._active_pgid)

    def block_volume(self, volroot):
        self.blocked.add(volroot)

    def unblock_volume(self, volroot):
        self.blocked.discard(volroot)
