"""Recreate the verified GUI extension and its writable RAW runtime on this boot."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile

HERE=Path(__file__).resolve().parent
ROOT=Path('/run/x5-raw-mode')
GALLERY=Path('/run/x5-raw-gallery')
GUI=Path('/usr/bin/ins_gui')
SETUP=Path('/usr/local/sbin/x6-owner-gui-setup')


def digest(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda:f.read(1024*1024),b''):h.update(chunk)
    return h.hexdigest()


def prepare():
    plan=json.loads((HERE/'pins.json').read_text())
    if not Path('/run/x6-boot/features-allowed').is_file():return
    assert digest(HERE/'ui-recipe.json')==plan['recipe']
    assert digest(HERE/'runtime.tar')==plan['runtime']
    recipe=json.loads((HERE/'ui-recipe.json').read_text())
    assert digest(GUI)==recipe['pin'] and digest(SETUP)==plan['base_setup']
    GALLERY.mkdir(mode=0o700,exist_ok=True)
    ROOT.mkdir(mode=0o700,exist_ok=True)
    # Both inputs are installed on the read-only system slot.
    with tarfile.open(HERE/'runtime.tar') as archive:archive.extractall(ROOT)
    (ROOT/'busy').write_text('0')
    (ROOT/'os').symlink_to('/run/x6-stats-overlay')
    target=GALLERY/'gui'
    shutil.copyfile(GUI,target)
    assert target.stat().st_size==recipe['end']
    with target.open('r+b') as f:
        for offset,old,new in recipe['edits']:
            old=bytes.fromhex(old)
            f.seek(offset)
            assert f.read(len(old))==old
            f.seek(offset)
            f.write(bytes.fromhex(new))
    assert digest(target)==plan['gui']
    helper=GALLERY/'setup'
    base=SETUP.read_bytes()
    old=recipe['pin'].encode()
    assert base.count(old)==1
    helper.write_bytes(base.replace(old,plan['gui'].encode()))
    assert digest(helper)==plan['setup']
    for source,dest in ((GUI,target),(SETUP,helper)):
        dest.chmod(0o755)
        os.setxattr(dest,'security.selinux',os.getxattr(source,'security.selinux'))
    mounted=[]
    try:
        for source,dest in ((target,GUI),(helper,SETUP)):
            subprocess.run(['mount','--bind',source,dest],check=True)
            mounted.append(dest)
        (ROOT/'boot-ready').write_text(Path('/proc/sys/kernel/random/boot_id').read_text())
    except BaseException:
        for dest in reversed(mounted):subprocess.run(['umount',dest],check=True)
        raise


if __name__=='__main__':prepare()
