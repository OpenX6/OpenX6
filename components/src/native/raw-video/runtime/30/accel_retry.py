"""Acquire the first accelerometer sample without blocking the recording thread."""
import os
import threading
import time

from motion_stream import MotionStream


class AccelCapture:
    def __init__(self, path, timeout=1.5, attempts=3):
        self.path = path
        self.timeout = timeout
        self.attempts = attempts
        self.stop = threading.Event()
        self.history = []
        self.result = None
        self.failure = None
        self.thread = threading.Thread(target=self.run, daemon=True)
        self.thread.start()

    def run(self):
        try:
            self.collect()
        except Exception as exc:
            self.failure = str(exc)

    def collect(self):
        for attempt in range(1, self.attempts + 1):
            if self.stop.is_set():
                break
            try:
                stream = MotionStream(self.path, 'accel')
            except OSError as exc:
                self.history.append(dict(attempt=attempt, error=str(exc)))
                if self.path.exists():
                    self.path.unlink()
            else:
                deadline = time.monotonic() + self.timeout
                while not (stream.ready.is_set() or self.stop.is_set()):
                    if stream.proc.poll() is not None or time.monotonic() >= deadline:
                        break
                    stream.ready.wait(.05)
                if stream.ready.is_set():
                    self.stop.wait()
                returncode = stream.proc.poll()
                result = stream.finish()
                # Data arriving while finish() drains the reader belongs to this take.
                if result['samples']:
                    self.history.append(dict(attempt=attempt, result='received'))
                    self.result = result
                    return
                reason = ('stopped_before_first_sample' if self.stop.is_set() else
                          'client_exited' if returncode is not None else 'first_sample_timeout')
                self.history.append(dict(attempt=attempt, error=result['error'] or reason,
                                         exit_code=returncode))
                self.path.unlink()
            if self.stop.wait(.1):
                break

    def finish(self):
        self.stop.set()
        self.thread.join(4)
        if self.thread.is_alive():
            raise RuntimeError('Accelerometer did not finish before timeout')
        if self.failure is not None:
            raise RuntimeError(self.failure)
        if self.result is None:
            with self.path.open('w') as f:
                f.write('counter_ticks,x,y,z\n')
                f.flush()
                os.fsync(f.fileno())
            self.result = dict(status='unavailable', samples=0, first_counter_ticks=None,
                               last_counter_ticks=None,
                               error=self.history[-1]['error'] if self.history else 'stopped_before_first_sample')
        return dict(self.result, attempts=self.history)
