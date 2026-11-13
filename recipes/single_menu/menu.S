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

 adrp x0,arrow_class
 add x0,x0,:lo12:arrow_class
 mov x1,#0
 bl new_object
 cbz x0,open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adr x0,ic_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl arrow_title
 mov x0,x19
 adr x1,ic_open
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]

 // Additional independent page; preserve the existing sync toggle.
 adrp x0,arrow_class
 add x0,x0,:lo12:arrow_class
 mov x1,#0
 bl new_object
 cbz x0,open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adr x0,nr_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl arrow_title
 mov x0,x19
 adr x1,nr_open
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
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
 adr x0,proxy_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl switch_title
 bl proxy_read_state
 mov w1,w0
 mov x0,x20
 bl switch_checked
 mov x0,x20
 adr x1,proxy_click
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]

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
 adr x0,preview_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl switch_title
 bl preview_read_state
 mov w1,w0
 mov x0,x20
 bl switch_checked
 mov x0,x20
 adr x1,preview_click
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]

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
 adr x0,stereo_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl switch_title
 bl stereo_read_state
 mov w1,w0
 mov x0,x20
 bl switch_checked
 mov x0,x20
 adr x1,stereo_click
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]

#if defined(OPENX6_STATS_EXTENSION) || defined(OPENX6_CHOICES)
 b ext_home
#else
 b .Lvisible_stats_row  // retired video NR controls remain hidden
#endif
 add x0,x0,:lo12:switch_class
 mov x1,#0
 bl new_object
 cbz x0,open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adr x0,hnr_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl switch_title
 bl hnr_read_state
 mov w1,w0
 mov x0,x20
 bl switch_checked
 mov x0,x20
 adr x1,hnr_click
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]

 adrp x0,arrow_class
 add x0,x0,:lo12:arrow_class
 mov x1,#0
 bl new_object
 cbz x0,open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adr x0,vnr_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl arrow_title
 mov x0,x19
 adr x1,vnr_open
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
.Lvisible_stats_row:
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
#ifdef OPENX6_STATS_EXTENSION
 adr x0,ext_setting_title
#else
 adr x0,stats_title
#endif
 bl new_string
 mov x1,x0
 mov x0,x20
 bl switch_title
 bl stats_read_state
 mov w1,w0
 mov x0,x20
 bl switch_checked
 mov x0,x20
 adr x1,stats_click
 bl new_slot
 add x9,x20,#0x4c0
 #ifdef OPENX6_STATS_EXTENSION
 b ext_finish_switch
#else
 stp x0,x1,[x9]
#endif
ext_after_stats:
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
 adr x0,ssd_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl switch_title
 bl ssd_read_state
 mov w1,w0
 mov x0,x20
 bl switch_checked
 mov x0,x20
 adr x1,ssd_click
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
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
 adr x0,tl_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl switch_title
 bl tl_read_state
 mov w1,w0
 mov x0,x20
 bl switch_checked
 mov x0,x20
 adr x1,tl_click
 bl new_slot
 add x9,x20,#0x4c0
 b vr_open_pp
// ^ one-word tail detour (was `stp x0,x1,[x9]` 0xA9000520): keeps vr_open byte-size fixed so the
// video-NR retire word at 0x715c304 still lands; the displaced store runs first in vr_open_pp (EOF).

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
vr_title: .asciz "Mod Settings"
sync_title: .asciz "120 MP Sync"
request_path: .asciz "/run/x6-vr180/request"
state_path: .asciz "/run/x6-vr180/state"

request_stage: .asciz "/run/x6-vr180/request.next"

