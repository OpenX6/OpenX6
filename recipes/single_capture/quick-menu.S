.text
.global info_update,info_init,settings_row,rate_unit
info_update:
 adr x0,version_label
 b version_update_next
info_init:
 adr x0,version_label
 b version_init_next

// The settings constructor sends its normal signal after adding the new row.
// Preserve the slot tuple (x0,x1) and sender (x2) for that signal.
settings_row:
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x0,x1,[sp,#32]
 str x2,[sp,#48]
 mov x19,x2
 adrp x0,row_class
 add x0,x0,:lo12:row_class
 mov x1,#0
 bl new_object
 cbz x0,settings_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adrp x0,quick_title
 add x0,x0,:lo12:quick_title
 bl localize
 mov x1,x0
 mov x0,x20
 bl set_title
 mov x0,x19
 adr x1,open_quick_capture
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
settings_done:
 ldr x2,[sp,#48]
 ldp x0,x1,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 b ew_signal
open_quick_capture:
 bti c
 ldr x9,[x0,#0xa8]
 cbnz x9,already_open
 adrp x1,quick_page
 add x1,x1,:lo12:quick_page
 mov x2,#0
 mov x3,#0
 mov x4,#0
 mov x5,#0
 mov x6,#0
 b create_subpage
already_open:
 ret

rate_unit:
 bti c
 stp x19,x30,[sp,#-16]!
 mov x19,x0
 adr x0,unit_label
 mov w1,#4
 bl new_text
 mov x1,x0
 add x0,x19,#0x610
 ldp x19,x30,[sp],#16
 b set_subtitle
unit_label: .asciz "Mbps"
version_label: .asciz "1.2.36 OpenX6 1.1"
