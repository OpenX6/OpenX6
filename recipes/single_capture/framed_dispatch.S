.text
.global framed_dispatch
framed_dispatch:
 // The original render hook saves its ChiRenderNode* at [sp] for fallback.
 stp x0,x1,[sp,#-48]!
 stp x2,x3,[sp,#16]
 mov x0,#-100
 adr x1,window
 add x2,sp,#32
 mov x3,#1
 mov x8,#78
 svc #0
 cmp x0,#1
 ldrb w9,[sp,#32]
 ldp x2,x3,[sp,#16]
 ldp x0,x1,[sp],#48
 b.ne fish_dispatch
 cmp w9,#80
 b.eq small
 cmp w9,#81
 b.eq square
 cmp w9,#84
 b.ne fish_dispatch
 mov w3,#1056
 b check_ports
small: mov w3,#496
 b check_ports
square: mov w3,#655
check_ports:
 // The render object's CHI session is the wrapper Node. Node::GetPipeline
 // and Pipeline::HasSnapshotJPEGStream use this descriptor and 0x30 stride.
 ldr x10,[sp]
 cbz x10,fish_dispatch
 ldr x10,[x10,#0x10]
 cbz x10,fish_dispatch
 ldr x10,[x10,#0x150]
 cbz x10,fish_dispatch
 ldr x10,[x10,#0x168]
 cbz x10,fish_dispatch
 ldr w11,[x10,#0x10]
 cmp w11,#4
 b.lo fish_dispatch
 add x10,x10,#0x38
 mov w11,#4
 mov x14,#480
 movk x14,#600,lsl #32
ports:
 ldr x12,[x10],#0x30
 cbz x12,fish_dispatch
 ldr x12,[x12]
 cbz x12,fish_dispatch
 ldur x13,[x12,#4]
 cmp x13,x14
 b.ne fish_dispatch
 subs w11,w11,#1
 b.ne ports
 cbz x0,fish_dispatch
 cbz x1,fish_dispatch
 ldr w9,[x1,#16]
 cmp w9,#3
 b.eq framed_nv12
 b framed_p010
window: .asciz "/run/x6-nativefish/pano-window"
