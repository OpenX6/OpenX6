"""Append the native Wi-Fi Settings choice after the stock page constructor."""
import struct
import subprocess
import tempfile
from pathlib import Path
from .gui_payload import linked_payload

CALL = 0x8e33cc
SYMBOLS = dict(native_wifi_init=0x8e2d10, new_object=0x4db3b0, group_add=0x58b350,
               text_utf8=0x4e4010, page_title=0x761eb0, arrow_title=0x963270,
               radio_title=0x8de490, radio_selected=0x927a80, new_slot=0x4e1830,
               create_subpage=0x7522b0, page_class=0x710bff8, arrow_class=0x715c318,
               radio_class=0x7136a00)

def branch(dest):
    distance = dest-CALL
    if distance%4 or not -(1<<27)<=distance<(1<<27):
        raise ValueError('unreachable Wi-Fi hook')
    return struct.pack('<I', 0x14000000 | ((distance//4)&0x3ffffff))

def patch(data, source, clang, lld):
    if data[CALL:CALL+4]!=branch(SYMBOLS['native_wifi_init']):
        raise ValueError('unknown Wi-Fi Settings constructor')
    buf=bytearray(data)
    ph=64+3*56
    seg=list(struct.unpack_from('<IIQQQQQQ',buf,ph))
    following=struct.unpack_from('<IIQQQQQQ',buf,ph+56)
    if seg[:2]!=[1,5] or seg[5]!=seg[6] or seg[2]+seg[5]!=len(buf) or following[:2]!=(1,6):
        raise ValueError('unexpected GUI mapping')
    base=(seg[3]+seg[5]+15)&~15
    with tempfile.TemporaryDirectory() as tmp:
        t=Path(tmp)
        script='\n'.join(f'{k} = 0x{v:x};' for k,v in SYMBOLS.items())
        script+=f'\nSECTIONS {{ . = 0x{base:x}; .payload : {{ *(.text*) *(.rodata*) }} .bad : {{ *(.data*) *(.bss*) *(.got*) }} ASSERT(SIZEOF(.bad)==0,"unexpected data relocations") /DISCARD/ : {{ *(.eh_frame*) *(.note*) }} }}'
        (t/'link.ld').write_text(script)
        subprocess.run([clang,'--target=aarch64-linux-gnu','-ffreestanding','-fpie','-fvisibility=hidden','-fno-builtin','-fno-stack-protector','-fno-unwind-tables','-fno-asynchronous-unwind-tables','-fno-jump-tables','-mgeneral-regs-only','-mbranch-protection=bti+pac-ret','-Oz','-c',source,'-o',t/'menu.o'],check=True)
        subprocess.run([lld,'-flavor','gnu','-O0','--no-relax','-T',t/'link.ld','-e','wifi_settings_init',t/'menu.o','-o',t/'linked'],check=True)
        entry,code=linked_payload(t/'linked')
    if not base<=entry<base+len(code) or base+len(code)>following[3]&~4095:
        raise ValueError('GUI mapping overflow')
    buf.extend(bytes(base-(seg[3]+seg[5]))+code)
    seg[5]=seg[6]=len(buf)-seg[2]
    struct.pack_into('<IIQQQQQQ',buf,ph,*seg)
    buf[CALL:CALL+4]=branch(entry)
    return bytes(buf)
