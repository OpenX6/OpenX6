"""Deploy only the exact signed output from the completed native launcher build."""
import ctypes
import json
import os
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
from camera_reader import deploy
from openx6.common import regular, require
from openx6.ui_language import render


def tr(message):
    return render(message, os.environ.get("OPENX6_UI_LANG", "en"))


def run(folder, pinned, ask, progress, byte_progress):
    path = folder / deploy.FINAL
    with regular(folder / 'RESULT.json') as stream:
        result = json.load(stream)
    require(len(pinned) == 64 and result.get('sha256') == pinned
            and result.get('candidate') == deploy.FINAL and result.get('install_allowed') is True
            and result.get('owner_authorization_embedded') is True, 'Signed build receipt mismatch')
    identity = deploy.discover()
    prompt = (f"X6 {identity[1]}  |  {tr('Serial ending')} {identity[0][-6:]}\n\n"
              + tr('Match the label on your camera. Use a powered, idle X6 with its SD card inserted and no firmware at the card root. Tap Allow on the camera if prompted.')
              + '\n\n' + tr('Deploy this build? Installation may begin immediately after readback verification. Your local package will be kept.'))
    if not ask(prompt):
        return 'CANCELLED'
    return deploy.to_card(path, result, identity, progress, byte_progress)


def main():
    def box(parent, text, title, flags):
        lang_id = 0x0804 if os.environ.get("OPENX6_UI_LANG") == "zh" else 0x0409
        return ctypes.windll.user32.MessageBoxExW(parent, text, title, flags, lang_id)
    def info(message):
        print(tr(message), flush=True)
    status = run(Path(sys.argv[1]), sys.argv[2], lambda m: box(None,m,'OpenX6 · ' + tr('Confirm camera'),0x124)==6,
                 info, lambda label,n,total: info(f'{tr(label)}: {100*n//total}%'))
    messages = {
        'CANCELLED': 'Cancelled. Nothing was sent.',
        'SCAN_REQUESTED': 'SD readback verified. A card scan was requested. Watch the camera for installation.',
        'SCAN_UNCONFIRMED': 'SD readback verified. Scan not confirmed. Check the camera before retrying.',
        'PUBLISHED': 'SD readback verified. The file is ready on the camera SD card.',
    }
    message = tr(messages[status])
    if status != 'CANCELLED':
        message += ' ' + tr('Keep the card and power connected. If an update is in progress, let it finish. '
                    'If the camera stays idle, restart normally with the card inserted. '
                    'At a charging screen press Power once.')
    info(message)
    if os.environ.get('OPENX6_FORM'): return
    box(None, message+'\n\n'+tr('Your build remains in:')+'\n'+sys.argv[1], 'OpenX6', 0x40)


if __name__ == '__main__':
    try:
        main()
    except Exception as e:
        if os.environ.get('OPENX6_FORM'):
            print(tr(str(e))+' · '+tr('Local build retained.'),file=sys.stderr,flush=True)
            raise SystemExit(1)
        ctypes.windll.user32.MessageBoxExW(None, tr(str(e))+'\n\n'+tr('Local build retained. Check the camera before retrying.'),
                                        'OpenX6 · '+tr('Wi-Fi deployment'), 0x30,
                                        0x0804 if os.environ.get('OPENX6_UI_LANG') == 'zh' else 0x0409)
        raise SystemExit(1)
