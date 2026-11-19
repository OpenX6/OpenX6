"""Sample the camera's read-only AE query, using one persistent native bus client."""
import contextlib
import os
from pathlib import Path
import re
import select
import signal
import struct
import subprocess
import time

PREF = Path('/data/x6-owner-features/show-exposure')
DATA = Path('/run/openx6-exposure')
DEMAND = Path('/run/openx6-exposure-visible')
SAMPLE = struct.Struct('<QiiI')
COMMAND = b'call-method kCamera kMediad Image ImageGetAEParam\n'
END = b'--------------------------------\r\n'
def parse(reply):
    if b'Error code: Success(0)' not in reply or b'ae_param_result:' not in reply:
        raise ValueError('no AE response')
    values = dict(re.findall(rb'^\s*(iso|shutter|iso_auto_switch|shutter_auto_switch):\s+(-?\d+|kAuto|kManual)\b', reply, re.M))
    iso, shutter = int(values[b'iso']), int(values[b'shutter'])
    if not 0 < iso <= 2000000 or not shutter or not -2000000000 <= shutter <= 600000:
        raise ValueError('invalid AE values')
    mask = int(values[b'shutter_auto_switch']==b'kAuto') | (int(values[b'iso_auto_switch']==b'kAuto')<<1)
    return shutter, iso, mask

def read_reply(fd):
    end = time.monotonic()+2
    data = bytearray()
    while not (b'Call result:' in data and data.endswith(END)):
        if len(data)>65536 or not select.select([fd], [], [], max(0,end-time.monotonic()))[0]:
            raise TimeoutError('bus prompt unavailable')
        chunk = os.read(fd,8192)
        if not chunk:
            raise EOFError('bus closed')
        data.extend(chunk)
    return bytes(data)
def wanted():
    if not PREF.is_file():
        return False
    try:
        stamp, = struct.unpack('<Q',DEMAND.read_bytes())
        return 0 <= time.monotonic()*1000-stamp < 2500
    except (OSError,struct.error):
        return False
@contextlib.contextmanager
def bus():
    # A terminal on stdout gives line-buffered replies. stdin is a pipe so that
    # the native client's line editor does not wait for cursor-position responses.
    master, slave=os.openpty()
    try:
        p=subprocess.Popen(['/usr/bin/ins_ibus_cli'],stdin=subprocess.PIPE,stdout=slave,stderr=subprocess.DEVNULL)
        try:
            yield p,master
        finally:
            p.stdin.close()
            p.terminate()
            try:p.wait(timeout=1)
            except subprocess.TimeoutExpired:
                p.kill();p.wait()
    finally:
        os.close(slave);os.close(master)
def main():
    def stop(*_):
        raise SystemExit
    signal.signal(signal.SIGTERM,stop)
    tmp=DATA.with_suffix('.next')
    try:
        while True:
            DATA.unlink(missing_ok=True)
            if wanted():
                try:
                    with bus() as (p,fd):
                        while wanted():
                            p.stdin.write(COMMAND);p.stdin.flush()
                            values=parse(read_reply(fd))
                            tmp.write_bytes(SAMPLE.pack(time.monotonic_ns()//1000000,*values))
                            os.replace(tmp,DATA)
                            time.sleep(.45)
                except (OSError,EOFError,ValueError,KeyError):
                    time.sleep(.5)
            else:time.sleep(.3)
    finally:
        DATA.unlink(missing_ok=True);tmp.unlink(missing_ok=True)
if __name__=='__main__':
    main()
