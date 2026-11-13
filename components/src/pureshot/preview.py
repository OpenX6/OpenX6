#!/usr/bin/env python3
"""Pause the camera's live preview while a PureShot job runs with the screen dark (heat; owner, 2026-11-04).

Measured on unit 5: with the screen dark the camera keeps its live preview running (qmmf ~1.2 cores). Under the
conversion load it costs ~0.63 W of ~9.9 W (9.89 -> 9.26 W), about 11 % more frames per hour once a long job is
heat-paced. Only StreamTypePreview is touched -- the same native call x6-preview-control makes while recording --
never the RAW photo stream (slot 7) or anything else.

One thread, polling every POLL s:
  pause    the daemon wants it (a job is running), the screen has been dark >= SCREEN_DARK_HOLD, no USB host, no
           phone app connected, and a fresh media query says the camera is idle with the preview running. The
           ownership marker is written first and a guard (its own transient unit, outside this service's cgroup)
           starts the preview again if this process dies while holding it; the daemon also does so at start-up.
  restore  at once when the daemon stops wanting it, a USB host attaches or the phone app connects.
  wake     the screen lights up -- a shutter press does that too (unit 5, 2026-11-04), so the camera is asked
           first: idle, or an Interval countdown (CaptureCounting: the shot comes ~5 s later) -> start the preview
           NOW, so a shot is always taken with the live view running; any other capture state, or a recording
           starting -> hand over: no call, and the preview is started as soon as the camera is idle again.
  busy     while dark, the camera leaving idle (a capture without the screen waking) -> hand over too.
  Why: owner test 2026-11-04, round 3 -- an Interval shot taken with the preview paused saved fine, but the camera
  never restarted the preview afterwards and qmmf restarted ~14 s later; a recording start brings the preview up
  by itself (and restarting under it is the kind of stream switch that crashed qmmf on this firmware). So after
  every shot/recording the preview is back to normal; a later dark screen pauses it again.
  page     (owner 2026-11-04: "stop the preview whenever the 16K Fast RAW Processing page is open") the GUI renews a
           heartbeat (/run/x6-pureshot/page) about once a second while one of the page's screens is in front and
           removes it when none is. A fresh heartbeat held for PAGE_HOLD pauses the preview with the screen ON too,
           job or not, under the same USB / phone-app / camera-idle rules. While paused for it the camera is asked
           every MEDIA_EVERY_PAGE: a capture or recording starting -> hand over, an Interval countdown -> start the
           preview now. Leaving the page (Back, a dialog, the live view) or a stale heartbeat (the GUI stopped) ends
           it like the screen waking: ask the camera, then restore or hand over.
Everything here is best effort and fails toward a RUNNING preview: an unreadable screen state never pauses, a failed
stop releases ownership, and every exit path restores."""
import json, os, shlex, stat, struct, subprocess, threading, time

POLL = 0.2
SCREEN_DARK_HOLD = 3.0
MEDIA_EVERY = 2.0          # s between camera-state queries while holding the pause
HANDED_EVERY = 0.5         # s between queries after a hand-over (start the preview within ~1 s of the shot)
RETRY_AFTER_FAIL = 60.0    # s before trying to pause again after a failed stop / guard
REARM_AFTER = 5.0          # s after a restore before pausing again
APP_PORT = 6666            # the Insta360 phone app's control connection (TCP)
MARKER = 'preview-paused'
PAGE_FILE = 'page'         # GUI -> daemon page-open heartbeat (contract.RUN_PAGE): {u64 CLOCK_MONOTONIC s, 'PPGO', u32}
PAGE_STALE = 3.0           # s: the GUI renews it about every second; older than this = the page is not open
PAGE_HOLD = 2.0            # s the page must stay open before pausing (a quick look in and out does nothing)
MEDIA_EVERY_PAGE = 1.0     # s between camera-state queries while paused for the page with the screen on
SLOT_PREVIEW = 0           # pipeline_status index of StreamTypePreview (enum Preview, Main, MainSec, Sub, Wifi, Bt, Jpeg, Raw, Audio)
RUNNING = 3


def call_preview(start):
    """Start/stop ONLY the native preview stream. True on 'Success(0)'."""
    method = 'PipelineStartStream' if start else 'PipelineStopStream'
    try:
        out = subprocess.run(['timeout', '-k', '1', '10', 'ins_ibus_cli', '--run', 'call-method', 'kCamera', 'kMediad',
                              'Pipeline', method, 'StreamTypePreview'], stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                             stderr=subprocess.STDOUT, timeout=15).stdout
    except (OSError, subprocess.SubprocessError):
        return False
    return b'Error code: Success(0)' in out


def query_media():
    """The camera's media share-state ('data' dict), or None."""
    try:
        out = subprocess.run(['timeout', '-k', '1', '8', 'ins_bll_cli', '{"cmd":"share_state","action":"get","type":"media"}'],
                             stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=12).stdout
        r = json.loads(out.decode('utf-8', 'replace'))
        return r.get('data') if r.get('ret') == 'ok' else None
    except (OSError, subprocess.SubprocessError, ValueError):
        return None


