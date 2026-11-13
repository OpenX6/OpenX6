.text
#include "../camera_hal/saveall.inc"
.global ae_entry,ae_cap_entry
.balign 16
ae_entry:
 bti c
 saveall
 bl ae_prepare
 str s0,[sp,#192]
 restoreall
 .incbin "stack.bin"
 b ae_continue
.balign 16
ae_cap_entry:
 bti c
 .incbin "cap.bin"
 saveall
 mov w0,w20
 mov w1,w26
 mov x2,x19
 fmov s0,s12
 bl ae_cap
 str s0,[sp,#384]
 restoreall
 ret
