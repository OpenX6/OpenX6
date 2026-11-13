// Format admission, color selection and iLog bitrate for the additional capture groups.
.text
.balign 4
capture_mode_17:
    cbz w15, loc_90d4f0
    cmp w0, #0x11
    b.ne loc_90d4f0
    b format_continue
loc_90d4f0:
    b format_previous
    .incbin "legacy-single.table"
    .incbin "legacy-proxy.table"
    .balign 16, 0
capture_mode_6:
    cbz w15, loc_90f870
    cmp w0, #6
    b.ne loc_90f870
    b format_continue
loc_90f870:
    b capture_mode_17
    .incbin "capture-single.table"
    .incbin "85.table"
    .balign 16, 0
capture_color:
    ldr w17, [x15, #0x60]
    cbnz w17, loc_911d00
    ldr w17, [x15, #4]
    eor w17, w17, #0x8000
    sub w17, w17, #0x12
    cbnz w17, loc_911cfc
    ldr w17, [x15, #0x1c]
    sub w17, w17, #0xa
    cmp w17, #2
    b.hi loc_911cfc
    ldr w17, [x15]
    cmp w17, #1
    b.hi loc_911cfc
    ldr x17, [sp, #0x40]
    ldr x17, [x17, #0xa88]
    cbz x17, loc_911cfc
    add x16, x17, #0xdac
    ldarb w16, [x16]
    cbz w16, loc_911cfc
    stp x15, x30, [sp, #-0x10]!
    mov x0, x17
    bl get_color
    cmp w0, #0
    cset w17, eq
    ldp x15, x30, [sp], #0x10
    mov w0, #4
    b color_continue
loc_911cfc:
    ldr w17, [x15, #0x60]
loc_911d00:
    b color_continue
    .incbin "86.table"
    .incbin "87.table"
    .balign 16, 0
capture_rate:
    b.ne loc_913298
    ldr x10, [sp, #0x2d8]
    cbz x10, loc_91329c
    ldp x11, x12, [x10]
    cbz x11, loc_91329c
    cmp x12, #0x19
    b.ne loc_91329c
    adr x13, crop_group
loc_913250:
    ldrb w14, [x11], #1
    ldrb w15, [x13], #1
    cmp w14, w15
    b.ne loc_91329c
    subs x12, x12, #1
    b.ne loc_913250
    adrp x11, media_got
    ldr x0, [x11, #0xa88]
    cbz x0, loc_913298
    add x16, x0, #0xdac
    ldarb w16, [x16]
    cbz w16, loc_913298
    bl get_color
    cmp w0, #1
    b.ne loc_9132a0
    mov w1, #0x3b00
    movk w1, #0x858, lsl #16
    str x1, [sp, #8]
loc_913298:
    b store_rate
loc_91329c:
    b regular_rate
loc_9132a0:
    b max_rate
crop_group:
    .ascii "All_NormalVideo_5K_1x1_60"
    .incbin "88.table"
    .incbin "89.table"
