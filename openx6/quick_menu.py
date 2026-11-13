"""Patch the X6 quick settings tile and Camera Info presentation callbacks."""
from .gui_payload import linked_text
import struct
from .common import require

LABEL='1.2.36 OpenX6 1.0'
SYMBOLS=dict(version_update_next=0x87d478,version_init_next=0x8cd0a4,row_class=0x715c318,
             new_object=0x4db3b0,group_add=0x58b350,quick_title=0x70d8ac8,localize=0x4e8aa0,
             set_title=0x963270,new_slot=0x4e1830,ew_signal=0x4e1880,quick_page=0x7134bb8,
             create_subpage=0x7522b0,new_text=0x4e4010,set_subtitle=0x955840)

def patch(data,source,clang,lld,label,settings=True):
    require(label==LABEL and label.encode()+b'\0' not in data,'Unexpected display label')
    b=bytearray(data)
    ph=64+3*56
    rx=list(struct.unpack_from('<II6Q',b,ph));rw=struct.unpack_from('<II6Q',b,ph+56)
    require(rx[:2]==[1,5] and rx[5]==rx[6] and rx[2]+rx[5]==len(b),'Unrecognized GUI code layout')
    start=(rx[3]+rx[5]+15)&~15
    code, labels = linked_text(source, start, SYMBOLS, ('info_update', 'info_init', 'rate_unit', 'settings_row'), clang, lld)
    syms = labels
    require(code.count(label.encode()+b'\0')==1 and start+len(code)<(rw[3]&~4095),'Invalid quick menu payload')
    def edit(site,old,new):
        require(struct.unpack_from('<I',b,site)[0]==old,f'Unexpected native GUI at {site:x}')
        struct.pack_into('<I',b,site,new)
    def branch(site,old,name,link=False):
        delta=syms[name]-site
        require(delta%4==0 and abs(delta)<1<<27,'Branch range')
        edit(site,old,(0x94000000 if link else 0x14000000)|((delta//4)&0x3ffffff))
    branch(0x87d474,0x10000120,'info_update')
    branch(0x8cd0a0,0x10d81fc0,'info_init')
    require(b[0x87d498:0x87d4a8]==b'1.2.36 O 1.0'.ljust(16,b'\0'),'Wrong previous display field')
    b[0x87d498:0x87d4a8]=bytes(16)
    if settings:
        # The short gear button opens Settings. Only the wide Quick Capture tile moves.
        def adrp(site,reg,addr):
            off=(addr>>12)-(site>>12)
            return 0x90000000|((off&3)<<29)|(((off>>2)&0x7ffff)<<5)|reg
        edit(0x8ab964,0xb0034445,adrp(0x8ab964,5,0x7132820))
        edit(0x8ab968,0x912640a6,0x910000a6|(0x820<<10))
        edit(0x8ab970,0x9108a0c1,0x910000c1)
        # Both initialization and language refresh use the localized video bitrate title.
        edit(0x8c2820,0xf9476cc0,0xf94000c0|((0xfa8//8)<<10))
        edit(0x8c7770,0xf9476c60,0xf9400060|((0xfa8//8)<<10))
        # Use the native card-write icon and remove the displaced Quick Capture status.
        edit(0x8c2860,0xf9417500,0xf9400100|((0x830//8)<<10))
        edit(0x87a480,0xd503233f,0xd503245f)
        edit(0x87a484,0xa9be7bfd,0xd65f03c0)
        # onCameraMode is an indirect EW slot: retain its BTI landing pad.
        edit(0x89f8e0,0xd503233f,0xd503245f)
        branch(0x89f8e4,0xa9bd7bfd,'rate_unit')
        branch(0x8dc564,0x97f014c7,'settings_row',True)
    b.extend(bytes(start-rx[3]-rx[5])+code)
    rx[5]=rx[6]=len(b)-rx[2]
    struct.pack_into('<II6Q',b,ph,*rx)
    return bytes(b)