.balign 16
.global nr_open, nr_pick0, nr_pick1, nr_pick2, nr_pick3, nr_pick, nr_paint, nr_read_state, nr_request
nr_open:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 str x23,[sp,#48]
 mov x19,x0
 ldr x9,[x19,#0xa8]
 cbnz x9,nr_open_done
 adrp x1,page_class
 add x1,x1,:lo12:page_class
 mov w2,#0
 mov x3,#0
 mov x4,#0
 mov x5,#0
 mov x6,#0
 bl create_subpage
 ldr x19,[x19,#0xa8]
 cbz x19,nr_open_done
 adrp x9,page_class
 add x9,x9,:lo12:page_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne nr_open_done
 adr x0,nr_title
 bl new_string
 mov x1,x0
 add x0,x19,#0x338
 bl page_title
 bl nr_read_state
 mov w22,w0
 mov w21,#0
 adr x23,nr_names
nr_loop:
 adrp x0,radio_class
 add x0,x0,:lo12:radio_class
 mov x1,#0
 bl new_object
 cbz x0,nr_open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 add x0,x23,x21,lsl #4
 bl new_string
 mov x1,x0
 mov x0,x20
 bl radio_title
 cmp w21,w22
 cset w1,eq
 mov x0,x20
 bl radio_selected
 mov x0,x19
 adr x1,nr_pick0
 add x1,x1,x21,lsl #4
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
 add w21,w21,#1
 cmp w21,#4
 b.lo nr_loop
nr_open_done:
 ldr x23,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret
.balign 16
nr_pick0:
 bti c
 mov w1,#0
 b nr_pick
.balign 16
nr_pick1:
 bti c
 mov w1,#1
 b nr_pick
.balign 16
nr_pick2:
 bti c
 mov w1,#2
 b nr_pick
.balign 16
nr_pick3:
 bti c
 mov w1,#3
 b nr_pick
nr_pick:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov x19,x0
 mov w20,w1
 mov w0,w1
 bl nr_request
 cbz w0,nr_paint_requested
 bl nr_read_state
 mov w20,w0
nr_paint_requested:
 mov x0,x19
 mov w1,w20
 bl nr_paint
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
nr_paint:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 ldr x19,[x0,#0x60]
 mov w20,w1
 mov w21,#0
nr_paint_loop:
 cbz x19,nr_paint_done
 add w21,w21,#1
 cmp w21,#128
 b.hi nr_paint_done
 adrp x9,radio_class
 add x9,x9,:lo12:radio_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne nr_paint_next
 ldr x10,[x19,#0x4c8]
 adr x9,nr_pick0
 sub x10,x10,x9
 cmp x10,#48
 b.hi nr_paint_next
 tst x10,#15
 b.ne nr_paint_next
 lsr x22,x10,#4
 cmp w22,w20
 cset w1,eq
 mov x0,x19
 bl radio_selected
nr_paint_next:
 ldr x19,[x19,#0x20]
 b nr_paint_loop
nr_paint_done:
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret
nr_read_state:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov w20,#0
 mov x0,#-100
 adr x1,nr_state_path
 mov w2,#0
 movk w2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,nr_read_done
 mov x19,x0
 add x1,sp,#32
 mov x2,#2
 mov x8,#63
 svc #0
 cmp x0,#1
 b.ne nr_read_close
 ldrb w20,[sp,#32]
 cmp w20,#3
 csel w20,w20,wzr,ls
nr_read_close:
 mov x0,x19
 mov x8,#57
 svc #0
nr_read_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
nr_request:
 bti c
 cmp w0,#3
 b.hi nr_request_invalid
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 strb w0,[sp,#32]
 mov w20,#-1
 mov x0,#-100
 adr x1,nr_request_stage
 mov w2,#0xc1
 movk w2,#0xa,lsl #16
 mov w3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,nr_request_done
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
 cbnz w20,nr_request_unlink
 mov x0,#-100
 adr x1,nr_request_stage
 mov x2,#-100
 adr x3,nr_request_path
 mov w4,#1
 mov x8,#276
 svc #0
 mov w20,w0
 cbz w20,nr_request_done
nr_request_unlink:
 mov x0,#-100
 adr x1,nr_request_stage
 mov w2,#0
 mov x8,#35
 svc #0
nr_request_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
nr_request_invalid:
 mov w0,#-1
 ret

.balign 16
nr_title: .asciz "RAW Photo Noise Reduction"
.balign 16
nr_names:
.asciz "Native"
.balign 16
.asciz "Medium"
.balign 16
.asciz "Low"
.balign 16
.asciz "Minimal"
nr_request_path: .asciz "/run/x6-raw-nr-menu/request"
nr_state_path: .asciz "/run/x6-raw-nr-menu/state"
nr_request_stage: .asciz "/run/x6-raw-nr-menu/request.next"

.balign 16
proxy_click:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 mov x19,x0
 ldrb w0,[x19,#0xb88]
 bl proxy_request
 cbz w0,proxy_click_done
 // Failure to submit never leaves a false selected state.
 bl proxy_read_state
 mov w1,w0
 mov x0,x19
 bl switch_checked
proxy_click_done:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
proxy_read_state:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov w20,#0
 mov x0,#-100
 adr x1,proxy_state_path
 mov w2,#0
 movk w2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,proxy_read_done
 mov x19,x0
 add x1,sp,#32
 mov x2,#2
 mov x8,#63
 svc #0
 cmp x0,#1
 b.ne proxy_read_close
 ldrb w20,[sp,#32]
 cmp w20,#1
 csel w20,w20,wzr,ls
proxy_read_close:
 mov x0,x19
 mov x8,#57
 svc #0
proxy_read_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
proxy_request:
 bti c
 cmp w0,#1
 b.hi proxy_request_invalid
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 strb w0,[sp,#32]
 mov w20,#-1
 mov x0,#-100
 adr x1,proxy_request_stage
 mov w2,#0xc1
 movk w2,#0xa,lsl #16
 mov w3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,proxy_request_done
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
 cbnz w20,proxy_request_unlink
 mov x0,#-100
 adr x1,proxy_request_stage
 mov x2,#-100
 adr x3,proxy_request_path
 mov w4,#1
 mov x8,#276
 svc #0
 mov w20,w0
 cbz w20,proxy_request_done
proxy_request_unlink:
 mov x0,#-100
 adr x1,proxy_request_stage
 mov w2,#0
 mov x8,#35
 svc #0
proxy_request_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
proxy_request_invalid:
 mov w0,#-1
 ret

.balign 16
proxy_title: .asciz "Disable Video Proxy"
proxy_state_path: .asciz "/run/x6-proxy-control/state"
proxy_request_path: .asciz "/run/x6-proxy-control/request"
proxy_request_stage: .asciz "/run/x6-proxy-control/request.next"

.balign 16
stereo_click:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 mov x19,x0
 ldrb w0,[x19,#0xb88]
 bl stereo_request
 cbz w0,stereo_click_done
 // Failure to submit never leaves a false selected state.
 bl stereo_read_state
 mov w1,w0
 mov x0,x19
 bl switch_checked
stereo_click_done:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
stereo_read_state:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov w20,#0
 mov x0,#-100
 adr x1,stereo_state_path
 mov w2,#0
 movk w2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,stereo_read_done
 mov x19,x0
 add x1,sp,#32
 mov x2,#2
 mov x8,#63
 svc #0
 cmp x0,#1
 b.ne stereo_read_close
 ldrb w20,[sp,#32]
 cmp w20,#1
 csel w20,w20,wzr,ls
stereo_read_close:
 mov x0,x19
 mov x8,#57
 svc #0
stereo_read_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
stereo_request:
 bti c
 cmp w0,#1
 b.hi stereo_request_invalid
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 strb w0,[sp,#32]
 mov w20,#-1
 mov x0,#-100
 adr x1,stereo_request_stage
 mov w2,#0xc1
 movk w2,#0xa,lsl #16
 mov w3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,stereo_request_done
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
 cbnz w20,stereo_request_unlink
 mov x0,#-100
 adr x1,stereo_request_stage
 mov x2,#-100
 adr x3,stereo_request_path
 mov w4,#1
 mov x8,#276
 svc #0
 mov w20,w0
 cbz w20,stereo_request_done
stereo_request_unlink:
 mov x0,#-100
 adr x1,stereo_request_stage
 mov w2,#0
 mov x8,#35
 svc #0
stereo_request_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
stereo_request_invalid:
 mov w0,#-1
 ret

.balign 16
stereo_title: .asciz "Link Stereo AE / WB"
stereo_state_path: .asciz "/run/x6-stereo-control/state"
stereo_request_path: .asciz "/run/x6-stereo-control/request"
stereo_request_stage: .asciz "/run/x6-stereo-control/request.next"

.balign 16
hnr_click:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 mov x19,x0
 ldrb w0,[x19,#0xb88]
 bl hnr_request
 cbz w0,hnr_click_done
 // Failure to submit never leaves a false selected state.
 bl hnr_read_state
 mov w1,w0
 mov x0,x19
 bl switch_checked
hnr_click_done:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
hnr_read_state:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov w20,#0
 mov x0,#-100
 adr x1,hnr_state_path
 mov w2,#0
 movk w2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,hnr_read_done
 mov x19,x0
 add x1,sp,#32
 mov x2,#2
 mov x8,#63
 svc #0
 cmp x0,#1
 b.ne hnr_read_close
 ldrb w20,[sp,#32]
 cmp w20,#1
 csel w20,w20,wzr,ls
hnr_read_close:
 mov x0,x19
 mov x8,#57
 svc #0
hnr_read_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
hnr_request:
 bti c
 cmp w0,#1
 b.hi hnr_request_invalid
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 strb w0,[sp,#32]
 mov w20,#-1
 mov x0,#-100
 adr x1,hnr_request_stage
 mov w2,#0xc1
 movk w2,#0xa,lsl #16
 mov w3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,hnr_request_done
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
 cbnz w20,hnr_request_unlink
 mov x0,#-100
 adr x1,hnr_request_stage
 mov x2,#-100
 adr x3,hnr_request_path
 mov w4,#1
 mov x8,#276
 svc #0
 mov w20,w0
 cbz w20,hnr_request_done
hnr_request_unlink:
 mov x0,#-100
 adr x1,hnr_request_stage
 mov w2,#0
 mov x8,#35
 svc #0
hnr_request_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
hnr_request_invalid:
 mov w0,#-1
 ret

.balign 16
hnr_title: .asciz "Disable Video HNR"
hnr_state_path: .asciz "/run/x6-hnr-control/state"
hnr_request_path: .asciz "/run/x6-hnr-control/request"
hnr_request_stage: .asciz "/run/x6-hnr-control/request.next"

.balign 16
.global vnr_open, vnr_pick0, vnr_pick1, vnr_pick2, vnr_pick3, vnr_pick, vnr_paint, vnr_read_state, vnr_request
vnr_open:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 str x23,[sp,#48]
 mov x19,x0
 ldr x9,[x19,#0xa8]
 cbnz x9,vnr_open_done
 adrp x1,page_class
 add x1,x1,:lo12:page_class
 mov w2,#0
 mov x3,#0
 mov x4,#0
 mov x5,#0
 mov x6,#0
 bl create_subpage
 ldr x19,[x19,#0xa8]
 cbz x19,vnr_open_done
 adrp x9,page_class
 add x9,x9,:lo12:page_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne vnr_open_done
 adr x0,vnr_title
 bl new_string
 mov x1,x0
 add x0,x19,#0x338
 bl page_title
 bl vnr_read_state
 mov w22,w0
 mov w21,#0
 adr x23,vnr_names
vnr_loop:
 adrp x0,radio_class
 add x0,x0,:lo12:radio_class
 mov x1,#0
 bl new_object
 cbz x0,vnr_open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 add x0,x23,x21,lsl #4
 bl new_string
 mov x1,x0
 mov x0,x20
 bl radio_title
 cmp w21,w22
 cset w1,eq
 mov x0,x20
 bl radio_selected
 mov x0,x19
 adr x1,vnr_pick0
 add x1,x1,x21,lsl #4
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
 add w21,w21,#1
 cmp w21,#4
 b.lo vnr_loop
vnr_open_done:
 ldr x23,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret
.balign 16
vnr_pick0:
 bti c
 mov w1,#0
 b vnr_pick
.balign 16
vnr_pick1:
 bti c
 mov w1,#1
 b vnr_pick
.balign 16
vnr_pick2:
 bti c
 mov w1,#2
 b vnr_pick
.balign 16
vnr_pick3:
 bti c
 mov w1,#3
 b vnr_pick
vnr_pick:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov x19,x0
 mov w20,w1
 mov w0,w1
 bl vnr_request
 cbz w0,vnr_paint_requested
 bl vnr_read_state
 mov w20,w0
vnr_paint_requested:
 mov x0,x19
 mov w1,w20
 bl vnr_paint
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
vnr_paint:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 ldr x19,[x0,#0x60]
 mov w20,w1
 mov w21,#0
vnr_paint_loop:
 cbz x19,vnr_paint_done
 add w21,w21,#1
 cmp w21,#128
 b.hi vnr_paint_done
 adrp x9,radio_class
 add x9,x9,:lo12:radio_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne vnr_paint_next
 ldr x10,[x19,#0x4c8]
 adr x9,vnr_pick0
 sub x10,x10,x9
 cmp x10,#48
 b.hi vnr_paint_next
 tst x10,#15
 b.ne vnr_paint_next
 lsr x22,x10,#4
 cmp w22,w20
 cset w1,eq
 mov x0,x19
 bl radio_selected
vnr_paint_next:
 ldr x19,[x19,#0x20]
 b vnr_paint_loop
vnr_paint_done:
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret
vnr_read_state:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov w20,#0
 mov x0,#-100
 adr x1,vnr_state_path
 mov w2,#0
 movk w2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,vnr_read_done
 mov x19,x0
 add x1,sp,#32
 mov x2,#2
 mov x8,#63
 svc #0
 cmp x0,#1
 b.ne vnr_read_close
 ldrb w20,[sp,#32]
 cmp w20,#3
 csel w20,w20,wzr,ls
vnr_read_close:
 mov x0,x19
 mov x8,#57
 svc #0
vnr_read_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
vnr_request:
 bti c
 cmp w0,#3
 b.hi vnr_request_invalid
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 strb w0,[sp,#32]
 mov w20,#-1
 mov x0,#-100
 adr x1,vnr_request_stage
 mov w2,#0xc1
 movk w2,#0xa,lsl #16
 mov w3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,vnr_request_done
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
 cbnz w20,vnr_request_unlink
 mov x0,#-100
 adr x1,vnr_request_stage
 mov x2,#-100
 adr x3,vnr_request_path
 mov w4,#1
 mov x8,#276
 svc #0
 mov w20,w0
 cbz w20,vnr_request_done
vnr_request_unlink:
 mov x0,#-100
 adr x1,vnr_request_stage
 mov w2,#0
 mov x8,#35
 svc #0
vnr_request_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
vnr_request_invalid:
 mov w0,#-1
 ret

.balign 16
vnr_title: .asciz "Video Spatial NR"
.balign 16
vnr_names:
.asciz "Native"
.balign 16
.asciz "Medium"
.balign 16
.asciz "Low"
.balign 16
.asciz "Minimal"
vnr_request_path: .asciz "/run/x6-video-nr-control/request"
vnr_state_path: .asciz "/run/x6-video-nr-control/state"
vnr_request_stage: .asciz "/run/x6-video-nr-control/request.next"

.balign 16
stats_click:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 mov x19,x0
 ldrb w0,[x19,#0xb88]
 bl stats_request
 cbz w0,stats_click_done
 // Failure to submit never leaves a false selected state.
 bl stats_read_state
 mov w1,w0
 mov x0,x19
 bl switch_checked
stats_click_done:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
stats_read_state:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov w20,#0
 mov x0,#-100
 adr x1,stats_state_path
 mov w2,#0
 movk w2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,stats_read_done
 mov x19,x0
 add x1,sp,#32
 mov x2,#2
 mov x8,#63
 svc #0
 cmp x0,#1
 b.ne stats_read_close
 ldrb w20,[sp,#32]
 cmp w20,#1
 csel w20,w20,wzr,ls
stats_read_close:
 mov x0,x19
 mov x8,#57
 svc #0
stats_read_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
stats_request:
 bti c
 cmp w0,#1
 b.hi stats_request_invalid
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 strb w0,[sp,#32]
 mov w20,#-1
 mov x0,#-100
 adr x1,stats_request_stage
 mov w2,#0xc1
 movk w2,#0xa,lsl #16
 mov w3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,stats_request_done
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
 cbnz w20,stats_request_unlink
 mov x0,#-100
 adr x1,stats_request_stage
 mov x2,#-100
 adr x3,stats_request_path
 mov w4,#1
 mov x8,#276
 svc #0
 mov w20,w0
 cbz w20,stats_request_done
stats_request_unlink:
 mov x0,#-100
 adr x1,stats_request_stage
 mov w2,#0
 mov x8,#35
 svc #0
stats_request_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
stats_request_invalid:
 mov w0,#-1
 ret

.balign 16
stats_title: .asciz "Stats Overlay"
stats_state_path: .asciz "/run/x6-stats-overlay/state"
stats_request_path: .asciz "/run/x6-stats-overlay/request"
stats_request_stage: .asciz "/run/x6-stats-overlay/request.next"


.balign 16
.global stats_timers, stats_tick, stats_debug, stats_layout
stats_timers:
 bti c
 autiasp
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 bl stats_timer_trampoline
 mov x19,x0
 bl pp_timer_hook               // one-word detour: original stats_tick + the PureShot 1 Hz reconcile (size-stable)
 mov x0,x19
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
stats_timer_trampoline:
 paciasp
 sub sp,sp,#0x50
 b stats_timers_original
stats_debug:
 bti c
 autiasp
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov x19,x0
 bl stats_debug_trampoline
 mov x0,x19
 bl stats_style
 adrp x20,stats_globals
 add x20,x20,:lo12:stats_globals
 ldr w9,[x20,#8]
 cbz w9,stats_debug_done
 add x0,x19,#0xe0
 ldr w1,[x20,#24]
 bl text_color
stats_debug_done:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
stats_debug_trampoline:
 paciasp
 stp x29,x30,[sp,#-0x30]!
 b stats_debug_original
stats_layout:
 bti c
 autiasp
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov x19,x0
 mov x20,x1
 bl stats_layout_trampoline
 adrp x9,stats_globals
 add x9,x9,:lo12:stats_globals
 ldr w9,[x9,#8]
 cbz w9,stats_layout_done
 // Rect {8,60,width-8,180}; native rotation supplies the parent size.
 cmp w20,#128
 b.lo stats_layout_done
 cmp w20,#2048
 b.hi stats_layout_done
 add x0,x19,#0xe0
 mov x1,#8
 mov x9,#60
 orr x1,x1,x9,lsl #32
 sub w2,w20,#8
 mov x9,#228
 orr x2,x2,x9,lsl #32
 bl set_bounds
stats_layout_done:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
stats_layout_trampoline:
 paciasp
 stp x29,x30,[sp,#-0x20]!
 b stats_layout_original
stats_root:
 adrp x9,stats_holder_page
 ldr x0,[x9,#0x830]
 adrp x9,stats_root_class_page
 ldr x1,[x9,#0xd40]
 b get_auto
stats_tick:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 sub sp,sp,#320
 adrp x19,stats_globals
 add x19,x19,:lo12:stats_globals
 bl stats_is_capture_page
 cbz w0,stats_hide
 mov x0,#1
 mov x1,sp
 mov x8,#113
 svc #0
 cbnz x0,stats_done
 ldr x20,[sp]
 ldr x9,[x19]
 cmp x20,x9
 b.eq stats_done
 str x20,[x19]
 bl stats_read_state
 cbz w0,stats_hide
 // Factory debug text remains owned by native factory mode.
 adrp x9,stats_holder_page
 ldr x0,[x9,#0xb68]
 adrp x9,stats_factory_class_page
 ldr x1,[x9,#0x338]
 bl get_auto
 ldr w9,[x0,#0x2c]
 cbnz w9,stats_hide
 mov x0,#-100
 adr x1,stats_status_path
 mov w2,#0
 movk w2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,stats_unknown
 mov x21,x0
 add x1,sp,#32
 mov x2,#257
 mov x8,#63
 svc #0
 mov x22,x0
 mov x0,x21
 mov x8,#57
 svc #0
 cmp x22,#18
 b.lt stats_unknown
 cmp x22,#256
 b.ge stats_unknown
 add x9,sp,#32
 sub x10,x22,#1
 ldrb w10,[x9,x10]
 cbnz w10,stats_unknown
 ldr x10,[x9]
 cmp x10,x20
 b.hi stats_unknown
 sub x10,x20,x10
 cmp x10,#3
 b.hi stats_unknown
 ldr w10,[x9,#8]
 cmp w10,#3
 b.hi stats_unknown
 mov w11,#0xffff
 movk w11,#0xffff,lsl #16
 cmp w10,#1
 b.ne 1f
 mov w11,#0xc0ff
 movk w11,#0xff00,lsl #16
1:
 cmp w10,#2
 b.ne 2f
 mov w11,#0x80ff
 movk w11,#0xff80,lsl #16
2:
 cmp w10,#3
 b.ne 4f
 mov w11,#0xb0b0
 movk w11,#0xffb0,lsl #16
4:
 str w11,[x19,#24]
 add x21,x9,#16
 b stats_show
stats_unknown:
 mov w11,#0xb0b0
 movk w11,#0xffb0,lsl #16
 str w11,[x19,#24]
 adr x21,stats_unknown_text
stats_show:
 bl stats_root
 mov x22,x0
 ldr w9,[x19,#8]
 cbnz w9,3f
 add x9,x0,#0x1000
 ldrb w10,[x9,#0x292]
 str w10,[x19,#12]
 mov w9,#1
 str w9,[x19,#8]
3:
 mov x0,x21
 bl factory_notify
 mov x0,x22
 mov w1,#1
 bl debug_visible
 b stats_done
stats_hide:
 ldr w9,[x19,#8]
 cbz w9,stats_done
 bl stats_root
 ldr w1,[x19,#12]
 bl debug_visible
 str wzr,[x19,#8]
stats_done:
 add sp,sp,#320
 ldp x19,x20,[sp,#16]
 ldp x21,x22,[sp,#32]
 ldp x29,x30,[sp],#48
 ret
stats_status_path: .asciz "/run/x6-stats-overlay/status"
stats_unknown_text: .asciz "Stats unavailable"

.balign 16
stats_style:
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov x19,x0
 adrp x20,stats_globals
 add x20,x20,:lo12:stats_globals
 ldr w9,[x20,#8]
 cbz w9,stats_style_off
 ldr x9,[x20,#32]
 cmp x9,x19
 b.eq stats_style_on
 ldr w9,[x19,#0x18c]
 str w9,[x20,#40]
 str x19,[x20,#32]
stats_style_on:
 adrp x0,stats_small_font
 add x0,x0,:lo12:stats_small_font
 b stats_style_load
stats_style_off:
 ldr x9,[x20,#32]
 cmp x9,x19
 b.ne stats_style_done
 str xzr,[x20,#32]
 adrp x0,stats_original_font
 add x0,x0,:lo12:stats_original_font
stats_style_load:
 adrp x1,stats_font_class
 add x1,x1,:lo12:stats_font_class
 bl stats_load_resource
 mov x1,x0
 add x0,x19,#0xe0
 bl stats_set_font
 ldr w9,[x20,#8]
 mov w1,#9
 cbnz w9,stats_style_align
 ldr w1,[x20,#40]
stats_style_align:
 add x0,x19,#0xe0
 bl stats_set_alignment
stats_style_done:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#32
 ret

.balign 16
stats_is_capture_page:
 stp x29,x30,[sp,#-16]!
 mov x29,sp
 adrp x0,stats_focus_holder
 add x0,x0,:lo12:stats_focus_holder
 adrp x1,stats_focus_class
 add x1,x1,:lo12:stats_focus_class
 bl get_auto
 cbz x0,stats_page_no
 bl thermal_focus
 cbz x0,stats_page_no
 ldr x9,[x0]
 adrp x10,stats_liveview_class
 add x10,x10,:lo12:stats_liveview_class
 cmp x9,x10
 b.eq stats_page_yes
 adrp x10,stats_capture_class
 add x10,x10,:lo12:stats_capture_class
 cmp x9,x10
 b.ne stats_page_no
stats_page_yes:
 mov w0,#1
 b stats_page_done
stats_page_no:
 mov w0,#0
stats_page_done:
 ldp x29,x30,[sp],#16
 ret

.balign 16
preview_click:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 mov x19,x0
 ldrb w0,[x19,#0xb88]
 bl preview_request
 cbz w0,preview_click_done
 // Failure to submit never leaves a false selected state.
 bl preview_read_state
 mov w1,w0
 mov x0,x19
 bl switch_checked
preview_click_done:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
preview_read_state:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov w20,#0
 mov x0,#-100
 adr x1,preview_state_path
 mov w2,#0
 movk w2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,preview_read_done
 mov x19,x0
 add x1,sp,#32
 mov x2,#2
 mov x8,#63
 svc #0
 cmp x0,#1
 b.ne preview_read_close
 ldrb w20,[sp,#32]
 cmp w20,#1
 csel w20,w20,wzr,ls
preview_read_close:
 mov x0,x19
 mov x8,#57
 svc #0
preview_read_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
preview_request:
 bti c
 cmp w0,#1
 b.hi preview_request_invalid
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 strb w0,[sp,#32]
 mov w20,#-1
 mov x0,#-100
 adr x1,preview_request_stage
 mov w2,#0xc1
 movk w2,#0xa,lsl #16
 mov w3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,preview_request_done
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
 cbnz w20,preview_request_unlink
 mov x0,#-100
 adr x1,preview_request_stage
 mov x2,#-100
 adr x3,preview_request_path
 mov w4,#1
 mov x8,#276
 svc #0
 mov w20,w0
 cbz w20,preview_request_done
preview_request_unlink:
 mov x0,#-100
 adr x1,preview_request_stage
 mov w2,#0
 mov x8,#35
 svc #0
preview_request_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
preview_request_invalid:
 mov w0,#-1
 ret

.balign 16
preview_title: .asciz "Disable Recording Preview"
preview_state_path: .asciz "/run/x6-preview-control/state"
preview_request_path: .asciz "/run/x6-preview-control/request"
preview_request_stage: .asciz "/run/x6-preview-control/request.next"

.text
.balign 16
.global custom_metadata,custom_set,custom_set2,custom_open,custom_key,custom_paint,custom_read,custom_write
custom_metadata:
 bti c
 stp x29,x30,[sp,#-32]!
 stp x19,x20,[sp,#16]
 mov x19,x0
 bl option_string1
 mov x0,x26
 mov w1,#2
 bl option_int2
 mov x0,x26
 mov w1,#3
 bl option_int3
 adr x0,custom_max_text
 bl new_string
 mov x1,x0
 mov x0,x19
 bl option_string2
 bl custom_label
 mov x1,x0
 mov x0,x19
 bl option_string3
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
// Entered after original PACIASP. Other levels reproduce displaced SUB.
custom_set:
 bti c
 cmp w1,#3
 b.eq custom_select
 sub sp,sp,#0x60
 b setter_continue
custom_set2:
 bti c
 cmp w1,#3
 b.eq custom_select
 sub sp,sp,#0x60
 b setter2_continue
custom_select:
 stp x29,x30,[sp,#-16]!
 bl custom_open
 ldp x29,x30,[sp],#16
 autiasp
 ret
custom_set_native:
 paciasp
 sub sp,sp,#0x60
 b setter_continue
custom_open:
 bti c
 stp x29,x30,[sp,#-80]!
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 stp x23,x24,[sp,#48]
 adrp x0,stats_focus_holder
 add x0,x0,:lo12:stats_focus_holder
 adrp x1,stats_focus_class
 add x1,x1,:lo12:stats_focus_class
 bl get_auto
 cbz x0,custom_open_end
 ldr x0,[x0,#0x28]
 cbz x0,custom_open_end
 ldr x9,[x0,#0xa8]
 cbnz x9,custom_open_end
 mov x19,x0
 adrp x1,custom_page_class
 add x1,x1,:lo12:custom_page_class
 mov w2,#0
 mov x3,#0
 mov x4,#0
 mov x5,#0
 mov x6,#0
 bl create_subpage
 ldr x19,[x19,#0xa8]
 cbz x19,custom_open_end
 adrp x9,custom_page_class
 add x9,x9,:lo12:custom_page_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne custom_open_end
 adr x0,custom_title
 bl new_string
 mov x1,x0
 add x0,x19,#0x338
 bl page_title
 bl custom_read
 adrp x9,custom_globals
 add x9,x9,:lo12:custom_globals
 str w0,[x9]
 mov w10,#1
 str w10,[x9,#4]
 str wzr,[x9,#8]
 adrp x0,custom_text_class
 add x0,x0,:lo12:custom_text_class
 mov x1,#0
 bl new_object
 cbz x0,custom_open_end
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 mov x0,x20
 mov x1,#20
 mov x9,#88
 orr x1,x1,x9,lsl #32
 mov x2,#370
 mov x9,#144
 orr x2,x2,x9,lsl #32
 bl custom_text_bounds
 adrp x0,stats_small_font
 add x0,x0,:lo12:stats_small_font
 adrp x1,stats_font_class
 add x1,x1,:lo12:stats_font_class
 bl stats_load_resource
 mov x1,x0
 mov x0,x20
 bl custom_text_font
 mov x0,x20
 mov w1,#0xffffffff
 bl text_color
 mov x0,x20
 mov w1,#0x12
 bl stats_set_alignment
 mov w21,#0
 adr x23,custom_labels
custom_button_loop:
 adrp x0,custom_button_class
 add x0,x0,:lo12:custom_button_class
 mov x1,#0
 bl new_object
 cbz x0,custom_open_end
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 add x0,x23,x21,lsl #3
 bl new_string
 mov x1,x0
 mov x0,x20
 bl custom_button_text
 mov x0,x20
 mov w1,#0x3333
 movk w1,#0xff33,lsl #16
 bl custom_button_color
 mov x0,x20
 mov w1,#1
 bl custom_button_background
 adrp x0,stats_small_font
 add x0,x0,:lo12:stats_small_font
 adrp x1,stats_font_class
 add x1,x1,:lo12:stats_font_class
 bl stats_load_resource
 mov x1,x0
 mov x0,x20
 bl custom_button_font
 mov x0,x20
 mov w1,#0xffffffff
 bl custom_button_foreground
 mov x0,x20
 mov w1,#60
 bl custom_button_height
 mov x0,x20
 mov x1,#25
 mov x2,#25
 bl custom_button_padding
 mov x0,x19
 adr x1,custom_key0
 add x1,x1,x21,lsl #4
 bl new_slot
 add x9,x20,#0x2e8
 stp x0,x1,[x9]
 mov w9,#3
 udiv w10,w21,w9
 msub w11,w10,w9,w21
 mov w12,#120
 mul w11,w11,w12
 add w11,w11,#20
 mov w12,#70
 mul w10,w10,w12
 add w10,w10,#150
 orr x1,x11,x10,lsl #32
 add w11,w11,#110
 add w10,w10,#60
 orr x2,x11,x10,lsl #32
 mov x0,x20
 bl set_bounds
 add w21,w21,#1
 cmp w21,#12
 b.lo custom_button_loop
 mov x0,x19
 bl custom_paint
custom_open_end:
 ldp x23,x24,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#80
 ret
.balign 16
custom_key0: bti c; mov w1,#1; b custom_key; nop
custom_key1: bti c; mov w1,#2; b custom_key; nop
custom_key2: bti c; mov w1,#3; b custom_key; nop
custom_key3: bti c; mov w1,#4; b custom_key; nop
custom_key4: bti c; mov w1,#5; b custom_key; nop
custom_key5: bti c; mov w1,#6; b custom_key; nop
custom_key6: bti c; mov w1,#7; b custom_key; nop
custom_key7: bti c; mov w1,#8; b custom_key; nop
custom_key8: bti c; mov w1,#9; b custom_key; nop
custom_key9: bti c; mov w1,#10; b custom_key; nop
custom_key10: bti c; mov w1,#0; b custom_key; nop
custom_key11: bti c; mov w1,#11; b custom_key; nop
custom_key:
 bti c
 stp x29,x30,[sp,#-48]!
 stp x19,x20,[sp,#16]
 mov x19,x0
 adrp x20,custom_globals
 add x20,x20,:lo12:custom_globals
 ldr w9,[x20]
 str wzr,[x20,#8]
 cmp w1,#11
 b.eq custom_save
 cmp w1,#10
 b.eq custom_delete
 b.hi custom_key_end
 ldr w10,[x20,#4]
 cmp w10,#1
 csel w9,wzr,w9,eq
 cmp w9,#100
 b.hs custom_redraw
 mov w10,#10
 madd w9,w9,w10,w1
 str w9,[x20]
 str wzr,[x20,#4]
 b custom_redraw
custom_delete:
 mov w10,#10
 udiv w9,w9,w10
 str w9,[x20]
 str wzr,[x20,#4]
 b custom_redraw
custom_save:
 cmp w9,#1
 b.lo custom_bad
 cmp w9,#320
 b.hi custom_bad
 mov w0,w9
 bl custom_write
 cbz w0,custom_io_error
 adrp x0,custom_settings_holder
 add x0,x0,:lo12:custom_settings_holder
 adrp x1,custom_settings_class
 add x1,x1,:lo12:custom_settings_class
 bl get_auto
 cbz x0,custom_io_error
 mov w1,#3
 bl custom_set_native
 bl custom_refresh_label
 mov x0,x19
 bl custom_refresh_visible
 mov w9,#1
 str w9,[x20,#8]
 b custom_redraw
custom_bad:
 mov w9,#2
 str w9,[x20,#8]
 b custom_redraw
custom_io_error:
 mov w9,#3
 str w9,[x20,#8]
custom_redraw:
 mov x0,x19
 bl custom_paint
custom_key_end:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
custom_paint:
 bti c
 stp x29,x30,[sp,#-80]!
 stp x19,x20,[sp,#16]
 mov x19,x0
 adrp x20,custom_globals
 add x20,x20,:lo12:custom_globals
 ldr w9,[x20,#8]
 adr x0,custom_bad_text
 cmp w9,#2
 b.eq custom_paint_text
 adr x0,custom_error_text
 cmp w9,#3
 b.eq custom_paint_text
 // Fixed width decimal, so title contains no unbounded formatting.
 adr x10,custom_value_template
 ldp x11,x12,[x10]
 stp x11,x12,[sp,#32]
 ldr w9,[x20]
 mov w10,#100
 udiv w11,w9,w10
 msub w9,w11,w10,w9
 add w11,w11,#48
 strb w11,[sp,#32]
 mov w10,#10
 udiv w11,w9,w10
 msub w9,w11,w10,w9
 add w11,w11,#48
 add w9,w9,#48
 strb w11,[sp,#33]
 strb w9,[sp,#34]
 add x0,sp,#32
 ldr w9,[x20,#8]
 cmp w9,#1
 b.ne custom_paint_text
 adr x0,custom_saved_text
custom_paint_text:
 bl new_string
 mov x20,x0
 ldr x19,[x19,#0x60]
custom_find_text:
 cbz x19,custom_paint_end
 ldr x9,[x19]
 adrp x10,custom_text_class
 add x10,x10,:lo12:custom_text_class
 cmp x9,x10
 b.eq custom_found_text
 ldr x19,[x19,#0x20]
 b custom_find_text
custom_found_text:
 mov x0,x19
 mov x1,x20
 bl custom_text_value
custom_paint_end:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#80
 ret
// Four-byte little endian total Mbps. Exact length and bounded values.
custom_read:
 bti c
 stp x29,x30,[sp,#-32]!
 str x19,[sp,#16]
 mov x0,#-100
 adr x1,custom_path
 mov x2,#0x8800
 movk x2,#0x8,lsl #16
 mov x3,#0
 mov x8,#56
 svc #0
 tbnz x0,#63,custom_read_default
 mov x19,x0
 add x1,sp,#24
 mov x2,#5
 mov x8,#63
 svc #0
 mov x9,x0
 mov x0,x19
 mov x8,#57
 svc #0
 cmp x9,#4
 b.ne custom_read_default
 ldr w0,[sp,#24]
 cmp w0,#1
 b.lo custom_read_default
 cmp w0,#320
 b.ls custom_read_end
custom_read_default:
 mov w0,#260
custom_read_end:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
custom_write:
 bti c
 stp x29,x30,[sp,#-208]!
 stp x19,x20,[sp,#16]
 mov w20,w0
 mov w19,#-1
 cmp w0,#1
 b.lo custom_write_fail
 cmp w0,#320
 b.hi custom_write_fail
 str w20,[sp,#32]
 mov x0,#-100
 adr x1,custom_next
 mov x2,#0x8041
 movk x2,#0x8,lsl #16
 orr x2,x2,#0x800
 mov x3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,custom_write_fail
 mov x19,x0
 add x1,sp,#48
 mov x8,#80
 svc #0
 cbnz x0,custom_write_close_fail
 ldr w9,[sp,#64]
 and w9,w9,#0xf000
 mov w10,#0x8000
 cmp w9,w10
 b.ne custom_write_close_fail
 ldr w9,[sp,#68]
 cmp w9,#1
 b.ne custom_write_close_fail
 ldr w9,[sp,#72]
 cbnz w9,custom_write_close_fail
 mov x0,x19
 mov x1,#0
 mov x8,#46
 svc #0
 cbnz x0,custom_write_close_fail
 mov x0,x19
 add x1,sp,#32
 mov x2,#4
 mov x8,#64
 svc #0
 cmp x0,#4
 b.ne custom_write_close_fail
 mov x0,x19
 mov x8,#82
 svc #0
 cbnz x0,custom_write_close_fail
 mov x0,x19
 mov x8,#57
 svc #0
 mov w19,#-1
 mov x0,#-100
 adr x1,custom_next
 mov x2,#-100
 adr x3,custom_path
 mov x4,#0
 mov x8,#276
 svc #0
 cbnz x0,custom_write_fail
 mov x0,#-100
 adr x1,custom_directory
 mov x2,#0xc000
 movk x2,#0x8,lsl #16
 mov x3,#0
 mov x8,#56
 svc #0
 tbnz x0,#63,custom_write_fail
 mov x19,x0
 mov x8,#82
 svc #0
 mov x20,x0
 mov x0,x19
 mov x8,#57
 svc #0
 cmp x20,#0
 cset w0,eq
 b custom_write_end
custom_write_close_fail:
 mov x0,x19
 mov x8,#57
 svc #0
custom_write_fail:
 mov w0,#0
custom_write_end:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#208
 ret
custom_title: .asciz "Custom Bitrate"
custom_text: .asciz "Custom"
custom_max_text: .asciz "Max"
custom_bad_text: .asciz "Enter 1-320 Mbps"
custom_error_text: .asciz "Save failed"
custom_saved_text: .asciz "Saved (next recording)"
custom_value_template: .ascii "260 Mbps total\0\0"
custom_labels: .asciz "1"; .zero 6; .asciz "2"; .zero 6; .asciz "3"; .zero 6
 .asciz "4"; .zero 6; .asciz "5"; .zero 6; .asciz "6"; .zero 6
 .asciz "7"; .zero 6; .asciz "8"; .zero 6; .asciz "9"; .zero 6
 .asciz "Del"; .zero 4; .asciz "0"; .zero 6; .asciz "OK"; .zero 5
custom_path: .asciz "/data/x6-owner-features/custom-bitrate-v1/value"
custom_next: .asciz "/data/x6-owner-features/custom-bitrate-v1/value.next"
custom_directory: .asciz "/data/x6-owner-features/custom-bitrate-v1"

.balign 16
.global custom_label,custom_click,custom_bind_metadata,custom_refresh_label
// Only a real click on Custom inside the native bitrate list opens the editor.
// Caller already checked the widget's disabled flag. Other lists call x7 intact.
custom_click:
 bti c
 cmp w1,#3
 b.ne custom_click_original
 ldr x9,[x0,#0x30]
 cbz x9,custom_click_original
 ldr x9,[x9]
 adrp x10,custom_bitrate_page_class
 add x10,x10,:lo12:custom_bitrate_page_class
 cmp x9,x10
 b.ne custom_click_original
 stp x29,x30,[sp,#-32]!
 str x19,[sp,#16]
 ldr x19,[x0,#0x30]
 mov x0,x19
 adr x1,custom_queued_open
 bl new_slot
 mov x2,x19
 bl custom_post_signal
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
custom_click_original:
 mov x16,x7
 br x16
custom_label:
 bti c
 stp x29,x30,[sp,#-64]!
 bl custom_read
 adr x9,custom_label_prefix
 ldr x10,[x9]
 str x10,[sp,#24]
 add x9,sp,#31
 mov w10,#100
 udiv w11,w0,w10
 msub w0,w11,w10,w0
 cbz w11,custom_label_tens
 add w12,w11,#48
 strb w12,[x9],#1
custom_label_tens:
 mov w10,#10
 udiv w12,w0,w10
 msub w0,w12,w10,w0
 orr w13,w11,w12
 cbz w13,custom_label_ones
 add w12,w12,#48
 strb w12,[x9],#1
custom_label_ones:
 add w0,w0,#48
 strb w0,[x9],#1
 adr x10,custom_label_suffix
 ldr w11,[x10]
 str w11,[x9]
 ldrh w11,[x10,#4]
 strh w11,[x9,#4]
 add x0,sp,#24
 bl new_string
 ldp x29,x30,[sp],#64
 ret
custom_label_prefix: .asciz "Custom "
custom_label_suffix: .asciz " Mbps"
.balign 16
custom_bind_metadata:
 bti c
 stp x29,x30,[sp,#-32]!
 stp x19,x20,[sp,#16]
 mov x19,x0
 mov x20,x1
 cbz x20,custom_bind_done
 ldr x9,[x20,#0x68]
 cbz x9,custom_bind_done
 ldr x10,[x9]
 adrp x11,custom_stringoptions_class
 add x11,x11,:lo12:custom_stringoptions_class
 cmp x10,x11
 b.ne custom_bind_done
 bl custom_label
 mov x1,x0
 ldr x0,[x20,#0x68]
 bl option_string3
custom_bind_done:
 mov x0,x19
 mov x1,x20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#32
 b custom_metadata_bind_native
custom_refresh_label:
 bti c
 stp x29,x30,[sp,#-32]!
 str x19,[sp,#16]
 adrp x0,custom_meta_holder
 add x0,x0,:lo12:custom_meta_holder
 adrp x1,custom_meta_class
 add x1,x1,:lo12:custom_meta_class
 bl get_auto
 cbz x0,custom_refresh_done
 add x19,x0,#2,lsl #12
 add x19,x19,#0x478
 ldr x19,[x19,#0x68]
 cbz x19,custom_refresh_done
 ldr x9,[x19]
 adrp x10,custom_stringoptions_class
 add x10,x10,:lo12:custom_stringoptions_class
 cmp x9,x10
 b.ne custom_refresh_done
 bl custom_label
 mov x1,x0
 mov x0,x19
 bl option_string3
custom_refresh_done:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret

.balign 16
.global custom_queued_open
custom_queued_open:
 bti c
 stp x29,x30,[sp,#-32]!
 str x19,[sp,#16]
 mov x19,x0
 adrp x0,stats_focus_holder
 add x0,x0,:lo12:stats_focus_holder
 adrp x1,stats_focus_class
 add x1,x1,:lo12:stats_focus_class
 bl get_auto
 cbz x0,custom_queued_end
 ldr x9,[x0,#0x28]
 cmp x9,x19
 b.ne custom_queued_end
 bl custom_open
custom_queued_end:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret

// Repaint the parent native bitrate list through the normal UI event queue.
// Native CreatSubPage establishes child+a0=parent and parent+a8=child.
.balign 16
.global custom_refresh_visible
custom_refresh_visible:
 bti c
 cbz x0,custom_visible_return
 ldr x9,[x0]
 adrp x10,custom_page_class
 add x10,x10,:lo12:custom_page_class
 cmp x9,x10
 b.ne custom_visible_return
 ldr x9,[x0,#0xa0]
 cbz x9,custom_visible_return
 ldr x10,[x9]
 adrp x11,custom_bitrate_page_class
 add x11,x11,:lo12:custom_bitrate_page_class
 cmp x10,x11
 b.ne custom_visible_return
 ldr x10,[x9,#0xa8]
 cmp x10,x0
 b.ne custom_visible_return
 add x0,x9,#2,lsl #12
 add x0,x0,#0x9c0
 ldr x10,[x0]
 adrp x11,custom_bitrate_list_class
 add x11,x11,:lo12:custom_bitrate_list_class
 cmp x10,x11
 b.ne custom_visible_return
 ldr x10,[x0,#0x30]
 cmp x10,x9
 b.ne custom_visible_return
 stp x29,x30,[sp,#-32]!
 str x19,[sp,#16]
 mov x19,x0
 adrp x1,custom_list_update_native
 add x1,x1,:lo12:custom_list_update_native
 bl new_slot
 mov x2,x19
 bl custom_post_signal
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
custom_visible_return:
 ret

// ---- Interval Timing "Custom" numeric editor (owner 2026-11-04) ----
// Seconds with one decimal (keypad 1-9 / . / 0 / OK). The committed value in ms goes to
// /data/x6-owner-features/interval-controls-v1/custom.value (staged, like custom bitrate); the
// controller publishes it as the mediad period. The title shows the controller's recommend-text
// (the Minimum target) so the owner sees the recommended minimum and may go lower at their risk.
// ival_globals: +0 value_ms, +4 firstkey, +8 status(0/1saved/2bad/3io), +12 whole, +16 mode
// (0 int,1 dot,2 frac), +20 frac; +64.. a 63-byte title buffer.
.balign 16
.global ival_queued_open,ival_open,ival_key,ival_paint,ival_read,ival_write,ival_title
ival_queued_open:
 bti c
 stp x29,x30,[sp,#-32]!
 str x19,[sp,#16]
 mov x19,x0
 adrp x0,stats_focus_holder
 add x0,x0,:lo12:stats_focus_holder
 adrp x1,stats_focus_class
 add x1,x1,:lo12:stats_focus_class
 bl get_auto
 cbz x0,ival_queued_end
 ldr x9,[x0,#0x28]
 cmp x9,x19
 b.ne ival_queued_end
 mov x0,x19
 bl ival_open
ival_queued_end:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
ival_open:
 bti c
 stp x29,x30,[sp,#-96]!
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 stp x23,x24,[sp,#48]
 mov x19,x0
 ldr x9,[x19,#0xa8]
 cbnz x9,ival_open_end
 adrp x1,custom_page_class
 add x1,x1,:lo12:custom_page_class
 mov w2,#0
 mov x3,#0
 mov x4,#0
 mov x5,#0
 mov x6,#0
 bl create_subpage
 ldr x19,[x19,#0xa8]
 cbz x19,ival_open_end
 adrp x9,custom_page_class
 add x9,x9,:lo12:custom_page_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne ival_open_end
 bl ival_title
 bl new_string
 mov x1,x0
 add x0,x19,#0x338
 bl page_title
 bl ival_read
 adrp x9,ival_globals
 add x9,x9,:lo12:ival_globals
 str w0,[x9]
 mov w10,#1
 str w10,[x9,#4]
 str wzr,[x9,#8]
 mov w11,#1000
 udiv w12,w0,w11
 msub w13,w12,w11,w0
 str w12,[x9,#12]
 mov w11,#100
 udiv w14,w13,w11
 str w14,[x9,#20]
 cmp w14,#0
 mov w15,#2
 csel w15,w15,wzr,ne
 str w15,[x9,#16]
 adrp x0,custom_text_class
 add x0,x0,:lo12:custom_text_class
 mov x1,#0
 bl new_object
 cbz x0,ival_open_end
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 mov x0,x20
 mov x1,#20
 mov x9,#88
 orr x1,x1,x9,lsl #32
 mov x2,#370
 mov x9,#144
 orr x2,x2,x9,lsl #32
 bl custom_text_bounds
 adrp x0,stats_small_font
 add x0,x0,:lo12:stats_small_font
 adrp x1,stats_font_class
 add x1,x1,:lo12:stats_font_class
 bl stats_load_resource
 mov x1,x0
 mov x0,x20
 bl custom_text_font
 mov x0,x20
 mov w1,#0xffffffff
 bl text_color
 mov x0,x20
 mov w1,#0x12
 bl stats_set_alignment
 mov w21,#0
 adr x23,ival_labels
ival_button_loop:
 adrp x0,custom_button_class
 add x0,x0,:lo12:custom_button_class
 mov x1,#0
 bl new_object
 cbz x0,ival_open_end
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 add x0,x23,x21,lsl #3
 bl new_string
 mov x1,x0
 mov x0,x20
 bl custom_button_text
 mov x0,x20
 mov w1,#0x3333
 movk w1,#0xff33,lsl #16
 bl custom_button_color
 mov x0,x20
 mov w1,#1
 bl custom_button_background
 adrp x0,stats_small_font
 add x0,x0,:lo12:stats_small_font
 adrp x1,stats_font_class
 add x1,x1,:lo12:stats_font_class
 bl stats_load_resource
 mov x1,x0
 mov x0,x20
 bl custom_button_font
 mov x0,x20
 mov w1,#0xffffffff
 bl custom_button_foreground
 mov x0,x20
 mov w1,#60
 bl custom_button_height
 mov x0,x20
 mov x1,#25
 mov x2,#25
 bl custom_button_padding
 mov x0,x19
 adr x1,ival_key0
 add x1,x1,x21,lsl #4
 bl new_slot
 add x9,x20,#0x2e8
 stp x0,x1,[x9]
 mov w9,#3
 udiv w10,w21,w9
 msub w11,w10,w9,w21
 mov w12,#120
 mul w11,w11,w12
 add w11,w11,#20
 mov w12,#70
 mul w10,w10,w12
 add w10,w10,#150
 orr x1,x11,x10,lsl #32
 add w11,w11,#110
 add w10,w10,#60
 orr x2,x11,x10,lsl #32
 mov x0,x20
 bl set_bounds
 add w21,w21,#1
 cmp w21,#12
 b.lo ival_button_loop
 mov x0,x19
 bl ival_paint
ival_open_end:
 ldp x23,x24,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#96
 ret
.balign 16
ival_key0: bti c; mov w1,#0; b ival_key; nop
ival_key1: bti c; mov w1,#1; b ival_key; nop
ival_key2: bti c; mov w1,#2; b ival_key; nop
ival_key3: bti c; mov w1,#3; b ival_key; nop
ival_key4: bti c; mov w1,#4; b ival_key; nop
ival_key5: bti c; mov w1,#5; b ival_key; nop
ival_key6: bti c; mov w1,#6; b ival_key; nop
ival_key7: bti c; mov w1,#7; b ival_key; nop
ival_key8: bti c; mov w1,#8; b ival_key; nop
ival_key9: bti c; mov w1,#9; b ival_key; nop
ival_key10: bti c; mov w1,#10; b ival_key; nop
ival_key11: bti c; mov w1,#11; b ival_key; nop
ival_key:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 mov x19,x0
 mov w20,w1
 adrp x21,ival_globals
 add x21,x21,:lo12:ival_globals
 str wzr,[x21,#8]
 cmp w20,#11
 b.eq ival_save
 ldr w9,[x21,#4]
 cbz w9,ival_key_nofirst
 str wzr,[x21,#12]
 str wzr,[x21,#16]
 str wzr,[x21,#20]
 str wzr,[x21,#4]
ival_key_nofirst:
 cmp w20,#9
 b.eq ival_dot
 cmp w20,#10
 add w11,w20,#1
 mov w12,#0
 csel w10,w12,w11,eq
 ldr w13,[x21,#16]
 cbnz w13,ival_key_frac
 ldr w14,[x21,#12]
 mov w15,#10
 madd w14,w14,w15,w10
 cmp w14,#120
 b.hi ival_redraw
 str w14,[x21,#12]
 b ival_redraw
ival_key_frac:
 cmp w13,#1
 b.ne ival_redraw
 str w10,[x21,#20]
 mov w14,#2
 str w14,[x21,#16]
 b ival_redraw
ival_dot:
 ldr w13,[x21,#16]
 cbnz w13,ival_redraw
 mov w14,#1
 str w14,[x21,#16]
 b ival_redraw
ival_save:
 ldr w9,[x21,#12]
 mov w10,#1000
 mul w9,w9,w10
 ldr w11,[x21,#16]
 cmp w11,#2
 b.lo ival_save_noval
 ldr w12,[x21,#20]
 mov w13,#100
 madd w9,w12,w13,w9
ival_save_noval:
 str w9,[x21]
 cmp w9,#100
 b.lo ival_bad
 mov w10,#0xd4c0
 movk w10,#1,lsl #16
 cmp w9,w10
 b.hi ival_bad
 mov w0,w9
 bl ival_write
 cbz w0,ival_io_error
 mov x0,x19                     // the Interval Timing page under this keypad: show the new value on its Custom row
 bl ival_relabel
 mov w9,#1
 str w9,[x21,#8]
 b ival_redraw
ival_bad:
 mov w9,#2
 str w9,[x21,#8]
 b ival_redraw
ival_io_error:
 mov w9,#3
 str w9,[x21,#8]
ival_redraw:
 mov x0,x19
 bl ival_paint
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret
ival_paint:
 bti c
 stp x29,x30,[sp,#-96]!
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 mov x19,x0
 adrp x20,ival_globals
 add x20,x20,:lo12:ival_globals
 ldr w9,[x20,#8]
 adr x0,ival_bad_text
 cmp w9,#2
 b.eq ival_paint_text
 adr x0,ival_io_text
 cmp w9,#3
 b.eq ival_paint_text
 adr x0,ival_saved_text
 cmp w9,#1
 b.eq ival_paint_text
 add x21,sp,#64
 ldr w10,[x20,#12]
 mov w11,#100
 udiv w12,w10,w11
 msub w13,w12,w11,w10
 cbz w12,ival_paint_tens
 add w14,w12,#48
 strb w14,[x21],#1
 mov w15,#1
 b ival_paint_tens2
ival_paint_tens:
 mov w15,#0
ival_paint_tens2:
 mov w11,#10
 udiv w12,w13,w11
 msub w13,w12,w11,w13
 orr w9,w15,w12
 cbz w9,ival_paint_ones
 add w14,w12,#48
 strb w14,[x21],#1
ival_paint_ones:
 add w14,w13,#48
 strb w14,[x21],#1
 ldr w10,[x20,#16]
 cbz w10,ival_paint_suffix
 mov w14,#46
 strb w14,[x21],#1
 cmp w10,#2
 b.lo ival_paint_suffix
 ldr w11,[x20,#20]
 add w14,w11,#48
 strb w14,[x21],#1
ival_paint_suffix:
 adr x10,ival_suffix
 ldrb w11,[x10]
 strb w11,[x21],#1
 ldrb w11,[x10,#1]
 strb w11,[x21],#1
 strb wzr,[x21]
 add x0,sp,#64
ival_paint_text:
 bl new_string
 mov x20,x0
 ldr x19,[x19,#0x60]
ival_find_text:
 cbz x19,ival_paint_end
 ldr x9,[x19]
 adrp x10,custom_text_class
 add x10,x10,:lo12:custom_text_class
 cmp x9,x10
 b.eq ival_found_text
 ldr x19,[x19,#0x20]
 b ival_find_text
ival_found_text:
 mov x0,x19
 mov x1,x20
 bl custom_text_value
ival_paint_end:
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#96
 ret
// --- w0 = the saved custom.value in ms when it is exactly 4 bytes and 100..120000, else 0 ---
ival_saved:
 bti c
 stp x29,x30,[sp,#-48]!
 stp x19,x20,[sp,#16]
 mov x0,#-100
 adr x1,ival_value_path
 mov x2,#0x8800
 movk x2,#0x8,lsl #16
 mov x3,#0
 mov x8,#56
 svc #0
 tbnz x0,#63,ival_saved_none
 mov x19,x0
 add x1,sp,#32
 mov x2,#5
 mov x8,#63
 svc #0
 mov x20,x0
 mov x0,x19
 mov x8,#57
 svc #0
 cmp x20,#4
 b.ne ival_saved_none
 ldr w0,[sp,#32]
 cmp w0,#100
 b.lo ival_saved_none
 mov w9,#0xd4c0
 movk w9,#1,lsl #16
 cmp w0,w9
 b.ls ival_saved_end
ival_saved_none:
 mov w0,#0
ival_saved_end:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
ival_read:
 bti c
 stp x29,x30,[sp,#-48]!
 stp x19,x20,[sp,#16]
 bl ival_saved
 cbnz w0,ival_read_end
ival_read_recommend:
 mov x0,#-100
 adr x1,ival_recommend_path
 mov x2,#0x8800
 movk x2,#0x8,lsl #16
 mov x3,#0
 mov x8,#56
 svc #0
 tbnz x0,#63,ival_read_default
 mov x19,x0
 add x1,sp,#32
 mov x2,#5
 mov x8,#63
 svc #0
 mov x20,x0
 mov x0,x19
 mov x8,#57
 svc #0
 cmp x20,#4
 b.ne ival_read_default
 ldr w0,[sp,#32]
 cbz w0,ival_read_default
 add w0,w0,#99
 mov w9,#100
 udiv w10,w0,w9
 mul w0,w10,w9
 cmp w0,#100
 b.hs ival_read_hi
 mov w0,#100
 b ival_read_end
ival_read_hi:
 mov w9,#0xd4c0
 movk w9,#1,lsl #16
 cmp w0,w9
 b.ls ival_read_end
 mov w0,#0xd4c0
 movk w0,#1,lsl #16
 b ival_read_end
ival_read_default:
 mov w0,#1000
ival_read_end:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
ival_write:
 bti c
 stp x29,x30,[sp,#-208]!
 stp x19,x20,[sp,#16]
 mov w20,w0
 mov w19,#-1
 cmp w0,#100
 b.lo ival_write_fail
 mov w9,#0xd4c0
 movk w9,#1,lsl #16
 cmp w0,w9
 b.hi ival_write_fail
 str w20,[sp,#32]
 mov x0,#-100
 adr x1,ival_next
 mov x2,#0x8041
 movk x2,#0x8,lsl #16
 orr x2,x2,#0x800
 mov x3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,ival_write_fail
 mov x19,x0
 add x1,sp,#48
 mov x8,#80
 svc #0
 cbnz x0,ival_write_close_fail
 ldr w9,[sp,#64]
 and w9,w9,#0xf000
 mov w10,#0x8000
 cmp w9,w10
 b.ne ival_write_close_fail
 ldr w9,[sp,#68]
 cmp w9,#1
 b.ne ival_write_close_fail
 ldr w9,[sp,#72]
 cbnz w9,ival_write_close_fail
 mov x0,x19
 mov x1,#0
 mov x8,#46
 svc #0
 cbnz x0,ival_write_close_fail
 mov x0,x19
 add x1,sp,#32
 mov x2,#4
 mov x8,#64
 svc #0
 cmp x0,#4
 b.ne ival_write_close_fail
 mov x0,x19
 mov x8,#82
 svc #0
 cbnz x0,ival_write_close_fail
 mov x0,x19
 mov x8,#57
 svc #0
 mov w19,#-1
 mov x0,#-100
 adr x1,ival_next
 mov x2,#-100
 adr x3,ival_value_path
 mov x4,#0
 mov x8,#276
 svc #0
 cbnz x0,ival_write_fail
 mov x0,#-100
 adr x1,ival_dir
 mov x2,#0xc000
 movk x2,#0x8,lsl #16
 mov x3,#0
 mov x8,#56
 svc #0
 tbnz x0,#63,ival_write_fail
 mov x19,x0
 mov x8,#82
 svc #0
 mov x20,x0
 mov x0,x19
 mov x8,#57
 svc #0
 cmp x20,#0
 cset w0,eq
 b ival_write_end
ival_write_close_fail:
 mov x0,x19
 mov x8,#57
 svc #0
ival_write_fail:
 mov w0,#0
ival_write_end:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#208
 ret
ival_title:
 bti c
 stp x29,x30,[sp,#-192]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 adr x20,ival_title_default
 mov x0,#-100
 adr x1,ival_rectext_path
 mov w2,#0x8800
 movk w2,#8,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,ival_title_done
 mov x19,x0
 add x1,sp,#48
 mov x8,#80
 svc #0
 cbnz x0,ival_title_close
 ldr w9,[sp,#64]
 mov w10,#0xf022
 and w9,w9,w10
 cmp w9,#0x8000
 b.ne ival_title_close
 ldr w9,[sp,#68]
 cmp w9,#1
 b.ne ival_title_close
 ldr w9,[sp,#72]
 cbnz w9,ival_title_close
 ldr x9,[sp,#96]
 sub x9,x9,#2
 cmp x9,#61
 b.hi ival_title_close
 mov x0,x19
 adrp x1,ival_globals
 add x1,x1,:lo12:ival_globals
 add x1,x1,#64
 mov x2,#63
 mov x8,#63
 svc #0
 cmp x0,#2
 b.lt ival_title_close
 adrp x20,ival_globals
 add x20,x20,:lo12:ival_globals
 add x20,x20,#64
 strb wzr,[x20,x0]
ival_title_close:
 mov x0,x19
 mov x8,#57
 svc #0
ival_title_done:
 mov x0,x20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#192
 ret

// --- x0 = the Custom row's label: "Custom 2.5 s" / "Custom 120 s" from the saved value (whole seconds and one
// decimal, the keypad's own resolution), or plain "Custom" when none is saved. Built in ival_globals+0x80. ---
.balign 16
.global ival_row_label,ival_relabel,ival_saved
ival_row_label:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 bl ival_saved
 adr x1,minimum_names
 add x1,x1,#32                  // "Custom"
 mov x20,x1
 cbz w0,ival_rl_plain
 mov w19,w0                     // ms (callee-saved: survives the helpers)
 adrp x0,ival_globals
 add x0,x0,:lo12:ival_globals
 add x0,x0,#0x80
 bl pp_strcpy                   // "Custom"
 mov w9,#32
 strb w9,[x0],#1                // " "
 mov w9,#1000
 udiv w1,w19,w9                 // whole seconds (<= 120)
 msub w19,w1,w9,w19             // the remaining ms
 bl pp_fmt_u3
 mov w9,#100
 udiv w10,w19,w9                // tenths
 cbz w10,ival_rl_suffix
 mov w11,#46
 strb w11,[x0],#1               // "."
 add w10,w10,#48
 strb w10,[x0],#1
ival_rl_suffix:
 adr x1,ival_suffix
 bl pp_strcpy                   // " s"
 adrp x20,ival_globals
 add x20,x20,:lo12:ival_globals
 add x20,x20,#0x80
ival_rl_plain:
 mov x0,x20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#32
 ret

// --- after a save, x0 = the keypad page: relabel the Custom row of the Interval Timing page under it. Native
// CreatSubPage links child+0xa0 = parent and parent+0xa8 = child; both links and the parent's class must hold, and
// the row is found ONLY by its slot callback (minimum_pick2) on a radio row -- never by position. ---
ival_relabel:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 str x21,[sp,#32]
 cbz x0,ival_relabel_ret
 ldr x19,[x0,#0xa0]             // the page under the keypad
 cbz x19,ival_relabel_ret
 ldr x9,[x19,#0xa8]
 cmp x9,x0                      // ... whose child is this keypad
 b.ne ival_relabel_ret
 adrp x9,page_class
 add x9,x9,:lo12:page_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne ival_relabel_ret
 bl ival_row_label
 bl new_string
 mov x20,x0
 ldr x19,[x19,#0x60]            // first child
 mov w21,#0
ival_relabel_loop:
 cbz x19,ival_relabel_ret
 add w21,w21,#1
 cmp w21,#128
 b.hi ival_relabel_ret
 adrp x9,radio_class
 add x9,x9,:lo12:radio_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne ival_relabel_next
 ldr x10,[x19,#0x4c8]
 adr x9,minimum_pick2
 cmp x9,x10
 b.ne ival_relabel_next
 mov x0,x19
 mov x1,x20
 bl radio_title
 b ival_relabel_ret             // one Custom row per page
ival_relabel_next:
 ldr x19,[x19,#0x20]
 b ival_relabel_loop
ival_relabel_ret:
 ldr x21,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
ival_title_default: .asciz "Custom interval"
ival_rectext_path: .asciz "/run/x6-interval-controls/recommend-text"
ival_recommend_path: .asciz "/run/x6-interval-controls/recommend"
ival_value_path: .asciz "/data/x6-owner-features/interval-controls-v1/custom.value"
ival_next: .asciz "/data/x6-owner-features/interval-controls-v1/custom.value.next"
ival_dir: .asciz "/data/x6-owner-features/interval-controls-v1"
ival_bad_text: .asciz "Enter 0.1-120 s"
ival_io_text: .asciz "Save failed"
ival_saved_text: .asciz "Saved"
ival_suffix: .asciz " s"
.balign 16
ival_labels: .asciz "1"; .zero 6; .asciz "2"; .zero 6; .asciz "3"; .zero 6
 .asciz "4"; .zero 6; .asciz "5"; .zero 6; .asciz "6"; .zero 6
 .asciz "7"; .zero 6; .asciz "8"; .zero 6; .asciz "9"; .zero 6
 .asciz "."; .zero 6; .asciz "0"; .zero 6; .asciz "OK"; .zero 5

.balign 16
ssd_click:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 mov x19,x0
 ldrb w0,[x19,#0xb88]
 bl ssd_request
 cbz w0,ssd_click_done
 // Failure to submit never leaves a false selected state.
 bl ssd_read_state
 mov w1,w0
 mov x0,x19
 bl switch_checked
ssd_click_done:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
ssd_read_state:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov w20,#0
 mov x0,#-100
 adr x1,ssd_state_path
 mov w2,#0x8800
 movk w2,#8,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,ssd_read_done
 mov x19,x0
 add x1,sp,#32
 mov x2,#2
 mov x8,#63
 svc #0
 cmp x0,#1
 b.ne ssd_read_close
 ldrb w20,[sp,#32]
 cmp w20,#1
 csel w20,w20,wzr,ls
ssd_read_close:
 mov x0,x19
 mov x8,#57
 svc #0
ssd_read_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
ssd_request:
 bti c
 cmp w0,#1
 b.hi ssd_request_invalid
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 strb w0,[sp,#32]
 mov w20,#-1
 mov x0,#-100
 adr x1,ssd_request_stage
 mov w2,#0x88c1
 movk w2,#8,lsl #16
 mov w3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,ssd_request_done
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
 cbnz w20,ssd_request_unlink
 mov x0,#-100
 adr x1,ssd_request_stage
 mov x2,#-100
 adr x3,ssd_request_path
 mov w4,#1
 mov x8,#276
 svc #0
 mov w20,w0
 cbz w20,ssd_request_done
ssd_request_unlink:
 mov x0,#-100
 adr x1,ssd_request_stage
 mov w2,#0
 mov x8,#35
 svc #0
ssd_request_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
ssd_request_invalid:
 mov w0,#-1
 ret

.balign 16
ssd_title: .asciz "Use External SSD"
ssd_state_path: .asciz "/run/x6-ssd/selected"
ssd_request_path: .asciz "/run/x6-ssd/request"
ssd_request_stage: .asciz "/run/x6-ssd/request.next"

.balign 4
.text
.global ssd_notify
ssd_notify:
sub sp, sp, #192
stp x0, x1, [sp, #0]
stp x2, x3, [sp, #16]
stp x4, x5, [sp, #32]
stp x6, x7, [sp, #48]
stp x8, x9, [sp, #64]
stp x10, x11, [sp, #80]
stp x12, x13, [sp, #96]
stp x14, x15, [sp, #112]
stp x16, x17, [sp, #128]
stp x18, x30, [sp, #144]
mrs x9, nzcv
str x9, [sp,#160]
mov x0,#-100
adr x1,ssd_notify_flag
mov x2,#0x8800
movk x2,#8,lsl #16
mov x3,#0
mov x8,#56
svc #0
cmp x0,#0
b.lt ssd_notify_native
str x0,[sp,#168]
add x1,sp,#176
mov x2,#2
mov x8,#63
svc #0
str x0,[sp,#184]
ldr x0,[sp,#168]
mov x8,#57
svc #0
ldr x0,[sp,#184]
cmp x0,#1
b.ne ssd_notify_native
ldrb w0,[sp,#176]
cmp w0,#49
b.ne ssd_notify_native
ldr x9,[sp,#160]
msr nzcv,x9
ldp x0, x1, [sp, #0]
ldp x2, x3, [sp, #16]
ldp x4, x5, [sp, #32]
ldp x6, x7, [sp, #48]
ldp x8, x9, [sp, #64]
ldp x10, x11, [sp, #80]
ldp x12, x13, [sp, #96]
ldp x14, x15, [sp, #112]
ldp x16, x17, [sp, #128]
ldp x18,x30,[sp,#144]
add sp,sp,#192
b ssd_toast_skip
ssd_notify_native:
ldr x9,[sp,#160]
msr nzcv,x9
ldp x0, x1, [sp, #0]
ldp x2, x3, [sp, #16]
ldp x4, x5, [sp, #32]
ldp x6, x7, [sp, #48]
ldp x8, x9, [sp, #64]
ldp x10, x11, [sp, #80]
ldp x12, x13, [sp, #96]
ldp x14, x15, [sp, #112]
ldp x16, x17, [sp, #128]
ldp x18,x30,[sp,#144]
add sp,sp,#192
b.eq 1f
b ssd_toast_skip
1: b ssd_toast_continue
ssd_notify_flag: .asciz "/run/x6-ssd/active"

.balign 16
.global ic_open
ic_open:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 str x23,[sp,#48]
 mov x19,x0
 ldr x9,[x19,#0xa8]
 cbnz x9,ic_open_done
 adrp x1,page_class
 add x1,x1,:lo12:page_class
 mov w2,#0
 mov x3,#0
 mov x4,#0
 mov x5,#0
 mov x6,#0
 bl create_subpage
 ldr x19,[x19,#0xa8]
 cbz x19,ic_open_done
 adrp x9,page_class
 add x9,x9,:lo12:page_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne ic_open_done
 adr x0,ic_title
 bl new_string
 mov x1,x0
 add x0,x19,#0x338
 bl page_title
 adrp x0,arrow_class
 add x0,x0,:lo12:arrow_class
 mov x1,#0
 bl new_object
 cbz x0,ic_open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adr x0,shutter_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl arrow_title
 mov x0,x19
 adr x1,shutter_open
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
 adrp x0,arrow_class
 add x0,x0,:lo12:arrow_class
 mov x1,#0
 bl new_object
 cbz x0,ic_open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adr x0,minimum_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl arrow_title
 mov x0,x19
 adr x1,minimum_open
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]

 adrp x0,switch_class
 add x0,x0,:lo12:switch_class
 mov x1,#0
 bl new_object
 cbz x0,ic_open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adr x0,f8k_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl switch_title
 bl f8k_read_state
 mov w1,w0
 mov x0,x20
 bl switch_checked
 mov x0,x20
 adr x1,f8k_click
 bl new_slot
 add x9,x20,#0x4c0
 // One-for-one detour: the 8K row's final slot store and the 16K row continue in
 // ic_open_f16k (end of file) so that ic_open keeps its exact size; see the comment there.
 b ic_open_f16k
ic_open_done:
 ldr x23,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret
ic_title: .asciz "Interval Settings"

.balign 16
.global shutter_open,shutter_read_state,shutter_request,shutter_pick0,shutter_pick1,shutter_pick2,shutter_pick3,shutter_pick4,shutter_pick5,shutter_pick6
shutter_open:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 str x23,[sp,#48]
 mov x19,x0
 ldr x9,[x19,#0xa8]
 cbnz x9,shutter_open_done
 adrp x1,page_class
 add x1,x1,:lo12:page_class
 mov w2,#0
 mov x3,#0
 mov x4,#0
 mov x5,#0
 mov x6,#0
 bl create_subpage
 ldr x19,[x19,#0xa8]
 cbz x19,shutter_open_done
 adrp x9,page_class
 add x9,x9,:lo12:page_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne shutter_open_done
 adr x0,shutter_title
 bl new_string
 mov x1,x0
 add x0,x19,#0x338
 bl page_title
 bl shutter_read_state
 mov w22,w0
 mov w21,#0
 adr x23,shutter_names
shutter_loop:
 adrp x0,radio_class
 add x0,x0,:lo12:radio_class
 mov x1,#0
 bl new_object
 cbz x0,shutter_open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 add x0,x23,x21,lsl #4
 bl new_string
 mov x1,x0
 mov x0,x20
 bl radio_title
 cmp w21,w22
 cset w1,eq
 mov x0,x20
 bl radio_selected
 mov x0,x19
 adr x1,shutter_pick0
 add x1,x1,x21,lsl #4
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
 add w21,w21,#1
 cmp w21,#7
 b.lo shutter_loop
shutter_open_done:
 ldr x23,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret
.balign 16
shutter_pick0:
 bti c
 mov w1,#0
 b shutter_pick
.balign 16
shutter_pick1:
 bti c
 mov w1,#1
 b shutter_pick
.balign 16
shutter_pick2:
 bti c
 mov w1,#2
 b shutter_pick
.balign 16
shutter_pick3:
 bti c
 mov w1,#3
 b shutter_pick
.balign 16
shutter_pick4:
 bti c
 mov w1,#4
 b shutter_pick
.balign 16
shutter_pick5:
 bti c
 mov w1,#5
 b shutter_pick
.balign 16
shutter_pick6:
 bti c
 mov w1,#6
 b shutter_pick
shutter_pick:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov x19,x0
 mov w20,w1
 mov w0,w1
 bl shutter_request
 cbz w0,shutter_paint_requested
 bl shutter_read_state
 mov w20,w0
shutter_paint_requested:
 mov x0,x19
 mov w1,w20
 bl shutter_paint
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
shutter_paint:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 ldr x19,[x0,#0x60]
 mov w20,w1
 mov w21,#0
shutter_paint_loop:
 cbz x19,shutter_paint_done
 add w21,w21,#1
 cmp w21,#128
 b.hi shutter_paint_done
 adrp x9,radio_class
 add x9,x9,:lo12:radio_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne shutter_paint_next
 ldr x10,[x19,#0x4c8]
 adr x9,shutter_pick0
 sub x10,x10,x9
 cmp x10,#96
 b.hi shutter_paint_next
 tst x10,#15
 b.ne shutter_paint_next
 lsr x22,x10,#4
 cmp w22,w20
 cset w1,eq
 mov x0,x19
 bl radio_selected
shutter_paint_next:
 ldr x19,[x19,#0x20]
 b shutter_paint_loop
shutter_paint_done:
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret
shutter_read_state:
 bti c
 adr x0,shutter_state_path
 mov w1,#-1
 mov w2,#6
 b ic_io
shutter_request:
 bti c
 mov w1,w0
 adr x0,shutter_request_path
 mov w2,#6
 b ic_io
shutter_title: .asciz "Shutter Max"
shutter_state_path: .asciz "/run/x6-interval-controls/shutter-state"
shutter_request_path: .asciz "/run/x6-interval-controls/shutter-request"
.balign 16
shutter_names:
.asciz "1/30 s"
.space 9
.asciz "1/15 s"
.space 9
.asciz "1/8 s"
.space 10
.asciz "1/4 s"
.space 10
.asciz "1/2 s"
.space 10
.asciz "1 s"
.space 12
.asciz "2 s"
.space 12

.balign 16
.global minimum_open,minimum_read_state,minimum_request,minimum_pick0,minimum_pick1,minimum_pick2
minimum_open:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 str x23,[sp,#48]
 mov x19,x0
 ldr x9,[x19,#0xa8]
 cbnz x9,minimum_open_done
 adrp x1,page_class
 add x1,x1,:lo12:page_class
 mov w2,#0
 mov x3,#0
 mov x4,#0
 mov x5,#0
 mov x6,#0
 bl create_subpage
 ldr x19,[x19,#0xa8]
 cbz x19,minimum_open_done
 adrp x9,page_class
 add x9,x9,:lo12:page_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne minimum_open_done
 adr x0,minimum_title
 bl new_string
 mov x1,x0
 add x0,x19,#0x338
 bl page_title
 bl minimum_read_state
 mov w22,w0
 mov w21,#0
 adr x23,minimum_names
minimum_loop:
 adrp x0,radio_class
 add x0,x0,:lo12:radio_class
 mov x1,#0
 bl new_object
 cbz x0,minimum_open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 cmp w21,#1
 b.ne minimum_label_not1
 bl ic_label
 b minimum_label_ready
minimum_label_not1:
 cmp w21,#2
 b.ne minimum_static_label
 bl ival_row_label              // "Custom 2.5 s" once a value is saved (owner 2026-11-04: like the bitrate's Custom)
 b minimum_label_ready
minimum_static_label:
 add x0,x23,x21,lsl #4
minimum_label_ready:
 bl new_string
 mov x1,x0
 mov x0,x20
 bl radio_title
 cmp w21,w22
 cset w1,eq
 mov x0,x20
 bl radio_selected
 mov x0,x19
 adr x1,minimum_pick0
 add x1,x1,x21,lsl #4
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
 add w21,w21,#1
 cmp w21,#3
 b.lo minimum_loop
minimum_open_done:
 ldr x23,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret
.balign 16
minimum_pick0:
 bti c
 mov w1,#0
 b minimum_pick
.balign 16
minimum_pick1:
 bti c
 mov w1,#1
 b minimum_pick
.balign 16
minimum_pick2:
 bti c
 mov w1,#2
 b minimum_pick
minimum_pick:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov x19,x0
 mov w20,w1
 mov w0,w1
 bl minimum_request
 cbz w0,minimum_pick_state_done
 bl minimum_read_state
 mov w20,w0
minimum_pick_state_done:
 // Custom selected (committed state 2): open the numeric editor through the UI event
 // queue, like custom_click, so nothing opens synchronously inside the radio click.
 cmp w20,#2
 b.ne minimum_pick_paint
 mov x0,x19
 adr x1,ival_queued_open
 bl new_slot
 mov x2,x19
 bl custom_post_signal
minimum_pick_paint:
 mov x0,x19
 mov w1,w20
 bl minimum_paint
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
minimum_paint:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 ldr x19,[x0,#0x60]
 mov w20,w1
 mov w21,#0
minimum_paint_loop:
 cbz x19,minimum_paint_done
 add w21,w21,#1
 cmp w21,#128
 b.hi minimum_paint_done
 adrp x9,radio_class
 add x9,x9,:lo12:radio_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne minimum_paint_next
 ldr x10,[x19,#0x4c8]
 adr x9,minimum_pick0
 sub x10,x10,x9
 cmp x10,#32
 b.hi minimum_paint_next
 tst x10,#15
 b.ne minimum_paint_next
 lsr x22,x10,#4
 cmp w22,w20
 cset w1,eq
 mov x0,x19
 bl radio_selected
minimum_paint_next:
 ldr x19,[x19,#0x20]
 b minimum_paint_loop
minimum_paint_done:
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret
minimum_read_state:
 bti c
 adr x0,minimum_state_path
 mov w1,#-1
 mov w2,#2
 b ic_io
minimum_request:
 bti c
 mov w1,w0
 adr x0,minimum_request_path
 mov w2,#2
 b ic_io
minimum_title: .asciz "Interval Timing"
minimum_state_path: .asciz "/run/x6-interval-controls/timing-state"
minimum_request_path: .asciz "/run/x6-interval-controls/timing-request"
.balign 16
minimum_names:
.asciz "Native"
.space 9
.asciz "Minimum"
.space 8
.asciz "Custom"
.space 9
.balign 16
.global ic_io,ic_label
// x0=path, w1=-1 read or value to write, w2=max index.
ic_io:
 bti c
 stp x29,x30,[sp,#-224]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 mov w20,w1
 mov w21,w2
 mov x1,x0
 mov x0,#-100
 mov w2,#0x8800
 movk w2,#8,lsl #16
 cmn w20,#1
 b.eq ic_io_open
 cmp w20,w21
 b.hi ic_io_fail
 add w2,w2,#1
ic_io_open:
 mov x8,#56
 svc #0
 tbnz x0,#63,ic_io_fail
 mov x19,x0
 add x1,sp,#64
 mov x8,#80
 svc #0
 cbnz x0,ic_io_bad_close
 ldr w9,[sp,#80]
 mov w10,#0xf022
 and w9,w9,w10
 cmp w9,#0x8000
 b.ne ic_io_bad_close
 ldr w9,[sp,#84]
 cmp w9,#1
 b.ne ic_io_bad_close
 ldr w9,[sp,#88]
 cbnz w9,ic_io_bad_close
 ldr x9,[sp,#112]
 cmp x9,#1
 b.ne ic_io_bad_close
 mov x0,x19
 add x1,sp,#200
 cmn w20,#1
 b.ne ic_io_write
 mov x2,#2
 mov x8,#63
 svc #0
 cmp x0,#1
 b.ne ic_io_bad_close
 ldrb w22,[sp,#200]
 cmp w22,w21
 csel w22,w22,wzr,ls
 b ic_io_close
ic_io_write:
 strb w20,[sp,#200]
 mov x2,#1
 mov x8,#64
 svc #0
 cmp x0,#1
 b.ne ic_io_bad_close
 mov w22,#0
 b ic_io_close
ic_io_bad_close:
 cmn w20,#1
 cset w22,ne
 neg w22,w22
ic_io_close:
 mov x0,x19
 mov x8,#57
 svc #0
 b ic_io_done
ic_io_fail:
 cmn w20,#1
 cset w22,ne
 neg w22,w22
ic_io_done:
 mov w0,w22
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#224
 ret
.balign 16
ic_label:
 bti c
 stp x29,x30,[sp,#-192]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 adr x20,ic_label_default
 mov x0,#-100
 adr x1,ic_label_path
 mov w2,#0x8800
 movk w2,#8,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,ic_label_done
 mov x19,x0
 add x1,sp,#48
 mov x8,#80
 svc #0
 cbnz x0,ic_label_close
 ldr w9,[sp,#64]
 mov w10,#0xf022
 and w9,w9,w10
 cmp w9,#0x8000
 b.ne ic_label_close
 ldr w9,[sp,#68]
 cmp w9,#1
 b.ne ic_label_close
 ldr w9,[sp,#72]
 cbnz w9,ic_label_close
 ldr x9,[sp,#96]
 sub x9,x9,#2
 cmp x9,#61
 b.hi ic_label_close
 mov x0,x19
 adrp x1,ic_globals
 add x1,x1,:lo12:ic_globals
 mov x2,#63
 mov x8,#63
 svc #0
 cmp x0,#2
 b.lt ic_label_close
 adrp x20,ic_globals
 add x20,x20,:lo12:ic_globals
 strb wzr,[x20,x0]
ic_label_close:
 mov x0,x19
 mov x8,#57
 svc #0
ic_label_done:
 mov x0,x20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#192
 ret
ic_label_path: .asciz "/run/x6-interval-controls/label"
ic_label_default: .asciz "Minimum (not ready)"

.balign 16
.global tl_click,tl_read_state,tl_request
tl_click:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 mov x19,x0
 ldrb w0,[x19,#0xb88]
 bl tl_request
 cbz w0,tl_click_done
 bl tl_read_state
 mov w1,w0
 mov x0,x19
 bl switch_checked
tl_click_done:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
tl_read_state:
 bti c
 adr x0,tl_state
 mov w1,#-1
 mov w2,#1
 b ic_io
tl_request:
 bti c
 mov w1,w0
 adr x0,tl_request_path
 mov w2,#1
 b ic_io
tl_title: .asciz "16K Timelapse 245 Mbps/eye"
tl_state: .asciz "/run/x6-interval-controls/tl-state"
tl_request_path: .asciz "/run/x6-interval-controls/tl-request"

.balign 16
.global f8k_click,f8k_read_state,f8k_request
f8k_click:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 mov x19,x0
 ldrb w0,[x19,#0xb88]
 bl f8k_request
 cbz w0,f8k_click_done
 bl f8k_read_state
 mov w1,w0
 mov x0,x19
 bl switch_checked
f8k_click_done:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
f8k_read_state:
 bti c
 adr x0,f8k_state
 mov w1,#-1
 mov w2,#1
 b ic_io
f8k_request:
 bti c
 mov w1,w0
 adr x0,f8k_request_path
 mov w2,#1
 b ic_io
f8k_title: .asciz "8K Fast RAW (no processing)"
f8k_state: .asciz "/run/x6-interval-controls/fast8k-state"
f8k_request_path: .asciz "/run/x6-interval-controls/fast8k-request"

// 16K Fast RAW (Interval Settings row 4, 2026-11-03). Frames are saved as unprocessed
// passthrough DNGs and converted later on the camera (pureshot-batch-v1), hence the title.
// The row is built by the out-of-line tail of ic_open below instead of inline: vr_open
// (before ic_open) reaches ic_title, tl_title, tl_read_state and tl_click with PC-relative
// adr/bl, so ic_open and everything up to tl_* must keep their size and addresses. That keeps
// vr_open byte-identical to the 9aa874b8 build, so the video-NR retire word (file offset
// 0x715c304) still lands where it did. The tail runs in ic_open's frame: x19 = Interval
// Settings page; x0:x1 = the 8K row's new_slot result and x9 = its slot address.
.balign 16
ic_open_f16k:
 stp x0,x1,[x9]
 adrp x0,switch_class
 add x0,x0,:lo12:switch_class
 mov x1,#0
 bl new_object
 cbz x0,ic_open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adr x0,f16k_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl switch_title
 bl f16k_read_state
 mov w1,w0
 mov x0,x20
 bl switch_checked
 mov x0,x20
 adr x1,f16k_click
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
 b ic_open_done

.balign 16
.global f16k_click,f16k_read_state,f16k_request
f16k_click:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 mov x19,x0
 ldrb w0,[x19,#0xb88]
 bl f16k_request
 cbz w0,f16k_click_done
 bl f16k_read_state
 mov w1,w0
 mov x0,x19
 bl switch_checked
f16k_click_done:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
f16k_read_state:
 bti c
 adr x0,f16k_state
 mov w1,#-1
 mov w2,#1
 b ic_io
f16k_request:
 bti c
 mov w1,w0
 adr x0,f16k_request_path
 mov w2,#1
 b ic_io
f16k_title: .asciz "16K Fast RAW (needs in-camera post)"
f16k_state: .asciz "/run/x6-interval-controls/fast16k-state"
f16k_request_path: .asciz "/run/x6-interval-controls/fast16k-request"

// ============================================================================
// PureShot post-processing page (2026-11-03). Root row added to Mod Settings via
// the ic_open_f16k tail-detour precedent; the page lists 120 MP passthrough groups
// read from /run/x6-pureshot/groups (daemon x6-pureshot-processor), lets the owner
// select groups, start/stop, and shows a live status row. All open flags are the
// on-device aarch64 values the vr_*/f16k_* code uses (0xa0000 read / 0xa00c1 write);
// every indirect-call entry starts with `bti c`; the tick reuses the one stats_timers
// trampoline. pp_globals is a window in the zero-init RW tail (added to compile_menu).
// ============================================================================
.equ PP_THROTTLE,0x00     // u64 last tick second (1 Hz throttle)
.equ PP_HB,0x08           // u32 1 = we wrote the page-open heartbeat file since we last removed it
.equ PP_PENDS,0x18        // u64 pending-since second (0 = no request in flight)
.equ PP_REQBASE,0x20      // u8  groups req_seq baseline at submit
.equ PP_SLEN,0x28         // u32 bytes of the last good status read (bounds the extra NUL-separated lines)
// Older groups (owner 2026-11-04 "keep load-more, may need > 8 groups"): each 8-group window is its OWN sub-page,
// pushed from the one above by "Show older groups" and left with Back. PP_STACK[d] = the page showing groups
// 8d+1..8d+8 (compare-only: never dereferenced unless it is the focused page), PP_GENS[d] = the groups gen pinned
// when it opened. Depth d is also the window: offset 8d into /run/x6-pureshot/groups-all.
.equ PP_STACK,0x30        // 16 x u64 page pointers (0 = none)
.equ PP_GENS,0xb0         // 16 x u32 pinned gens
.equ PP_DEPTHS,16
.equ PP_STATUS,0x100      // 256-byte status record buffer
.equ PP_GROUPS,0x200      // 544-byte window buffer: 32-byte header + 8 records (ends +0x420)
.equ PP_TITLE,0x420       // 32-byte sub-page title ("Groups 9-16"); ival_globals starts at +0x510

.balign 16
.global vr_open_pp
// Runs in vr_open's 48-byte frame: x19 = Mod Settings page, x0:x1 = tl row's new_slot
// result, x9 = tl row+0x4c0. No `bti c` (reached by direct `b`, like ic_open_f16k).
vr_open_pp:
 stp x0,x1,[x9]                 // displaced store: close the 16K Timelapse row
 adrp x0,arrow_class
 add x0,x0,:lo12:arrow_class
 mov x1,#0
 bl new_object
 cbz x0,open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adr x0,pp_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl arrow_title
 mov x0,x19                     // ARROW row: slot owner = the page
 adr x1,pp_open
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
 b open_done

.balign 16
.global pp_open
// --- PureShot page v2 (owner UI feedback 2026-11-04): a plain status line (label+value row, no arrow), real
// action buttons (Start/Stop follows the daemon state; Select all pending), and the groups as check-mark rows
// (multi-select; a completed group is greyed out and labelled "done" by the daemon). Every row still keeps its
// tap slot at +0x4c0/+0x4c8 (SetEntryBgBase), and pp_paint/pp_tick find rows ONLY by that callback (ABA-safe). ---
pp_open:
 bti c
 mov w1,#0                      // from Mod Settings: the newest groups (depth 0)
 b pp_open_at
// --- x0 = the page to push from (Mod Settings, or the pp page one level up), w1 = depth d (groups 8d+1..8d+8) ---
.balign 16
.global pp_open_at
pp_open_at:
 bti c
 stp x29,x30,[sp,#-80]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 stp x23,x24,[sp,#48]
 stp x25,x26,[sp,#64]
 mov x19,x0
 mov w25,w1                     // depth (callee-saved)
 cmp w25,#PP_DEPTHS
 b.hs pp_open_done
 ldr x9,[x19,#0xa8]
 cbnz x9,pp_open_done           // reopen guard
 adrp x1,page_class
 add x1,x1,:lo12:page_class
 mov w2,#0
 mov x3,#0
 mov x4,#0
 mov x5,#0
 mov x6,#0
 bl create_subpage
 ldr x19,[x19,#0xa8]
 cbz x19,pp_open_done
 adrp x9,page_class
 add x9,x9,:lo12:page_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne pp_open_done
 adrp x23,pp_globals
 add x23,x23,:lo12:pp_globals
 add x9,x23,#PP_STACK
 str x19,[x9,w25,uxtw #3]       // this page now owns depth d ...
 add w10,w25,#1
pp_open_forget:
 cmp w10,#PP_DEPTHS
 b.hs pp_open_forgot
 str xzr,[x9,w10,uxtw #3]       // ... and any deeper entry belonged to a page that is gone
 add w10,w10,#1
 b pp_open_forget
pp_open_forgot:
 str xzr,[x23,#PP_PENDS]
 lsl w0,w25,#3
 bl pp_read_groups              // this page's window (zeros the header on failure -> count 0)
 ldr w9,[x23,#(PP_GROUPS+4)]
 add x10,x23,#PP_GENS
 str w9,[x10,w25,uxtw #2]       // pin the gen this page was built from
 ldrb w26,[x23,#(PP_GROUPS+9)]
 and w26,w26,#1                 // GF_MORE: another window follows ...
 add w9,w25,#1
 cmp w9,#PP_DEPTHS
 csel w26,w26,wzr,lo            // ... and there is a depth left to show it (w26 callee-saved)
 mov w0,w25
 bl pp_page_name                // "16K Fast RAW" on top; "Groups 9-16" further down
 bl new_string
 mov x1,x0
 add x0,x19,#0x338
 bl page_title
 // --- status: label+value row "Status" / daemon text; no arrow. Slot pp_refresh: a tap rescans/acks. ---
 adrp x0,dualtext_class
 add x0,x0,:lo12:dualtext_class
 mov x1,#0
 bl new_object
 cbz x0,pp_open_done
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adr x0,pp_status_caption
 bl new_string
 mov x1,x0
 mov x0,x20
 bl dualtext_title
 bl pp_status_label
 bl new_string
 mov x1,x0
 mov x0,x20
 bl dualtext_desc
 mov x0,x20
 mov w1,#2                      // SetEntryBgBase.Type 2 = top of the status card (Status / Progress / Time left)
 bl entry_type
 mov x0,x19
 adr x1,pp_refresh
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
 // --- owner UI feedback (2026-11-04, "status bigger; frame %, total % and time left"): two more label+value rows
 // in the same card; values = the daemon's extra status lines; tap = refresh, like the status row ---
 adr x0,pp_prog_caption
 mov w1,#1                      // status line 1 = "Total 23%, frame 45%"
 adr x2,pp_prog_row
 mov w3,#3                      // middle of the card
 mov x4,x19
 bl pp_add_line_row
 cbz x0,pp_open_paint
 adr x0,pp_eta_caption
 mov w1,#2                      // status line 2 = "about 2 min 40 s"
 adr x2,pp_eta_row
 mov w3,#4                      // bottom of the card
 mov x4,x19
 bl pp_add_line_row
 cbz x0,pp_open_paint
 mov w1,#16                     // 16 px gap below the card (the About page's last row uses the same 16)
 bl entry_pad_bottom
 // --- v3 (owner, 2026-11-04 "still not buttons"): the two actions are NATIVE settings-list buttons --
 // SystemSet::SettingListButton, the About page's full-width "Export LOG" pill. Width = the page's own width
 // (Bounds +0x50/+0x58), falling back to 480 (portrait) when that is not plausible. ---
 ldr w9,[x19,#0x50]             // Bounds.Point1.X
 ldr w10,[x19,#0x58]            // Bounds.Point2.X
 sub w21,w10,w9
 cmp w21,#240
 b.lt pp_open_w480
 cmp w21,#1024
 b.le pp_open_wok
pp_open_w480:
 mov w21,#480
pp_open_wok:
 // Start/Stop: one button; label + look follow the daemon state (Start = brand light, Stop = white/red);
 // pp_paint keeps both current
 bl pp_startstop_label          // x0 = label, w1 = button type
 mov w2,w1
 mov x1,x0
 mov x0,x19
 adr x3,pp_startstop
 mov w4,w21
 bl pp_add_button
 cbz x0,pp_open_paint
 // Select all pending: the secondary (dark) native button
 mov x0,x19
 adr x1,pp_selall_title
 mov w2,#1
 adr x3,pp_selall
 mov w4,w21
 bl pp_add_button
 cbz x0,pp_open_paint
 // --- N group rows: check-mark rows (slots pp_pick0..7, owner = the row itself) ---
 ldrb w22,[x23,#(PP_GROUPS+8)]  // count
 cmp w22,#8
 csel w22,w22,wzr,ls            // invalid count -> 0 rows (never build > 8 or garbage)
 mov w21,#0
pp_row_loop:
 cmp w21,w22
 b.hs pp_rows_done
 adrp x0,checked_class
 add x0,x0,:lo12:checked_class
 mov x1,#0
 bl new_object
 cbz x0,pp_open_paint
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 add x24,x23,#(PP_GROUPS+32)
 add x24,x24,x21,lsl #6         // x24 = this row's record (callee-saved: must survive the bl calls)
 add x0,x24,#16                 // title = record+16 (daemon text: date/time, count, progress or "done")
 bl new_string
 mov x1,x0
 mov x0,x20
 bl checked_title
 ldrb w9,[x24,#10]              // record flags
 and w1,w9,#1                   // RF_SELECTED -> check mark ...
 tst w9,#4
 csel w1,wzr,w1,ne              // ... never on a completed group (nothing left to process)
 mov x0,x20
 bl radio_selected              // SetEntrySelectable.OnSetSelected
 ldrb w1,[x24,#10]
 ubfx w1,w1,#2,#1               // RF_COMPLETE -> greyed out
 mov x0,x20
 bl checked_disable
 mov w0,w21                     // card position: the group rows (+ the older-groups row) form one card
 add w1,w22,w26                 // w26 = 1 when the "Show older groups" row closes the card
 bl pp_card_type
 mov w1,w0
 mov x0,x20
 bl entry_type
 mov x0,x20                     // slot owner = the row: pp_pick flips that row's own check mark
 adr x1,pp_pick0
 add x1,x1,x21,lsl #4
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
 add w21,w21,#1
 b pp_row_loop
pp_rows_done:
 // --- older-groups arrow (slot pp_page, owner = this page): pushes the next window as a page of its own ---
 cbz w26,pp_open_paint
 adrp x0,arrow_class
 add x0,x0,:lo12:arrow_class
 mov x1,#0
 bl new_object
 cbz x0,pp_open_paint
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 adr x0,pp_older_title
 bl new_string
 mov x1,x0
 mov x0,x20
 bl arrow_title
 mov w0,w22                     // the last row of the groups card
 add w1,w22,#1
 bl pp_card_type
 mov w1,w0
 mov x0,x20
 bl entry_type
 mov x0,x19
 adr x1,pp_page
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
pp_open_paint:
 mov x0,x19
 bl pp_paint                    // paint once
pp_open_done:
 ldp x25,x26,[sp,#64]
 ldp x23,x24,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#80
 ret

// --- pick stubs: 16-byte aligned, index in w1, common handler pp_pick ---
.balign 16
.global pp_pick0,pp_pick1,pp_pick2,pp_pick3,pp_pick4,pp_pick5,pp_pick6,pp_pick7,pp_pick
pp_pick0:
 bti c
 mov w1,#0
 b pp_pick
.balign 16
pp_pick1:
 bti c
 mov w1,#1
 b pp_pick
.balign 16
pp_pick2:
 bti c
 mov w1,#2
 b pp_pick
.balign 16
pp_pick3:
 bti c
 mov w1,#3
 b pp_pick
.balign 16
pp_pick4:
 bti c
 mov w1,#4
 b pp_pick
.balign 16
pp_pick5:
 bti c
 mov w1,#5
 b pp_pick
.balign 16
pp_pick6:
 bti c
 mov w1,#6
 b pp_pick
.balign 16
pp_pick7:
 bti c
 mov w1,#7
 b pp_pick
// x0 = the check-mark row (slot owner), w1 = index. A tap on SetEntryTextChecked only signals this slot (the class
// does not override privateOnClickBackground), so WE flip the mark: read Selected, set the opposite, then request.
pp_pick:
 bti c
 stp x29,x30,[sp,#-80]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 str x23,[sp,#48]
 mov x19,x0                     // the row
 mov w20,w1                     // index
 adrp x21,pp_globals
 add x21,x21,:lo12:pp_globals
 // the tap is on the focused page: find its depth and reload THAT window (PP_GROUPS may still hold another page's,
 // e.g. right after Back from an older-groups page), then require the gen that page was built from
 bl pp_focused                  // x0 = our focused page or 0, w1 = its depth
 cbz x0,pp_pick_epi
 str w1,[sp,#56]
 lsl w0,w1,#3
 bl pp_read_groups
 ldr w1,[sp,#56]
 add x9,x21,#PP_GENS
 ldr w10,[x9,w1,uxtw #2]        // page gen pinned at open
 ldr w9,[x21,#(PP_GROUPS+4)]    // current groups gen
 cmp w9,w10
 b.ne pp_pick_epi               // list changed under the tap -> ignore it (ABA-safe); the status line says so
 ldrb w9,[x21,#(PP_GROUPS+8)]
 cmp w20,w9
 b.hs pp_pick_epi               // no such record in this window
 add x22,x21,#(PP_GROUPS+32)
 add x22,x22,x20,lsl #6         // record
 ldrb w9,[x22,#10]
 tbnz w9,#2,pp_pick_epi         // RF_COMPLETE: nothing left to process -> tap ignored (row is greyed out)
 ldrb w23,[x19,#0x5bc]          // current check mark (SetEntrySelectable.Selected)
 eor w23,w23,#1                 // the new state (callee-saved: survives the calls)
 mov x0,x19
 mov w1,w23
 bl radio_selected              // show it at once (optimistic; pp_tick reconciles with the daemon)
 cmp w23,#0
 mov w0,#1                      // OP_SELECT
 mov w10,#2                     // OP_DESELECT
 csel w0,w0,w10,ne
 mov w1,w20                     // arg = index
 ldr w2,[x21,#(PP_GROUPS+4)]    // gen_lo (== this page's pinned gen, checked above)
 ldr w3,[x22]                   // group id
 bl pp_request
 // mark pending (optimistic); DO NOT reconcile on failure/EEXIST -> leave to pp_tick (GUI-F2)
 mov x0,#1
 add x1,sp,#64
 mov x8,#113
 svc #0
 ldr x9,[sp,#64]
 str x9,[x21,#PP_PENDS]
 ldrb w9,[x21,#(PP_GROUPS+10)]
 strb w9,[x21,#PP_REQBASE]
pp_pick_epi:
 ldr x23,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#80
 ret

// --- w0 = daemon state byte from /run/x6-pureshot/status, or 0xff when it cannot be read ---
.balign 16
.global pp_state
pp_state:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 adrp x19,pp_globals
 add x19,x19,:lo12:pp_globals
 bl pp_status_label             // refreshes PP_STATUS; returns PP_STATUS+16 only on a good read
 add x9,x19,#PP_STATUS
 add x9,x9,#16
 cmp x0,x9
 mov w0,#0xff
 b.ne pp_state_ret
 ldrb w0,[x19,#(PP_STATUS+12)]  // STATUS_HDR '<QIBBBB': state at +12
pp_state_ret:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret

// --- x0 = the Start/Stop button label for the current state, w1 = its native button type: running/paused/
// stopping -> "Stop processing" on type 2 (white, red text); otherwise "Start processing" on type 0 (brand light) ---
.balign 16
.global pp_startstop_label
pp_startstop_label:
 bti c
 stp x29,x30,[sp,#-16]!
 mov x29,sp
 bl pp_state
 sub w0,w0,#4                   // ST_RUNNING 4, ST_PAUSED 5, ST_STOPPING 6
 cmp w0,#2
 adr x0,pp_start_title
 adr x1,pp_stop_title
 csel x0,x1,x0,ls
 cset w1,ls
 lsl w1,w1,#1                   // 2 while stoppable, else 0
 ldp x29,x30,[sp],#16
 ret

// --- one native settings-list button (SystemSet::SettingListButton = the About page's "Export LOG" pill): embedded
// in the page's list flow like the rows, full width, 16 px gap below. x0 = page, x1 = C title, w2 = button type
// (0 brand light / 1 dark / 2 white+red), x3 = OnClick callback (slot owner = the page), w4 = width.
// Returns x0 = the button or 0. The OnClick slot sits at button+0x2e8 (where the About page stores its own). ---
.balign 16
.global pp_add_button
pp_add_button:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 stp x23,x24,[sp,#48]
 mov x19,x0                     // page
 mov x21,x1                     // title
 mov w22,w2                     // type
 mov x23,x3                     // callback
 mov w24,w4                     // width
 adrp x0,listbutton_class
 add x0,x0,:lo12:listbutton_class
 mov x1,#0
 bl new_object
 cbz x0,pp_add_button_ret
 mov x20,x0
 mov w1,#1
 bl set_embedded                // arranged by the page's list like the rows (the About page does this first)
 mov x0,x19
 mov x1,x20
 mov w2,#0
 bl group_add
 mov x0,x21
 bl new_string
 mov x1,x0
 mov x0,x20
 bl button_text
 mov x0,x20
 mov w1,w22
 bl button_type
 mov w1,w24
 orr x1,x1,x1,lsl #32           // ButtonWidthRange (min, max) = (width, width): full width, like the About page
 mov x0,x20
 bl button_width
 mov x0,x20
 mov x1,#16                     // ContentPadding left 16, top 0 (the class default) ...
 mov x2,#16
 orr x2,x2,x2,lsl #32           // ... right 16, bottom 16: the gap below the pill
 bl content_padding
 mov x0,x19
 mov x1,x23
 bl new_slot
 add x9,x20,#0x2e8              // ButtonBase.OnClick
 stp x0,x1,[x9]
 mov x0,x20
pp_add_button_ret:
 ldp x23,x24,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret

// --- one label+value row (Widgets::SetEntryDualText) of the status card: x0 = C caption, w1 = status line index
// (pp_status_line), x2 = tap slot callback (owner = the page; pp_paint finds the row ONLY by it), w3 = card type,
// x4 = page. Returns x0 = the row or 0. ---
.balign 16
.global pp_add_line_row
pp_add_line_row:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 stp x23,x24,[sp,#48]
 mov x21,x0                     // caption
 mov w22,w1                     // status line index
 mov x23,x2                     // callback
 mov w24,w3                     // card type
 mov x19,x4                     // page
 adrp x0,dualtext_class
 add x0,x0,:lo12:dualtext_class
 mov x1,#0
 bl new_object
 cbz x0,pp_alr_ret
 mov x20,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 mov x0,x21
 bl new_string
 mov x1,x0
 mov x0,x20
 bl dualtext_title
 mov w0,w22
 bl pp_status_line
 bl new_string
 mov x1,x0
 mov x0,x20
 bl dualtext_desc
 mov x0,x20
 mov w1,w24
 bl entry_type
 mov x0,x19
 mov x1,x23
 bl new_slot
 add x9,x20,#0x4c0
 stp x0,x1,[x9]
 mov x0,x20
pp_alr_ret:
 ldp x23,x24,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret

// --- w0 = the SetEntryBgBase.Type for card position w0 of w1 rows: 1 alone, 2 top, 3 middle, 4 bottom (leaf) ---
.balign 16
.global pp_card_type
pp_card_type:
 cmp w1,#1
 b.ls pp_card_single
 cbz w0,pp_card_top
 sub w9,w1,#1
 cmp w0,w9
 b.hs pp_card_bottom
 mov w0,#3
 ret
pp_card_single:
 mov w0,#1
 ret
pp_card_top:
 mov w0,#2
 ret
pp_card_bottom:
 mov w0,#4
 ret

// --- the one Start/Stop button (x0 = page): START when idle/ready/done, STOP while running/paused/stopping ---
.balign 16
.global pp_startstop
pp_startstop:
 bti c
 stp x29,x30,[sp,#-16]!
 mov x29,sp
 bl pp_state
 sub w0,w0,#4
 cmp w0,#2
 mov w0,#0x10                   // OP_START
 mov w1,#0x11                   // OP_STOP
 csel w0,w1,w0,ls
 ldp x29,x30,[sp],#16
 b pp_action

// --- shared paint/reconcile walk; x0 = page ---
.balign 16
.global pp_paint
pp_paint:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 stp x23,x24,[sp,#48]           // x24 = record ptr; it must survive the bl calls below (AAPCS64 callee-saved)
 mov x22,x0                     // page
 adrp x23,pp_globals
 add x23,x23,:lo12:pp_globals
 bl pp_lookup                   // w0 = this page's depth (the caller loaded its window into PP_GROUPS)
 tbnz w0,#31,pp_paint_done      // not one of our pages: touch nothing
 add x9,x23,#PP_GENS
 ldr w10,[x9,w0,uxtw #2]        // page gen pinned at open
 ldr w9,[x23,#(PP_GROUPS+4)]    // current groups gen
 cmp w9,w10
 cset w21,eq                    // w21 = 1 if gens match
 ldr x19,[x22,#0x60]            // first child
 mov w20,#0                     // walk guard
pp_paint_loop:
 cbz x19,pp_paint_done
 add w20,w20,#1
 cmp w20,#128
 b.hi pp_paint_done
 ldr x9,[x19,#0x2f0]            // a native button's OnClick {owner +0x2e8, callback +0x2f0} identifies Start/Stop
 adr x10,pp_startstop
 cmp x9,x10
 b.eq pp_paint_startstop
 ldr x9,[x19,#0x4c8]            // slot callback identifies the row (never position or class: ABA-safe)
 adr x10,pp_refresh
 cmp x9,x10
 b.eq pp_paint_status
 adr x10,pp_prog_row
 cmp x9,x10
 b.eq pp_paint_prog
 adr x10,pp_eta_row
 cmp x9,x10
 b.eq pp_paint_eta
 adr x10,pp_pick0
 sub x11,x9,x10
 cmp x11,#112                   // 7*16: valid pick-callback range for 8 picks
 b.hi pp_paint_next
 tst x11,#15
 b.ne pp_paint_next
 cbz w21,pp_paint_next          // gen mismatch -> don't reconcile picks
 ldr x9,[x23,#PP_PENDS]
 cbnz x9,pp_paint_next          // request in flight -> leave optimistic state
 lsr x11,x11,#4                 // index
 ldrb w12,[x23,#(PP_GROUPS+8)]
 cmp x11,x12
 b.hs pp_paint_next             // no such record in this window
 add x24,x23,#(PP_GROUPS+32)
 add x24,x24,x11,lsl #6         // record (x24: x9-x15 are destroyed by the calls -- the x11 version crashed)
 add x0,x24,#16
 bl new_string
 mov x1,x0
 mov x0,x19
 bl checked_title
 ldrb w9,[x24,#10]
 and w1,w9,#1
 tst w9,#4
 csel w1,wzr,w1,ne              // a completed group never shows the check mark
 mov x0,x19
 bl radio_selected
 ldrb w1,[x24,#10]
 ubfx w1,w1,#2,#1
 mov x0,x19
 bl checked_disable
 b pp_paint_next
pp_paint_status:
 cbz w21,pp_paint_changed
 bl pp_status_label
 b pp_paint_status_set
pp_paint_changed:
 adr x0,pp_changed_title
pp_paint_status_set:
 bl new_string
 mov x1,x0
 mov x0,x19
 bl dualtext_desc
 b pp_paint_next
pp_paint_prog:
 mov w0,#1                      // "Total 23%, frame 45%"
 b pp_paint_line
pp_paint_eta:
 mov w0,#2                      // "about 2 min 40 s"
pp_paint_line:
 bl pp_status_line
 bl new_string
 mov x1,x0
 mov x0,x19
 bl dualtext_desc
 b pp_paint_next
pp_paint_startstop:
 bl pp_startstop_label          // x0 = label, w1 = button type
 mov w24,w1                     // x24 is callee-saved (saved above): the type survives new_string
 bl new_string
 mov x1,x0
 mov x0,x19
 bl button_text
 mov x0,x19
 mov w1,w24
 bl button_type                 // Start: brand light; Stop: white with red text (no-op when unchanged)
pp_paint_next:
 ldr x19,[x19,#0x20]            // next sibling
 b pp_paint_loop
pp_paint_done:
 ldp x23,x24,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret

// --- 1 Hz tick from stats_timers; AAPCS-clean, preserves x19/x20, must not rely on x0 ---
.balign 16
.global pp_tick
pp_tick:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 adrp x19,pp_globals
 add x19,x19,:lo12:pp_globals
 mov x0,#1                      // CLOCK_MONOTONIC
 add x1,sp,#48
 mov x8,#113
 svc #0
 ldr x20,[sp,#48]              // seconds
 ldr x9,[x19,#PP_THROTTLE]
 cmp x20,x9
 b.eq pp_tick_done              // same second -> skip
 str x20,[x19,#PP_THROTTLE]
 bl pp_focused                  // x0 = our focused page or 0, w1 = its depth
 cbz x0,pp_tick_away
 mov x22,x0                     // our focused page
 mov w21,w1
 add x9,x19,#PP_STACK           // depth d is in front: every deeper page was left with Back -> forget it
 add w10,w21,#1
pp_tick_forget:
 cmp w10,#PP_DEPTHS
 b.hs pp_tick_beat
 str xzr,[x9,w10,uxtw #3]
 add w10,w10,#1
 b pp_tick_forget
pp_tick_beat:
 mov w0,w21
 mov x1,x20
 bl pp_beat                     // page-open heartbeat: the daemon pauses the live view while one of ours is open
 lsl w0,w21,#3
 bl pp_read_groups              // refresh this page's window
 // clear pending if 3 s elapsed or the daemon's req_seq advanced past our baseline
 ldr x9,[x19,#PP_PENDS]
 cbz x9,pp_tick_paint
 sub x10,x20,x9
 cmp x10,#3
 b.hs pp_tick_clear
 ldrb w10,[x19,#(PP_GROUPS+10)]
 ldrb w11,[x19,#PP_REQBASE]
 cmp w10,w11
 b.eq pp_tick_paint
pp_tick_clear:
 str xzr,[x19,#PP_PENDS]
pp_tick_paint:
 mov x0,x22
 bl pp_paint
 b pp_tick_done
pp_tick_away:
 bl pp_unbeat                   // none of our pages is in front (Back, a dialog, the live view): drop the heartbeat
pp_tick_done:
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret

// --- timer hook: the target of stats_timers' detoured `bl` (one word). Keeps stats_timers size-stable so no
// existing entry moves. Runs the original stats_tick, then the PureShot 1 Hz reconcile; x19 (the
// EwProcessTimers return value stats_timers holds) is callee-saved across both. Reached via a direct bl
// (no BTI landing pad needed). ---
.balign 16
.global pp_timer_hook
pp_timer_hook:
 stp x29,x30,[sp,#-16]!
 bl stats_tick
 bl pp_tick
 ldp x29,x30,[sp],#16
 ret

// --- action handlers (x0 = page). Build an 8-byte request and submit. ---
.balign 16
.global pp_refresh,pp_selall,pp_page,pp_start,pp_stop,pp_prog_row,pp_eta_row
pp_refresh:
 bti c
 mov w0,#0x20                   // OP_RESCAN_ACK
 b pp_action
// distinct tap callbacks for the Progress / Time left rows (pp_paint identifies rows by callback); a tap = refresh
.balign 16
pp_prog_row:
 bti c
 b pp_refresh
.balign 16
pp_eta_row:
 bti c
 b pp_refresh
.balign 16
pp_selall:
 bti c
 mov w0,#0x03                   // OP_SELECT_ALL
 b pp_action
// "Show older groups" (x0 = the page it sits on, the slot owner): push the next 8 groups as a page of their own;
// Back returns to this one. (It used to send OP_PAGE, which moved ONE shared window and stranded the page on it.)
.balign 16
pp_page:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 mov x19,x0
 bl pp_lookup                   // this page's depth
 tbnz w0,#31,pp_page_ret
 add w1,w0,#1                   // one level further down
 cmp w1,#PP_DEPTHS
 b.hs pp_page_ret
 mov x0,x19
 bl pp_open_at
pp_page_ret:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
.balign 16
pp_start:
 bti c
 mov w0,#0x10                   // OP_START
 b pp_action
.balign 16
pp_stop:
 bti c
 mov w0,#0x11                   // OP_STOP
 b pp_action
pp_action:
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 adrp x19,pp_globals
 add x19,x19,:lo12:pp_globals
 mov w1,#0                      // arg
 ldr w2,[x19,#(PP_GROUPS+4)]    // gen_lo (informational; the daemon does not act on it)
 mov w3,#0                      // id
 bl pp_request
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret

// --- w0 = window offset W (8 x depth): fill PP_GROUPS with the 32-byte header of /run/x6-pureshot/groups-all and
// records W..W+7 of it; the header's count becomes the records in THIS window and GF_MORE (flags bit 0) says that
// more follow. groups-all = 32 + 64 x count bytes, count <= 128 (16 pages). A daemon that predates it has only the
// 8-group file: the top page then reads that, without an older-groups row. Returns w0 = 1 ok / 0 fail (header
// zeroed -> count 0). ---
.balign 16
.global pp_read_groups
pp_read_groups:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 str x23,[sp,#48]
 adrp x19,pp_globals
 add x19,x19,:lo12:pp_globals
 mov w22,w0                     // window offset
 mov w20,#0                     // result
 mov x0,#-100
 adr x1,pp_groups_all_path
 mov w2,#0
 movk w2,#0xa,lsl #16           // 0xa0000 O_RDONLY|O_LARGEFILE|O_CLOEXEC (the value the vr_*/pp_* readers use)
 mov x8,#56
 svc #0
 tbnz x0,#63,pp_rg_old
 mov x21,x0                     // fd
 bl pp_fd_size                  // regular, single link, root-owned -> its size, else -1
 tbnz x0,#63,pp_rg_close
 mov x23,x0                     // size
 mov x0,x21
 add x1,x19,#PP_GROUPS
 mov x2,#32
 mov x8,#63                     // the header
 svc #0
 cmp x0,#32
 b.ne pp_rg_close
 ldr w9,[x19,#PP_GROUPS]
 movz w10,#0x5350
 movk w10,#0x4147,lsl #16       // 'PSGA' little-endian = 0x41475350
 cmp w9,w10
 b.ne pp_rg_close
 ldrb w11,[x19,#(PP_GROUPS+8)]  // records in the file
 cmp w11,#128
 b.hi pp_rg_close
 lsl x9,x11,#6
 add x9,x9,#32
 cmp x9,x23
 b.ne pp_rg_close               // exactly 32 + 64 x count, nothing more or less
 subs w12,w11,w22
 csel w12,w12,wzr,hi            // records from W on (0 when the list no longer reaches this window)
 mov w13,#8
 cmp w12,#8
 csel w23,w12,w13,ls            // ... at most 8 (x23 callee-saved: survives the read)
 cbz w23,pp_rg_window
 mov x0,x21
 add x1,x19,#(PP_GROUPS+32)
 lsl x2,x23,#6
 lsl x3,x22,#6
 add x3,x3,#32                  // file offset of record W
 mov x8,#67                     // pread64
 svc #0
 lsl x9,x23,#6
 cmp x0,x9
 b.ne pp_rg_close
pp_rg_window:
 ldrb w9,[x19,#(PP_GROUPS+8)]   // records in the file
 add w10,w22,#8
 cmp w9,w10
 cset w10,hi                    // more records after this window
 ldrb w11,[x19,#(PP_GROUPS+9)]
 and w11,w11,#0xfe
 orr w11,w11,w10
 strb w11,[x19,#(PP_GROUPS+9)]
 strb w23,[x19,#(PP_GROUPS+8)]  // count = this window's records
 mov w20,#1
 b pp_rg_close
pp_rg_old:
 cbnz w22,pp_rg_done            // older windows exist only in groups-all
 mov x0,#-100
 adr x1,pp_groups_path
 mov w2,#0
 movk w2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,pp_rg_done
 mov x21,x0
 bl pp_fd_size
 cmp x0,#544
 b.ne pp_rg_close
 mov x0,x21
 add x1,x19,#PP_GROUPS
 mov x2,#544
 mov x8,#63
 svc #0
 cmp x0,#544
 b.ne pp_rg_close
 ldr w9,[x19,#PP_GROUPS]        // magic
 movz w10,#0x5350
 movk w10,#0x3147,lsl #16       // 'PSG1' little-endian = 0x31475350
 cmp w9,w10
 b.ne pp_rg_close
 ldrb w9,[x19,#(PP_GROUPS+8)]
 cmp w9,#8
 b.hi pp_rg_close               // never more than one window's records
 ldrb w9,[x19,#(PP_GROUPS+9)]
 and w9,w9,#0xfe                // no older windows without groups-all
 strb w9,[x19,#(PP_GROUPS+9)]
 mov w20,#1
pp_rg_close:
 mov x0,x21
 mov x8,#57
 svc #0
pp_rg_done:
 cbnz w20,pp_rg_ret
 add x9,x19,#PP_GROUPS
 stp xzr,xzr,[x9]              // on failure: zero header (magic/gen/count/flags) -> count 0
pp_rg_ret:
 mov w0,w20
 ldr x23,[sp,#48]
 ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret

// --- x0 = fd -> x0 = its size when it is a regular, single-link, root-owned file without setuid/sticky/group-write
// bits, else -1 (fstat; leaf) ---
.balign 16
.global pp_fd_size
pp_fd_size:
 sub sp,sp,#128
 mov x1,sp
 mov x8,#80
 svc #0
 tbnz x0,#63,pp_fs_bad
 ldr w9,[sp,#16]                // st_mode
 mov w10,#0xf022
 and w9,w9,w10
 cmp w9,#0x8000
 b.ne pp_fs_bad
 ldr w9,[sp,#20]                // st_nlink
 cmp w9,#1
 b.ne pp_fs_bad
 ldr w9,[sp,#24]                // st_uid
 cbnz w9,pp_fs_bad
 ldr x0,[sp,#48]                // st_size
 add sp,sp,#128
 ret
pp_fs_bad:
 mov x0,#-1
 add sp,sp,#128
 ret

// --- w0 = the depth page x0 is registered at, or -1 (leaf: x9-x11) ---
.balign 16
.global pp_lookup
pp_lookup:
 bti c
 cbz x0,pp_lookup_miss
 adrp x9,pp_globals
 add x9,x9,:lo12:pp_globals
 add x9,x9,#PP_STACK
 mov w10,#0
pp_lookup_loop:
 ldr x11,[x9,w10,uxtw #3]
 cmp x11,x0
 b.eq pp_lookup_hit
 add w10,w10,#1
 cmp w10,#PP_DEPTHS
 b.lo pp_lookup_loop
pp_lookup_miss:
 mov w0,#-1
 ret
pp_lookup_hit:
 mov w0,w10
 ret

// --- x0 = the focused page when it is one of ours (else 0), w1 = its depth. Ours = page class, the status row (slot
// pp_refresh -- every pp page has one) among its children, found ONLY by that callback like pp_paint does, and
// registered in PP_STACK: a page of the same class that reuses a freed pp page's address (ABA) has other rows and is
// never taken for ours. ---
.balign 16
.global pp_focused
pp_focused:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 str x19,[sp,#16]
 adrp x0,stats_focus_holder
 add x0,x0,:lo12:stats_focus_holder
 adrp x1,stats_focus_class
 add x1,x1,:lo12:stats_focus_class
 bl get_auto
 cbz x0,pp_focused_no
 ldr x19,[x0,#0x28]             // focused page
 cbz x19,pp_focused_no
 adrp x9,page_class
 add x9,x9,:lo12:page_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne pp_focused_no
 ldr x9,[x19,#0x60]             // Native views precede the refresh row.
 mov w12,#0
 adr x11,pp_refresh
pp_focused_walk:
 cbz x9,pp_focused_no
 add w12,w12,#1
 cmp w12,#128
 b.hi pp_focused_no
 ldr x10,[x9,#0x4c8]
 cmp x10,x11
 b.eq pp_focused_found          // the status row (slot pp_refresh) is on this page
 ldr x9,[x9,#0x20]              // next sibling
 b pp_focused_walk
pp_focused_found:
 mov x0,x19
 bl pp_lookup
 tbnz w0,#31,pp_focused_no
 mov w1,w0
 mov x0,x19
 b pp_focused_ret
pp_focused_no:
 mov x0,#0
 mov w1,#0
pp_focused_ret:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret

// --- page-open heartbeat for the daemon (owner 2026-11-04: "stop the live view while in this page"). w0 = depth,
// x1 = CLOCK_MONOTONIC seconds. Writes 16 bytes {u64 seconds, 'PPGO', u32 depth} to page.next and renames it over
// /run/x6-pureshot/page; the daemon treats the page as open while that stamp is <= 3 s old. ---
.balign 16
.global pp_beat,pp_unbeat
pp_beat:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 str x1,[sp,#32]
 movz w9,#0x5050
 movk w9,#0x4f47,lsl #16        // 'PPGO' little-endian = 0x4f475050
 str w9,[sp,#40]
 str w0,[sp,#44]
 mov x0,#-100
 adr x1,pp_beat_stage
 mov w2,#0x8241
 movk w2,#0xa,lsl #16           // 0xa8241 O_WRONLY|O_CREAT|O_TRUNC|O_NOFOLLOW|O_LARGEFILE|O_CLOEXEC (aarch64)
 mov w3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,pp_beat_ret
 mov x19,x0
 add x1,sp,#32
 mov x2,#16
 mov x8,#64
 svc #0
 mov x20,x0
 mov x0,x19
 mov x8,#57
 svc #0
 cmp x20,#16
 b.ne pp_beat_ret
 mov x0,#-100
 adr x1,pp_beat_stage
 mov x2,#-100
 adr x3,pp_beat_path
 mov w4,#0                      // plain rename: replaces the previous stamp
 mov x8,#276
 svc #0
 cbnz w0,pp_beat_ret
 adrp x9,pp_globals
 add x9,x9,:lo12:pp_globals
 mov w10,#1
 str w10,[x9,#PP_HB]
pp_beat_ret:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
// --- remove the heartbeat once when none of our pages is in front any more (leaf) ---
pp_unbeat:
 bti c
 adrp x9,pp_globals
 add x9,x9,:lo12:pp_globals
 ldr w10,[x9,#PP_HB]
 cbz w10,pp_unbeat_ret
 str wzr,[x9,#PP_HB]
 mov x0,#-100
 adr x1,pp_beat_path
 mov w2,#0
 mov x8,#35                     // unlinkat
 svc #0
pp_unbeat_ret:
 ret

// --- x0 = the page title for depth w0: "16K Fast RAW" on top, "Groups 9-16" below (numbers from the window just
// read into PP_GROUPS), "Older groups" when that window came back empty ---
.balign 16
.global pp_page_name,pp_strcpy,pp_fmt_u3
pp_page_name:
 bti c
 adr x9,pp_page_title
 cbz w0,pp_pn_plain
 adrp x10,pp_globals
 add x10,x10,:lo12:pp_globals
 ldrb w11,[x10,#(PP_GROUPS+8)]
 adr x9,pp_older_name
 cbz w11,pp_pn_plain
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 lsl w19,w0,#3                  // window offset
 add w20,w19,w11                // number of the last group shown
 add x0,x10,#PP_TITLE
 adr x1,pp_groups_word
 bl pp_strcpy                   // "Groups "
 add w1,w19,#1
 bl pp_fmt_u3
 mov w9,#45
 strb w9,[x0],#1                // "-"
 mov w1,w20
 bl pp_fmt_u3
 adrp x0,pp_globals
 add x0,x0,:lo12:pp_globals
 add x0,x0,#PP_TITLE
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#32
 ret
pp_pn_plain:
 mov x0,x9
 ret
// --- copy the C string x1 to x0 (with its NUL); x0 = the copy's NUL (leaf) ---
pp_strcpy:
 ldrb w9,[x1],#1
 strb w9,[x0]
 cbz w9,pp_strcpy_end
 add x0,x0,#1
 b pp_strcpy
pp_strcpy_end:
 ret
// --- write w1 (0..999) in decimal at x0, no leading zeros, NUL after it; x0 = that NUL (leaf) ---
pp_fmt_u3:
 mov w9,#100
 udiv w10,w1,w9                 // hundreds
 msub w11,w10,w9,w1
 mov w12,#0
 cbz w10,pp_fmt_tens
 add w13,w10,#48
 strb w13,[x0],#1
 mov w12,#1
pp_fmt_tens:
 mov w9,#10
 udiv w10,w11,w9                // tens
 msub w11,w10,w9,w11            // ones
 orr w13,w12,w10
 cbz w13,pp_fmt_ones
 add w13,w10,#48
 strb w13,[x0],#1
pp_fmt_ones:
 add w13,w11,#48
 strb w13,[x0],#1
 strb wzr,[x0]
 ret

// --- read /run/x6-pureshot/status into PP_STATUS; returns x0 = NUL-terminated text ptr ---
.balign 16
.global pp_status_label
pp_status_label:
 bti c
 stp x29,x30,[sp,#-64]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 str x21,[sp,#32]
 sub sp,sp,#144
 adrp x19,pp_globals
 add x19,x19,:lo12:pp_globals
 mov x0,#-100
 adr x1,pp_status_path
 mov w2,#0
 movk w2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,pp_sl_default
 mov x21,x0
 mov x1,sp
 mov x8,#80
 svc #0
 tbnz x0,#63,pp_sl_close_default
 ldr w9,[sp,#16]
 mov w10,#0xf022
 and w9,w9,w10
 cmp w9,#0x8000
 b.ne pp_sl_close_default
 ldr w9,[sp,#20]
 cmp w9,#1
 b.ne pp_sl_close_default
 ldr w9,[sp,#24]
 cbnz w9,pp_sl_close_default
 ldr x9,[sp,#48]
 cmp x9,#18
 b.lo pp_sl_close_default
 cmp x9,#255
 b.hi pp_sl_close_default
 mov x0,x21
 add x1,x19,#PP_STATUS
 mov x2,#255
 mov x8,#63
 svc #0
 cmp x0,#18
 b.lo pp_sl_close_default
 add x9,x19,#PP_STATUS
 strb wzr,[x9,x0]               // NUL-terminate at bytes-read (bounded < 256)
 str w0,[x19,#PP_SLEN]          // the extra lines are only valid below this length (older reads leave stale bytes)
 mov x0,x21
 mov x8,#57
 svc #0
 add x0,x19,#PP_STATUS
 add x0,x0,#16                  // text begins after the 16-byte status header
 b pp_sl_done
pp_sl_close_default:
 mov x0,x21
 mov x8,#57
 svc #0
pp_sl_default:
 adr x0,pp_default_status
pp_sl_done:
 add sp,sp,#144
 ldr x21,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#64
 ret

// --- x0 = n (1 = Progress, 2 = Time left) -> x0 = that NUL-separated line after the main status text, read fresh;
// "-" when the status cannot be read or the daemon publishes fewer lines. Scans ONLY the bytes this read returned
// (PP_SLEN): the buffer is never cleared, so an older longer record leaves stale lines past the new terminator. ---
.balign 16
.global pp_status_line
pp_status_line:
 bti c
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov w19,w0                     // n (callee-saved: survives pp_status_label)
 bl pp_status_label             // refreshes PP_STATUS; returns PP_STATUS+16 only on a good read
 adrp x20,pp_globals
 add x20,x20,:lo12:pp_globals
 add x9,x20,#PP_STATUS
 add x10,x9,#16
 cmp x0,x10
 b.ne pp_line_dash              // read failed -> no extra lines
 ldr w11,[x20,#PP_SLEN]
 add x11,x9,x11                 // end = first byte past the read (holds the terminating NUL)
pp_line_skip:
 cmp x10,x11
 b.hs pp_line_dash
 ldrb w12,[x10],#1
 cbnz w12,pp_line_skip          // advance past this string's NUL
 subs w19,w19,#1
 b.ne pp_line_skip
 cmp x10,x11
 b.hs pp_line_dash              // the n-th line would start at/after the end -> absent
 mov x0,x10
 b pp_line_ret
pp_line_dash:
 adr x0,pp_dash
pp_line_ret:
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#32
 ret

// --- submit an 8-byte request: w0=op, w1=arg, w2=gen_lo, w3=id. Returns w0=0 ok / !=0 fail. ---
.balign 16
.global pp_request
pp_request:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 strb w0,[sp,#32]
 strb w1,[sp,#33]
 strb w2,[sp,#34]
 mov w9,#0x50                   // 'P' mark
 strb w9,[sp,#35]
 str w3,[sp,#36]                // id (LE u32)
 mov w20,#-1
 mov x0,#-100
 adr x1,pp_req_stage
 mov w2,#0xc1
 movk w2,#0xa,lsl #16           // 0xa00c1 O_WRONLY|O_CREAT|O_EXCL|O_NOFOLLOW|O_CLOEXEC
 mov w3,#0600
 mov x8,#56
 svc #0
 tbnz x0,#63,pp_req_done
 mov x19,x0
 add x1,sp,#32
 mov x2,#8
 mov x8,#64
 svc #0
 cmp x0,#8
 cset w20,ne
 mov x0,x19
 mov x8,#57
 svc #0
 cbnz w20,pp_req_unlink
 mov x0,#-100
 adr x1,pp_req_stage
 mov x2,#-100
 adr x3,pp_req_path
 mov w4,#1                      // RENAME_NOREPLACE
 mov x8,#276
 svc #0
 mov w20,w0
 cbz w20,pp_req_done
pp_req_unlink:
 mov x0,#-100
 adr x1,pp_req_stage
 mov w2,#0
 mov x8,#35
 svc #0
pp_req_done:
 mov w0,w20
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret

pp_title: .asciz "16K Fast RAW Processing"
pp_page_title: .asciz "16K Fast RAW"
pp_status_caption: .asciz "Status"
pp_prog_caption: .asciz "Progress"
pp_eta_caption: .asciz "Time left"
pp_dash: .asciz "-"
pp_selall_title: .asciz "Select all pending"
pp_older_title: .asciz "Show older groups"
pp_start_title: .asciz "Start processing"
pp_stop_title: .asciz "Stop processing"
pp_default_status: .asciz "Service not running"
pp_changed_title: .asciz "List changed - reopen page"
pp_groups_path: .asciz "/run/x6-pureshot/groups"
pp_groups_all_path: .asciz "/run/x6-pureshot/groups-all"
pp_beat_path: .asciz "/run/x6-pureshot/page"
pp_beat_stage: .asciz "/run/x6-pureshot/page.next"
pp_older_name: .asciz "Older groups"
pp_groups_word: .asciz "Groups "
pp_status_path: .asciz "/run/x6-pureshot/status"
pp_req_path: .asciz "/run/x6-pureshot/request"
pp_req_stage: .asciz "/run/x6-pureshot/request.next"

thermal_capture_page = stats_is_capture_page
#include "thermal_touch.inc"

#if defined(OPENX6_STATS_EXTENSION) || defined(OPENX6_CHOICES)
#include "extension_pages.inc"
#include "extension_labels.inc"
#endif
