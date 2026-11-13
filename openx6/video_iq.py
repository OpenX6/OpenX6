"""Build the 1.2.36 native video image controls from checked ARM64 instructions."""
import struct
from .gui_payload import linked_text
from .settings_keypads import require, branch


EXPORTS = {
    'gui': ('labels', 'mask_attach', 'mask_get', 'store_selection', 'read_first', 'read_again', 'wire', 'nr_write', 'nr_read_path'),
    'iq': ('mapped_init', 'mode_choice', 'keep_choice', 'gain', 'tf_call', 'tf_master', 'tf_pass', 'anr_channels'),
    'cam': ('mapped_init', 'abf_call', 'abf_factor', 'hnr_enable'),
}

def assemble(source, address, symbols, clang, lld):
    body, labels = linked_text(source, address, symbols, EXPORTS[source.stem], clang, lld)
    symbols.update(labels)
    return body


def patch(data, kind, sources, clang, lld, *, extension_menu=False):
    buf = bytearray(data)
    def header(i):
        return list(struct.unpack_from('<II6Q',buf,64+56*i))
    def store(i,h):
        struct.pack_into('<II6Q',buf,64+56*i,*h)
    def offset(addr):
        for i in range(struct.unpack_from('<H',buf,56)[0]):
            h=header(i)
            if h[0]==1 and h[3]<=addr<h[3]+h[5]:
                return addr-h[3]+h[2]
        raise ValueError(f'unmapped instruction {addr:x}')
    def edit(addr,old,new):
        i=offset(addr)
        require(struct.unpack_from('<I',buf,i)[0]==old,f'unexpected {kind} instruction at {addr:x}')
        struct.pack_into('<I',buf,i,new)
    def hook(addr,old,label,link=False):
        edit(addr,old,branch(addr,symbols[label],link))

    if kind=='gui':
        rx,rw=header(3),header(4)
        start=(rx[3]+rx[5]+15)&~15
        require(rx[:2]==[1,5] and rx[2]+rx[5]==len(buf),'unexpected GUI extent')
        symbols=dict(new_string=0x4e3e80,string3=0x57e8d0,string4=0x57e920,labels_next=0x81d848,
                     attach_next=0x83ddd4,get_next=0x83dadc,store_next=0x874d3c,store_abort=0x874dc4,
                     read_first_next=0x86aeb4,read_again_next=0x86aedc,send_bll=0x4f2120,nr_read_next=0x700a2a8)
        body=assemble(sources/'gui.S',start,symbols,clang,lld)
        require(start+len(body)<(rw[3]&~4095),'GUI text overflow')
        for addr in (0x81d770,0x81d7c4,0x81d818):
            edit(addr,0x52800061,0x528000a1)
        for addr,old,label in [(0x81d844,0xf940bbe0,'labels'),(0x83ddd0,0xd2936d07,'mask_attach'),
                               (0x83dad8,0xd2936d04,'mask_get'),(0x874d38,0xa9065bf5,'store_selection'),
                               (0x86aeb0,0xb9404400,'read_first'),(0x86aed8,0xb9404405,'read_again'),
                               (0x700a304,0x71000c1f,'nr_write'),(0x700a2a4,0x30000ca1,'nr_read_path')]:
            hook(addr,old,label)
        for addr in (0x632060,0x657f00):
            hook(addr,branch(addr,symbols['send_bll'],True),'wire',True)
        if extension_menu:
            site = 0x7009304
            jump, = struct.unpack_from('<I', buf, offset(site))
            require(jump >> 26 == 5, 'Missing extension menu branch')
            dest = site + ((jump & 0x3ffffff) - ((jump & 0x2000000) << 1)) * 4
            adr, callback, parent, add_row = struct.unpack_from('<4I', buf, offset(dest))
            rel = ((adr >> 29) & 3) | (((adr >> 5) & 0x7ffff) << 2)
            rel -= (rel & 0x100000) << 1
            require(adr & 0x9f00001f == 0x10000001 and dest + rel == 0x700a3c0
                    and callback & 0x9f00001f == 0x10000002 and parent == 0xaa1303e0
                    and add_row >> 26 == 0x25, 'Missing noise control in extension menu')
        else:
            edit(0x7009304,branch(0x7009304,0x70093b4),branch(0x7009304,0x7009364))
        pos=offset(0x700a3c0)
        require(buf[pos:pos+17]==b'Video Spatial NR\0' and buf[pos+32:pos+39]==b'Native\0','unexpected NR labels')
        title=b'Video Noise Reduction\0'
        buf[pos:pos+len(title)]=title
        buf.extend(bytes(start-rx[3]-rx[5])+body)
        rx[5]=rx[6]=len(buf)-rx[2]
        store(3,rx)
        return bytes(buf),symbols

    require(kind in ('iq','cam'),'unknown image')
    segment=0 if kind=='iq' else 3
    rx,rw,dynamic=header(segment),header(1),header(2)
    require(rx[:2]==[1,5] and rw[:2]==[1,6],'unexpected library layout')
    start=(rx[3]+rx[5]+15)&~15
    mapped=(rw[3]+rw[6]+7)&~7
    if kind=='iq':
        symbols=dict(original_init=0x10300,mapped=mapped,noise_mapped=mapped+8,substr=0x10370,
                     effective_key=0x41a10,stock_key=0x1d1a0,no_key=0x1d1b8,keep_next=0x1d1c0,
                     gain_next=0x1d210,tf_mapper=0x331f0,tf_master_next=0x349ec,
                     tf_pass_next=0x34d3c,anr_next=0x170c4)
        sites=[(0x1d19c,0xb40000e0,'mode_choice'),(0x1d1bc,0x52815909,'keep_choice'),
               (0x1d20c,0xbc7a5917,'gain'),(0x349e8,0x39400129,'tf_master'),
               (0x34d38,0x38696a77,'tf_pass'),(0x170c0,0x394241ac,'anr_channels')]
    else:
        symbols=dict(original_init=0x38eb60,noise_mapped=mapped,original_abf=0xc09930,
                     abf_next=0xfafd3c,hnr_next=0xb4ef6c)
        sites=[(0xfafd38,0xbd418b21,'abf_factor'),(0xb4ef68,0xb9401a68,'hnr_enable')]
    body=assemble(sources/(kind+'.S'),start,symbols,clang,lld)
    for addr,old,label in sites:
        hook(addr,old,label)
    if kind=='iq':
        hook(0x32d90,branch(0x32d90,0x331f0,True),'tf_call',True)
    else:
        hook(0xa31a5c,branch(0xa31a5c,0xc09930,True),'abf_call',True)
    index=start-rx[3]+rx[2]
    if segment==0:
        require(index+len(body)<=rw[2] and not any(buf[index:index+len(body)]),'occupied IQ text')
    else:
        require(rx[2]+rx[5]==len(buf) and mapped+8<rx[3],'unexpected HAL extent')
        buf.extend(bytes(index-len(buf)))
    buf[index:index+len(body)]=body
    rx[5]=rx[6]=start+len(body)-rx[3]
    rw[6]=symbols['noise_mapped']+8-rw[3]
    store(segment,rx)
    store(1,rw)
    loc,=[i for i in range(dynamic[2],dynamic[2]+dynamic[5],16)
          if struct.unpack_from('<QQ',buf,i)==(12,symbols['original_init'])]
    struct.pack_into('<Q',buf,loc+8,symbols['mapped_init'])
    return bytes(buf),symbols
