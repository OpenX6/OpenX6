.text
.p2align 2
.global exposure_bridge
exposure_bridge:
    bti c
    paciasp
    sub sp, sp, #0xa0
    stp x29, x30, [sp, #0x20]
    stp x20, x21, [sp, #0x30]
    stp x22, x23, [sp, #0x40]
    str x24, [sp, #0x50]
    add x29, sp, #0x20
    // Forward the four native stack arguments without changing their layout.
    ldp x9, x10, [sp, #0xa0]
    ldp x11, x12, [sp, #0xb0]
    stp x9, x10, [sp]
    stp x11, x12, [sp, #0x10]
    mov x20, x6
    mov w21, w10
    ldr w22, [x19, #0xd64]
    ldr w23, [x1, #0x3c]
    mov w24, w2
    bl native_fill
    str w0, [sp, #0x60]
    cmp w0, #1
    b.ne done
    mov x0, x20
    mov w1, w21
    mov w2, w22
    mov w3, w23
    mov w4, w24
    bl fix_exposure
done:
    ldr w0, [sp, #0x60]
    ldr x24, [sp, #0x50]
    ldp x22, x23, [sp, #0x40]
    ldp x20, x21, [sp, #0x30]
    ldp x29, x30, [sp, #0x20]
    add sp, sp, #0xa0
    autiasp
    ret
