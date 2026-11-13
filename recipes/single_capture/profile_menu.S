// FreeFrame profile, frame-rate and bitrate controls. Branch destinations in the
// original GUI are resolved by the linker. Data spans belong to these hooks.
.text
.global capture_start
capture_start:
// Frame rate and iLog selectors for the physical recording channel.
.org 0xc00
    cmp w1, #4
    b.ne capture_start+0xd18
    cmp w2, #2
    b.ne capture_start+0xd18
    cmp w3, #3
    b.ne capture_start+0xd18
    cbnz w4, capture_start+0xd18
    cmp w5, #2
    b.ne capture_start+0xd18
    cbnz w6, capture_start+0xd18
    ldr w9, [sp]
    cbnz w9, capture_start+0xd18
    cmp w7, #1
    b.eq capture_start+0xc48
    cmp w7, #3
    b.ne capture_start+0xd18
    mov w12, #5
    b capture_start+0xc4c
    mov w12, #4
    stp x19, x30, [sp, #-0x20]!
    str x20, [sp, #0x10]
    mov w20, w12
    mov x9, #0x8a10
    add x19, x0, x9
    ldr x9, [x0, #0xcc8]
    cbz x9, capture_start+0xcbc
    ldr x10, [x19]
    ldr x11, [x9]
    cmp x10, x11
    b.ne capture_start+0xcbc
    ldr w10, [x9, #0x20]
    cmp w10, #5
    b.ne capture_start+0xcbc
    add x10, x19, #0x28
    add x11, x9, #0x28
    mov w13, #0
    ldrb w14, [x10, x13]
    cmp w13, w20
    csinc w14, w14, wzr, ne
    ldrb w15, [x11, x13]
    cmp w14, w15
    b.ne capture_start+0xcbc
    add w13, w13, #1
    cmp w13, #0xf
    b.lo capture_start+0xc90
    mov x0, x9
    b capture_start+0xd08
    ldr x0, [x19]
    mov x1, #0
    bl address_4db3b0
    cbnz x0, capture_start+0xcd4
    mov x0, x19
    b capture_start+0xd08
    ldp x9, x10, [x19, #0x28]
    stp x9, x10, [x0, #0x28]
    ldr x9, [x19, #0x38]
    str x9, [x0, #0x38]
    ldur x9, [x19, #0x3e]
    stur x9, [x0, #0x3e]
    mov w9, #5
    str w9, [x0, #0x20]
    add x10, x0, #0x28
    mov w9, #1
    strb w9, [x10, x20]
    add x10, x10, #0xf
    strb w9, [x10, x20]
    ldr x20, [sp, #0x10]
    ldp x19, x30, [sp], #0x20
    autiasp
    ret
    stp x29, x30, [sp, #-0x50]!
    b address_834288
    ldr x9, [x21, #0xcb8]
    cbz x9, capture_start+0xdd0
    ldr x10, [x9, #0x50]
    cbz x10, capture_start+0xdd0
    cmp w22, #4
    b.ne capture_start+0xd90
    cmp w23, #2
    b.ne capture_start+0xd90
    cmp w24, #3
    b.ne capture_start+0xd90
    cbnz w25, capture_start+0xd90
    cmp w26, #2
    b.ne capture_start+0xd90
    cbnz w27, capture_start+0xd90
    cmp w20, #3
    b.ne capture_start+0xd90
    ldr w11, [sp]
    cbnz w11, capture_start+0xd90
    ldr w11, [x10]
    mov w12, #0x31
    movk w12, #0x30, lsl #16
    cmp w11, w12
    b.ne capture_start+0xdd0
    ldr w11, [x10, #4]
    cmp w11, #0x30
    b.ne capture_start+0xdd0
    adr x10, capture_start+0xee0
    b capture_start+0xdb0
    ldr w11, [x10]
    mov w12, #0x39
    movk w12, #0x30, lsl #16
    cmp w11, w12
    b.ne capture_start+0xdd0
    ldrh w11, [x10, #4]
    cbnz w11, capture_start+0xdd0
    adr x10, capture_start+0xee3
    stp x0, x30, [sp, #-0x20]!
    str x9, [sp, #0x10]
    mov x0, x10
    bl address_4e3e80
    ldr x9, [sp, #0x10]
    cbz x0, capture_start+0xdcc
    str x0, [x9, #0x50]
    ldp x0, x30, [sp], #0x20
    mov x1, x0
    b address_835e10
    ldr w21, [x3, w1, uxtw #2]
    cmp w21, #0x64
    b.ne capture_start+0xdf4
    stp x0, x30, [sp, #-0x10]!
    bl capture_start+0xe44
    mov w21, w0
    ldp x0, x30, [sp], #0x10
    b address_87066c
    ldr w22, [x5, w3, uxtw #2]
    cmp w22, #0x64
    b.ne capture_start+0xe1c
    stp x0, x30, [sp, #-0x20]!
    str x1, [sp, #0x10]
    bl capture_start+0xe44
    mov w22, w0
    ldr x1, [sp, #0x10]
    ldp x0, x30, [sp], #0x20
    b address_870c44
    mov w0, w21
    cmp w0, #0x64
    b.ne capture_start+0xe40
    stp x1, x2, [sp, #-0x20]!
    str x30, [sp, #0x10]
    bl capture_start+0xe44
    ldr x30, [sp, #0x10]
    ldp x1, x2, [sp], #0x20
    b address_876ccc
    stp x29, x30, [sp, #-0x10]!
    adrp x0, address_7160000
    ldr x0, [x0, #0xfc0]
    adrp x1, address_7166000
    ldr x1, [x1, #0x558]
    mov x9, x0
    mov x0, x1
    mov x1, x9
    bl address_4db7a0
    cbz x0, capture_start+0xed4
    ldr w9, [x0, #0x155c]
    cmp w9, #4
    b.ne capture_start+0xed4
    ldr w9, [x0, #0x1558]
    cmp w9, #2
    b.ne capture_start+0xed4
    ldr w9, [x0, #0x15f0]
    cmp w9, #3
    b.ne capture_start+0xed4
    ldr w9, [x0, #0x15d0]
    cmp w9, #0
    b.ne capture_start+0xed4
    ldr w9, [x0, #0x1ad8]
    cmp w9, #2
    b.ne capture_start+0xed4
    ldr w9, [x0, #0x1ad0]
    cmp w9, #0
    b.ne capture_start+0xed4
    ldr w9, [x0, #0x1588]
    cmp w9, #3
    b.ne capture_start+0xed4
    ldr w9, [x0, #0x1af8]
    cmp w9, #0
    b.ne capture_start+0xed4
    mov w0, #0x5a
    b capture_start+0xed8
    mov w0, #0x64
    ldp x29, x30, [sp], #0x10
    ret
    .byte 0x39, 0x30, 0x0, 0x31, 0x30, 0x30, 0x0, 0x0
    cbnz w1, capture_start+0xf48
    cmp w2, #3
    b.ne capture_start+0xf48
    cbnz w6, capture_start+0xf48
    cmp w3, #0
    b.eq capture_start+0xf08
    cmp w3, #2
    b.ne capture_start+0xf48
    stp x0, x30, [sp, #-0x10]!
    bl capture_start+0xf48
    ldp x9, x30, [sp], #0x10
    sub x10, x0, x9
    mov x11, #0x9568
    cmp x10, x11
    b.ne capture_start+0xf30
    mov x10, #0x9530
    add x0, x9, x10
    ret
    mov x11, #0x94f8
    cmp x10, x11
    b.ne capture_start+0xf44
    mov x10, #0x94c0
    add x0, x9, x10
    ret
    cmp w1, #0
    b address_83ac68
    mov x12, #0x7d78
    cbnz w22, capture_start+0xfb8
    cmp w23, #3
    b.ne capture_start+0xfb8
    cmp w24, #2
    b.eq capture_start+0xf6c
    cbnz w24, capture_start+0xfb8
    sub x9, x0, x21
    mov x10, #0xa150
    cmp x9, x10
    b.ne capture_start+0xf88
    mov x10, #0x9f40
    add x0, x21, x10
    b capture_start+0xfb8
    mov x10, #0xa200
    cmp x9, x10
    b.ne capture_start+0xfa0
    mov x10, #0x9d30
    add x0, x21, x10
    b capture_start+0xfb8
    mov x10, #0xa2b0
    cmp x9, x10
    b.ne capture_start+0xfb8
    mov x10, #0x9de0
    add x0, x21, x10
    b capture_start+0xfb8
    b address_841c24
    mov x12, #0x7e08
    cbnz w22, capture_start+0x1024
    cmp w23, #3
    b.ne capture_start+0x1024
    cmp w24, #2
    b.eq capture_start+0xfd8
    cbnz w24, capture_start+0x1024
    sub x9, x0, x21
    mov x10, #0xa990
    cmp x9, x10
    b.ne capture_start+0xff4
    mov x10, #0xa780
    add x0, x21, x10
    b capture_start+0x1024
    mov x10, #0xaaf0
    cmp x9, x10
    b.ne capture_start+0x100c
    mov x10, #0xa4c0
    add x0, x21, x10
    b capture_start+0x1024
    mov x10, #0xaba0
    cmp x9, x10
    b.ne capture_start+0x1024
    mov x10, #0xa570
    add x0, x21, x10
    b capture_start+0x1024
    b address_842c14
    cbnz w1, capture_start+0x104c
    ldr w9, [x0, #0x15d0]
    cbnz w9, capture_start+0x104c
    ldr w9, [x0, #0x1af8]
    cbnz w9, capture_start+0x104c
    ldr w9, [x0, #0x155c]
    cmp w9, #3
    b.ne capture_start+0x104c
    b address_5e8a48
    cmp w1, #1
    b address_5e8a70
    .byte 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0
    stp x0, x9, [sp, #-0x20]!
    str x30, [sp, #0x10]
    bl capture_start+0x10e4
    ldr x9, [sp, #8]
    cmp w9, w0
    ldr x0, [sp]
    ldr x30, [sp, #0x10]
    add sp, sp, #0x20
    b address_700b09c
    stp x0, x30, [sp, #-0x10]!
    bl capture_start+0x10e4
    mov w10, w0
    ldp x0, x30, [sp], #0x10
    cmp w0, w10
    b address_700b274
    stp x0, x30, [sp, #-0x10]!
    bl capture_start+0x10e4
    mov w10, w0
    ldp x0, x30, [sp], #0x10
    cmp w0, #0x154
    b.hi capture_start+0x10bc
    cmp w0, w10
    csel w0, w0, w10, ls
    cmp w0, w10
    b address_700b240
    stp x9, x30, [sp, #-0x10]!
    bl capture_start+0x10e4
    cmp w0, #0x140
    adr x0, capture_start+0x1180
    b.ne capture_start+0x10dc
    adr x0, address_700b3ca
    ldp x9, x30, [sp], #0x10
    b address_700b12c
    stp x29, x30, [sp, #-0x10]!
    adrp x0, address_7160000
    ldr x1, [x0, #0xfc0]
    adrp x0, address_7166000
    ldr x0, [x0, #0x558]
    bl address_4db7a0
    cbz x0, capture_start+0x1174
    ldr w9, [x0, #0x155c]
    cmp w9, #4
    b.ne capture_start+0x1174
    ldr w9, [x0, #0x1558]
    cmp w9, #2
    b.ne capture_start+0x1174
    ldr w9, [x0, #0x15f0]
    cmp w9, #3
    b.ne capture_start+0x1174
    ldr w9, [x0, #0x15d0]
    cmp w9, #0
    b.ne capture_start+0x1174
    ldr w9, [x0, #0x1ad8]
    cmp w9, #2
    b.ne capture_start+0x1174
    ldr w9, [x0, #0x1ad0]
    cmp w9, #0
    b.ne capture_start+0x1174
    ldr w9, [x0, #0x1588]
    cmp w9, #3
    b.ne capture_start+0x1174
    ldr w9, [x0, #0x1af8]
    cmp w9, #0
    b.ne capture_start+0x1174
    ldr w9, [x0, #0x1560]
    cmp w9, #7
    b.ne capture_start+0x1174
    mov w0, #0x154
    b capture_start+0x1178
    mov w0, #0x140
    ldp x29, x30, [sp], #0x10
    ret
    .byte 0x45, 0x6e, 0x74, 0x65, 0x72, 0x20, 0x31, 0x2d, 0x33, 0x34, 0x30, 0x20, 0x4d, 0x62, 0x70, 0x73
    .byte 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0
    cmp w1, #2
    b.ne capture_start+0x11f8
    cmp w2, #3
    b.ne capture_start+0x11f8
    cbnz w3, capture_start+0x11f8
    cbnz w4, capture_start+0x11f8
    stp x0, x30, [sp, #-0x10]!
    adrp x0, address_7163000
    ldr x0, [x0, #0x600]
    adrp x1, address_7160000
    ldr x1, [x1, #0x4e8]
    bl address_4db7a0
    cbz x0, capture_start+0x11e8
    ldr w9, [x0, #0x30]
    cmp w9, #1
    b.hi capture_start+0x11e8
    mov x9, #0x8318
    b capture_start+0x11ec
    mov x9, #0x83d8
    ldp x0, x30, [sp], #0x10
    add x0, x0, x9
    ret
    cmp w1, #0
    b address_8338d8
    cmp w1, #3
    b.ne capture_start+0x122c
    cbnz w2, capture_start+0x122c
    cbnz w3, capture_start+0x122c
    cmp w4, #2
    b.ne capture_start+0x122c
    b capture_start+0x1be8
    b.ne capture_start+0x122c
    mov x9, #0x8cf0
    add x0, x0, x9
    ret
    cmp w1, #1
    b address_837408
    cmp w1, #1
    b.ne capture_start+0x12dc
    cmp w2, #2
    b.ne capture_start+0x12dc
    cmp w3, #3
    b.ne capture_start+0x12dc
    cbnz w4, capture_start+0x12dc
    cbnz w6, capture_start+0x12dc
    stp x19, x30, [sp, #-0x10]!
    mov x19, x0
    ldr x0, [x19, #0xcc8]
    cbz x0, capture_start+0x1290
    mov x9, #0x8a58
    ldr x10, [x19, x9]
    ldr x9, [x0]
    cmp x9, x10
    b.ne capture_start+0x1290
    ldr w9, [x0, #0x20]
    cmp w9, #1
    b.ne capture_start+0x1290
    ldrb w9, [x0, #0x31]
    cmp w9, #1
    b.eq capture_start+0x12d0
    mov x9, #0x8a58
    ldr x0, [x19, x9]
    mov x1, #0
    bl address_4db3b0
    cbz x0, capture_start+0x12c8
    str xzr, [x0, #0x28]
    str xzr, [x0, #0x30]
    str xzr, [x0, #0x38]
    stur xzr, [x0, #0x3e]
    mov w9, #1
    str w9, [x0, #0x20]
    strb w9, [x0, #0x31]
    strb w9, [x0, #0x40]
    b capture_start+0x12d0
    mov x9, #0x7fb8
    add x0, x19, x9
    ldp x19, x30, [sp], #0x10
    autiasp
    ret
    b capture_start+0xc00
    b address_834288
// Full-resolution profiles and their labels in the native options list.
.org 0x1900
    ldr w28, [x7, #0x155c]
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    b capture_start+0x1de4
    b.ne capture_start+0x196c
    ldr w16, [x7, #0x1558]
    cmp w16, #2
    b.ne capture_start+0x196c
    ldr w16, [x7, #0x15f0]
    cmp w16, #3
    b.ne capture_start+0x196c
    ldr w16, [x7, #0x1560]
    cmp w16, #0xa
    b.ne capture_start+0x196c
    ldr w16, [x7, #0x1588]
    cmp w16, #3
    b.ne capture_start+0x196c
    ldr w16, [x7, #0x15d0]
    cmp w16, #0
    b.ne capture_start+0x196c
    ldr w16, [x7, #0x1ad0]
    cmp w16, #0
    b.ne capture_start+0x196c
    ldr w16, [x7, #0x1af8]
    cmp w16, #0
    b.ne capture_start+0x196c
    mov w28, #2
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b address_841ba8
    ldr w28, [x7, #0x155c]
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    b capture_start+0x1dfc
    b.ne capture_start+0x19e4
    ldr w16, [x7, #0x1558]
    cmp w16, #2
    b.ne capture_start+0x19e4
    ldr w16, [x7, #0x15f0]
    cmp w16, #3
    b.ne capture_start+0x19e4
    ldr w16, [x7, #0x1560]
    cmp w16, #0xa
    b.ne capture_start+0x19e4
    ldr w16, [x7, #0x1588]
    cmp w16, #3
    b.ne capture_start+0x19e4
    ldr w16, [x7, #0x15d0]
    cmp w16, #0
    b.ne capture_start+0x19e4
    ldr w16, [x7, #0x1ad0]
    cmp w16, #0
    b.ne capture_start+0x19e4
    ldr w16, [x7, #0x1af8]
    cmp w16, #0
    b.ne capture_start+0x19e4
    mov w28, #2
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b address_842b98
    ldr w25, [x5, #0x155c]
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    b capture_start+0x1e14
    b.ne capture_start+0x1a5c
    ldr w16, [x5, #0x1558]
    cmp w16, #2
    b.ne capture_start+0x1a5c
    ldr w16, [x5, #0x15f0]
    cmp w16, #3
    b.ne capture_start+0x1a5c
    ldr w16, [x5, #0x1560]
    cmp w16, #0xa
    b.ne capture_start+0x1a5c
    ldr w16, [x5, #0x1588]
    cmp w16, #3
    b.ne capture_start+0x1a5c
    ldr w16, [x5, #0x15d0]
    cmp w16, #0
    b.ne capture_start+0x1a5c
    ldr w16, [x5, #0x1ad0]
    cmp w16, #0
    b.ne capture_start+0x1a5c
    ldr w16, [x5, #0x1af8]
    cmp w16, #0
    b.ne capture_start+0x1a5c
    mov w25, #2
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b address_83b7b8
    ldr w25, [x5, #0x155c]
    stp x16, x17, [sp, #-0x10]!
    mrs x17, nzcv
    b capture_start+0x1e2c
    b.ne capture_start+0x1ad4
    ldr w16, [x5, #0x1558]
    cmp w16, #2
    b.ne capture_start+0x1ad4
    ldr w16, [x5, #0x15f0]
    cmp w16, #3
    b.ne capture_start+0x1ad4
    ldr w16, [x5, #0x1560]
    cmp w16, #0xa
    b.ne capture_start+0x1ad4
    ldr w16, [x5, #0x1588]
    cmp w16, #3
    b.ne capture_start+0x1ad4
    ldr w16, [x5, #0x15d0]
    cmp w16, #0
    b.ne capture_start+0x1ad4
    ldr w16, [x5, #0x1ad0]
    cmp w16, #0
    b.ne capture_start+0x1ad4
    ldr w16, [x5, #0x1af8]
    cmp w16, #0
    b.ne capture_start+0x1ad4
    mov w25, #2
    msr nzcv, x17
    ldp x16, x17, [sp], #0x10
    b address_83d118
    .byte 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0
    .byte 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0
    stp x19, x20, [sp, #-0x40]!
    stp x21, x22, [sp, #0x10]
    stp x23, x24, [sp, #0x20]
    str x30, [sp, #0x30]
    mov x21, x0
    mov w22, w1
    mov w23, w2
    bl capture_start+0x11a0
    mov x19, x0
    mov w24, #0
    cmp w22, #2
    b.ne capture_start+0x1bd0
    cmp w23, #3
    b.ne capture_start+0x1bd0
    mov x9, #0x8318
    add x9, x21, x9
    cmp x0, x9
    b.ne capture_start+0x1bd0
    ldr x20, [x21, #0x8b8]
    cbz x20, capture_start+0x1b84
    ldr x9, [x19]
    ldr x10, [x20]
    cmp x9, x10
    b.ne capture_start+0x1b84
    ldr w10, [x20, #0x20]
    cmp w10, #5
    b.ne capture_start+0x1b84
    ldr x10, [x20, #0x28]
    ldr x11, [x19, #0x28]
    orr x11, x11, #0x1000000
    orr x11, x11, #0x1000000000000
    cmp x10, x11
    b.eq capture_start+0x1bc8
    ldr x0, [x19]
    mov x1, #0
    bl address_4db3b0
    cbz x0, capture_start+0x1bd0
    mov x20, x0
    ldp x9, x10, [x19, #0x28]
    str x9, [x20, #0x28]
    str x10, [x20, #0x30]
    ldr x9, [x19, #0x38]
    str x9, [x20, #0x38]
    mov w9, #5
    str w9, [x20, #0x20]
    mov w9, #1
    strb w9, [x20, #0x2b]
    strb w9, [x20, #0x35]
    strb w9, [x20, #0x2e]
    strb w9, [x20, #0x38]
    mov x19, x20
    mov w24, #1
    mov x0, x19
    ldr x30, [sp, #0x30]
    ldp x23, x24, [sp, #0x20]
    ldp x21, x22, [sp, #0x10]
    ldp x19, x20, [sp], #0x40
    ret
    cmp w5, #6
    b.eq capture_start+0x1bfc
    cmp w5, #3
    b.eq capture_start+0x1bfc
    cmp w5, #1
    b capture_start+0x121c
    cmp w1, #3
    ccmp w1, #6, #4, ne
    b.ne capture_start+0x1c28
    cmp w2, #2
    b.ne capture_start+0x1c28
    cmp w3, #3
    b.ne capture_start+0x1c28
    cbnz w4, capture_start+0x1c28
    cbnz w6, capture_start+0x1c28
    mov w1, #1
    b capture_start+0x1234
    stp x0, x1, [sp, #-0x20]!
    str x30, [sp, #0x10]
    bl capture_start+0x1d24
    ldr x30, [sp, #0x10]
    ldp x0, x1, [sp], #0x20
    ldr x19, [x0, #0x68]
    b address_57b484
    stp x0, x1, [sp, #-0x20]!
    str x30, [sp, #0x10]
    bl capture_start+0x1d24
    ldr x30, [sp, #0x10]
    ldp x0, x1, [sp], #0x20
    ldr x19, [x0, #0x60]
    b address_57b604
    ldr x9, [x0, #0x10]
    add x10, x9, #0x840
    cmp x10, x0
    b.ne capture_start+0x1ce4
    ldr x10, [x9]
    adrp x11, address_712a000
    add x11, x11, #0x650
    cmp x10, x11
    b.ne capture_start+0x1ce4
    ldr x12, [x0, #0x78]
    cbz x12, capture_start+0x1cdc
    ldr x10, [x12, #0x10]
    cbnz x10, capture_start+0x1cdc
    mov x11, #0x8318
    add x11, x9, x11
    ldr x14, [x11]
    ldr x15, [x12]
    cmp x14, x15
    b.ne capture_start+0x1cdc
    ldr w14, [x12, #0x20]
    cmp w14, #5
    b.ne capture_start+0x1cdc
    ldr x14, [x12, #0x28]
    ldr x15, [x11, #0x28]
    orr x15, x15, #0x1000000
    orr x15, x15, #0x1000000000000
    cmp x14, x15
    b.ne capture_start+0x1cdc
    mov w9, #2
    ret
    mov w9, #1
    ret
    mov w9, #0
    ret
    stp x1, x30, [sp, #-0x10]!
    bl capture_start+0x1c64
    ldp x1, x30, [sp], #0x10
    cmp w9, #2
    b.ne capture_start+0x1d1c
    cmp w1, #5
    b.hs capture_start+0x1d1c
    adr x10, capture_start+0x1dc8
    ldrb w0, [x10, w1, uxtw]
    ldp x29, x30, [sp], #0x30
    autiasp
    ret
    ldr x12, [x0, #0x78]
    b address_57b250
    stp x19, x20, [sp, #-0x20]!
    str x30, [sp, #0x10]
    mov x19, x0
    bl capture_start+0x1c64
    cbz w9, capture_start+0x1d84
    mov w20, w9
    ldr x19, [x19, #0x68]
    cbz x19, capture_start+0x1d84
    add x10, x19, #0x40
    mov w11, #0x35
    adr x0, capture_start+0x1dcd
    cmp w20, #2
    b.eq capture_start+0x1d60
    mov w11, #0x37
    adr x0, capture_start+0x1ddd
    bl capture_start+0x1d90
    add x10, x19, #0x58
    mov w11, #0x33
    adr x0, capture_start+0x1dd5
    cmp w20, #2
    b.eq capture_start+0x1d80
    mov w11, #0x36
    adr x0, capture_start+0x1de0
    bl capture_start+0x1d90
    ldr x30, [sp, #0x10]
    ldp x19, x20, [sp], #0x20
    ret
    ldr x13, [x10]
    cbz x13, capture_start+0x1dc4
    ldrh w14, [x13]
    cmp w14, w11
    b.ne capture_start+0x1dc4
    ldrh w14, [x13, #2]
    cmp w14, #0x4b
    b.ne capture_start+0x1dc4
    stp x10, x30, [sp, #-0x10]!
    bl address_4e3e80
    ldp x10, x30, [sp], #0x10
    cbz x0, capture_start+0x1dc4
    str x0, [x10]
    ret
    .byte 0x1, 0x3, 0x6, 0x2, 0x4, 0x37, 0x4b, 0x20, 0x46, 0x75, 0x6c, 0x6c, 0x0, 0x36, 0x4b, 0x20
    .byte 0x46, 0x75, 0x6c, 0x6c, 0x0, 0x35, 0x4b, 0x0, 0x33, 0x4b, 0x0, 0x0
    cmp w28, #1
    b.eq capture_start+0x1df8
    cmp w28, #3
    b.eq capture_start+0x1df8
    cmp w28, #6
    b capture_start+0x1910
    cmp w28, #1
    b.eq capture_start+0x1e10
    cmp w28, #3
    b.eq capture_start+0x1e10
    cmp w28, #6
    b capture_start+0x1988
    cmp w25, #1
    b.eq capture_start+0x1e28
    cmp w25, #3
    b.eq capture_start+0x1e28
    cmp w25, #6
    b capture_start+0x1a00
    cmp w25, #1
    b.eq capture_start+0x1e40
    cmp w25, #3
    b.eq capture_start+0x1e40
    cmp w25, #6
    b capture_start+0x1a78
