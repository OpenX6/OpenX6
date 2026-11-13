.text
.global hnr_switch
hnr_switch:
 bti j
 sub sp,sp,#80
 stp x9,x10,[sp]
 stp x11,x12,[sp,#16]
 stp x13,x14,[sp,#32]
 stp x15,x16,[sp,#48]
 mrs x9,nzcv
 str x9,[sp,#64]
 cmp w8,#1
 b.ne done
 adr x9,disable_hnr
 ldrb w10,[x9]
 cmp w10,#1
 b.ne done
 // Match current request's VIDEO DOUBLE profile, never its shared tuning table.
 ldr w12,[x19,#0x230]
 cmp w12,#1
 b.hi done
 ldr x9,[x19,#0x318]
 cbz x9,done
 ldr x9,[x9,#8]
 cbz x9,done
 adr x10,profile_prefix
 mov w11,#45
compare:
 ldrb w12,[x9],#1
 ldrb w13,[x10],#1
 cmp w12,w13
 b.ne done
 subs w11,w11,#1
 b.ne compare
 mov w8,wzr
.global hnr_after
hnr_after:
 nop
done:
 ldr x9,[sp,#64]
 msr nzcv,x9
 ldp x15,x16,[sp,#48]
 ldp x13,x14,[sp,#32]
 ldp x11,x12,[sp,#16]
 ldp x9,x10,[sp]
 add sp,sp,#80
 .incbin "displaced.bin"
 b native_resume
.global disable_hnr
disable_hnr: .byte HNR_OFF
profile_prefix: .ascii "sensor_imx01f_usecase1_video_usecase2_double_"
