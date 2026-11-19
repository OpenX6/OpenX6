.text
.global row_bridge
row_bridge:
 bti c
 stp x29,x30,[sp,#-16]!
 mov x0,x19
 bl exposure_row
 ldp x29,x30,[sp],#16
 // Replay first instruction of the next native switch constructor.
 adrp x0,switch_class
 b next_row