def camera_idle(m):
    try:
        return (m['capture']['capture_status'] == 'CaptureIdle' and m['record']['record_status'] == 'RecordIdle'
                and m.get('file_repair', {}).get('repair_state', 'FileRepairIdle') == 'FileRepairIdle')
    except (KeyError, TypeError):
        return False


def countdown(m):
    """An Interval series counting down to its first shot (record idle): the shot is still seconds away."""
    try:
        return m['capture']['capture_status'] == 'CaptureCounting' and m['record']['record_status'] == 'RecordIdle'
    except (KeyError, TypeError):
        return False


def preview_state(m):
    try:
        return m['pipeline']['pipeline_status']['status'][SLOT_PREVIEW]
    except (KeyError, IndexError, TypeError):
        return None


def screen_dark():
    """True only when the panel backlight is powered down (bl_power 4). Unreadable -> False (never pause blind)."""
    try:
        for d in os.listdir('/sys/class/backlight'):
            with open('/sys/class/backlight/%s/bl_power' % d) as f:
                return f.read().strip() == '4'
    except OSError:
        pass
    return False


def app_connected(paths=('/proc/net/tcp', '/proc/net/tcp6')):
    """An ESTABLISHED TCP connection on the phone app's port (the app's live view may use the preview)."""
    port = ':%04X' % APP_PORT
    for p in paths:
        try:
            with open(p) as f:
                next(f, None)
                for ln in f:
                    x = ln.split()
                    if len(x) > 3 and x[1].endswith(port) and x[3] == '01':
                        return True
        except OSError:
            pass
    return False


def page_open(run_dir, clock=time.monotonic, uid=0):
    """True while the GUI's page heartbeat is fresh: a regular, single-link, root-owned 16-byte file {u64 seconds of
    CLOCK_MONOTONIC (the clock time.monotonic reads), 'PPGO', u32 depth}. Anything else -> False (never pause on a
    doubtful stamp)."""
    try:
        fd = os.open(os.path.join(run_dir, PAGE_FILE), os.O_RDONLY | os.O_NOFOLLOW | getattr(os, 'O_CLOEXEC', 0))
    except OSError:
        return False
    try:
        st = os.fstat(fd)
        if not stat.S_ISREG(st.st_mode) or st.st_nlink != 1 or st.st_uid != uid or st.st_size != 16:
            return False
        b = os.read(fd, 17)
    except OSError:
        return False
    finally:
        os.close(fd)
    if len(b) != 16 or b[8:12] != b'PPGO':
        return False
    t = struct.unpack('<Q', b[:8])[0]
    now = clock()
    return t <= now + 1.0 and now - t <= PAGE_STALE


def guard_script(marker, pid):
    return ('M=%s; P=%d; while [ -f "$M" ] && kill -0 "$P" 2>/dev/null; do sleep 1; done; '
            'if [ -f "$M" ]; then timeout -k 1 10 ins_ibus_cli --run call-method kCamera kMediad Pipeline '
            'PipelineStartStream StreamTypePreview; rm -f "$M"; fi') % (shlex.quote(marker), pid)


def spawn_guard(marker, pid, n):
    """A transient unit that starts the preview again if `pid` dies while the marker exists (a crash, a kill, a stop
    -- this service's own cgroup is killed with it, so the guard must live outside it). Exits when the marker goes."""
    try:
        r = subprocess.run(['systemd-run', '--collect', '--unit=x6-pp-preview-guard-%d-%d' % (pid, n),
                            '--property=RuntimeMaxSec=172800', '/bin/sh', '-c', guard_script(marker, pid)],
                           stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=15)
        return r.returncode == 0
    except (OSError, subprocess.SubprocessError):
        return False


def restore_leftover(run_dir, call=call_preview):
    """Daemon start-up: a marker left by an earlier instance means the preview may still be stopped -> start it."""
    m = os.path.join(run_dir, MARKER)
    if os.path.exists(m):
        ok = call(True)
        try:
            os.unlink(m)
        except FileNotFoundError:
            pass
        return 'restored' if ok else 'restore-call-failed'
    return None


