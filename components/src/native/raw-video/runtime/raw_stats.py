"""Publish RAW telemetry through the existing shooting-page overlay."""
import json
import os
import shutil
import struct
import time
from collections import deque
from pathlib import Path

ROOT = Path('/run/x5-raw-mode')
RAW = ROOT / 'raw-stats'
STOCK = Path('/run/x6-stats-overlay')
CARD = Path('/mnt/quickreader')
from capture_window import RESERVE

# Bytes per stereo pair from unit-five sealed footage and same-frame profile replays.
# Used only until this card supplies a measurement for the selected profile.
REFERENCE_PAIR_BYTES = {31: 29000000, 32: 29000000, 30: 853196800 / 35, 50: 13_794_509, 60: 10_430_874}


def select_source(directory):
    link = ROOT / 'os'
    if link.is_symlink() and os.readlink(link) == str(directory):
        return
    tmp = ROOT / 'os.next'
    tmp.unlink(missing_ok=True)
    tmp.symlink_to(directory)
    tmp.replace(link)


def duration(seconds):
    seconds = max(0, int(seconds))
    return '%d:%02d' % divmod(seconds, 60)


def temperature_lines(now):
    # The ordinary Stats producer already handles the temperature-row tap
    # and reads the two sensors and battery only when expanded.
    try:
        data = (STOCK / 'status').read_bytes()
        stamp, = struct.unpack_from('<Q', data)
        if 0 <= now // 1_000_000_000 - stamp <= 4:
            lines = data[16:].split(b'\0', 1)[0].decode('ascii').splitlines()
            for i, line in enumerate(lines):
                if line.startswith('SoC '):
                    extra = lines[i+1:i+2]
                    return [line] + [s for s in extra if s.startswith('S1 ')]
    except (OSError, struct.error, UnicodeError):
        pass
    return ['SoC -- C']


class Card:
    def __init__(self):
        self.key = None
        self.averages = {}
        self.last_speed = None

    def read(self):
        from storage import selected_external
        spec, _ = selected_external(CARD)
        speed = (Path(spec['path']) / 'speed').read_text().strip()
        key = (spec['boot'], spec['diskseq'], spec['uuid'], speed)
        if key != self.key:
            self.key = key
            self.averages.clear()
            self.last_speed = None
        return float(speed) / 1000, shutil.disk_usage(CARD).free

    def pair_size(self, fps):
        if fps not in self.averages:
            for receipt in sorted(CARD.glob('RAWVIDEO-*/seal.json'), reverse=True):
                try:
                    data = json.loads(receipt.read_text())
                    if data.get('layout')=='single-bayer-v1':
                        if {31:'CAM0',32:'CAM1'}.get(fps)==data['eye'] and data['frames']>0:
                            self.averages[fps]=data['bytes']/data['frames']
                            break
                        continue
                    if data['fps_setting'] == fps and data['pairs'] > 0 and data['bytes'] > 0:
                        if fps in (50,60):
                            with (receipt.parent/'capture.x6b').open('rb') as source:
                                if source.read(8) != {50:b'X6M50AB2',60:b'X6QBLK0B'}[fps]:
                                    continue
                        self.averages[fps] = data['bytes'] / data['pairs']
                        break
                except (ValueError, KeyError, OSError):
                    continue
            else:
                self.averages[fps] = REFERENCE_PAIR_BYTES[fps]
        return self.averages[fps]


class Writes:
    def __init__(self, run):
        self.run = run
        self.offset = 0
        self.pending = b''
        self.count = 0
        self.highest = None
        self.size = 0
        self.recent = deque()
        self.first_start = None
        self.last_end = None
        self.first_id = None

    def update(self, now):
        try:
            if self.first_id is None:
                self.first_id = int((self.run / 'first-request').read_text())
        except (OSError, ValueError):
            pass
        try:
            with (self.run / 'card-data/bench.log').open('rb') as f:
                f.seek(self.offset)
                data = f.read(1024 * 1024)
                self.offset = f.tell()
        except FileNotFoundError:
            return
        lines = (self.pending + data).split(b'\n')
        self.pending = lines.pop()
        for line in lines:
            if not line.startswith(b'disk,'):
                continue
            _, req, _, size, begin, end = line.split(b',')
            req, size, begin, end = map(int, (req, size, begin, end))
            self.count += 1
            self.highest = max(self.highest or req, req)
            self.size += size
            self.recent.append((end, size))
            if self.first_start is None:
                self.first_start = begin
            self.last_end = end
        while self.recent and self.recent[0][0] <= now - 1_000_000_000:
            self.recent.popleft()

    def speed(self, now):
        if self.first_start is None:
            return None
        span = max(1, min(1_000_000_000, now - self.first_start))
        return sum(size for _, size in self.recent) * 1000 / span

    def gaps(self):
        if self.first_id is None or self.highest is None:
            return 0
        return self.highest - self.first_id + 1 - self.count


class Overlay:
    def __init__(self):
        RAW.mkdir(exist_ok=True)
        select_source(STOCK)
        self.card = Card()
        self.writer = None
        self.next_update = 0
        self.previous = None

    def update(self, fps, run, stopping):
        state = (fps, run, stopping)
        now = time.monotonic_ns()
        if state == self.previous and now < self.next_update:
            return
        self.previous = state
        self.next_update = now + (500_000_000 if run else 1_000_000_000)
        if not fps:
            select_source(STOCK)
            self.writer = None
            return
        if run and (self.writer is None or self.writer.run != run):
            self.writer = Writes(run)
        w = self.writer if run else None
        label = f'{fps}p'
        if fps in (31, 32):
            label = ('8K30 F' if fps == 32 else '8K30 B') if run else '8K Single'
        unit = 'frames' if fps in (31,32) else 'pairs'
        if w:
            w.update(now)
        try:
            link, free = self.card.read()
            avg = self.card.pair_size(fps)
            if w and w.count:
                avg = w.size / w.count
            usable = max(0, free - RESERVE)
            remaining = duration(usable / ((30 if fps in (31,32) else fps) * avg)) if avg else '--'
            space = f'Free {free/1e9:.0f}GB ~{remaining}'
            if w:
                speed = w.speed(now)
                rate = '--' if speed is None else f'{speed:.0f}'
                stage = 'Saving' if stopping else ('REC' if w.count else 'Starting')
                elapsed = duration((now-w.first_start)/1e9) if w.first_start and not stopping else ''
                lines = [f'{stage} {label} {elapsed} Gaps {w.gaps()}',
                         f'{rate} MB/s  {w.count} {unit}', space]
            else:
                last = self.card.last_speed
                rate = '--' if last is None else f'{last:.0f}'
                lines = [f'RAW {label} {link:g}Gb/s', f'Last {rate} MB/s', space]
        except (OSError, KeyError, ValueError):
            self.card.key = None
            lines = [f'RAW {label}', 'External SSD required', 'Free --  Time --']
        lines.extend(temperature_lines(now))
        tmp = RAW / 'status.tmp'
        with tmp.open('wb') as f:
            f.write(struct.pack('<QII', now//1_000_000_000, 3, 0))
            f.write(('\n'.join(lines) + '\0').encode('ascii'))
        tmp.replace(RAW/'status')
        select_source(RAW)

    def finish(self, fps, seal):
        w = self.writer
        if w:
            w.update(time.monotonic_ns())
            if w.first_start and w.last_end > w.first_start and w.count == seal['pairs']:
                self.card.last_speed = seal['bytes'] * 1000 / (w.last_end - w.first_start)
                self.card.averages[fps] = seal['bytes'] / seal['pairs']
        self.writer = None
