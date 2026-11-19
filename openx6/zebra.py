"""Append the highlight warning callback to a validated LCD GUI."""
import subprocess as sp, tempfile, struct, hashlib
from pathlib import Path
from .gui_payload import linked_payload
from .panel_patch import SYMS,SITE
H=struct.Struct('<II6Q')
def patch(data,D,clang,lld,expected):
 if hashlib.sha256(data).hexdigest()!=expected: raise ValueError('unknown highlight warning GUI base')
 llvm=Path(clang).parent
 def read(o,n):return data[o:o+n]
 hd=read(0,512);menu,rw,code=[list(H.unpack_from(hd,64+n*56)) for n in (3,4,6)];assert menu[:2]==code[:2]==[1,5] and rw[:2]==[1,6]
 def signed(v,n):return (v^(1<<(n-1)))-(1<<(n-1))
 bl,=struct.unpack('<I',read(SITE,4));assert bl>>26==0x25
 tramp=SITE+4*signed(bl&0x3ffffff,26);off=tramp-menu[3]+menu[2];assert 0<=off-menu[2]<=menu[5]-12
 raw=read(off,12);a,b,c=struct.unpack('<III',raw);assert a&0x9f00001f==0x90000010 and b&0xffc003ff==0x91000210 and c==0xd61f0200
 old=(tramp&~4095)+4096*signed(((a>>29)&3)|(((a>>5)&0x7ffff)<<2),21)+((b>>10)&4095)
 assert code[3]<=old<code[3]+code[5] and code[5]==code[6]
 pos=(code[3]+code[5]+15)&~15;state=(rw[3]+rw[6]+15)&~15;assert state+36512<=code[3]&~65535
 syms=SYMS|dict(app_id=SYMS['global_obj'],app_class=SYMS['global_class'],focus_id=0x702fc28,focus_class=0x7107ff8,live_class=0x7103e00,capture_class=0x7103350,s=state,previous=old,button_class=0x7157278,image_class=0x714e128,resource_class=0x7032df0,back=0x584320,set_title=0x9587b0,set_bg=0x9586e0,set_drawbg=0x958770,set_font=0x9588a0,set_fg=0x958950,set_align=0x958680,new_slot=0x4e1830,new_string=0x4e3e80,image_bitmap=0x91e1d0,image_visible=0x91d5f0,set_bitmap=0x8162c0,invalidate=0x583360,create_bitmap=0x4bb5d0,lock_bitmap=0x4bbc90,unlock_bitmap=0x4bbf90)
 with tempfile.TemporaryDirectory() as temp:
  t=Path(temp);(t/'ld').write_text('\n'.join(f'{k}={v};'.replace('=',' = ') for k,v in syms.items())+f'\nSECTIONS {{ . = {pos}; .payload : {{ *(.text*) *(.rodata*) }} .bad : {{ *(.got*) *(.data*) *(.bss*) }} ASSERT(SIZEOF(.bad)==0,"unexpected data") /DISCARD/ : {{ *(.note*) *(.eh_frame*) }} }}')
  sp.run([llvm/'clang','--target=aarch64-linux-gnu','-Oz','-ffreestanding','-fpie','-fno-builtin','-fno-stack-protector','-fno-asynchronous-unwind-tables','-fno-jump-tables','-mgeneral-regs-only','-mbranch-protection=bti+pac-ret','-c',D/'view.c','-o',t/'o'],check=True)
  sp.run([lld,'-flavor','gnu','--no-relax','--pic-veneer','-T',t/'ld','-e','entry',t/'o','-o',t/'elf'],check=True)
  entry,payload=linked_payload(t/'elf')
 end=code[2]+code[5];tail=bytes(pos-code[3]-code[5])+payload;code[5]+=len(tail);code[6]=code[5];rw[6]=state+36512-rw[3]
 pages=(entry>>12)-(tramp>>12);assert 0<pages<1<<20
 edits=[(off,raw[:8],struct.pack('<II',0x90000010|((pages&3)<<29)|((pages>>2)<<5),0x91000210|((entry&4095)<<10))),(end,b'',tail)]
 for i,h in [(4,rw),(6,code)]:o=64+56*i;edits.append((o,hd[o:o+56],H.pack(*h)))
 assert end==len(data)
 out=bytearray(data)
 for o,b,a in edits:
  assert out[o:o+len(b)]==b
  out[o:o+len(b)]=a
 return bytes(out)
