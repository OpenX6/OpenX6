#include "paths.h"
#include "map.inc"
.text
.global mapped_init, mode_choice, keep_choice, gain
// The loader serializes DT_INIT. Mapping once avoids filesystem access per frame.
mapped_init:
 bti c
 stp x0,x1,[sp,#-48]!
 stp x2,x30,[sp,#16]
 map_pref pref,mapped
 map_pref noise,noise_mapped
done:
 ldp x2,x30,[sp,#16]
 ldp x0,x1,[sp],#48
 b original_init
mode_choice:
 mov w26,#0
 cbz x0,no_key
 adrp x9,mapped
 ldr x9,[x9,:lo12:mapped]
 cbz x9,stock_key
 ldrb w26,[x9]
 sub w9,w26,#3
 cmp w9,#1
 b.hi no_override
 stp x0,x30,[sp,#-16]!
 adr x1,video_key
 bl substr
 cmp x0,#0
 csel w26,w26,wzr,ne
 ldp x0,x30,[sp],#16
 b stock_key
no_override:
 mov w26,#0
 b stock_key
keep_choice:
 cmp w8,#0
 csel w0,w26,wzr,ne
 mov w9,#0xac8
 b keep_next
gain:
 ldr s23,[x8,w26,uxtw #2]
 cbnz w26,finish
 cmp w0,#3
 b.eq extra
 cmp w0,#4
 b.ne finish
 fmov s23,wzr
 b finish
extra:
 ldr s23,low_value
finish:
 b gain_next
low_value: .float 0.30
video_key: .asciz "usecase1_video"
pref: .asciz SHARP
noise: .asciz NOISE
.balign 4
// Same effective per-sensor color key that the native ASF mapper consumes.
noise_level:
 adrp x9,noise_mapped
 ldr x9,[x9,:lo12:noise_mapped]
 cbz x9,level_zero
 ldrb w9,[x9]
 sub w10,w9,#1
 cmp w10,#2
 b.hi level_zero
 stp x19,x30,[sp,#-32]!
 str w9,[sp,#16]
 bl effective_key
 cbz x0,level_restore_zero
 mov x19,x0
 adr x1,video_key
 bl substr
 cbz x0,level_restore_zero
 mov x0,x19
 adr x1,ilog_key
 bl substr
 ldr w9,[sp,#16]
 cmp x0,#0
 csel w0,w9,wzr,ne
 ldp x19,x30,[sp],#32
 ret
level_restore_zero:
 ldp x19,x30,[sp],#32
level_zero:
 mov w0,wzr
 ret
ilog_key: .asciz "_ilog"
.balign 4
.global tf_call,tf_master,tf_pass,anr_channels
tf_call:
 stp x27,x30,[sp,#-48]!
 stp x0,x1,[sp,#16]
 str x2,[sp,#32]
 ldr w0,[x0,#0x218]
 bl noise_level
 adr x27,level_tag
 add x27,x27,w0,uxtw
 ldp x0,x1,[sp,#16]
 ldr x2,[sp,#32]
 bl tf_mapper
 ldp x27,x30,[sp],#48
 ret
// The native mapper saved incoming x27 at FP+0x18.
.macro tf_level
 ldr x16,[x29,#0x18]
 adr x17,level_tag
 sub x16,x16,x17
.endm
tf_master:
 ldrb w9,[x9]
 stp x16,x17,[sp,#-16]!
 tf_level
 sub x16,x16,#2
 cmp x16,#1
 csel w9,wzr,w9,ls
 ldp x16,x17,[sp],#16
 b tf_master_next
tf_pass:
 ldrb w23,[x19,x9]
 stp x16,x17,[sp,#-32]!
 mrs x17,nzcv
 str x17,[sp,#16]
 tf_level
 cmp x16,#1
 b.ne 1f
 cmp x8,#2
 csel w23,wzr,w23,lo
1:
 ldr x17,[sp,#16]
 msr nzcv,x17
 ldp x16,x17,[sp],#32
 b tf_pass_next
level_tag: .word 0
anr_channels:
 // w11 and w12 are the native luma/chroma enables. Live arguments and
 // interpolator constants across this extra call must survive the vendor TLS lookup.
 ldrb w12,[x13,#0x90]
 stp x0,x1,[sp,#-160]!
 stp x2,x3,[sp,#16]
 stp x4,x5,[sp,#32]
 stp x6,x7,[sp,#48]
 stp x8,x9,[sp,#64]
 stp x10,x11,[sp,#80]
 stp x12,x13,[sp,#96]
 stp x14,x15,[sp,#112]
 stp x16,x17,[sp,#128]
 stp x18,x30,[sp,#144]
 str q0,[sp,#-16]!
 mov w0,w21
 bl noise_level
 cmp w0,#3
 ldr q0,[sp],#16
 ldp x0,x1,[sp]
 ldp x2,x3,[sp,#16]
 ldp x4,x5,[sp,#32]
 ldp x6,x7,[sp,#48]
 ldp x8,x9,[sp,#64]
 ldp x10,x11,[sp,#80]
 ldp x12,x13,[sp,#96]
 ldp x14,x15,[sp,#112]
 ldp x16,x17,[sp,#128]
 ldp x18,x30,[sp,#144]
 add sp,sp,#160
 csel w11,wzr,w11,eq
 csel w12,wzr,w12,eq
 b anr_next
