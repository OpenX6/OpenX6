.arch armv8.3-a+fp
.text
// The caller provides a writable 65x49 array of (source x,y), normalized to [0,1].
// The original conversion extrapolates the border and converts to ICA virtual coordinates.
cmp w2, #65
b.ne original
cmp w3, #49
b.ne original
cmp w5, #67
b.ne original
cmp w6, #51
b.ne original
cbz x1, original
stp x0, x1, [sp, #-16]!
mov x0, #-100
adr x1, marker
stp x2, x3, [sp, #-16]!
mov x2, #0
mov x8, #48 // faccessat(AT_FDCWD, marker, F_OK)
svc #0
cmp x0, #0
ldp x2, x3, [sp], #16
ldp x0, x1, [sp], #16
b.ne reflect_if_enabled
ldr s7,[x20,#0x1358]
fmov s6,#1.0
fcmp s7,#0.0
b.le original
ldr w10,[x20,#0x135c]
ldr w11,[x20,#0x1360]
cmp w10,w11
b.ne rectangular_input
fdiv s7,s6,s7
fmax s7,s7,s6
b scale_ready
rectangular_input:
 // The recording readout can itself be cropped. Do not sample past its edges.
 fmov s7,s6
scale_ready:
 // Query the framing code published by the existing preview controller.
 stp x0,x1,[sp,#-48]!
 stp x2,x3,[sp,#16]
 mov x0,#-100
 adr x1,window
 add x2,sp,#32
 mov x3,#1
 mov x8,#78
 svc #0
 cmp x0,#1
 ldrb w13,[sp,#32]
 ldp x2,x3,[sp,#16]
 ldp x0,x1,[sp],#48
 fmov s16,#1.0
 fmov s17,#1.0
 b.ne grid
 ldr w10,[x20,#0x135c]
 cmp w10,#1920
 b.ne record_geometry
 cmp w13,#65
 b.ne native6
 ldr s18,crop5
 b change
native6:
 cmp w13,#66
 b.ne rect6
 ldr s18,six
 b change
rect6:
 cmp w13,#67
 b.ne rect4
 ldr s18,crop6
 ldr s17,aspect6
 b change
rect4:
 cmp w13,#68
 b.ne grid
 ldr s18,crop4
 ldr s17,aspect4
 b change
record_geometry:
 cmp w13,#67
 b.ne record4
 cmp w10,#1000
 b.ne grid
 ldr s18,rec6
 ldr s17,fit6
 b change
record4:
 cmp w13,#68
 b.ne grid
 cmp w10,#960
 b.ne grid
 ldr s18,rec4
 ldr s16,fit4
change:
 fmul s7,s7,s18
grid:
 fmov s5,#0.5
mov x9, x1
mov w10, #0
mov w12, #64
ucvtf s3, w12
mov w12, #48
ucvtf s4, w12
row:
 ucvtf s1,w10
 fdiv s1,s1,s4
 fsub s1,s1,s5
 fabs s19,s1
 fadd s19,s19,s19
 fcmp s19,s17
 cset w14,gt
 fmadd s1,s1,s7,s5
 mov w11,#0
col:
 ucvtf s0,w11
 fdiv s0,s0,s3
 fsub s0,s0,s5
 fabs s19,s0
 fadd s19,s19,s19
 fcmp s19,s16
 b.gt dark
 cbnz w14,dark
 fmadd s0,s0,s7,s5
 // Outside the centered square, sample the dark sensor corner. The ICA
 // otherwise repeats the last lit edge pixel across the pillarbox.
 fcmp s0,#0.0
 fccmp s0,s6,#2,ge
 b.hi pillar
 stp s0,s1,[x9],#8
 b advance
dark:
 stp wzr,wzr,[x9],#8
 b advance
pillar:
 fcmp s0,s6
 fcsel s2,s6,s5,gt
 b.gt right_black
 fmov s2,wzr
right_black:
 str s2,[x9],#4
 str wzr,[x9],#4
advance:
 add w11,w11,#1
 cmp w11,#65
 b.lo col
 add w10,w10,#1
 cmp w10,#49
 b.lo row
reflect_if_enabled:
 ldr w9,[sp,#176]
 cmp w9,#1
 b.ne original
 stp x0,x1,[sp,#-32]!
 stp x2,x3,[sp,#16]
 mov x0,#-100
 adr x1,flip_marker
 mov x2,#0
 mov x8,#48
 svc #0
 cmp x0,#0
 ldp x2,x3,[sp,#16]
 ldp x0,x1,[sp],#32
 b.ne original
 // One temporary 65x49 float-pair grid, padded to a 16-byte stack boundary.
 mov x9,#25488
 sub sp,sp,x9
 str x30,[sp,#25480]
 mov x9,sp
 mov x10,x1
 mov w11,#49
flip_row:
 add x12,x10,#512
 mov w13,#65
flip_pixel:
 ldr x14,[x12],#-8
 str x14,[x9],#8
 subs w13,w13,#1
 b.ne flip_pixel
 add x10,x10,#520
 subs w11,w11,#1
 b.ne flip_row
 mov x1,sp
 bl stock_convert
 ldr x30,[sp,#25480]
 mov x9,#25488
 add sp,sp,x9
 ret
original: b stock_convert
flip_marker: .asciz "/run/x6-nativefish/unmirror-video"
marker: .asciz "/run/x6-nativefish/enabled"

window: .asciz "/run/x6-nativefish/ff-window"
.zero 3
.balign 4
crop5: .float 0.639583333
six: .float 0.900000000
crop6: .float 0.781250000
aspect6: .float 0.661333333
crop4: .float 1.0
aspect4: .float 0.783333333
rec6: .float 1.134072581
fit6: .float 0.881777778
rec4: .float 1.0
fit4: .float 0.957446809
