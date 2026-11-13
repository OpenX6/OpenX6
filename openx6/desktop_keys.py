"""Local enrollment for the offline desktop builder."""
import base64
import struct
import shutil
import subprocess
from pathlib import Path
from .common import regular, require
from .owner import ADB, ED, SSH, create_profile, ed25519_public, ssh_public


def read_project_public(path):
    with regular(path) as stream:
        data = stream.read(4097)
    if len(data) == 32:
        return ed25519_public(data)
    line = ssh_public(data)
    require(line.startswith(b'ssh-ed25519 '), 'Expected Ed25519 public key')
    return ed25519_public(base64.b64decode(line.split()[1])[-32:])


def android_public(key):
    result = subprocess.run(['openssl', 'rsa', '-in', str(key), '-passin', 'pass:',
                             '-noout', '-modulus'], check=True, capture_output=True)
    prefix = b'Modulus='
    require(result.stdout.startswith(prefix), 'Unable to read Android RSA modulus.')
    n = int(result.stdout[len(prefix):].strip(), 16)
    require(n.bit_length() == 2048, 'Expected Android RSA2048.')
    wire = (struct.pack('<II', 64, (-pow(n, -1, 1 << 32)) & 0xffffffff)
            + n.to_bytes(256, 'little') + pow(2, 4096, n).to_bytes(256, 'little')
            + struct.pack('<I', 65537))
    return base64.b64encode(wire) + b'\n'


def create_holder(directory):
    directory = Path(directory)
    for tool in ('ssh-keygen', 'openssl'):
        require(shutil.which(tool), 'Missing local tool: ' + tool)
    directory.mkdir(mode=0o700, exist_ok=False)
    try:
        for name in ('access', 'maintenance'):
            subprocess.run(['ssh-keygen', '-q', '-t', 'ed25519', '-C', name, '-N', '',
                            '-f', str(directory/name)], check=True, capture_output=True)
        subprocess.run(['openssl', 'genrsa', '-out', str(directory/'adbkey'), '2048'],
                       check=True, capture_output=True)
        (directory/'adbkey.pub').write_bytes(android_public(directory/'adbkey'))
        files = {SSH: directory/'access.pub', ADB: directory/'adbkey.pub'}
        for name in ('access', 'maintenance'):
            (directory/(name+'.raw.pub')).write_bytes(read_project_public(directory/(name+'.pub')))
        for role,path in ED.items():
            files[path] = directory/(('maintenance' if role == 'update' else 'access')+'.raw.pub')
        create_profile(files, directory/'owner-public.json')
    except BaseException:
        shutil.rmtree(directory)
        raise
    return directory/'owner-public.json'
