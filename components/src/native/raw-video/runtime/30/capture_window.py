"""Feed complete timestamped Bayer pairs until the shutter or card-space stop."""
import os
import re
import select
import shutil
import time
from take_status import save_stop_reason

RESERVE = 6_000_000_000
MAX_RECORD = 4096 + 48 * 3840 * 160 * 4
EXPOSURE = re.compile(rb'MainVideo\d_Sensor0_cam([01])" req=(\d+) clk=\d+ exp=(\d+)\b')
APPLIED = re.compile(rb' applied_ptr=(0x[0-9a-fA-F]+|0) dur=(\d+)\b')
FENCE = re.compile(rb' (\d+)\.(\d+): fence: .*MainVideo\d_IFE0_cam([01])" req=(\d+) req2=(\d+) port=9 status=0 .* fd=(\d+)')


def feed(pipe, writer, run, card, durations):
    os.set_blocking(pipe.fileno(), False)
    pending = {}
    carry = b''
    first = last = last_seen = None
    sent = 0
    deadline = time.monotonic() + 12
    next_check = 0
    reason = 'shutter'
    while not (run/'stop').exists():
        now = time.monotonic()
        if now >= next_check:
            if shutil.disk_usage(card).free <= RESERVE:
                reason = 'card-space'
                break
            next_check = now + .25
        if now >= deadline:
            reason = 'frame-timeout'
            break
        if not select.select([pipe], [], [], .15)[0]:
            continue
        lines = (carry + os.read(pipe.fileno(), 262144)).split(b'\n')
        carry = lines.pop()
        for line in lines:
            if b': primary:' in line:
                match = EXPOSURE.search(line)
                if match:
                    eye, req, start = map(int, match.groups()[:3])
                    applied = APPLIED.match(line, match.end())
                    dur = int(applied[2]) if applied and int(applied[1], 0) else 0
                    pending.setdefault(req, {})[2 + eye] = (start, dur)
            else:
                match = FENCE.search(line)
                if not match:
                    continue
                sec, frac, eye, req, req2, fd = match.groups()
                eye, req = int(eye), int(req)
                assert req == int(req2)
                if first is None:
                    first = req + 12
                    (run/'first-request').write_text(str(first))
                last_seen = max(last_seen or req, req)
                pending.setdefault(req, {})[eye] = (int(fd), int(sec)*10**9 + int(frac.ljust(9,b'0')))
        for req in sorted(pending):
            row = pending[req]
            if first is None:
                continue
            if req < first or req <= (last or 0) or req < last_seen - 120:
                del pending[req]
            elif len(row) == 4:
                writer.write(('%d %d %d %d %d %d %d\n' % (req, *row[0], *row[1], row[2][0], row[3][0])).encode())
                durations.write(f"{req},{row[2][1] or ''},{row[3][1] or ''}\n")
                del pending[req]
                if not sent:
                    first_stamp = row[2][0]
                    first_sent = req
                    (run/'capturing').touch()
                last_stamp = row[2][0]
                last = req
                sent += 1
                deadline = now + 5
    save_stop_reason(run, reason)
    if sent < 2:
        raise RuntimeError('Too few paired frames to measure cadence')
    return first, last, sent, (last_stamp-first_stamp)/(last-first_sent)
