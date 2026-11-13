.text
.global display_version
.type display_version, %function
display_version:
    paciasp
    stp x29, x30, [sp, #-32]!
    mov x29, sp
    str x19, [sp, #16]
    mov x19, x0
    adr x0, display_text
    bl EwNewStringAnsi
    mov x1, x0
    mov x2, #0x35e0
    add x0, x19, x2
    ldr x19, [sp, #16]
    ldp x29, x30, [sp], #32
    autiasp
    b WidgetsSetEntryDualText_OnSetStringDesc
    nop
display_text:
    .asciz "1.2.36 S"
    .balign 4
.size display_version, .-display_version
