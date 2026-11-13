.text
.global owner_gui_start_shim
owner_gui_start_shim:
 bti c
 paciasp
 stp x29,x30,[sp,#-16]!
 mov x29,sp
 bl original_print
 mov x0,#0
 adr x1,start_name
 bl original_dlsym
 cbz x0,1f
 blr x0
1: ldp x29,x30,[sp],#16
 autiasp
 ret
start_name: .asciz "x6_owner_gui_start"

.balign 4
.global owner_gui_stop_shim
owner_gui_stop_shim:
 bti c
 paciasp
 stp x29,x30,[sp,#-16]!
 mov x29,sp
 mov x0,#0
 adr x1,stop_name
 bl original_dlsym
 cbz x0,2f
 blr x0
 cbz w0,2f
 /* Failed join: end this GUI process without freeing a live dispatcher. */
 mov w0,#70
 mov x8,#94
 svc #0
 brk #0
2: bl original_done
 ldp x29,x30,[sp],#16
 autiasp
 ret
stop_name: .asciz "x6_owner_gui_stop"
