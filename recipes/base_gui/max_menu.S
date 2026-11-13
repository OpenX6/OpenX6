.text
.global max_menu
max_menu:
    bti c
    paciasp
    stp x29,x30,[sp,#-32]!
    mov x29,sp
    str x19,[sp,#16]
    mov x19,x0
    // Preserve the original High string assignment at this call site.
    bl CommClassStringOptionList05_OnSetOption01
    // x26 is the bitrate integer vessel in the original metadata constructor.
    mov x0,x26
    mov w1,#2
    bl CommClassIntOptionList05_OnSetOption02
    adr x0,max_label
    bl EwNewStringAnsi
    mov x1,x0
    mov x0,x19
    bl CommClassStringOptionList05_OnSetOption02
    ldr x19,[sp,#16]
    ldp x29,x30,[sp],#32
    autiasp
    ret
max_label:
    .asciz "Max"
    .balign 4
