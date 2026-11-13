// Capture profile selectors, frame-rate aliases and resolution captions.
.text
.global mode_extension
mode_extension:
    stp x19, x20, [sp, #-0x40]!
    stp x21, x22, [sp, #0x10]
    stp x23, x24, [sp, #0x20]
    str x30, [sp, #0x30]
    mov x21, x0
    mov w22, w1
    mov w23, w2
    bl capture_base+0x11a0
    mov x19, x0
    mov w24, #0
    cmp w22, #2
    b.ne mode_extension+0xc8
    cmp w23, #3
    b.ne mode_extension+0xc8
    mov x9, #0x8318
    add x9, x21, x9
    cmp x0, x9
    b.ne mode_extension+0xc8
    ldr x20, [x21, #0x8b8]
    cbz x20, mode_extension+0x80
    ldr x9, [x19]
    ldr x10, [x20]
    cmp x9, x10
    b.ne mode_extension+0x80
    ldr w10, [x20, #0x20]
    cmp w10, #7
    b.ne mode_extension+0x80
    ldr x10, [x20, #0x28]
    mov x11, #0x101010101010101
    and x11, x11, #0xffff00ffffffffff
    cmp x10, x11
    b.eq mode_extension+0xc0
    ldr x0, [x19]
    mov x1, #0
    bl native_4db3b0
    cbz x0, mode_extension+0xc8
    mov x20, x0
    ldp x9, x10, [x19, #0x28]
    str x9, [x20, #0x28]
    str x10, [x20, #0x30]
    ldr x9, [x19, #0x38]
    str x9, [x20, #0x38]
    mov w9, #7
    str w9, [x20, #0x20]
    mov x9, #0x101010101010101
    and x9, x9, #0xffff00ffffffffff
    str x9, [x20, #0x28]
    stur x9, [x20, #0x32]
    mov x19, x20
    mov w24, #1
    mov x0, x19
    ldr x30, [sp, #0x30]
    ldp x23, x24, [sp, #0x20]
    ldp x21, x22, [sp, #0x10]
    ldp x19, x20, [sp], #0x40
    ret
    cmp w5, #7
    b.eq mode_extension+0x100
    cbnz w5, mode_extension+0x10c
    mov x10, #0x8cf0
    add x0, x0, x10
    mov w10, #7
    mov w11, #3
    b mode_extension+0x16c
    mov x10, #0x8cf0
    add x0, x0, x10
    ret
    b capture_base+0x1be8
    cmp w2, #2
    b.ne mode_extension+0x168
    cmp w3, #3
    b.ne mode_extension+0x168
    cbnz w4, mode_extension+0x168
    cbnz w6, mode_extension+0x168
    cmp w1, #7
    b.eq mode_extension+0x158
    cbnz w1, mode_extension+0x168
    mov x10, #0x8a10
    add x0, x0, x10
    stp x29, x30, [sp, #-0x10]!
    mov w10, #0xf
    mov w11, #6
    bl mode_extension+0x16c
    ldp x29, x30, [sp], #0x10
    autiasp
    ret
    mov x10, #0x88a8
    add x0, x0, x10
    autiasp
    ret
    b capture_base+0x1c00
    stp x19, x20, [sp, #-0x20]!
    stp x21, x30, [sp, #0x10]
    mov x19, x0
    mov w20, w10
    mov w21, w11
    ldr x0, [x19]
    mov x1, xzr
    bl native_4db3b0
    cbz x0, mode_extension+0x1c0
    mov w9, #1
    str w9, [x0, #0x20]
    add x10, x0, #0x28
    mov w12, wzr
    strb wzr, [x10, w12, uxtw]
    add w12, w12, #1
    cmp w12, w20, lsl #1
    b.lo mode_extension+0x1a0
    strb w9, [x10, w21, uxtw]
    add x10, x10, w20, uxtw
    strb w9, [x10, w21, uxtw]
    b mode_extension+0x1c4
    mov x0, x19
    ldp x21, x30, [sp, #0x10]
    ldp x19, x20, [sp], #0x20
    ret
    stp x0, x1, [sp, #-0x20]!
    str x30, [sp, #0x10]
    bl mode_extension+0x2c4
    ldr x30, [sp, #0x10]
    ldp x0, x1, [sp], #0x20
    ldr x19, [x0, #0x68]
    b native_57b484
    stp x0, x1, [sp, #-0x20]!
    str x30, [sp, #0x10]
    bl mode_extension+0x2c4
    ldr x30, [sp, #0x10]
    ldp x0, x1, [sp], #0x20
    ldr x19, [x0, #0x60]
    b native_57b604
    ldr x9, [x0, #0x10]
    add x10, x9, #0x840
    cmp x10, x0
    b.ne mode_extension+0x284
    ldr x10, [x9]
    adrp x11, native_712a000
    add x11, x11, #0x650
    cmp x10, x11
    b.ne mode_extension+0x284
    ldr x12, [x0, #0x78]
    cbz x12, mode_extension+0x27c
    ldr x10, [x12, #0x10]
    cbnz x10, mode_extension+0x27c
    mov x11, #0x8318
    add x11, x9, x11
    ldr x14, [x11]
    ldr x15, [x12]
    cmp x14, x15
    b.ne mode_extension+0x27c
    ldr w14, [x12, #0x20]
    cmp w14, #7
    b.ne mode_extension+0x27c
    ldr x14, [x12, #0x28]
    mov x15, #0x101010101010101
    and x15, x15, #0xffff00ffffffffff
    cmp x14, x15
    b.ne mode_extension+0x27c
    mov w9, #2
    ret
    mov w9, #1
    ret
    mov w9, #0
    ret
    stp x1, x30, [sp, #-0x10]!
    bl mode_extension+0x208
    ldp x1, x30, [sp], #0x10
    cmp w9, #2
    b.ne mode_extension+0x2bc
    cmp w1, #7
    b.hs mode_extension+0x2bc
    adr x10, mode_extension+0x394
    ldrb w0, [x10, w1, uxtw]
    ldp x29, x30, [sp], #0x30
    autiasp
    ret
    ldr x12, [x0, #0x78]
    b native_57b250
    stp x19, x20, [sp, #-0x20]!
    str x30, [sp, #0x10]
    mov x19, x0
    bl mode_extension+0x208
    cbz w9, mode_extension+0x354
    mov w20, w9
    ldr x19, [x19, #0x68]
    cbz x19, mode_extension+0x354
    add x10, x19, #0x28
    adr x0, mode_extension+0x39f
    adr x1, mode_extension+0x39b
    cmp w20, #2
    csel x9, x1, x0, eq
    csel x0, x0, x1, eq
    bl mode_extension+0x360
    add x10, x19, #0x40
    adr x0, mode_extension+0x3aa
    adr x1, mode_extension+0x3a7
    cmp w20, #2
    csel x9, x1, x0, eq
    csel x0, x0, x1, eq
    bl mode_extension+0x360
    add x10, x19, #0x58
    adr x0, mode_extension+0x3b7
    adr x1, mode_extension+0x3b4
    cmp w20, #2
    csel x9, x1, x0, eq
    csel x0, x0, x1, eq
    bl mode_extension+0x360
    add x10, x19, #0x60
    adr x0, mode_extension+0x3c7
    adr x1, mode_extension+0x3c1
    cmp w20, #2
    csel x9, x1, x0, eq
    csel x0, x0, x1, eq
    bl mode_extension+0x360
    ldr x30, [sp, #0x10]
    ldp x19, x20, [sp], #0x20
    ret
    ldr x13, [x10]
    cbz x13, mode_extension+0x390
    ldrh w14, [x13], #2
    ldrb w15, [x9], #1
    cmp w14, w15
    b.ne mode_extension+0x390
    cbnz w15, mode_extension+0x368
    stp x10, x30, [sp, #-0x10]!
    bl native_4e3e80
    ldp x10, x30, [sp], #0x10
    cbz x0, mode_extension+0x390
    str x0, [x10]
    ret
    .byte 1
    .byte 3
    .byte 6
    .byte 2
    .byte 4
    .zero 1
    .byte 7
    .ascii "16K"
    .zero 1
    .ascii "6K Crop"
    .zero 1
    .ascii "5K"
    .zero 1
    .ascii "7.3K Full"
    .zero 1
    .ascii "3K"
    .zero 1
    .ascii "6.7K Full"
    .zero 1
    .ascii "1080P"
    .zero 1
    .ascii "5K Crop"
    .zero 2
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x7, #0x1558]
    cmp w16, #2
    b.ne mode_extension+0x4ec
    ldr w16, [x7, #0x15f0]
    cmp w16, #3
    b.ne mode_extension+0x4ec
    ldr w16, [x7, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x4ec
    ldr w16, [x7, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x4ec
    ldr w16, [x7, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x4ec
    ldr w16, [x7, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x434
    ldr w16, [x7, #0x1560]
    cmp w16, #0xa
    b.ne mode_extension+0x434
    mov w28, #2
    b mode_extension+0x4f8
    ldr w16, [x7, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x454
    ldr w16, [x7, #0x1560]
    cmp w16, #0xb
    b.ne mode_extension+0x454
    mov w28, #2
    b mode_extension+0x4f8
    ldr w16, [x7, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x474
    ldr w16, [x7, #0x1560]
    cmp w16, #0xc
    b.ne mode_extension+0x474
    mov w28, #2
    b mode_extension+0x4f8
    ldr w16, [x7, #0x155c]
    cmp w16, #0
    b.ne mode_extension+0x494
    ldr w16, [x7, #0x1560]
    cmp w16, #7
    b.ne mode_extension+0x494
    mov w28, #4
    b mode_extension+0x4f8
    ldr w16, [x7, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0x4c0
    ldr w16, [x7, #0x1560]
    cmp w16, #6
    b.ne mode_extension+0x4c0
    ldr w16, [x7, #0x1588]
    cmp w16, #3
    b.ne mode_extension+0x4c0
    mov w28, #4
    b mode_extension+0x4f8
    ldr w16, [x7, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0x4ec
    ldr w16, [x7, #0x1560]
    cmp w16, #5
    b.ne mode_extension+0x4ec
    ldr w16, [x7, #0x1588]
    cmp w16, #1
    b.ne mode_extension+0x4ec
    mov w28, #4
    b mode_extension+0x4f8
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b capture_base+0x1900
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_841ba8
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x7, #0x1558]
    cmp w16, #2
    b.ne mode_extension+0x620
    ldr w16, [x7, #0x15f0]
    cmp w16, #3
    b.ne mode_extension+0x620
    ldr w16, [x7, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x620
    ldr w16, [x7, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x620
    ldr w16, [x7, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x620
    ldr w16, [x7, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x568
    ldr w16, [x7, #0x1560]
    cmp w16, #0xa
    b.ne mode_extension+0x568
    mov w28, #2
    b mode_extension+0x62c
    ldr w16, [x7, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x588
    ldr w16, [x7, #0x1560]
    cmp w16, #0xb
    b.ne mode_extension+0x588
    mov w28, #2
    b mode_extension+0x62c
    ldr w16, [x7, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x5a8
    ldr w16, [x7, #0x1560]
    cmp w16, #0xc
    b.ne mode_extension+0x5a8
    mov w28, #2
    b mode_extension+0x62c
    ldr w16, [x7, #0x155c]
    cmp w16, #0
    b.ne mode_extension+0x5c8
    ldr w16, [x7, #0x1560]
    cmp w16, #7
    b.ne mode_extension+0x5c8
    mov w28, #4
    b mode_extension+0x62c
    ldr w16, [x7, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0x5f4
    ldr w16, [x7, #0x1560]
    cmp w16, #6
    b.ne mode_extension+0x5f4
    ldr w16, [x7, #0x1588]
    cmp w16, #3
    b.ne mode_extension+0x5f4
    mov w28, #4
    b mode_extension+0x62c
    ldr w16, [x7, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0x620
    ldr w16, [x7, #0x1560]
    cmp w16, #5
    b.ne mode_extension+0x620
    ldr w16, [x7, #0x1588]
    cmp w16, #1
    b.ne mode_extension+0x620
    mov w28, #4
    b mode_extension+0x62c
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b capture_base+0x1978
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_842b98
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x5, #0x1558]
    cmp w16, #2
    b.ne mode_extension+0x754
    ldr w16, [x5, #0x15f0]
    cmp w16, #3
    b.ne mode_extension+0x754
    ldr w16, [x5, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x754
    ldr w16, [x5, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x754
    ldr w16, [x5, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x754
    ldr w16, [x5, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x69c
    ldr w16, [x5, #0x1560]
    cmp w16, #0xa
    b.ne mode_extension+0x69c
    mov w25, #2
    b mode_extension+0x760
    ldr w16, [x5, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x6bc
    ldr w16, [x5, #0x1560]
    cmp w16, #0xb
    b.ne mode_extension+0x6bc
    mov w25, #2
    b mode_extension+0x760
    ldr w16, [x5, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x6dc
    ldr w16, [x5, #0x1560]
    cmp w16, #0xc
    b.ne mode_extension+0x6dc
    mov w25, #2
    b mode_extension+0x760
    ldr w16, [x5, #0x155c]
    cmp w16, #0
    b.ne mode_extension+0x6fc
    ldr w16, [x5, #0x1560]
    cmp w16, #7
    b.ne mode_extension+0x6fc
    mov w25, #4
    b mode_extension+0x760
    ldr w16, [x5, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0x728
    ldr w16, [x5, #0x1560]
    cmp w16, #6
    b.ne mode_extension+0x728
    ldr w16, [x5, #0x1588]
    cmp w16, #3
    b.ne mode_extension+0x728
    mov w25, #4
    b mode_extension+0x760
    ldr w16, [x5, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0x754
    ldr w16, [x5, #0x1560]
    cmp w16, #5
    b.ne mode_extension+0x754
    ldr w16, [x5, #0x1588]
    cmp w16, #1
    b.ne mode_extension+0x754
    mov w25, #4
    b mode_extension+0x760
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b capture_base+0x19f0
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_83b7b8
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x5, #0x1558]
    cmp w16, #2
    b.ne mode_extension+0x888
    ldr w16, [x5, #0x15f0]
    cmp w16, #3
    b.ne mode_extension+0x888
    ldr w16, [x5, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x888
    ldr w16, [x5, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x888
    ldr w16, [x5, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x888
    ldr w16, [x5, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x7d0
    ldr w16, [x5, #0x1560]
    cmp w16, #0xa
    b.ne mode_extension+0x7d0
    mov w25, #2
    b mode_extension+0x894
    ldr w16, [x5, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x7f0
    ldr w16, [x5, #0x1560]
    cmp w16, #0xb
    b.ne mode_extension+0x7f0
    mov w25, #2
    b mode_extension+0x894
    ldr w16, [x5, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x810
    ldr w16, [x5, #0x1560]
    cmp w16, #0xc
    b.ne mode_extension+0x810
    mov w25, #2
    b mode_extension+0x894
    ldr w16, [x5, #0x155c]
    cmp w16, #0
    b.ne mode_extension+0x830
    ldr w16, [x5, #0x1560]
    cmp w16, #7
    b.ne mode_extension+0x830
    mov w25, #4
    b mode_extension+0x894
    ldr w16, [x5, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0x85c
    ldr w16, [x5, #0x1560]
    cmp w16, #6
    b.ne mode_extension+0x85c
    ldr w16, [x5, #0x1588]
    cmp w16, #3
    b.ne mode_extension+0x85c
    mov w25, #4
    b mode_extension+0x894
    ldr w16, [x5, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0x888
    ldr w16, [x5, #0x1560]
    cmp w16, #5
    b.ne mode_extension+0x888
    ldr w16, [x5, #0x1588]
    cmp w16, #1
    b.ne mode_extension+0x888
    mov w25, #4
    b mode_extension+0x894
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b capture_base+0x1a68
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_83d118
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x8, #0x1558]
    cmp w16, #2
    b.ne mode_extension+0x9bc
    ldr w16, [x8, #0x15f0]
    cmp w16, #3
    b.ne mode_extension+0x9bc
    ldr w16, [x8, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x9bc
    ldr w16, [x8, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x9bc
    ldr w16, [x8, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x9bc
    ldr w16, [x8, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x904
    ldr w16, [x8, #0x1560]
    cmp w16, #0xa
    b.ne mode_extension+0x904
    mov w9, #0xa
    b mode_extension+0x9cc
    ldr w16, [x8, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x924
    ldr w16, [x8, #0x1560]
    cmp w16, #0xb
    b.ne mode_extension+0x924
    mov w9, #0xb
    b mode_extension+0x9cc
    ldr w16, [x8, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0x944
    ldr w16, [x8, #0x1560]
    cmp w16, #0xc
    b.ne mode_extension+0x944
    mov w9, #0xb
    b mode_extension+0x9cc
    ldr w16, [x8, #0x155c]
    cmp w16, #0
    b.ne mode_extension+0x964
    ldr w16, [x8, #0x1560]
    cmp w16, #7
    b.ne mode_extension+0x964
    mov w9, #7
    b mode_extension+0x9cc
    ldr w16, [x8, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0x990
    ldr w16, [x8, #0x1560]
    cmp w16, #6
    b.ne mode_extension+0x990
    ldr w16, [x8, #0x1588]
    cmp w16, #3
    b.ne mode_extension+0x990
    mov w9, #7
    b mode_extension+0x9cc
    ldr w16, [x8, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0x9bc
    ldr w16, [x8, #0x1560]
    cmp w16, #5
    b.ne mode_extension+0x9bc
    ldr w16, [x8, #0x1588]
    cmp w16, #1
    b.ne mode_extension+0x9bc
    mov w9, #7
    b mode_extension+0x9cc
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    ldr w9, [x8, #0x1560]
    b native_841bbc
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_841bbc
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x8, #0x1558]
    cmp w16, #2
    b.ne mode_extension+0xaf4
    ldr w16, [x8, #0x15f0]
    cmp w16, #3
    b.ne mode_extension+0xaf4
    ldr w16, [x8, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0xaf4
    ldr w16, [x8, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0xaf4
    ldr w16, [x8, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0xaf4
    ldr w16, [x8, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0xa3c
    ldr w16, [x8, #0x1560]
    cmp w16, #0xa
    b.ne mode_extension+0xa3c
    mov w9, #0xa
    b mode_extension+0xb04
    ldr w16, [x8, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0xa5c
    ldr w16, [x8, #0x1560]
    cmp w16, #0xb
    b.ne mode_extension+0xa5c
    mov w9, #0xb
    b mode_extension+0xb04
    ldr w16, [x8, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0xa7c
    ldr w16, [x8, #0x1560]
    cmp w16, #0xc
    b.ne mode_extension+0xa7c
    mov w9, #0xb
    b mode_extension+0xb04
    ldr w16, [x8, #0x155c]
    cmp w16, #0
    b.ne mode_extension+0xa9c
    ldr w16, [x8, #0x1560]
    cmp w16, #7
    b.ne mode_extension+0xa9c
    mov w9, #7
    b mode_extension+0xb04
    ldr w16, [x8, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0xac8
    ldr w16, [x8, #0x1560]
    cmp w16, #6
    b.ne mode_extension+0xac8
    ldr w16, [x8, #0x1588]
    cmp w16, #3
    b.ne mode_extension+0xac8
    mov w9, #7
    b mode_extension+0xb04
    ldr w16, [x8, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0xaf4
    ldr w16, [x8, #0x1560]
    cmp w16, #5
    b.ne mode_extension+0xaf4
    ldr w16, [x8, #0x1588]
    cmp w16, #1
    b.ne mode_extension+0xaf4
    mov w9, #7
    b mode_extension+0xb04
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    ldr w9, [x8, #0x1560]
    b native_842bac
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_842bac
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x6, #0x1558]
    cmp w16, #2
    b.ne mode_extension+0xc2c
    ldr w16, [x6, #0x15f0]
    cmp w16, #3
    b.ne mode_extension+0xc2c
    ldr w16, [x6, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0xc2c
    ldr w16, [x6, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0xc2c
    ldr w16, [x6, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0xc2c
    ldr w16, [x6, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0xb74
    ldr w16, [x6, #0x1560]
    cmp w16, #0xa
    b.ne mode_extension+0xb74
    mov w20, #0xa
    b mode_extension+0xc3c
    ldr w16, [x6, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0xb94
    ldr w16, [x6, #0x1560]
    cmp w16, #0xb
    b.ne mode_extension+0xb94
    mov w20, #0xb
    b mode_extension+0xc3c
    ldr w16, [x6, #0x155c]
    cmp w16, #7
    b.ne mode_extension+0xbb4
    ldr w16, [x6, #0x1560]
    cmp w16, #0xc
    b.ne mode_extension+0xbb4
    mov w20, #0xb
    b mode_extension+0xc3c
    ldr w16, [x6, #0x155c]
    cmp w16, #0
    b.ne mode_extension+0xbd4
    ldr w16, [x6, #0x1560]
    cmp w16, #7
    b.ne mode_extension+0xbd4
    mov w20, #7
    b mode_extension+0xc3c
    ldr w16, [x6, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0xc00
    ldr w16, [x6, #0x1560]
    cmp w16, #6
    b.ne mode_extension+0xc00
    ldr w16, [x6, #0x1588]
    cmp w16, #3
    b.ne mode_extension+0xc00
    mov w20, #7
    b mode_extension+0xc3c
    ldr w16, [x6, #0x155c]
    cmp w16, #4
    b.ne mode_extension+0xc2c
    ldr w16, [x6, #0x1560]
    cmp w16, #5
    b.ne mode_extension+0xc2c
    ldr w16, [x6, #0x1588]
    cmp w16, #1
    b.ne mode_extension+0xc2c
    mov w20, #7
    b mode_extension+0xc3c
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    ldr w20, [x6, #0x1560]
    b native_83b7cc
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_83b7cc
    .zero 8
    cbnz w1, mode_extension+0xd18
    cmp w2, #1
    b.ne mode_extension+0xd18
    orr w9, w3, w4
    cbnz w9, mode_extension+0xd18
    mov x9, #0x8318
    add x10, x0, x9
    ldr w9, [x10, #0x20]
    cmp w9, #3
    b.ne mode_extension+0xd18
    mov x11, #0x100
    movk x11, #1, lsl #16
    movk x11, #1, lsl #32
    ldr x9, [x10, #0x28]
    cmp x9, x11
    b.ne mode_extension+0xd18
    ldr x9, [x0, #0x8b8]
    cbz x9, mode_extension+0xccc
    ldr x12, [x9]
    ldr x13, [x10]
    cmp x12, x13
    b.ne mode_extension+0xccc
    ldr x12, [x9, #0x28]
    add x11, x11, #1
    cmp x12, x11
    b.ne mode_extension+0xccc
    ldr w12, [x9, #0x20]
    cmp w12, #4
    b.ne mode_extension+0xccc
    mov x0, x9
    ret
    stp x19, x30, [sp, #-0x10]!
    mov x19, x10
    ldr x0, [x10]
    mov x1, xzr
    bl native_4db3b0
    cbz x0, mode_extension+0xd0c
    ldp x8, x9, [x19, #0x28]
    stp x8, x9, [x0, #0x28]
    ldr w8, [x19, #0x38]
    str w8, [x0, #0x38]
    mov w8, #4
    str w8, [x0, #0x20]
    mov w8, #1
    strb w8, [x0, #0x28]
    strb w8, [x0, #0x32]
    b mode_extension+0xd10
    mov x0, x19
    ldp x19, x30, [sp], #0x10
    ret
    b mode_extension+0x0
    cbnz w1, mode_extension+0xd84
    cbnz w2, mode_extension+0xd84
    cmp w3, #1
    b.ne mode_extension+0xd84
    orr w9, w4, w6
    cbnz w9, mode_extension+0xd84
    mov x9, #0x8788
    add x9, x0, x9
    stp x19, x30, [sp, #-0x10]!
    mov x19, x9
    ldr x0, [x9]
    mov x1, xzr
    bl native_4db3b0
    cbz x0, mode_extension+0xd74
    mov w9, #1
    str w9, [x0, #0x20]
    stp xzr, xzr, [x0, #0x28]
    stur xzr, [x0, #0x38]
    stur xzr, [x0, #0x3e]
    strb w9, [x0, #0x31]
    strb w9, [x0, #0x40]
    b mode_extension+0xd78
    mov x0, x19
    ldp x19, x30, [sp], #0x10
    autiasp
    ret
    b mode_extension+0x110
    cmp w1, #1
    b.ne mode_extension+0xd9c
    cbnz w5, mode_extension+0xd9c
    cbnz w2, mode_extension+0xd9c
    mov w5, #1
    b capture_base+0x1200
    stp x0, x1, [sp, #-0x20]!
    str x30, [sp, #0x10]
    bl mode_extension+0xdd0
    ldr x30, [sp, #0x10]
    ldp x0, x1, [sp], #0x20
    b mode_extension+0x1d0
    stp x0, x1, [sp, #-0x20]!
    str x30, [sp, #0x10]
    bl mode_extension+0xdd0
    ldr x30, [sp, #0x10]
    ldp x0, x1, [sp], #0x20
    b mode_extension+0x1ec
    ldr x9, [x0, #0x10]
    add x10, x9, #0x840
    cmp x10, x0
    b.ne mode_extension+0xe9c
    ldr x10, [x9]
    adrp x11, native_712a000
    add x11, x11, #0x650
    cmp x10, x11
    b.ne mode_extension+0xe9c
    ldr x11, [x0, #0x68]
    cbz x11, mode_extension+0xe9c
    ldr x13, [x11, #0x28]
    cbz x13, mode_extension+0xe9c
    ldr x12, [x0, #0x78]
    cbz x12, mode_extension+0xe40
    ldr x14, [x12, #0x10]
    cbnz x14, mode_extension+0xe40
    ldr w14, [x12, #0x20]
    cmp w14, #4
    b.ne mode_extension+0xe40
    ldr x14, [x12, #0x28]
    mov x15, #0x101
    movk x15, #1, lsl #16
    movk x15, #1, lsl #32
    cmp x14, x15
    b.ne mode_extension+0xe40
    adr x10, mode_extension+0xea0
    b mode_extension+0xe64
    adr x10, mode_extension+0xea0
    mov x14, x13
    mov x15, x10
    ldrh w16, [x14], #2
    ldrb w17, [x15], #1
    cmp w16, w17
    b.ne mode_extension+0xe9c
    cbnz w16, mode_extension+0xe4c
    adr x10, mode_extension+0xea5
    mov x14, x13
    mov x15, x10
    ldrh w16, [x14], #2
    ldrb w17, [x15], #1
    cmp w16, w17
    b.ne mode_extension+0xe84
    cbnz w16, mode_extension+0xe6c
    ret
    stp x11, x30, [sp, #-0x10]!
    mov x0, x10
    bl native_4e3e80
    ldp x11, x30, [sp], #0x10
    cbz x0, mode_extension+0xe9c
    str x0, [x11, #0x28]
    ret
    .ascii "9.7K"
    .zero 1
    .ascii "16K"
    .zero 4
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x7, #0x1558]
    cmp w16, #0
    b.ne mode_extension+0xf0c
    ldr w16, [x7, #0x15f0]
    cmp w16, #1
    b.ne mode_extension+0xf0c
    ldr w16, [x7, #0x155c]
    cmp w16, #0
    b.ne mode_extension+0xf0c
    ldr w16, [x7, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0xf0c
    ldr w16, [x7, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0xf0c
    ldr w16, [x7, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0xf0c
    mov w28, #1
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_841ba8
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b mode_extension+0x3d0
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x7, #0x1558]
    cmp w16, #0
    b.ne mode_extension+0xf78
    ldr w16, [x7, #0x15f0]
    cmp w16, #1
    b.ne mode_extension+0xf78
    ldr w16, [x7, #0x155c]
    cmp w16, #0
    b.ne mode_extension+0xf78
    ldr w16, [x7, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0xf78
    ldr w16, [x7, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0xf78
    ldr w16, [x7, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0xf78
    mov w28, #1
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_842b98
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b mode_extension+0x504
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x5, #0x1558]
    cmp w16, #0
    b.ne mode_extension+0xfe4
    ldr w16, [x5, #0x15f0]
    cmp w16, #1
    b.ne mode_extension+0xfe4
    ldr w16, [x5, #0x155c]
    cmp w16, #0
    b.ne mode_extension+0xfe4
    ldr w16, [x5, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0xfe4
    ldr w16, [x5, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0xfe4
    ldr w16, [x5, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0xfe4
    mov w25, #1
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_83b7b8
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b mode_extension+0x638
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x5, #0x1558]
    cmp w16, #0
    b.ne mode_extension+0x1050
    ldr w16, [x5, #0x15f0]
    cmp w16, #1
    b.ne mode_extension+0x1050
    ldr w16, [x5, #0x155c]
    cmp w16, #0
    b.ne mode_extension+0x1050
    ldr w16, [x5, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x1050
    ldr w16, [x5, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x1050
    ldr w16, [x5, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x1050
    mov w25, #1
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_83d118
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b mode_extension+0x76c
    .zero 4
    cbnz w1, mode_extension+0x1138
    cmp w2, #1
    b.ne mode_extension+0x1138
    orr w9, w3, w4
    cbnz w9, mode_extension+0x1138
    mov x9, #0x8318
    add x10, x0, x9
    ldr w9, [x10, #0x20]
    cmp w9, #3
    b.ne mode_extension+0x1138
    mov x11, #0x100
    movk x11, #1, lsl #16
    movk x11, #1, lsl #32
    ldr x9, [x10, #0x28]
    cmp x9, x11
    b.ne mode_extension+0x1138
    ldr x9, [x0, #0x8b8]
    cbz x9, mode_extension+0x10e4
    ldr x12, [x9]
    ldr x13, [x10]
    cmp x12, x13
    b.ne mode_extension+0x10e4
    ldr x12, [x9, #0x28]
    mov x11, #0x101
    movk x11, #0x101, lsl #16
    movk x11, #1, lsl #32
    cmp x12, x11
    b.ne mode_extension+0x10e4
    ldr w12, [x9, #0x20]
    cmp w12, #5
    b.ne mode_extension+0x10e4
    mov x0, x9
    ret
    stp x19, x30, [sp, #-0x10]!
    mov x19, x10
    ldr x0, [x19]
    mov x1, xzr
    bl native_4db3b0
    cbz x0, mode_extension+0x112c
    ldp x8, x9, [x19, #0x28]
    stp x8, x9, [x0, #0x28]
    ldr w8, [x19, #0x38]
    str w8, [x0, #0x38]
    mov w8, #5
    str w8, [x0, #0x20]
    mov w8, #1
    strb w8, [x0, #0x28]
    strb w8, [x0, #0x32]
    strb w8, [x0, #0x2b]
    strb w8, [x0, #0x35]
    b mode_extension+0x1130
    mov x0, x19
    ldp x19, x30, [sp], #0x10
    ret
    b mode_extension+0xc50
    cmp w1, #3
    b.ne mode_extension+0x11a8
    cbnz w2, mode_extension+0x11a8
    cmp w3, #1
    b.ne mode_extension+0x11a8
    orr w9, w4, w6
    cbnz w9, mode_extension+0x11a8
    mov x9, #0x8788
    add x9, x0, x9
    stp x19, x30, [sp, #-0x10]!
    mov x19, x9
    ldr x0, [x19]
    mov x1, xzr
    bl native_4db3b0
    cbz x0, mode_extension+0x1198
    mov w9, #1
    str w9, [x0, #0x20]
    stp xzr, xzr, [x0, #0x28]
    stur xzr, [x0, #0x38]
    stur xzr, [x0, #0x3e]
    strb w9, [x0, #0x2e]
    strb w9, [x0, #0x3d]
    b mode_extension+0x119c
    mov x0, x19
    ldp x19, x30, [sp], #0x10
    autiasp
    ret
    b mode_extension+0xd1c
    cmp w1, #1
    b.ne mode_extension+0x11c4
    cbnz w2, mode_extension+0x11c4
    cmp w5, #3
    b.ne mode_extension+0x11c4
    mov w5, #1
    b mode_extension+0xd88
    ldr x9, [x0, #0x10]
    add x10, x9, #0x840
    cmp x0, x10
    b.ne mode_extension+0x1244
    ldr x10, [x9]
    adrp x11, native_712a000
    add x11, x11, #0x650
    cmp x10, x11
    b.ne mode_extension+0x1244
    ldr x12, [x0, #0x78]
    cbz x12, mode_extension+0x123c
    ldr x10, [x12, #0x10]
    cbnz x10, mode_extension+0x123c
    mov x10, #0x8318
    ldr x10, [x9, x10]
    ldr x14, [x12]
    cmp x14, x10
    b.ne mode_extension+0x123c
    ldr w14, [x12, #0x20]
    cmp w14, #5
    b.ne mode_extension+0x123c
    ldr x14, [x12, #0x28]
    mov x15, #0x101
    movk x15, #0x101, lsl #16
    movk x15, #1, lsl #32
    cmp x14, x15
    b.ne mode_extension+0x123c
    mov w9, #2
    ret
    mov w9, #1
    ret
    mov w9, #0
    ret
    stp x1, x30, [sp, #-0x10]!
    bl mode_extension+0x11c8
    ldp x1, x30, [sp], #0x10
    cmp w9, #2
    b.ne mode_extension+0x127c
    cmp w1, #5
    b.hs mode_extension+0x127c
    adr x10, mode_extension+0x138c
    ldrb w0, [x10, w1, uxtw]
    ldp x29, x30, [sp], #0x30
    autiasp
    ret
    b mode_extension+0x28c
    stp x0, x1, [sp, #-0x20]!
    str x30, [sp, #0x10]
    bl mode_extension+0x12d0
    ldr x30, [sp, #0x10]
    ldp x0, x1, [sp], #0x20
    cmp w9, #2
    b.ne mode_extension+0x12a4
    ldr x19, [x0, #0x68]
    b native_57b484
    b mode_extension+0xda0
    stp x0, x1, [sp, #-0x20]!
    str x30, [sp, #0x10]
    bl mode_extension+0x12d0
    ldr x30, [sp, #0x10]
    ldp x0, x1, [sp], #0x20
    cmp w9, #2
    b.ne mode_extension+0x12cc
    ldr x19, [x0, #0x60]
    b native_57b604
    b mode_extension+0xdb8
    stp x19, x20, [sp, #-0x20]!
    str x30, [sp, #0x10]
    mov x19, x0
    bl mode_extension+0x11c8
    mov w20, w9
    cbz w20, mode_extension+0x1340
    ldr x19, [x19, #0x68]
    cbz x19, mode_extension+0x1340
    cmp w20, #2
    b.ne mode_extension+0x1314
    add x10, x19, #0x28
    adr x0, mode_extension+0x1391
    bl mode_extension+0x1350
    add x10, x19, #0x40
    adr x0, mode_extension+0x1396
    bl mode_extension+0x1350
    b mode_extension+0x1340
    add x10, x19, #0x40
    ldr x13, [x10]
    cbz x13, mode_extension+0x1340
    adr x14, mode_extension+0x1396
    ldrh w15, [x13], #2
    ldrb w16, [x14], #1
    cmp w15, w16
    b.ne mode_extension+0x1340
    cbnz w16, mode_extension+0x1324
    adr x0, mode_extension+0x139b
    bl mode_extension+0x1350
    mov w9, w20
    ldr x30, [sp, #0x10]
    ldp x19, x20, [sp], #0x20
    ret
    ldr x13, [x10]
    cbz x13, mode_extension+0x1388
    mov x14, x0
    ldrh w15, [x13], #2
    ldrb w16, [x14], #1
    cmp w16, w15
    b.ne mode_extension+0x1374
    cbnz w16, mode_extension+0x135c
    ret
    stp x10, x30, [sp, #-0x10]!
    bl native_4e3e80
    ldp x10, x30, [sp], #0x10
    cbz x0, mode_extension+0x1388
    str x0, [x10]
    ret
    .zero 1
    .byte 1
    .byte 3
    .byte 2
    .byte 4
    .ascii "9.7K"
    .zero 1
    .ascii "7.4K"
    .zero 1
    .ascii "5K"
    .zero 3
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x7, #0x1558]
    cmp w16, #0
    b.ne mode_extension+0x1400
    ldr w16, [x7, #0x15f0]
    cmp w16, #1
    b.ne mode_extension+0x1400
    ldr w16, [x7, #0x155c]
    cmp w16, #3
    b.ne mode_extension+0x1400
    ldr w16, [x7, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x1400
    ldr w16, [x7, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x1400
    ldr w16, [x7, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x1400
    mov w28, #2
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_841ba8
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b mode_extension+0xeac
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x7, #0x1558]
    cmp w16, #0
    b.ne mode_extension+0x146c
    ldr w16, [x7, #0x15f0]
    cmp w16, #1
    b.ne mode_extension+0x146c
    ldr w16, [x7, #0x155c]
    cmp w16, #3
    b.ne mode_extension+0x146c
    ldr w16, [x7, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x146c
    ldr w16, [x7, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x146c
    ldr w16, [x7, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x146c
    mov w28, #2
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_842b98
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b mode_extension+0xf18
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x5, #0x1558]
    cmp w16, #0
    b.ne mode_extension+0x14d8
    ldr w16, [x5, #0x15f0]
    cmp w16, #1
    b.ne mode_extension+0x14d8
    ldr w16, [x5, #0x155c]
    cmp w16, #3
    b.ne mode_extension+0x14d8
    ldr w16, [x5, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x14d8
    ldr w16, [x5, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x14d8
    ldr w16, [x5, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x14d8
    mov w25, #2
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_83b7b8
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b mode_extension+0xf84
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x5, #0x1558]
    cmp w16, #0
    b.ne mode_extension+0x1544
    ldr w16, [x5, #0x15f0]
    cmp w16, #1
    b.ne mode_extension+0x1544
    ldr w16, [x5, #0x155c]
    cmp w16, #3
    b.ne mode_extension+0x1544
    ldr w16, [x5, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x1544
    ldr w16, [x5, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x1544
    ldr w16, [x5, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x1544
    mov w25, #2
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_83d118
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b mode_extension+0xff0
    cbnz w1, mode_extension+0x163c
    cmp w2, #1
    b.ne mode_extension+0x163c
    orr w9, w3, w4
    cbnz w9, mode_extension+0x163c
    mov x9, #0x8318
    add x10, x0, x9
    ldr w9, [x10, #0x20]
    cmp w9, #3
    b.ne mode_extension+0x163c
    mov x11, #0x100
    movk x11, #1, lsl #16
    movk x11, #1, lsl #32
    ldr x9, [x10, #0x28]
    cmp x9, x11
    b.ne mode_extension+0x163c
    ldr x9, [x0, #0x8b8]
    cbz x9, mode_extension+0x15d8
    ldr x12, [x9]
    ldr x13, [x10]
    cmp x12, x13
    b.ne mode_extension+0x15d8
    ldr x12, [x9, #0x28]
    mov x11, #0x101
    movk x11, #0x101, lsl #16
    movk x11, #0x101, lsl #32
    movk x11, #0x100, lsl #48
    cmp x12, x11
    b.ne mode_extension+0x15d8
    ldr w12, [x9, #0x20]
    cmp w12, #7
    b.ne mode_extension+0x15d8
    mov x0, x9
    ret
    stp x19, x30, [sp, #-0x10]!
    mov x19, x10
    ldr x0, [x19]
    mov x1, xzr
    bl native_4db3b0
    cbz x0, mode_extension+0x1630
    ldp x8, x9, [x19, #0x28]
    stp x8, x9, [x0, #0x28]
    ldr w8, [x19, #0x38]
    str w8, [x0, #0x38]
    mov w8, #7
    str w8, [x0, #0x20]
    mov w8, #1
    strb w8, [x0, #0x28]
    strb w8, [x0, #0x32]
    strb w8, [x0, #0x2b]
    strb w8, [x0, #0x35]
    strb w8, [x0, #0x2d]
    strb w8, [x0, #0x37]
    strb w8, [x0, #0x2f]
    strb w8, [x0, #0x39]
    b mode_extension+0x1634
    mov x0, x19
    ldp x19, x30, [sp], #0x10
    ret
    b mode_extension+0x1060
    cmp w1, #5
    b.eq mode_extension+0x1650
    cmp w1, #7
    b.ne mode_extension+0x16d0
    cbnz w2, mode_extension+0x16d0
    cmp w3, #1
    b.ne mode_extension+0x16d0
    orr w9, w4, w6
    cbnz w9, mode_extension+0x16d0
    mov x9, #0x8788
    add x9, x0, x9
    stp x19, x30, [sp, #-0x10]!
    mov x19, x9
    str w1, [sp, #-0x10]!
    ldr x0, [x19]
    mov x1, xzr
    bl native_4db3b0
    ldr w10, [sp], #0x10
    cbz x0, mode_extension+0x16c0
    mov w9, #1
    str w9, [x0, #0x20]
    stp xzr, xzr, [x0, #0x28]
    stur xzr, [x0, #0x38]
    stur xzr, [x0, #0x3e]
    cmp w10, #7
    b.ne mode_extension+0x16b4
    strb w9, [x0, #0x31]
    strb w9, [x0, #0x40]
    b mode_extension+0x16c4
    strb w9, [x0, #0x2e]
    strb w9, [x0, #0x3d]
    b mode_extension+0x16c4
    mov x0, x19
    ldp x19, x30, [sp], #0x10
    autiasp
    ret
    b mode_extension+0x113c
    cmp w1, #1
    b.ne mode_extension+0x16f4
    cbnz w2, mode_extension+0x16f4
    cmp w5, #5
    b.eq mode_extension+0x16f0
    cmp w5, #7
    b.ne mode_extension+0x16f4
    mov w5, #1
    b mode_extension+0x11ac
    ldr x9, [x0, #0x10]
    add x10, x9, #0x840
    cmp x0, x10
    b.ne mode_extension+0x1778
    ldr x10, [x9]
    adrp x11, native_712a000
    add x11, x11, #0x650
    cmp x10, x11
    b.ne mode_extension+0x1778
    ldr x12, [x0, #0x78]
    cbz x12, mode_extension+0x1770
    ldr x10, [x12, #0x10]
    cbnz x10, mode_extension+0x1770
    mov x10, #0x8318
    ldr x10, [x9, x10]
    ldr x14, [x12]
    cmp x14, x10
    b.ne mode_extension+0x1770
    ldr w14, [x12, #0x20]
    cmp w14, #7
    b.ne mode_extension+0x1770
    ldr x14, [x12, #0x28]
    mov x15, #0x101
    movk x15, #0x101, lsl #16
    movk x15, #0x101, lsl #32
    movk x15, #0x100, lsl #48
    cmp x14, x15
    b.ne mode_extension+0x1770
    mov w9, #2
    ret
    mov w9, #1
    ret
    mov w9, #0
    ret
    stp x1, x30, [sp, #-0x10]!
    bl mode_extension+0x16f8
    ldp x1, x30, [sp], #0x10
    cmp w9, #2
    b.ne mode_extension+0x17b0
    cmp w1, #7
    b.hs mode_extension+0x17b0
    adr x10, mode_extension+0x195c
    ldrb w0, [x10, w1, uxtw]
    ldp x29, x30, [sp], #0x30
    autiasp
    ret
    b mode_extension+0x124c
    stp x0, x1, [sp, #-0x20]!
    str x30, [sp, #0x10]
    bl mode_extension+0x1804
    ldr x30, [sp, #0x10]
    ldp x0, x1, [sp], #0x20
    cmp w9, #2
    b.ne mode_extension+0x17d8
    ldr x19, [x0, #0x68]
    b native_57b484
    b mode_extension+0x1280
    stp x0, x1, [sp, #-0x20]!
    str x30, [sp, #0x10]
    bl mode_extension+0x1804
    ldr x30, [sp, #0x10]
    ldp x0, x1, [sp], #0x20
    cmp w9, #2
    b.ne mode_extension+0x1800
    ldr x19, [x0, #0x60]
    b native_57b604
    b mode_extension+0x12a8
    stp x19, x20, [sp, #-0x20]!
    str x30, [sp, #0x10]
    mov x19, x0
    bl mode_extension+0x16f8
    mov w20, w9
    cbz w20, mode_extension+0x1910
    ldr x19, [x19, #0x68]
    cbz x19, mode_extension+0x1910
    cmp w20, #2
    b.ne mode_extension+0x1860
    add x10, x19, #0x28
    adr x0, mode_extension+0x1963
    bl mode_extension+0x1920
    add x10, x19, #0x40
    adr x0, mode_extension+0x1967
    bl mode_extension+0x1920
    add x10, x19, #0x50
    adr x0, mode_extension+0x196f
    bl mode_extension+0x1920
    add x10, x19, #0x60
    adr x0, mode_extension+0x1979
    bl mode_extension+0x1920
    b mode_extension+0x1910
    add x10, x19, #0x28
    ldr x13, [x10]
    cbz x13, mode_extension+0x188c
    adr x14, mode_extension+0x1963
    ldrh w15, [x13], #2
    ldrb w16, [x14], #1
    cmp w15, w16
    b.ne mode_extension+0x188c
    cbnz w16, mode_extension+0x1870
    adr x0, mode_extension+0x1982
    bl mode_extension+0x1920
    add x10, x19, #0x40
    ldr x13, [x10]
    cbz x13, mode_extension+0x18b8
    adr x14, mode_extension+0x1967
    ldrh w15, [x13], #2
    ldrb w16, [x14], #1
    cmp w15, w16
    b.ne mode_extension+0x18b8
    cbnz w16, mode_extension+0x189c
    adr x0, mode_extension+0x196c
    bl mode_extension+0x1920
    add x10, x19, #0x50
    ldr x13, [x10]
    cbz x13, mode_extension+0x18e4
    adr x14, mode_extension+0x196f
    ldrh w15, [x13], #2
    ldrb w16, [x14], #1
    cmp w15, w16
    b.ne mode_extension+0x18e4
    cbnz w16, mode_extension+0x18c8
    adr x0, mode_extension+0x1986
    bl mode_extension+0x1920
    add x10, x19, #0x60
    ldr x13, [x10]
    cbz x13, mode_extension+0x1910
    adr x14, mode_extension+0x1979
    ldrh w15, [x13], #2
    ldrb w16, [x14], #1
    cmp w15, w16
    b.ne mode_extension+0x1910
    cbnz w16, mode_extension+0x18f4
    adr x0, mode_extension+0x198b
    bl mode_extension+0x1920
    mov w9, w20
    ldr x30, [sp, #0x10]
    ldp x19, x20, [sp], #0x20
    ret
    ldr x13, [x10]
    cbz x13, mode_extension+0x1958
    mov x14, x0
    ldrh w15, [x13], #2
    ldrb w16, [x14], #1
    cmp w16, w15
    b.ne mode_extension+0x1944
    cbnz w16, mode_extension+0x192c
    ret
    stp x10, x30, [sp, #-0x10]!
    bl native_4e3e80
    ldp x10, x30, [sp], #0x10
    cbz x0, mode_extension+0x1958
    str x0, [x10]
    ret
    .zero 1
    .byte 7
    .byte 1
    .byte 5
    .byte 3
    .byte 2
    .byte 4
    .ascii "10K"
    .zero 1
    .ascii "7.4K"
    .zero 1
    .ascii "5K"
    .zero 1
    .ascii "7.4K Crop"
    .zero 1
    .ascii "10K Crop"
    .zero 1
    .ascii "16K"
    .zero 1
    .ascii "2.7K"
    .zero 1
    .ascii "1080P"
    .zero 4
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x7, #0x1558]
    cmp w16, #0
    b.ne mode_extension+0x1a04
    ldr w16, [x7, #0x15f0]
    cmp w16, #1
    b.ne mode_extension+0x1a04
    ldr w16, [x7, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x1a04
    ldr w16, [x7, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x1a04
    ldr w16, [x7, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x1a04
    ldr w16, [x7, #0x155c]
    cmp w16, #5
    b.eq mode_extension+0x19f4
    cmp w16, #7
    b.ne mode_extension+0x1a04
    mov w28, #1
    b mode_extension+0x19f8
    mov w28, #2
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_841ba8
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b mode_extension+0x13a0
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x7, #0x1558]
    cmp w16, #0
    b.ne mode_extension+0x1a80
    ldr w16, [x7, #0x15f0]
    cmp w16, #1
    b.ne mode_extension+0x1a80
    ldr w16, [x7, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x1a80
    ldr w16, [x7, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x1a80
    ldr w16, [x7, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x1a80
    ldr w16, [x7, #0x155c]
    cmp w16, #5
    b.eq mode_extension+0x1a70
    cmp w16, #7
    b.ne mode_extension+0x1a80
    mov w28, #1
    b mode_extension+0x1a74
    mov w28, #2
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_842b98
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b mode_extension+0x140c
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x5, #0x1558]
    cmp w16, #0
    b.ne mode_extension+0x1afc
    ldr w16, [x5, #0x15f0]
    cmp w16, #1
    b.ne mode_extension+0x1afc
    ldr w16, [x5, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x1afc
    ldr w16, [x5, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x1afc
    ldr w16, [x5, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x1afc
    ldr w16, [x5, #0x155c]
    cmp w16, #5
    b.eq mode_extension+0x1aec
    cmp w16, #7
    b.ne mode_extension+0x1afc
    mov w25, #1
    b mode_extension+0x1af0
    mov w25, #2
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_83b7b8
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b mode_extension+0x1478
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    ldr w16, [x5, #0x1558]
    cmp w16, #0
    b.ne mode_extension+0x1b78
    ldr w16, [x5, #0x15f0]
    cmp w16, #1
    b.ne mode_extension+0x1b78
    ldr w16, [x5, #0x15d0]
    cmp w16, #0
    b.ne mode_extension+0x1b78
    ldr w16, [x5, #0x1ad0]
    cmp w16, #0
    b.ne mode_extension+0x1b78
    ldr w16, [x5, #0x1af8]
    cmp w16, #0
    b.ne mode_extension+0x1b78
    ldr w16, [x5, #0x155c]
    cmp w16, #5
    b.eq mode_extension+0x1b68
    cmp w16, #7
    b.ne mode_extension+0x1b78
    mov w25, #1
    b mode_extension+0x1b6c
    mov w25, #2
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b native_83d118
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b mode_extension+0x14e4
