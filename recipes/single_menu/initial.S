.text
.balign 16
.global vr_settings_init, vr_open, vr_click, vr_read_state, vr_request
// One native root page constructor hook; no changes to photo parameters.
vr_settings_init:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov x19,x0
 bl settings_init_original
 adrp x0,arrow_class
 add x0,x0,:lo12:arrow_class
 mov x1,#0
 bl new_object
 cbz x0,init_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov x2,#0x29c0
 add x2,x19,x2
 bl group_add_behind
 adr x0,vr_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl arrow_title
 mov x0,x19
 adr x1,vr_open
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
init_done:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
vr_open:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov x19,x0
 ldr x9,[x19,#0xa8]
 cbnz x9,open_done
 // The native manager owns, animates and releases the new page.
 adrp x1,page_class
 add x1,x1,:lo12:page_class
 mov w2,#0
 mov x3,#0
 mov x4,#0
 mov x5,#0
 mov x6,#0
 bl create_subpage
 ldr x19,[x19,#0xa8]
 cbz x19,open_done
 adrp x9,page_class
 add x9,x9,:lo12:page_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne open_done
 adr x0,vr_title
 bl new_string
 mov x1,x0
 add x0,x19,#0x338
 bl page_title
 adrp x0,switch_class
 add x0,x0,:lo12:switch_class
 mov x1,#0
 bl new_object
 cbz x0,open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adr x0,sync_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl switch_title
 bl vr_read_state
 mov w1,w0
 mov x0,x20
 bl switch_checked
 mov x0,x20
 adr x1,vr_click
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
open_done:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
vr_click:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 mov x19,x0
 ldrb w0,[x19,#0xb88]
 bl vr_request
 cbz w0,click_done
 // Failure to submit never leaves a false selected state.
 bl vr_read_state
 mov w1,w0
 mov x0,x19
 bl switch_checked
click_done:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
vr_read_state:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov w20,#0
 mov x0,#-100
 adr x1,state_path
 mov w2,#0
 movk w2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,read_done
 mov x19,x0
 add x1,sp,#32
 mov x2,#2
 mov x8,#63
 svc #0
 cmp x0,#1
 b.ne read_close
 ldrb w20,[sp,#32]
 cmp w20,#1
 csel w20,w20,wzr,ls
read_close:
 mov x0,x19
 mov x8,#57
 svc #0
read_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
vr_request:
 bti c
 cmp w0,#1
 b.hi request_invalid
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 strb w0,[sp,#32]
 mov w20,#-1
 mov x0,#-100
 adr x1,request_stage
 mov w2,#0xc1
 movk w2,#0xa,lsl #16
 mov w3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,request_done
 mov x19,x0
 add x1,sp,#32
 mov x2,#1
 mov x8,#64
 svc #0
 cmp x0,#1
 cset w20,ne
 mov x0,x19
 mov x8,#57
 svc #0
 cbnz w20,request_unlink
 mov x0,#-100
 adr x1,request_stage
 mov x2,#-100
 adr x3,request_path
 mov w4,#1
 mov x8,#276
 svc #0
 mov w20,w0
 cbz w20,request_done
request_unlink:
 mov x0,#-100
 adr x1,request_stage
 mov w2,#0
 mov x8,#35
 svc #0
request_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
request_invalid:
 mov w0,#-1
 ret
vr_title: .asciz "VR180 Settings"
sync_title: .asciz "120 MP Sync (test)"
request_path: .asciz "/run/x6-vr180/request"
state_path: .asciz "/run/x6-vr180/state"

request_stage: .asciz "/run/x6-vr180/request.next"
