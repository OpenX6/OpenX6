"""Public-only receipts for a consumer desktop upgrade. Run inside the original WSL distro."""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import struct
import subprocess
import sys
import tempfile
import zipfile

DOMAIN = b'X6 OWNER MWFI V1\0'
SIGNER = '/ins/etc/x6_owner_update_ed25519.pub'
ROLES = {SIGNER, '/ins/etc/x6_owner_authorized_keys', '/ins/etc/x6_owner_gui_ed25519.pub',
         '/ins/etc/x6_owner_factory_ed25519.pub', '/adb_keys'}


def need(ok, message):
    if not ok:
        raise ValueError(message)


def identity(holder):
    # Public data only. Validate against the enrolled snapshot again after the upgrade.
    profile = (holder/'owner-public.json').read_bytes()
    keys = json.loads(profile)['files']
    need(set(keys) == ROLES, 'unexpected public roles')
    keys = {k:base64.b64decode(v, validate=True) for k,v in keys.items()}
    key = (holder/'maintenance.raw.pub').read_bytes()
    need(len(key) == 32 and key == keys[SIGNER], 'maintenance public key mismatch')
    return dict(profile_sha256=hashlib.sha256(profile).hexdigest(),
                public_sha256={k:hashlib.sha256(v).hexdigest() for k,v in keys.items()}), key


def signature(key, record):
    with tempfile.TemporaryDirectory() as folder:
        q = Path(folder)
        (q/'public.der').write_bytes(bytes.fromhex('302a300506032b6570032100')+key)
        (q/'message').write_bytes(DOMAIN+record[:136])
        (q/'signature').write_bytes(record[136:])
        cmd = ['openssl','pkeyutl','-verify','-rawin','-pubin','-keyform','DER',
               '-inkey',str(q/'public.der'),'-in',str(q/'message'),'-sigfile',str(q/'signature')]
        return subprocess.run(cmd, capture_output=True).returncode


def package(folder, original, key):
    result = json.loads((folder/'RESULT.json').read_bytes())
    need(result.get('install_allowed') is True and result.get('owner_authorization_embedded') is True
         and result.get('candidate') == 'Insta360X6FW.bin', 'expected a signed consumer SD package')
    binfile = folder/'Insta360X6FW.bin'
    size = binfile.stat().st_size
    with binfile.open('rb') as stream:
        h = stream.read(4864)
        u32 = lambda i: struct.unpack_from('<I', h, i)[0]
        cert = u32(4356)
        need(h[:4] == b'MWFI' and u32(4) == 1 and u32(92) == 3 and u32(96) == 0
             and u32(4096) == 1 and u32(4100) == 4864 and u32(4104) >= 576
             and u32(4352) == 4096 and u32(4360) == 4096 and cert == 4864+u32(4104)+64
             and u32(4608) == 4097 and u32(4612) == cert+4160 and u32(4616) == 256
             and u32(4612)+320 == size, 'invalid MWFI/FwCert layout')
        at = cert+512
        stream.seek(0)
        whole = hashlib.sha256()
        canonical = hashlib.sha256()
        remaining = at
        while remaining:
            block = stream.read(min(4 << 20, remaining))
            need(block, 'truncated host module')
            whole.update(block)
            canonical.update(block)
            remaining -= len(block)
        record = stream.read(200)
        need(len(record) == 200 and record[:8] == b'X6UPD01\0' and
             struct.unpack_from('<IIQ', record, 8) == (1, 0, size), 'invalid signed record')
        whole.update(record)
        canonical.update(bytes(200))
        while block := stream.read(4 << 20):
            whole.update(block)
            canonical.update(block)
    need(whole.hexdigest() == result['sha256'] and canonical.digest() == record[24:56]
         and canonical.hexdigest() == result['canonical_sha256'], 'package contents do not match signature')
    need(signature(key, record) == 0, 'original Holder signature failed')
    # RFC 8032 public test vector. Use a different vector if the enrolled public key coincides.
    other = bytes.fromhex('d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a')
    if key == other:
        other = bytes.fromhex('3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c')
    need(signature(other, record) == 1, 'wrong-key negative control failed')
    guard_bytes = (folder/'guard-config.json').read_bytes()
    with zipfile.ZipFile(binfile) as archive:
        need(guard_bytes in archive.read('x6-install-guard'), 'guard configuration absent from signed binary')
    guard = json.loads(guard_bytes)
    embedded = {'/'+k:hashlib.sha256(base64.b64decode(v, validate=True)).hexdigest()
                for k,v in guard['InitialKeys'].items()}
    need(guard['InstallAllowed'] is True and embedded == original['public_sha256'],
         'signed guard differs from the original access/maintenance keys')
    return dict(bytes=size, sha256=whole.hexdigest(), original_signature=True,
                wrong_signer_rejected=True, five_embedded_roles_match=True)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    sub = p.add_subparsers(dest='action', required=True)
    a = sub.add_parser('snapshot')
    a.add_argument('receipt', type=Path)
    b = sub.add_parser('compare')
    b.add_argument('snapshot', type=Path)
    b.add_argument('previous', type=Path)
    b.add_argument('current', type=Path)
    b.add_argument('receipt', type=Path)
    args = p.parse_args()
    public, key = identity(Path.home()/'.local/share/OpenX6/Holder')
    if args.action == 'compare':
        need(public == json.loads(args.snapshot.read_bytes()), 'Holder changed after upgrade')
        need(not (args.previous/'Insta360X6FW.bin').samefile(args.current/'Insta360X6FW.bin'),
             'select the two separate EXE output files')
        report = dict(previous=package(args.previous, public, key), current=package(args.current, public, key),
                      holder_unchanged=True, maintenance_fingerprint=public['public_sha256'][SIGNER])
    else:
        report = public
    with args.receipt.open('x', encoding='utf-8') as out:
        out.write(json.dumps(report, indent=2)+'\n')
    print('PASS', args.action)


if __name__ == '__main__':
    main()
