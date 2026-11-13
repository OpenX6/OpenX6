"""Account-local OpenX6 identity with Windows CurrentUser DPAPI."""
import base64
import ctypes
from ctypes import wintypes
import json
import os
from pathlib import Path
import shutil
import struct
import tempfile
import winreg
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ed25519, rsa
from openx6.common import require, sha, save
from openx6.owner import SSH, ADB, ED, validate_owner

class Blob(ctypes.Structure):
    _fields_ = [('cbData', wintypes.DWORD), ('pbData', ctypes.c_void_p)]

def protect(data, encrypt):
    buf = ctypes.create_string_buffer(data)
    source = Blob(len(data), ctypes.addressof(buf))
    dest = Blob()
    library = ctypes.WinDLL('crypt32', use_last_error=True)
    api = library.CryptProtectData if encrypt else library.CryptUnprotectData
    if not api(ctypes.byref(source), None, None, None, None, 1, ctypes.byref(dest)):
        raise ctypes.WinError(ctypes.get_last_error())
    try:
        return ctypes.string_at(dest.pbData, dest.cbData)
    finally:
        ctypes.memset(dest.pbData, 0, dest.cbData)
        ctypes.windll.kernel32.LocalFree(ctypes.c_void_p(dest.pbData))
        ctypes.memset(buf, 0, len(data))

def public_roles(access, update, adb):
    n = adb.public_key().public_numbers().n
    require(n.bit_length() == 2048 and adb.public_key().public_numbers().e == 65537, 'Invalid ADB identity')
    wire = struct.pack('<II', 64, -pow(n,-1,1<<32) & 0xffffffff) + n.to_bytes(256,'little') + pow(2,4096,n).to_bytes(256,'little') + struct.pack('<I',65537)
    return {SSH:access.public_key().public_bytes(serialization.Encoding.OpenSSH,serialization.PublicFormat.OpenSSH)+b'\n',
            ADB:base64.b64encode(wire)+b'\n', ED['gui']:access.public_key().public_bytes_raw(),
            ED['factory']:access.public_key().public_bytes_raw(), ED['update']:update.public_key().public_bytes_raw()}

def identity():
    base = Path(os.environ['LOCALAPPDATA'])/'OpenX6'/'Native'
    folder = base/'Holder'
    base.mkdir(parents=True,exist_ok=True)
    with winreg.CreateKey(winreg.HKEY_CURRENT_USER,r'Software\OpenX6\Native') as reg:
        try: pin = winreg.QueryValueEx(reg,'HolderProfileSHA256')[0]
        except FileNotFoundError: pin = None
        if not folder.exists():
            require(pin is None, 'Original Windows Holder missing. Restore the original Windows account backup.')
            # Rename publishes all files together. Concurrent first launches cannot replace a winner.
            stage = Path(tempfile.mkdtemp(dir=base))
            try:
                keys = [ed25519.Ed25519PrivateKey.generate(),ed25519.Ed25519PrivateKey.generate(),rsa.generate_private_key(public_exponent=65537,key_size=2048)]
                pem = [k.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()).decode('ascii') for k in keys]
                (stage/'holder.dpapi').write_bytes(protect(json.dumps(pem).encode(),True))
                roles = public_roles(*keys)
                save(stage/'owner-public.json',dict(schema=1,files={k:base64.b64encode(v).decode() for k,v in roles.items()}))
                (stage/'maintenance.raw.pub').write_bytes(roles[ED['update']])
                try: stage.rename(folder)
                except FileExistsError: pass
            finally:
                if stage.exists(): shutil.rmtree(stage)
        roles = validate_owner(folder/'owner-public.json')
        current = sha((folder/'owner-public.json').read_bytes())
        require(pin in (None,current), 'Windows Holder public identity changed.')
        pem = json.loads(protect((folder/'holder.dpapi').read_bytes(),False))
        access, update, adb = [serialization.load_pem_private_key(s.encode('ascii'),password=None) for s in pem]
        require(roles==public_roles(access,update,adb) and (folder/'maintenance.raw.pub').read_bytes()==roles[ED['update']], 'Windows Holder mismatch')
        if pin is None: winreg.SetValueEx(reg,'HolderProfileSHA256',0,winreg.REG_SZ,current)
    return roles,update
