"""Native hotspot connection and camera confirmation; WSL handles the offline build."""
import ctypes
import os
from pathlib import Path
import sys
from threading import Event, Thread

sys.path.insert(0,str(Path(__file__).resolve().parent))
from camera_reader.export import UPDATERS, CACHE, export_to_cache, verify_modules, discover
from camera_reader.power import NOTICE
from openx6.official import inspect
from openx6.ui_language import render


def tr(message):
    return render(message, os.environ.get("OPENX6_UI_LANG", "en"))


def main():
    firmware = Path(sys.argv[1])
    inspect(firmware)
    if sys.argv[2:] == ['--inspect']:
        return tr('Verified X6 1.2.36')
    parent = Path(os.environ['LOCALAPPDATA'])/'OpenX6'/'Camera'
    if (parent/CACHE).exists():
        return verify_modules(parent/CACHE)
    serial, version = discover()
    if version.lower().removeprefix('v') not in UPDATERS:
        raise ValueError('On-camera unpack requires stock 0.8.825 or 0.8.827.')
    message = (f'Insta360 X6 {version} · SN …{serial[-6:]}\n\n'
               + tr('Match this serial suffix to your camera. Keep it on and connected. Tap Allow on the camera if prompted. OpenX6 will save three verified modules on this computer.')
               + '\n\n' + tr(NOTICE.splitlines()[0]) + '\n' + tr('Continue?'))
    if ctypes.windll.user32.MessageBoxExW(None, message, 'OpenX6', 0x124, 0x0804 if os.environ.get("OPENX6_UI_LANG") == "zh" else 0x0409) != 6:
        return None
    def progress(message):
        print(tr(message), file=sys.stderr, flush=True)
    cancel = Event()
    if os.environ.get('OPENX6_FORM'):
        def watch_cancel():
            if os.read(0, 1) == b'C':
                cancel.set()
        Thread(target=watch_cancel, daemon=True).start()
    return export_to_cache(firmware, parent, serial, progress, cancel,
                           byte_progress=lambda label,n,total: progress(f'{label} {100*n//total}%'))


if __name__=='__main__':
    try:
        path = main()
    except Exception as exc:
        print(tr(str(exc)), file=sys.stderr, flush=True)
        raise SystemExit(1)
    if path:
        sys.stdout.buffer.write((str(path)+'\n').encode('utf-8'))
