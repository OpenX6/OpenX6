"""Rebuild the hardware service, bounded USB receiver and per-command factory gate."""
import struct

from .common import require
from .compiled_recipes import extend_text, payload
from .raw_nr_recipe import branch
from .toolchain import compiler_tools


def hardware_service(stock):
    out = bytearray(stock)
    # Allow the ADB compositions, then stop the exact receiver and propagate its status.
    struct.pack_into('<I', out, 0x266c9c, branch(0x266c9c, 0x266cac))
    old = b'pkill ins_usb\0'
    name = 0x317d90
    require(stock[name:name+len(old)] == old, 'Unexpected receiver stop command')
    out[name:name+len(old)] = b'/sbin/x6-stop\0'
    struct.pack_into('<II', out, 0x26001c, 0x2a0003f4, branch(0x260020, 0x260198))
    struct.pack_into('<I', out, 0x2629d0, branch(0x2629d0, 0x2629e4))
    return bytes(out)


def usb_receiver(stock, tools):
    site, base = 0xe10c, 0x2e600
    code = payload('factory_usb', base, ['copy_guard.S'],
                   dict(reject_copy=0xde5c, continue_copy=site+4), tools or compiler_tools(),
                   inputs={'payload-address.bin': stock[site:site+4]})
    out = bytearray(stock)
    extend_text(out, 2, base, code)
    struct.pack_into('<I', out, site, branch(site, base))
    return bytes(out)


def factory_api(stock, tools):
    site, base = 0x296cb0, 0x35eb00
    code = payload('factory_usb', base, ['factory_gate.S'],
                   dict(owner_memcpy=0x957d0, owner_alloc=0x94060, owner_original_resume=site+8),
                   tools or compiler_tools(), inputs={'native-prologue.bin': stock[site:site+8]})
    out = bytearray(stock)
    extend_text(out, 0, base, code)
    # Keep an indirect-call-compatible BTI landing. Replay the prologue only on dispatch.
    struct.pack_into('<II', out, site, 0xd503245f, branch(site+4, base))
    return bytes(out)
