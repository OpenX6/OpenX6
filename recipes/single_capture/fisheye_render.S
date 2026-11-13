.text
.global render_hook
render_hook:
 // The original entry signed the caller's link register.
 stp x0,x1,[sp,#-64]!
 stp x2,x3,[sp,#16]
 stp x4,x30,[sp,#32]
 mov x0,#-100
 adr x1,choice
 add x2,sp,#48
 mov x3,#1
 mov x8,#78
 svc #0
 cmp x0,#1
 b.ne fallback
 ldrb w9,[sp,#48]
 cmp w9,#70
 b.eq read_eye
 cmp w9,#76
 b.ne fallback
read_eye:
 mov x0,#-100
 adr x1,front
 mov x2,#0
 mov x8,#48
 svc #0
 cmp x0,#0
 cset w2,eq
 ldp x0,x1,[sp,#16]
 ldrb w9,[sp,#48]
 cmp w9,#76
 b.ne fisheye
 cbz x1,fallback
 ldr w9,[x1,#16]
 cmp w9,#3
 b.eq linear8
 bl linear_p010
 b rendered
linear8:
 bl linear_nv12
 b rendered
fisheye:
 bl framed_dispatch
rendered:
 cbz w0,fallback
 ldp x4,x30,[sp,#32]
 add sp,sp,#64
 mov w0,#0
 autiasp
 ret
fallback:
 ldp x4,x30,[sp,#32]
 ldp x2,x3,[sp,#16]
 ldp x0,x1,[sp],#64
 sub sp,sp,#0x160
 b original_render
choice: .asciz "/run/x6-nativefish/pano-choice"
front: .asciz "/run/x6-nativefish/pano-front"

.section .rodata
.balign 16
.global preview_linear_grid
.hidden preview_linear_grid
preview_linear_grid: .incbin "linear-grid.bin"
