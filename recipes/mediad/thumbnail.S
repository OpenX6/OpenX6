.text
.global gate
.macro restore
 ldr x9,[sp,#64]
 msr nzcv,x9
 ldp x0,x1,[sp,#0]
 ldp x2,x3,[sp,#16]
 ldp x8,x9,[sp,#32]
 ldp x10,x30,[sp,#48]
 add sp,sp,#96
.endm
gate:
 hint #34
 sub sp,sp,#96
 stp x0,x1,[sp,#0]
 stp x2,x3,[sp,#16]
 stp x8,x9,[sp,#32]
 stp x10,x30,[sp,#48]
 mrs x9,nzcv
 str x9,[sp,#64]
 adr x1,disabled_path
 bl flag
 cbnz w0,skip
 adr x1,owned_path
 bl flag
 cbnz w0,skip
 restore
 b native_callback
skip:
 restore
 ret
flag:
 mov x0,#-100
 mov x2,#0x800
 movk x2,#0xa,lsl#16
 mov x3,#0
 mov x8,#56
 svc #0
 tbnz x0,#63,no
 mov x9,x0
 strh wzr,[sp,#80]
 add x1,sp,#80
 mov x2,#2
 mov x8,#63
 svc #0
 mov x10,x0
 mov x0,x9
 mov x8,#57
 svc #0
 cmp x10,#1
 b.ne no
 ldrb w0,[sp,#80]
 cmp w0,#1
 cset w0,eq
 ret
no:
 mov w0,#0
 ret
.balign 4
disabled_path: .asciz "/data/x6-owner-features/preview-v1/disabled"
owned_path: .asciz "/run/x6-preview-control/owned"
