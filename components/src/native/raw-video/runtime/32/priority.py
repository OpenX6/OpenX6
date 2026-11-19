"""All temporary RAW jobs on this camera yield the CPU and card to capture."""
from contextlib import contextmanager
import ctypes, os, sys
from pathlib import Path
from contextlib import ExitStack
BASE=Path('/run/x5-raw-io')
libc=ctypes.CDLL(None,use_errno=True)
def acquire(name,mode):
    BASE.mkdir(mode=0o700,exist_ok=True)
    fd=os.open(BASE/name,os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
    if libc.flock(fd,mode):
        err=ctypes.get_errno();os.close(fd);raise OSError(err,os.strerror(err))
    return fd

@contextmanager
def conversion_pair():
    gate=acquire('turn',1)
    try:disk=acquire('io',1)
    finally:os.close(gate)
    try:yield
    finally:os.close(disk)

if __name__=='__main__':
    # Keep both locks across exec. The kernel releases them if capture exits.
    gate=acquire('turn',2)
    disk=acquire('io',2)
    for fd in (gate,disk):os.set_inheritable(fd,True)
    os.execv(sys.executable,[sys.executable,*sys.argv[1:]])
