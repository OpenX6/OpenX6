"""Exercise a locked developer build using the source and fixture in the test kit."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile


def main():
    run, firmware, material = map(Path, sys.argv[1:4])
    mode = sys.argv[4]
    if mode not in ('check', 'gui'):
        raise ValueError('Unknown acceptance mode')
    sys.path.insert(0, str(run/'source'))
    from openx6.build import build, preflight_modules
    from openx6.desktop_keys import create_holder
    from sdk.verify_menu_candidate import verify
    manifest = run/'field_budget/module.json'
    dest = run/'output/OpenX6 Build'
    docs, _ = preflight_modules([manifest], full=True)
    assert [m['id'] for m in docs] == ['field_budget']
    answer = subprocess.check_output([sys.executable, '-IB', str(manifest.with_name('budget.py')), 'max'])
    assert json.loads(answer)['video_gib_per_hour'] == 100.58
    with tempfile.TemporaryDirectory(dir=run) as tmp:
        temp = Path(tmp)
        secret = material
        if not secret.is_dir(): raise ValueError('Expected an X6 module folder')
        holder = temp/'Holder'
        create_holder(holder)
        profile = holder/'owner-public.json'
        if mode == 'gui':
            for label, path in [('Official firmware',firmware), ('X6 module folder',secret), ('Holder profile',profile),
                                ('Add JSON',manifest), ('Output parent',dest.parent)]:
                print(f'{label}: {path}', flush=True)
            print('Select Full OpenX6 build, check the module and build. Close the window after Menu readback OK.',flush=True)
            subprocess.run([sys.executable, '-B', '-m', 'openx6.desktop'], cwd=run/'source', check=True)
        else:
            build(firmware, secret, [manifest], dest, holder=profile, progress=lambda s: print(s, flush=True))
    report = verify(dest, [manifest])
    result = json.loads((dest/'RESULT.json').read_bytes())
    if report['install_allowed'] is not False or report['modules'] != ['field_budget'] or result['installer_kind'] != 'guard-locked':
        raise ValueError('Unexpected lock or selected modules')
    (run/'readback.json').write_text(json.dumps(report,indent=2)+'\n')
    print('PASS',run/'readback.json',flush=True)


if __name__ == '__main__':
    os.umask(0o077)
    main()
