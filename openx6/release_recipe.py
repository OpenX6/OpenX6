"""Verify a publisher signature on the pinned component profile before using it."""
import base64
import struct
import subprocess
import tempfile
from pathlib import Path
from .common import regular, require
from .desktop_keys import read_project_public

NAMESPACE = 'openx6.component-profile.v1'


def verify_recipe(profile, signature, public_key):
    raw = read_project_public(public_key)
    algorithm = b'ssh-ed25519'
    wire = struct.pack('>I',len(algorithm))+algorithm+struct.pack('>I',len(raw))+raw
    with regular(signature) as sig:
        signature_bytes = sig.read(16385)
    require(0 < len(signature_bytes) <= 16384, 'Invalid release signature')
    with tempfile.TemporaryDirectory(prefix='openx6-recipe-') as tmp:
        allowed = Path(tmp)/'allowed'
        allowed.write_bytes(b'publisher ssh-ed25519 '+base64.b64encode(wire)+b'\n')
        detached = Path(tmp)/'profile.sig'
        detached.write_bytes(signature_bytes)
        with regular(profile) as body:
            result = subprocess.run(['ssh-keygen','-Y','verify','-f',str(allowed),'-I','publisher',
                                     '-n',NAMESPACE,'-s',str(detached)],stdin=body,capture_output=True)
    require(result.returncode == 0, 'Project signature on component profile rejected')
