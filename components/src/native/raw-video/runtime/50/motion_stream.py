"""Read timestamped samples from the camera's calibrated Qualcomm sensor service."""
import errno
import os
import re
import subprocess
import threading

SAMPLE = re.compile(rb'\s*([-+\d.eE]+)\s+([-+\d.eE]+)\s+([-+\d.eE]+)\s+timestamps:\s*(\d+)\s*')

class MotionStream:
    def __init__(self, path, sensor):
        self.ready = threading.Event()
        self.samples = 0
        self.first = self.last = self.error = None
        self.output = path.open('x')
        try:
            self.output.write('counter_ticks,x,y,z\n')
            fs = os.statvfs(path.parent)
            seconds = 60 + fs.f_bavail * fs.f_frsize // (30 * 49 * 4096)
            master, slave = os.openpty()
            self.source = os.fdopen(master, 'rb', buffering=0)
            try:
                self.proc = subprocess.Popen(['sns_client_example_cpp', sensor, '200', '0', str(seconds), '0'],
                                             stdout=slave, stderr=slave, close_fds=True)
            except BaseException:
                self.source.close()
                raise
            finally:
                os.close(slave)
            self.reader = threading.Thread(target=self.read)
            try:
                self.reader.start()
            except BaseException:
                self.proc.kill()
                self.proc.wait()
                self.source.close()
                raise
        except BaseException:
            self.output.close()
            raise

    def read(self):
        pending = b''
        try:
            while True:
                data = self.source.read(32768)
                if not data:
                    break
                lines = (pending + data).split(b'\n')
                pending = lines.pop()
                for line in lines:
                    match = SAMPLE.fullmatch(line)
                    if match:
                        x, y, z, timestamp = match.groups()
                        tick = int(timestamp)
                        self.output.write(b','.join([timestamp, x, y, z]).decode('ascii') + '\n')
                        if self.first is None:
                            self.first = tick
                        self.last = tick
                        self.samples += 1
                        self.ready.set()
        except OSError as exc:
            if exc.errno != errno.EIO:
                self.error = str(exc)

    def finish(self):
        exited_early = self.proc.poll() is not None
        self.proc.terminate()
        try:
            self.proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            self.proc.wait()
        self.reader.join()
        self.source.close()
        with self.output:
            self.output.flush()
            os.fsync(self.output.fileno())
        status = 'interrupted' if self.error or exited_early else 'ok'
        return dict(status=status if self.samples else 'unavailable', samples=self.samples,
                    first_counter_ticks=self.first, last_counter_ticks=self.last, error=self.error)
