"""Reconcile one public selection with the result of an unprivileged handler."""
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import tempfile
import time


def invoke(program, choice, directory, timeout):
    with tempfile.TemporaryFile(dir=directory) as output:
        proc = subprocess.Popen([sys.executable, '-I', '-B', str(program), choice],
                                stdin=subprocess.DEVNULL, stdout=output, stderr=subprocess.DEVNULL,
                                cwd=directory, env={'LANG':'C'}, start_new_session=True)
        try:
            status = proc.wait(timeout=timeout)
        finally:
            try:
                os.killpg(proc.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            proc.wait()
        if status:
            raise ValueError('handler exited with status '+str(status))
        output.seek(0)
        raw = output.read(8193)
        if len(raw) > 8192:
            raise ValueError('handler output too large')
        data = json.loads(raw.decode('utf-8'))
        if not isinstance(data, dict):
            raise ValueError('handler must return a JSON object')
        return data


def read_input(path):
    try:
        with path.open('rb') as f:
            raw = f.read(49)
    except FileNotFoundError:
        return None
    return raw.decode('ascii') if re.fullmatch(rb'[a-z][a-z0-9_]{0,47}', raw) else None


def reconcile(program, source, directory, previous, timeout=3):
    dest = directory/'result.json'
    selected = read_input(source)
    if selected is None:
        dest.unlink(missing_ok=True)
        return None
    if selected == previous and dest.is_file():
        return previous
    dest.unlink(missing_ok=True)
    data = invoke(program, selected, directory, timeout)
    payload = json.dumps({'selection':selected, 'result':data}, allow_nan=False).encode()
    if len(payload) > 16384:
        raise ValueError('encoded result too large')
    fd, name = tempfile.mkstemp(dir=directory)
    try:
        with os.fdopen(fd, 'wb') as f:
            f.write(payload)
        if read_input(source) != selected:
            return None
        os.replace(name, dest)
        return selected
    finally:
        Path(name).unlink(missing_ok=True)


if __name__ == '__main__':
    if os.geteuid() == 0:
        raise SystemExit('module worker requires an unprivileged account')
    program, source, directory = map(Path, sys.argv[1:])
    # Fresh process or changed module: run again even if the saved choice is identical.
    previous = None
    while True:
        try:
            previous = reconcile(program, source, directory, previous)
        except (OSError, ValueError, RecursionError, subprocess.TimeoutExpired):
            (directory/'result.json').unlink(missing_ok=True)
            previous = None
            time.sleep(4)
        time.sleep(0.5)
