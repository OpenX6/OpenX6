"""Validate the currently selected external volume, including USB disconnect/reconnect."""
import json
import os
import stat
from pathlib import Path

def mounted_device(root, state):
    if not state or not state.get('uuid') or (Path('/run/x6-ssd/active').read_bytes()!=b'1'):
        raise ValueError('Select a mounted external drive')
    device = os.stat(state['dev'])
    if not stat.S_ISBLK(device.st_mode) or root.stat().st_dev != device.st_rdev:
        raise ValueError('External drive changed')
    if state['boot'] != Path('/proc/sys/kernel/random/boot_id').read_text().strip():
        raise ValueError('Stale storage state')
    diskseq = Path('/sys/class/block')/state['disk']/'diskseq'
    if diskseq.read_text().strip() != str(state['diskseq']):
        raise ValueError('External drive reconnected')
    return state['dev']


SSD_REQUIRED = 'External SSD required'

def selected_external(root):
    try:
        state = json.loads(Path('/run/x6-ssd/state.json').read_text())
        if not isinstance(state, dict):
            raise ValueError('Storage unavailable')
        device = mounted_device(root, state)
        return state, device
    except (OSError, ValueError, KeyError, TypeError) as error:
        raise ValueError(SSD_REQUIRED) from error
