.text
.macro restore_callers
ldr x9,[sp,#160]
msr nzcv,x9
ldp x0, x1, [sp, #0]
ldp x2, x3, [sp, #16]
ldp x4, x5, [sp, #32]
ldp x6, x7, [sp, #48]
ldp x8, x9, [sp, #64]
ldp x10, x11, [sp, #80]
ldp x12, x13, [sp, #96]
ldp x14, x15, [sp, #112]
ldp x16, x17, [sp, #128]
ldp x18,x30,[sp,#144]
add sp,sp,#192
.endm
.global entry
entry:
sub sp, sp, #192
stp x0, x1, [sp, #0]
stp x2, x3, [sp, #16]
stp x4, x5, [sp, #32]
stp x6, x7, [sp, #48]
stp x8, x9, [sp, #64]
stp x10, x11, [sp, #80]
stp x12, x13, [sp, #96]
stp x14, x15, [sp, #112]
stp x16, x17, [sp, #128]
stp x18, x30, [sp, #144]
mrs x9, nzcv
str x9, [sp,#160]
mov x0,#-100
adr x1,flag
mov x2,#0x8800
movk x2,#8,lsl #16
mov x3,#0
mov x8,#56
svc #0
cmp x0,#0
b.lt native
str x0,[sp,#168]
add x1,sp,#176
mov x2,#2
mov x8,#63
svc #0
str x0,[sp,#184]
ldr x0,[sp,#168]
mov x8,#57
svc #0
ldr x0,[sp,#184]
cmp x0,#1
b.ne native
ldrb w0,[sp,#176]
cmp w0,#49
b.ne native
restore_callers
b active_target
native:
restore_callers
b.ne 1f
b taken
1: b fall
flag: .asciz "/run/x6-ssd/active"
