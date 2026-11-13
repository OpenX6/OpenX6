// Select the centered 3712-pixel main pass for the two matching IPE nodes.
.text
    mov w8, #0xf80
    eor w9, w24, w8
    eor w10, w23, w8
    orr w9, w9, w10
    cbnz w9, resume
    ldp w9, w10, [x22, #8]
    eor w9, w9, w8
    eor w10, w10, w8
    orr w9, w9, w10
    cbnz w9, resume
    ldr x5, [x29, #0x68]
    ldr x8, [x5]
    mov x9, #0xe80
    orr x9, x9, x9, lsl #32
    eor x8, x8, x9
    cbnz x8, resume
    add x8, x21, #0x2b0
    adr x9, node_prefix
prefix_loop:
    ldrb w10, [x9], #1
    cbz w10, index
    ldrb w11, [x8], #1
    eor w10, w10, w11
    cbnz w10, resume
    b prefix_loop
index:
    ldrb w10, [x8], #1
    sub w10, w10, #0x32
    and w10, w10, #0xfffffffd
    cbnz w10, resume
    adr x9, node_suffix
suffix_loop:
    ldrb w10, [x9], #1
    ldrb w11, [x8], #1
    eor w11, w10, w11
    cbnz w11, resume
    cbnz w10, suffix_loop
    mov w8, #0x80
    stp w8, w8, [x22]
    mov w8, #0xe80
    stp w8, w8, [x22, #8]
resume:
    ldr x5, [x29, #0x68]
    b hal_continue
node_prefix:
    .incbin "path-prefix.bin"
    .asciz "NoStitchingOfflineMainVideo0_IPE"
node_suffix:
    .asciz "_cam0_MainPass_Default_Ins0"
    .byte 0