class PreviewPause(threading.Thread):
    def __init__(self, want, run_dir, usb=lambda: False, screen=screen_dark, media=query_media, call=call_preview,
                 app=app_connected, guard=spawn_guard, clock=time.monotonic, log=None, pid=None, page=lambda: False):
        super().__init__(name='pp-preview-pause', daemon=True)
        self.want, self.usb, self.screen, self.media, self.call, self.app, self.guard = want, usb, screen, media, call, app, guard
        self.page = page
        self.clock = clock
        self.log = log or (lambda *a: None)
        self.pid = pid or os.getpid()
        self.marker = os.path.join(run_dir, MARKER)
        self.owned = False
        self.handed = False            # handed over to a capture/recording; watch for the camera going idle again
        self.events = []               # (clock, what) for tests and the diagnostic
        self._dark_since = None
        self._page_since = None
        self.for_page = False          # the current pause was taken for the open page (not the dark screen)
        self._not_before = 0.0
        self._last_media = -1e9
        self._n = 0
        self._stop = threading.Event()

    # ---- thread ----
    def run(self):
        while not self._stop.wait(POLL):
            self.safe_tick()

    def safe_tick(self):
        try:
            self.tick()
        except Exception as e:         # never leave the preview stopped because of a bug here
            self._note('error %r' % (e,))
            if self.owned:
                try:
                    self._restore('error')
                except Exception:
                    pass

    def shutdown(self):
        self._stop.set()
        if self.owned:
            self._restore('daemon exit')

    def _note(self, what):
        self.events.append((self.clock(), what))
        del self.events[:-50]
        self.log('preview', what)

    # ---- ownership ----
    def _take(self):
        try:
            fd = os.open(self.marker, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            try:
                os.write(fd, b'%d\n' % self.pid)
            finally:
                os.close(fd)
        except OSError:
            return False
        self._n += 1
        if not self.guard(self.marker, self.pid, self._n):
            self._release()
            return False
        return True

    def _release(self):
        try:
            os.unlink(self.marker)             # the guard sees it gone and exits without a call
        except FileNotFoundError:
            pass

    def _restore(self, why):
        ok = self.call(True)
        self._release()
        self.owned = False
        self._not_before = self.clock() + REARM_AFTER
        self._note('restore (%s): %s' % (why, 'ok' if ok else 'call failed'))

    def _hand_over(self, why):
        self._release()
        self.owned = False
        self.handed = True
        self._last_media = self.clock()
        self._note('hand over (%s)' % why)

    # ---- one poll ----
    def tick(self):
        now = self.clock()
        dark = self.screen()
        if dark:
            if self._dark_since is None:
                self._dark_since = now
        else:
            self._dark_since = None
        page = bool(self.page())
        if page:
            if self._page_since is None:
                self._page_since = now
        else:
            self._page_since = None
        want = bool(self.want())
        if self.owned:
            if not dark and not page:
                m = self.media()                       # a shutter press lights the screen too: ask the camera first
                if m is None or camera_idle(m) or countdown(m):
                    if m is not None and countdown(m):
                        why = 'interval countdown'
                    else:
                        why = 'page closed' if self.for_page else 'screen on'
                    self._restore(why)
                else:
                    self._hand_over('screen lit by a shot/recording')
            elif not page and not want:
                self._restore('job not running')
            elif self.usb():
                self._restore('usb host')
            elif self.app():
                self._restore('phone app')
            elif page and not dark:
                # paused for the open page with the screen on: no wake-up event marks a shutter press here, so ask
                # the camera often -- a shot or recording gets its streams, a countdown gets the preview back now
                if now - self._last_media >= MEDIA_EVERY_PAGE:
                    self._last_media = now
                    m = self.media()
                    if m is not None and not camera_idle(m):
                        if countdown(m):
                            self._restore('interval countdown')
                        else:
                            self._hand_over('capture/recording on the page')
            elif now - self._last_media >= MEDIA_EVERY:
                self._last_media = now
                m = self.media()
                if m is not None and not camera_idle(m):
                    self._hand_over('capture/recording while dark')
            return
        if self.handed:
            if now - self._last_media < HANDED_EVERY:
                return
            self._last_media = now
            m = self.media()
            if m is None or not camera_idle(m):
                return                                 # still shooting: the camera owns its streams
            self.handed = False
            self._not_before = now + REARM_AFTER
            if preview_state(m) == RUNNING:
                self._note('after the shot: camera restarted the preview itself')
            else:                                      # never leave it stopped after a shot (round 3: qmmf restarted)
                ok = self.call(True)
                self._note('after the shot: preview still stopped -> start: %s' % ('ok' if ok else 'call failed'))
            return
        job_go = want and dark and now - self._dark_since >= SCREEN_DARK_HOLD
        page_go = page and now - self._page_since >= PAGE_HOLD
        if not (job_go or page_go) or now < self._not_before:
            return
        if self.usb() or self.app():
            return
        m = self.media()
        if m is None or not camera_idle(m) or preview_state(m) != RUNNING:
            self._not_before = now + REARM_AFTER
            return
        if not self._take():
            self._not_before = now + RETRY_AFTER_FAIL
            self._note('no guard: not pausing')
            return
        if self.call(False):
            self.owned = True
            self.for_page = not job_go
            self._last_media = now
            self._note('pause (page open)' if self.for_page else 'pause (screen dark %.0f s)' % (now - self._dark_since))
        else:
            self._release()
            self._not_before = now + RETRY_AFTER_FAIL
            self._note('stop call failed')
