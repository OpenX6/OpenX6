"""Replace the old periodic toast call with one in-place EW panel."""
import struct,subprocess,tempfile
from pathlib import Path
from .gui_payload import linked_payload
from .common import require
SITE=0x977424
SYMS=dict(timers=0x4e9240,ticks=0x4d8c00,global_obj=0x702d108,global_class=0x70f97c8,
          rect_class=0x714dee8,text_class=0x714e2b0,font_resource=0x702cc48,font_class=0x712a580,
          buffer_direction=0x4f26b0,direction_vertical=0x4f2810,body_resource=0x702cd98,number_resource=0x702cee8,
          get_auto=0x4db7a0,new_object=0x4db3b0,load_resource=0x4df4d0,lock_obj=0x4dbbf0,
          unlock_obj=0x4dbd30,group_add=0x58b350,group_remove=0x58b330,bounds=0x589d10,
          set_bounds=0x589d00,rect_color=0x91cb70,rect_visible=0x91cc40,text_font=0x91a7e0,
          text_color=0x91f9f0,alignment=0x91f8c0,new_utf8=0x4e4010,text_string=0x91f970)
def patch(data,src,clang,lld):
    b=bytearray(data)
    ph=64+3*56
    rx=list(struct.unpack_from('<IIQQQQQQ',b,ph));rw=list(struct.unpack_from('<IIQQQQQQ',b,ph+56))
    require(rx[:2]==[1,5] and rx[2]+rx[5]==len(b) and rx[5]==rx[6] and rw[:2]==[1,6],'unexpected GUI segments')
    bl=struct.unpack_from('<I',b,SITE)[0]
    old=SITE+4*(((bl&0x3ffffff)^0x2000000)-0x2000000)
    require(bl>>26==0x25 and rx[3]<=old<rx[3]+rx[5],'old notice missing')
    offset=rx[2]+old-rx[3]
    require(b[offset:offset+8]==bytes.fromhex('5f2403d5fd7bbda9') and b'/run/x6-sd-update-notice\x00Preparing update on SD.' in b[offset:offset+450],'wrong old notice hook')
    base=(rx[3]+rx[5]+15)&~15
    state=(rw[3]+rw[6]+15)&~15
    with tempfile.TemporaryDirectory() as temp:
        t=Path(temp)
        ld='\n'.join(f'{k} = {a};' for k,a in (SYMS|dict(panel=state)).items())
        ld+=f'\nSECTIONS {{ . = {base}; .payload : {{ *(.text*) *(.rodata*) }} .bad : {{ *(.got*) *(.data*) *(.bss*) }} ASSERT(SIZEOF(.bad)==0,"unexpected relocations") /DISCARD/ : {{ *(.note*) *(.eh_frame*) }} }}'
        (t/'patch.ld').write_text(ld)
        subprocess.run([clang,'--target=aarch64-linux-gnu','-Oz','-ffreestanding','-fpie','-fno-builtin','-fno-stack-protector','-fno-asynchronous-unwind-tables','-fno-jump-tables','-mgeneral-regs-only','-mbranch-protection=bti+pac-ret','-c',src,'-o',t/'panel.o'],check=True)
        subprocess.run([lld,'-flavor','gnu','-O0','--no-relax','-T',t/'patch.ld','-e','entry',t/'panel.o','-o',t/'panel'],check=True)
        entry,code=linked_payload(t/'panel')
    require(base<=entry<base+len(code)<(rw[3]&~4095) and 0<=entry-SITE<1<<27,'panel placement')
    b.extend(bytes(base-rx[3]-rx[5])+code)
    rx[5]=rx[6]=len(b)-rx[2];rw[6]=state+112-rw[3]
    struct.pack_into('<IIQQQQQQ',b,ph,*rx);struct.pack_into('<IIQQQQQQ',b,ph+56,*rw)
    struct.pack_into('<I',b,SITE,0x94000000|(entry-SITE)//4)
    return bytes(b)
