.text
.global max_media
max_media:
    // bit 1 distinguishes Max; bit 0 is the original forced-High condition.
    tbz w23,#1,original_write
    ldr w10,[x19,#0x140] // native StreamConfig::stream_use_case
    cmp w10,#18         // kVideoPanoMain0
    b.eq check_type
    cmp w10,#19         // kVideoPanoMain1
    b.ne original_write
check_type:
    // Some timelapse YUV paths reuse the use-case enum. Match stream type too.
    ldr x10,[x19,#8]
    cmp x10,#11
    b.eq primary_type
    cmp x10,#15
    b.ne original_write
    adr x11,secondary_name
    b compare_type
primary_type:
    adr x11,primary_name
compare_type:
    ldr x12,[x19]
compare_byte:
    ldrb w13,[x12],#1
    ldrb w14,[x11],#1
    cmp w13,w14
    b.ne original_write
    subs x10,x10,#1
    b.ne compare_byte
apply_max:
    mov w9,#0x6800
    movk w9,#0x989,lsl #16 // 160,000,000 bps per primary stream
original_write:
    cbnz w9,write_nonzero
    b advance_stream
write_nonzero:
    b store_bitrate
primary_name:
    .asciz "main_stream"
secondary_name:
    .asciz "main_sec_stream"
