"""WSL worker; keep the holder and build scratch on the guest's private filesystem."""
import os
import shutil
import sys
import tempfile
from pathlib import Path, PureWindowsPath

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT))
from openx6.common import digest, regular, require
from openx6.consumer import build_sd, local_holder
from openx6.official import inspect
from openx6.build import build


def windows_path(s):
    path = PureWindowsPath(s)
    drive = path.drive
    require(path.is_absolute() and len(drive) == 2 and drive[0].isascii()
            and drive[0].isalpha() and drive[1] == ':' and '..' not in path.parts,
            'Choose an absolute file or folder on a local Windows drive.')
    return Path('/mnt') / drive[0].lower() / Path(*path.parts[1:])


def remembered_holder(private, firmware):
    """Keep the original account's identity. Only a fresh import may enroll."""
    holder = private/'Holder'
    pin = private/'holder-profile.sha256'
    pending = private/'first-enrollment.pending'
    inspect(firmware)
    pinned = os.path.lexists(pin)
    if not os.path.lexists(holder):
        require(not pinned and pending.is_file(),
                'Existing Holder is missing. Restore your original Holder backup in this WSL account.')
    profile = local_holder(holder)
    current = digest(profile)+'\n'
    if pinned:
        with regular(pin, secret=True) as stream:
            require(stream.read(66) == current.encode(), 'Holder changed since the last upgrade build.')
    else:
        # The guest directory lock serializes enrollment and pin publication.
        with tempfile.NamedTemporaryFile(dir=private, delete=False) as tmp:
            name = Path(tmp.name)
            try:
                tmp.write(current.encode())
                tmp.flush()
                os.fsync(tmp.fileno())
                os.replace(name, pin)
            finally:
                name.unlink(missing_ok=True)
    pending.unlink(missing_ok=True)
    return profile


def run(firmware, material, output, private, *, upgraded=False):
    output=output/'OpenX6 SD Build'
    require(not output.exists(), 'Choose a folder without an existing OpenX6 SD Build.')
    require(shutil.disk_usage(output.parent).free >= 2*1024**3, 'Output drive needs 2 GiB free.')
    private.mkdir(parents=True,exist_ok=True,mode=0o700)
    with tempfile.TemporaryDirectory(dir=private) as d:
        job=Path(d)
        build_input = material
        require(material.is_dir(), 'Select the verified X6 module folder.')
        def progress(message):
            print(message, flush=True)
        if upgraded:
            profile = remembered_holder(private, firmware)
            result = build(firmware, build_input, (), job/'result', holder=profile,
                           progress=progress, sd=True,
                           byte_progress=lambda label,n,total: progress(f'{label} {100*n//total}%'))
        else:
            result = build_sd(firmware, build_input, job/'result', private/'Holder', progress=progress)
        output.mkdir()
        files=[]
        try:
            for name in ('RESULT.json','SHA256SUMS','release-inputs.json','package.json','guard-config.json',
                         result['candidate']):
                src=job/'result'/name
                dst=output/name
                # No scan filename is exposed until the complete copy is verified.
                temporary=output/(name+'.partial')
                with src.open('rb') as a,temporary.open('xb') as b:
                    files.append(temporary)
                    shutil.copyfileobj(a,b,4<<20)
                    b.flush()
                    os.fsync(b.fileno())
                require(digest(temporary)==digest(src),'Output readback failed: '+name)
                temporary.rename(dst)
                files.append(dst)
        except BaseException:
            for path in files: path.unlink(missing_ok=True)
            output.rmdir()
            raise
        print('Verified: '+result['sha256'],flush=True)


if __name__=='__main__':
    os.umask(0o077)
    require(os.getuid()!=0, 'Run the worker as the unprivileged OpenX6 user.')
    args = sys.argv[1:]
    upgraded = args[:1] == ['--upgrade']
    if upgraded:
        args = args[1:]
    require(len(args) == 3, 'Choose firmware, material and a destination.')
    run(*(windows_path(s) for s in args), Path.home()/'.local/share/OpenX6', upgraded=upgraded)
