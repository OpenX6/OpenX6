"""Build the holder-authenticated ADB components from the verified stock input."""
import struct

from .common import require
from .compiled_recipes import SOURCES


def owner_adb(stock, path):
    if path == '/lib/systemd/system/adbd.service':
        return (SOURCES/'adb/adbd.service').read_bytes()
    out = bytearray(stock)
    if path == '/sbin/adbd':
        # Force the secure property result to true before the native AUTH check.
        struct.pack_into('<I', out, 0x46bc, 0x52800020)  # mov w0, #1
        marker = b'/tmp/.adb.root'
        require(out.count(marker) == 4, 'Unexpected ADB root-marker references')
        return bytes(out.replace(marker, b'/run/.adb.root'))

    require(path == '/usr/lib/libadbd.so.0.0.0', 'Unrecognized ADB component')
    # Decline interactive key enrollment; the native RSA verification code remains in place.
    struct.pack_into('<II', out, 0x18b40, 0xd503245f, 0xd65f03c0)  # bti c; ret
    old_path = b'/data/misc/adb/adb_keys\0'
    require(out.count(old_path) == 1, 'Unexpected ADB trusted-key path')
    return bytes(out.replace(old_path, b'/adb_keys\0'.ljust(len(old_path), b'\0')))
