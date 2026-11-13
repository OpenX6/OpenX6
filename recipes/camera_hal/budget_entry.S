.text
#include "saveall.inc"
.global budget_entry
.balign 16
budget_entry:
 bti c
 saveall
 mov w1,w3
 bl budget
 str x0,[sp,#24]
 restoreall
 .incbin "displaced.bin"
 b budget_continue
