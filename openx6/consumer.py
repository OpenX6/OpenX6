"""Offline SD build for a local holder. No camera or removable-media operations."""
import base64
import os
import shutil
import subprocess
from pathlib import Path

from .build import ROOT, build
from .common import regular, require
from .desktop_keys import android_public, create_holder
from .official import inspect
from .owner import ADB, ED, SSH, adb_public, ssh_public, validate_owner


def local_holder(directory):
    """Reuse the same private identity and reject partial/mismatched backups."""
    profile = directory / 'owner-public.json'
    if not directory.exists():
        directory.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        create_holder(directory)
    require(not directory.is_symlink() and directory.is_dir()
            and directory.stat().st_mode & 0o077 == 0
            and directory.stat().st_uid == os.getuid(), 'Holder folder must be private.')
    public = validate_owner(profile)
    for name, roles in [('access', ['gui', 'factory']), ('maintenance', ['update'])]:
        key = directory / name
        with regular(key, secret=True):
            line = subprocess.check_output(['ssh-keygen', '-y', '-P', '', '-f', str(key)],
                                           stderr=subprocess.DEVNULL)
        raw = base64.b64decode(ssh_public(line).split()[1])[-32:]
        require(all(public[ED[role]] == raw for role in roles), 'Holder identity mismatch.')
        if name == 'access':
            require(public[SSH] == ssh_public(line), 'SSH identity mismatch.')
    with regular(directory/'adbkey', secret=True):
        line = android_public(directory/'adbkey')
    require(public[ADB] == adb_public(line), 'ADB identity mismatch.')
    return profile


def build_sd(firmware, material, destination, directory, progress=print, byte_progress=None):
    inspect(firmware)  # Reject unsupported firmware before creating a new identity.
    require(Path(material).is_dir(), 'Select the three-module folder exported by your X6.')
    return build(firmware, material, (), destination, holder=local_holder(directory),
                 progress=progress, sd=True, byte_progress=byte_progress)


def seal(out, packed, profile, go):
    require(packed['installer_kind'] == 'guard-ready' and packed['install_allowed'],
            'Refusing to seal a locked developer candidate.')
    source = out/'component-source'
    shutil.copytree(ROOT/'desktop-runtime/sign', source/'host-authorize')
    env = dict(os.environ, GOFLAGS='', CGO_ENABLED='0', GOWORK='off', GOTOOLCHAIN='local',
               GOPROXY='off', GOSUMDB='off')
    for k in ('GOOS', 'GOARCH', 'GOARM64'):
        env.pop(k, None)
    directory = out/'seal'
    directory.mkdir(mode=0o700)
    for name in ('host-authorize', 'verifier'):
        subprocess.run([go,'build','-mod=vendor','-trimpath','-o',str(directory/name),'./'+name],
                       cwd=source, env=env, check=True)
    canonical = directory/'canonical.bin'
    (out/packed['file']).rename(canonical)
    canonical.chmod(0o400)
    public = directory/'owner.pub'
    shutil.copyfile(out/'overlay/ins/etc/x6_owner_update_ed25519.pub', public)
    subprocess.run([directory/'host-authorize','--package',canonical,'--key',profile.parent/'maintenance',
                    '--expected-public',public,'--sha256',packed['sha256'],
                    '--out',directory/'canonical.bin.owner-auth'], check=True)
    # The verifier re-opens the input, authenticates the signature, checks all
    # module boundaries/digests, and verifies the embedded record after writing.
    subprocess.run([directory/'verifier','seal-offline',directory], check=True)
    final = out/'Insta360X6FW.bin'
    (directory/'single.bin').rename(final)
    return final
