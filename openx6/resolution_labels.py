"""Rename only FreeFrame's two full-readout labels and restore the vendor captions on exit."""
import struct

OLD_TABLE = b'\x01\x03\x06\x02\x047K Full\x006K Full\x005K\x003K\x00'
NEW_TABLE = OLD_TABLE[:5] + bytes(16) + OLD_TABLE[21:]
GUARD = bytes.fromhex('ae054079df2d0171c1000054')
def require(value, text):
    if not value:raise ValueError(text)
def branch(source,target):
    step=target-source
    require(step%4==0 and -(1<<27)<=step<1<<27,'branch out of range')
    return 0x14000000 | (step//4 & 0x3ffffff)
def patch(data):
    require(data[:6]==b'\x7fELF\x02\x01' and struct.unpack_from('<H',data,18)[0]==183,'expected little-endian ARM64')
    require(data.count(OLD_TABLE)==1 and len(NEW_TABLE)==len(OLD_TABLE),'unexpected resolution table')
    table=data.index(OLD_TABLE)
    where=data.rfind(GUARD,table-96,table)
    require(where>=0 and data[where+len(GUARD):where+len(GUARD)+4]==bytes.fromhex('ea7bbfa9'),'unexpected caption selector')
    phoff=struct.unpack_from('<Q',data,32)[0];stride,count=struct.unpack_from('<HH',data,54)
    require(stride==56,'unexpected program headers')
    headers=[(phoff+i*stride,list(struct.unpack_from('<IIQQQQQQ',data,phoff+i*stride))) for i in range(count)]
    rx=[(ph,s) for ph,s in headers if s[:2]==[1,5] and s[2]<=where+4<table<s[2]+s[5]]
    require(len(rx)==1,'selector not in a unique executable mapping')
    ph,s=rx[0];require(s[2]+s[5]==len(data) and s[5]==s[6],'unexpected executable layout')
    site=where+4-s[2]+s[3]; at=(len(data)+3)&~3;helper=at-s[2]+s[3]
    # W20 is 2 for FreeFrame, 1 when restoring native labels. W14 is
    # the second UTF-16 character; the existing check on the first still runs.
    # cmp w20,#2; b.ne native; cmp w14,#75; b resume; native: cmp w14,#46; b resume
    body=struct.pack('<6I',0x71000a9f,0x54000061,0x71012ddf,branch(helper+12,site+4),0x7100b9df,branch(helper+20,site+4))
    labels=(b'7.3K Full\x00',b'6.7K Full\x00')
    original_offsets=(table+5,table+13)
    redirects=[]
    for label,old_offset in zip(labels,original_offsets):
        positions=[]
        for off in range(where-180,where,4):
            ins,=struct.unpack_from('<I',data,off)
            if ins&0x9f00001f!=0x10000000:continue
            imm=((ins>>29)&3)|(((ins>>5)&0x7ffff)<<2)
            if imm&(1<<20):imm-=1<<21
            if off+imm==old_offset:positions.append(off)
        require(len(positions)==1,'could not locate the exact FreeFrame caption reference')
        off=positions[0];dest=helper+len(body);relative=dest-(off-s[2]+s[3])
        require(-(1<<20)<=relative<1<<20,'caption out of ADR range')
        redirects.append((off,0x10000000|((relative&3)<<29)|(((relative>>2)&0x7ffff)<<5)))
        body+=label
    next_map=min(x[3] for _,x in headers if x[0]==1 and x[3]>helper)
    require(helper+len(body)<=next_map&~4095,'payload would overlap following mapping')
    buf=bytearray(data)
    buf[table:table+len(OLD_TABLE)]=NEW_TABLE
    for off,ins in redirects:struct.pack_into('<I',buf,off,ins)
    struct.pack_into('<I',buf,where+4,branch(site,helper))
    buf.extend(bytes(at-len(buf))+body);s[5]=s[6]=len(buf)-s[2]
    struct.pack_into('<IIQQQQQQ',buf,ph,*s)
    return bytes(buf)
