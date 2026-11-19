"""Headless RAW metering. Commands go to the active sensor's HAL client."""
import errno
import math
import os
from pathlib import Path


def exposure_command(ms, iso):
    return f"U:manualaemode=0,manual_exp={ms},manual_iso={iso}\n"


def start_commands(eye, ms, iso):
    return (f"A:id={eye},psize=7680x7680,pformat=raw10,fpsrange=30-30,sensor_mode=5\n"
            f"W:1\n{exposure_command(ms, iso)}W:1\nP:2147483647\n")


class RawAutoExposure:
    def __init__(self, eye, ms, iso, auto_time, auto_iso):
        self.start_ms = self.ms = ms
        self.start_iso = self.iso = iso
        self.auto_time, self.auto_iso = auto_time, auto_iso
        self.initial = start_commands(eye, ms, iso)
        self.reference = None
        self.last_update = 0
        self.fd = None
        self.finished = False
        os.mkfifo("commands", 0o600)

    def tick(self, stop):
        if self.finished:
            return
        if self.fd is None:
            try:
                self.fd = os.open("commands", os.O_WRONLY | os.O_NONBLOCK)
            except OSError as exc:
                if exc.errno == errno.ENXIO:  # The client is still opening the pipe.
                    return
                raise
            self.send(self.initial)
        if stop:
            self.send("D\nQ\n")
            self.finished = True
            self.close()
            return
        try:
            fields = Path("ae-meter").read_text().split()
        except FileNotFoundError:
            return
        frame, level = int(fields[0]), float(fields[1])
        if frame < 70 or frame - self.last_update < 21:
            return
        if self.reference is None:
            self.reference = max(1., level)
        error = math.log2(self.reference / max(.5, level))
        if abs(error) < .12:
            return
        budget = self.ms * self.iso * 2 ** max(-1.6, min(1.6, .85 * error))
        ms, iso = self.start_ms, self.start_iso
        if self.auto_time:
            choice = ms * math.sqrt(budget / (ms * iso)) if self.auto_iso else budget / iso
            ms = max(1, min(32, round(choice)))
        if self.auto_iso:
            iso = max(100, min(6400, round(budget / ms)))
            if self.auto_time:
                ms = max(1, min(32, round(budget / iso)))
        if (ms, iso) != (self.ms, self.iso):
            self.send(exposure_command(ms, iso))
            self.ms, self.iso, self.last_update = ms, iso, frame

    def send(self, command):
        data = command.encode("ascii")
        if os.write(self.fd, data) != len(data):
            raise OSError("Incomplete RAW exposure command")

    def close(self):
        if self.fd is not None:
            os.close(self.fd)
            self.fd = None
