"""Append the stock confirmation UI without relocating any already-built menu."""
from pathlib import Path
import struct,subprocess,tempfile
from .gui_payload import linked_payload
from .panel_patch import SYMS,SITE
from openx6.common import require

def patch(data,src,clang,lld):
    b=bytearray(data);ph=64+3*56
    rx=list(struct.unpack_from('<II6Q',b,ph));rw=list(struct.unpack_from('<II6Q',b,ph+56))
    require(rx[:2]==[1,5] and rx[2]+rx[5]==len(b) and rw[:2]==[1,6],'unexpected GUI extent')
    site=struct.unpack_from('<I',b,SITE)[0];old=SITE+4*(((site&0x3ffffff)^0x2000000)-0x2000000)
    require(site>>26==0x25 and rx[3]<=old<rx[3]+rx[5] and b'/run/x6-sd-update-notice\0' in b[rx[2]:],'missing update UI')
    trampoline=(rx[3]+rx[5]+3)&~3
    state=(rw[3]+rw[6]+15)&~15
    base=(state+160+65535)&~65535
    symbols=SYMS|dict(previous=old,state=state,root_id=SYMS['global_obj'],root_class=SYMS['global_class'],button_class=0x7157278,key_text=0x9587b0,key_font=0x9588a0,key_color=0x9586e0,key_bg=0x958770,key_fg=0x958950,key_height=0x958680,key_width=0x969710,new_slot=0x4e1830)
    with tempfile.TemporaryDirectory() as temp:
        t=Path(temp)
        (t/'ld').write_text('\n'.join(f'{k} = {a};' for k,a in symbols.items())+f'\nSECTIONS {{ . = {base}; .payload : {{ *(.text*) *(.rodata*) }} .bad : {{ *(.got*) *(.data*) *(.bss*) }} ASSERT(SIZEOF(.bad)==0,"unexpected relocations") /DISCARD/ : {{ *(.note*) *(.eh_frame*) }} }}')
        subprocess.run([clang,'--target=aarch64-linux-gnu','-Oz','-ffunction-sections','-ffreestanding','-fpie','-fno-builtin','-fno-stack-protector','-fno-asynchronous-unwind-tables','-fno-jump-tables','-mgeneral-regs-only','-mbranch-protection=bti+pac-ret','-c',src,'-o',t/'o'],check=True)
        subprocess.run([lld,'-flavor','gnu','--no-relax','--pic-veneer','-T',t/'ld','-e','confirm_entry',t/'o','-o',t/'elf'],check=True)
        entry,code=linked_payload(t/'elf')
    require(trampoline+12<(rw[3]&~4095) and 0<trampoline-SITE<1<<27 and base<=entry<base+len(code),'confirmation layout')
    # The NOTE and GNU_PROPERTY headers describe the same bytes. Keep the latter.
    note=list(struct.unpack_from('<II6Q',b,64+6*56));prop=list(struct.unpack_from('<II6Q',b,64+8*56))
    require(note[0]==4 and prop[0]==0x6474e553 and note[2:]==prop[2:],'unexpected GNU property mapping')
    pages=(entry>>12)-(trampoline>>12)
    require(0<pages<1<<20,'trampoline ADRP range')
    stub=struct.pack('<III',0x90000010|((pages&3)<<29)|((pages>>2)<<5),0x91000210|((entry&4095)<<10),0xd61f0200)
    b.extend(bytes(trampoline-rx[3]-rx[5])+stub)
    rx[5]=rx[6]=len(b)-rx[2];rw[6]=state+160-rw[3]
    append=(len(b)+65535)&~65535
    b.extend(bytes(append-len(b))+code)
    struct.pack_into('<II6Q',b,64+6*56,1,5,append,base,base,len(code),len(code),65536)
    struct.pack_into('<II6Q',b,ph,*rx);struct.pack_into('<II6Q',b,ph+56,*rw)
    struct.pack_into('<I',b,SITE,0x94000000|(trampoline-SITE)//4)
    return bytes(b)
