"""Style both numeric editors and integrate the native quick-settings shortcuts."""
from .gui_payload import linked_text
import struct

SYMBOLS = dict(settings=0x702e578, settings_class=0x70fad38, get_auto=0x4db7a0,
               rate_get=0x5ffc90, rate_set=0x63bbe0, new_ref=0x4dd150, notify=0x4e27e0,
               std_label=0x70d6418, high_label=0x70d6220, localize=0x4e8aa0,
               new_string=0x4e3e80, set_subtitle=0x955840, key_bitmap=0x701ed40,
               bitmap_class=0x712a5e0, set_bitmap=0x9586b0, load_resource=0x4df4d0,
               set_width=0x969710, set_background_color=0x9586e0)

def require(condition, message):
    if not condition:
        raise ValueError(message)

def branch(site, dest, link=False):
    diff = dest - site
    require(diff % 4 == 0 and abs(diff) < 1 << 27, 'invalid branch')
    return (0x94000000 if link else 0x14000000) | ((diff // 4) & 0x3ffffff)

def patch(data, clang, lld, assembly):
    buf = bytearray(data)
    ph = 64 + 3 * 56
    rx = list(struct.unpack_from('<II6Q', buf, ph))
    rw = struct.unpack_from('<II6Q', buf, ph+56)
    require(rx[:2] == [1,5] and rx[5] == rx[6] and rx[2]+rx[5] == len(buf), 'unexpected executable extent')
    delta = rx[3]-rx[2]
    def off(addr):
        return addr-delta if addr >= rx[3] else addr
    def word(addr):
        return struct.unpack_from('<I',buf,off(addr))[0]
    def edit(addr, old, new):
        require(word(addr) == old, f'unexpected instruction at {addr:x}')
        struct.pack_into('<I',buf,off(addr),new)
    # The only Custom-page constructor sets the validated page title through an ADR.
    title = data.index(b'Custom Bitrate\0',rx[2])+delta
    opens = []
    for p in range(rx[3],rx[3]+rx[5]-0x80,4):
        if word(p) != 0xd503245f:
            continue
        adr = word(p+0x7c)
        imm = ((adr>>5 & 0x7ffff)<<2) | (adr>>29 & 3)
        if adr & 0x9f00001f == 0x10000000 and p+0x7c+((imm ^ 0x100000)-0x100000) == title:
            opens.append(p)
    require(len(opens)==1, 'unexpected Custom-page constructor')
    page, = opens
    def pc(old):
        return page + old-0x700ada4
    def adr_target(site, reg):
        insn = word(site)
        require(insn & 0x9f00001f == 0x10000000 | reg, 'unexpected literal address')
        imm = ((insn >> 5 & 0x7ffff) << 2) | (insn >> 29 & 3)
        return site + ((imm ^ 0x100000) - 0x100000)

    rate_grid = pc(0x700aed4)
    interval_grid = rate_grid + 0xaf0
    for grid, special_key in [(rate_grid,b'Del'),(interval_grid,b'.')]:
        labels = off(adr_target(grid,23))
        items = [str(i).encode() for i in range(1,10)] + [special_key,b'0',b'OK']
        require(data[labels:labels+96] == b''.join(s.ljust(8,b'\0') for s in items), 'unexpected keypad bindings')
    def interval_site(public_addr):
        return interval_grid + public_addr - 0x700b924
    symbols = dict(SYMBOLS, add_behind=0x58b340, set_row_type=0x962c80, set_row_top_padding=0x964820, timer_class=0x70f0528, timer_enabled=0x597d10,
                   button_class=0x7157278,save_slot=pc(0x700b0b0),interval_slot=interval_site(0x700bb00),
                   interval_relabel=interval_site(0x700c160),new_object=0x4db3b0,
                   lock_object=0x4dbbf0,unlock_object=0x4dbd30,new_slot=0x4e1830,
                   image_class=0x714e128,confirm_bitmap=0x701f1f0,set_image=0x91e1d0,
                   bounds=0x589d00,group_add=0x58b350,group_remove=0x58b330,
                   rect_class=0x714dee8,rect_color=0x91cb70,icon_next=0x8c2894,set_key_text=0x9587b0,custom_label=pc(0x700b5fc),refresh_list=pc(0x700b7a0))
    require(word(symbols['custom_label']+8)==branch(symbols['custom_label']+8,pc(0x700b278),True),'unexpected label reader')
    start = (rx[3]+rx[5]+15)&~15
    body, labels = linked_text(assembly, start, symbols, ('after_interval_save', 'after_save', 'insert_mod_row', 'insert_quick_row', 'make_rate_ref', 'rate_icon', 'show_rate', 'style_key'), clang, lld)
    symbols.update(labels)
    # Follow the existing shortcut constructor. Its address varies with the built GUI.
    call = word(0x8dc564)
    require(call >> 26 == 0x25, 'missing settings constructor call')
    quick = 0x8dc564 + 4 * (((call & 0x3ffffff) ^ 0x2000000) - 0x2000000)
    require(word(0x8dc684) == 0xd287cd0e and word(0x8dcbcc) == 0x52800061, 'unexpected Display Settings row')
    require(word(0x7009038) == 0xd2853802 and word(0x700903c) == 0x8b020262, 'missing Camera Info anchor')
    edit(quick+0x3c,branch(quick+0x3c,0x58b350,True),branch(quick+0x3c,symbols['insert_quick_row'],True))
    edit(0x7009040,branch(0x7009040,0x58b340,True),branch(0x7009040,symbols['insert_mod_row'],True))
    edit(0x8c2860,0xf9441900,branch(0x8c2860,symbols['rate_icon']))
    edit(0x8c2904,branch(0x8c2904,SYMBOLS['new_ref'],True),branch(0x8c2904,symbols['make_rate_ref'],True))
    edit(0x87a484,0xd65f03c0,branch(0x87a484,symbols['show_rate']))
    old=word(0x89f8e4)
    old_func=0x89f8e4+((old&0x3ffffff)<<2)
    require(old>>26==5 and word(old_func)==0xd503245f and b'Mbps\0' in data[off(old_func):off(old_func)+64], 'unexpected tile callback')
    edit(0x89f8e4,old,branch(0x89f8e4,symbols['show_rate']))
    for addr,orig,target in [(0x700b174,symbols['refresh_list'],'after_save'),(0x700af20,0x9586e0,'style_key')]:
        site=pc(addr)
        edit(site,branch(site,orig,True),branch(site,symbols[target],True))
    # Keep the current value on successful saves. Invalid input and I/O errors
    # still use the original error paths.
    site=pc(0x700b230)
    require(word(site)&0x9f00001f==0x10000000, 'missing original confirmation text')
    edit(site,word(site),0xd503201f)
    interval_save = interval_site(0x700bc10)
    edit(interval_save,branch(interval_save,symbols['interval_relabel'],True),
         branch(interval_save,symbols['after_interval_save'],True))
    # The native success message would replace the numeric value. Only suppress
    # that conditional jump; validation and storage failures retain their messages.
    edit(interval_site(0x700bc8c),0x540004a0,0xd503201f)
    interval_style = interval_grid + (0x700af20-0x700aed4)
    edit(interval_style,branch(interval_style,SYMBOLS['set_background_color'],True),
         branch(interval_style,symbols['style_key'],True))
    def immediate(addr,base,old,new,shift):
        for grid in (rate_grid,interval_grid):
            edit(grid+addr-0x700aed4,base|(old<<shift),base|(new<<shift))
    for addr,reg,old,new in [(0x700ae7c,1,20,16),(0x700ae80,9,88,80),(0x700ae88,2,370,464),
                            (0x700ae8c,9,144,150),(0x700af6c,1,25,0),(0x700af70,2,25,0)]:
        immediate(addr,0xd2800000|reg,old,new,5)
    for addr,reg,old,new in [(0x700af60,1,60,88),(0x700af9c,12,120,152),(0x700afa8,12,70,100)]:
        immediate(addr,0x52800000|reg,old,new,5)
    for addr,reg,old,new in [(0x700afa4,11,20,16),(0x700afb0,10,150,160),
                            (0x700afb8,11,110,144),(0x700afbc,10,60,88)]:
        immediate(addr,0x11000000|(reg<<5)|reg,old,new,10)
    for addr in [0x700ae9c,0x700af34]:
        immediate(addr,0x91000000,0xc48,0xe78,10)
    require(start+len(body)<(rw[3]&~4095),'no space for GUI extension')
    buf.extend(bytes(start-rx[3]-rx[5])+body)
    rx[5]=rx[6]=len(buf)-rx[2]
    struct.pack_into('<II6Q',buf,ph,*rx)
    return bytes(buf)
