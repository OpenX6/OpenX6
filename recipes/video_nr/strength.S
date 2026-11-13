.text
.global video_nr_strength
video_nr_strength:
 bti j
 sub sp,sp,#128
 stp x8,x9,[sp]
 stp x10,x11,[sp,#16]
 stp x12,x13,[sp,#32]
 stp x14,x15,[sp,#48]
 stp x16,x17,[sp,#64]
 stp q0,q1,[sp,#80]
 mrs x8,nzcv
 str x8,[sp,#112]
 // Current source-locked ANR input. Never change a shared tuning profile.
 ldr x8,[x22,#0x2e8]
 cbz x8,done
 ldr x8,[x8,#8]
 cbz x8,done
 adr x9,profile_prefix
 mov w10,#45
prefix_loop:
 ldrb w11,[x8],#1
 ldrb w12,[x9],#1
 cmp w11,w12
 b.ne done
 subs w10,w10,#1
 b.ne prefix_loop
 cmp w27,#1
 b.hi done
 ldr w13,[sp,#0x108]
 cmp w13,#3
 b.hs done
 ldr w14,[x22,#0x50]
 cbz w14,done
 cmp w14,#4
 b.hi done
 adr x8,nr_gain
 ldr s1,[x8]
 adrp x8,native_curves
 add x8,x8,:lo12:native_curves
 mov w9,#0x1338
 umaddl x8,w27,w9,x8
 mov w9,#0x668
 umaddl x8,w13,w9,x8
 add x8,x8,#8
 mov x17,x8
.global nr_before, nr_after
nr_before:
 fmov s0,#1.0
 fcmp s1,s0
 b.eq nr_after
 b.hi nr_after
 fcmp s1,#0.0
 b.lt nr_after
 mov w10,#6
 // Six native threshold curves, each four passes of 17 floats.
curve_loop:
 mov x9,x8
 mov w11,#17
 mul w11,w11,w14
sample_loop:
 ldr s0,[x9]
 fmul s0,s0,s1
 frintp s0,s0
 str s0,[x9],#4
 subs w11,w11,#1
 b.ne sample_loop
 add x8,x8,#0x110
 subs w10,w10,#1
 b.ne curve_loop
nr_after:
 nop
done:
 ldr x8,[sp,#112]
 msr nzcv,x8
 ldp q0,q1,[sp,#80]
 ldp x16,x17,[sp,#64]
 ldp x14,x15,[sp,#48]
 ldp x12,x13,[sp,#32]
 ldp x10,x11,[sp,#16]
 ldp x8,x9,[sp]
 add sp,sp,#128
 // Displaced native instruction; original PAC/BTI and epilogue are preserved.
 .incbin "displaced.bin"
 b native_resume
.balign 4
.global nr_gain
nr_gain: .float NR_GAIN
profile_prefix: .ascii "sensor_imx01f_usecase1_video_usecase2_double_"
