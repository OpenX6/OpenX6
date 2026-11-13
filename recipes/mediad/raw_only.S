.text
.balign 16
.global dng_mask_hook
dng_mask_hook:
 bti j
 and w20,w1,#0xff
 cmp w2,#1
 b.ne 1f
 mov w20,#1
 1: b mask_continue
.balign 16
.global dng_idle_hook
dng_idle_hook:
 bti j
 cmp w21,#1
 b.eq 2f
 b idle_continue
2:
 ldar w6,[x18]
 ubfx w21,w6,#3,#1
 b idle_epilogue
