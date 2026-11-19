import ctypes
cam='/usr/lib/hw/camera.qcom.so'
probe_defs={
 'primary':('108af7c','node=+72(%x19):string req=+8(%x20):u64 clk=+24(%x20):u64 exp=%x0:u64 applied_ptr=-80(%x29):x64 dur=%x22:u64'),
 'aux':('108ae28','node=+72(%x19):string req=+8(%x20):u64 clk=+24(%x20):u64 exp=%x0:u64'),
 'expo':('13ae1c8','sensor=%x23:x64 sof=%x19:u64 dur=%x20:u64 start=%x22:u64'),
 'release':('1421360','port=+0(%x1):u32 req=%x2:u64 buf=%x3:x64 name=+0(+8(%x3)):string'),
 'fence':('898e80','node=+72(%x0):string req=%x2:u64 req2=+24(%x1):u64 port=+0(+16(%x1)):u32 status=+96(%x1):u32 buf=+0(+48(%x1)):x64 name=+0(+8(+0(+48(%x1)))):string fd=+60(+0(+48(%x1))):s32 childfd=+60(+1512(+0(+48(%x1)))):s32 addr=+48(+0(+48(%x1))):x64 childaddr=+48(+1512(+0(+48(%x1)))):x64 handle=+56(+0(+48(%x1))):u32'),
}
def up(line):
 fd=os.open(T/'uprobe_events',os.O_WRONLY|os.O_APPEND)
 try:os.write(fd,(line+'\n').encode())
 finally:os.close(fd)
