#include "paths.h"
#include "map.inc"
.text
.global mapped_init,abf_call,abf_factor,hnr_enable
mapped_init:
 bti c
 stp x0,x1,[sp,#-32]!
 stp x2,x30,[sp,#16]
 map_pref noise,noise_mapped
 ldp x2,x30,[sp,#16]
 ldp x0,x1,[sp],#32
 b original_init
// x9 = ISPInputData. Use the current request's raw tuning tuple. The compact
// "default" tuple carries no feature3 and must take the native path.
request_level:
 adrp x10,noise_mapped
 ldr x10,[x10,:lo12:noise_mapped]
 cbz x10,zero
 ldrb w10,[x10]
 sub w11,w10,#1
 cmp w11,#2
 b.hi zero
 add x9,x9,#0x24,lsl #12
 ldr x9,[x9,#0xba0]
 cbz x9,zero
 ldr w11,[x9]
 cmp w11,#1
 b.eq zero
 ldr w11,[x9,#0x10]    // usecase1: video_preview=2, video=3
 sub w11,w11,#2
 cmp w11,#1
 b.hi zero
 ldr w11,[x9,#0x30]    // feature3: iLog=1
 cmp w11,#1
 csel w10,w10,wzr,eq
 ret
zero:
 mov w10,wzr
 ret
abf_call:
 stp x28,x30,[sp,#-16]!
 mov x9,x21
 bl request_level
 adr x28,minimal_tag
 cmp w10,#3
 csel x28,x28,xzr,eq
 bl original_abf
 ldp x28,x30,[sp],#16
 ret
abf_factor:
 // The ABF calculator saves incoming x28 at FP+16. Intermediary
 // IQInterface::BPCABF411CalculateSetting preserves it through the call.
 stp x16,x17,[sp,#-16]!
 ldr x16,[x29,#16]
 adr x17,minimal_tag
 sub x16,x16,x17
 ldr s1,[x25,#0x188]
 cbnz x16,1f
 fmov s1,wzr
1:
 ldp x16,x17,[sp],#16
 b abf_next
hnr_enable:
 ldr w8,[x19,#0x18]
 cbz w8,1f
 stp x9,x10,[sp,#-32]!
 stp x11,x30,[sp,#16]
 mov x9,x20
 bl request_level
 cmp w10,#2
 csel w8,wzr,w8,hs
 ldp x11,x30,[sp,#16]
 ldp x9,x10,[sp],#32
1:
 b hnr_next
minimal_tag: .byte 0
noise: .asciz NOISE
