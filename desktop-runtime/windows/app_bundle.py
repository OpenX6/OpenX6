"""Build the consumer application from an explicit list of tracked source files."""
import hashlib
from pathlib import Path
import shutil
import stat
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[2]
INPUTS = ('LICENSE', 'NOTICE.md', 'openx6', 'components', 'recipes', 'install-guard', 'release', 'sdk',
          'desktop-runtime/sign', 'desktop-runtime/windows/bridge.py')

def copy_release_notices(release):
    (release/'sdk').mkdir(exist_ok=True)
    shutil.copyfile(ROOT/'LICENSE', release/'LICENSE')
    shutil.copyfile(ROOT/'sdk/LICENSE', release/'sdk/LICENSE')
    shutil.copyfile(ROOT/'NOTICE.md', release/'OpenX6-NOTICE.md')
    shutil.copyfile(ROOT/'RELEASE_NOTES.md', release/'RELEASE_NOTES.md')


def bundle(path):
    files = subprocess.check_output(['git', 'ls-files', '-z', '--', *INPUTS], cwd=ROOT)
    names = sorted(name.decode() for name in files.rstrip(b'\0').split(b'\0'))
    required = {'desktop-runtime/windows/bridge.py', 'openx6/build.py', 'LICENSE', 'sdk/LICENSE', 'NOTICE.md'}
    if not required.issubset(names):
        raise ValueError('Consumer source inventory missing.')
    with zipfile.ZipFile(path, 'x') as archive:
        for name in names:
            source = ROOT/name
            if not stat.S_ISREG(source.lstat().st_mode):
                raise ValueError('Expected regular source file: '+name)
            member = zipfile.ZipInfo(name, (1980, 1, 1, 0, 0, 0))
            member.create_system = 3
            member.external_attr = (stat.S_IFREG | 0o644) << 16
            archive.writestr(member, source.read_bytes(), compress_type=zipfile.ZIP_DEFLATED)
    return hashlib.sha256(path.read_bytes()).hexdigest()
