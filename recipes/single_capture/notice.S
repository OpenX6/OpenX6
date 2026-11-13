.text
.global entry
entry:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 bl timers
 mov w19,w0
 bl ticks
 adrp x21,last_check
 add x21,x21,:lo12:last_check
 ldr w9,[x21]
 sub w10,w0,w9
 cmp w10,#1500
 b.lo done
 str w0,[x21]
 mov x0,#-100
 adr x1,marker
 mov x2,#0x800
 movk x2,#0xa,lsl #16 // NONBLOCK, NOFOLLOW, CLOEXEC
 mov x3,#0
 mov x8,#56
 svc #0
 tbnz x0,#63,done
 mov w20,w0
 mov w1,#5 // LOCK_SH | LOCK_NB
 mov x8,#32
 svc #0
 mov x22,x0
 mov w0,w20
 mov x8,#57
 svc #0
 cmn x22,#11 // EWOULDBLOCK: the coordinator is actively staging
 b.ne done
 adrp x0,global_obj
 add x0,x0,:lo12:global_obj
 adrp x1,global_class
 add x1,x1,:lo12:global_class
 bl get_auto
 cbz x0,done
 ldr x20,[x0,#0x7d0] // native ApplicationApplication field
 cbz x20,done
 adr x0,message
 mov w1,#(message_end-message)
 bl new_utf8
 cbz x0,done
 mov x1,x0
 mov x0,x20
 mov w2,#0
 mov w3,#2200
 bl show_toast
done:
 mov w0,w19
 ldp x19,x20,[sp,#16]
 ldp x21,x22,[sp,#32]
 ldp x29,x30,[sp],#48
 ret
marker: .asciz "/run/x6-sd-update-notice"
message: .ascii "Preparing update on SD. Keep card inserted.\n正在准备更新，请勿拔卡"
message_end:
