.text
.global max_record
max_record:
    // Preserve the original store's register/flag state around the native getter.
    sub sp,sp,#0x2b0
    stp x0,x1,[sp,#0]
    stp x2,x3,[sp,#0x10]
    stp x4,x5,[sp,#0x20]
    stp x6,x7,[sp,#0x30]
    stp x8,x9,[sp,#0x40]
    stp x10,x11,[sp,#0x50]
    stp x12,x13,[sp,#0x60]
    stp x14,x15,[sp,#0x70]
    stp x16,x17,[sp,#0x80]
    stp x18,x30,[sp,#0x90]
    stp q0,q1,[sp,#0xb0]
    stp q2,q3,[sp,#0xd0]
    stp q4,q5,[sp,#0xf0]
    stp q6,q7,[sp,#0x110]
    stp q8,q9,[sp,#0x130]
    stp q10,q11,[sp,#0x150]
    stp q12,q13,[sp,#0x170]
    stp q14,q15,[sp,#0x190]
    stp q16,q17,[sp,#0x1b0]
    stp q18,q19,[sp,#0x1d0]
    stp q20,q21,[sp,#0x1f0]
    stp q22,q23,[sp,#0x210]
    stp q24,q25,[sp,#0x230]
    stp q26,q27,[sp,#0x250]
    stp q28,q29,[sp,#0x270]
    stp q30,q31,[sp,#0x290]
    mrs x16,nzcv
    str x16,[sp,#0xa0]
    str wzr,[sp,#0xa8]
    cbz w1,restore
    // StreamConfigGroup* was spilled by the original caller; its first field
    // is the exact group_id string used by the factory stream parser.
    ldr x10,[sp,#0x2d8]
    cbz x10,stock_usecase
    ldp x11,x12,[x10]
    cbz x11,stock_usecase
    adr x13,active_groups
next_group:
    ldrb w14,[x13],#1
    cbz w14,stock_usecase
    cmp x12,x14
    b.ne skip_group
    mov x15,#0
compare_group:
    ldrb w16,[x11,x15]
    ldrb w17,[x13,x15]
    cmp w16,w17
    b.ne skip_group
    add x15,x15,#1
    cmp x15,x14
    b.lo compare_group
    ldr x10,[x19,#8]
    cmp x10,#11
    b.ne restore
    mov w10,#1
    str w10,[sp,#0xa8]
    b primary
skip_group:
    add x13,x13,x14
    b next_group
stock_usecase:
    ldr w10,[x19,#0x140]
    cmp w10,#18
    b.eq check_type
    cmp w10,#19
    b.eq check_type
    b restore
check_type:
    ldr x10,[x19,#8]
    cmp x10,#11
    b.eq primary
    cmp x10,#15
    b.ne restore
    adr x11,secondary_name
    b compare_type
primary:
    adr x11,primary_name
compare_type:
    ldr x12,[x19]
compare_byte:
    ldrb w13,[x12],#1
    ldrb w14,[x11],#1
    cmp w13,w14
    b.ne restore
    subs x10,x10,#1
    b.ne compare_byte
    adr x16,config_mgr_got
    ldr x16,[x16]
    ldr x0,[x16]
    cbz x0,restore
    bl native_get_config
    // tl::expected<4-byte bitrate config,int>: value then has_value byte.
    ubfx x16,x0,#32,#8
    cmp w16,#1
    b.ne restore
    cmp w0,#3
    b.eq custom_rate
    cmp w0,#2
    b.ne restore
max_policy:
    ldr w9,[sp,#0xa8]
    cbz w9,stock_max
    mov w1,#0x6740
    movk w1,#0x0e9a,lsl #16
    str x1,[sp,#8]
    b restore
stock_max:
    // Use the native effective color snapshot, not a stale UI string.
    adr x16,color_mgr_got
    ldr x0,[x16]
    cbz x0,restore
    add x16,x0,#0xdac
    ldarb w16,[x16]
    cbz w16,restore
    bl native_color
    cmp w0,#2
    b.hi restore
    cmp w0,#1
    mov w1,#30592
    movk w1,#1678,lsl #16
    mov w2,#42112
    movk w2,#1983,lsl #16
    csel w1,w2,w1,eq
    ldr w9,[sp,#0xa8]
    lsl w1,w1,w9
    str x1,[sp,#8]
    b restore
custom_rate:
    bl custom_read
    cbz w0,max_policy
    mov w1,#41248
    movk w1,#7,lsl #16
    mul w1,w0,w1
    ldr w9,[sp,#0xa8]
    lsl w1,w1,w9
    str x1,[sp,#8]
restore:
    ldr x16,[sp,#0xa0]
    msr nzcv,x16
    ldp q0,q1,[sp,#0xb0]
    ldp q2,q3,[sp,#0xd0]
    ldp q4,q5,[sp,#0xf0]
    ldp q6,q7,[sp,#0x110]
    ldp q8,q9,[sp,#0x130]
    ldp q10,q11,[sp,#0x150]
    ldp q12,q13,[sp,#0x170]
    ldp q14,q15,[sp,#0x190]
    ldp q16,q17,[sp,#0x1b0]
    ldp q18,q19,[sp,#0x1d0]
    ldp q20,q21,[sp,#0x1f0]
    ldp q22,q23,[sp,#0x210]
    ldp q24,q25,[sp,#0x230]
    ldp q26,q27,[sp,#0x250]
    ldp q28,q29,[sp,#0x270]
    ldp q30,q31,[sp,#0x290]
    ldp x0,x1,[sp,#0]
    ldp x2,x3,[sp,#0x10]
    ldp x4,x5,[sp,#0x20]
    ldp x6,x7,[sp,#0x30]
    ldp x8,x9,[sp,#0x40]
    ldp x10,x11,[sp,#0x50]
    ldp x12,x13,[sp,#0x60]
    ldp x14,x15,[sp,#0x70]
    ldp x16,x17,[sp,#0x80]
    ldp x18,x30,[sp,#0x90]
    add sp,sp,#0x2b0
    str w1,[x19,#0xb0]
    b continuation
primary_name:
    .asciz "main_stream"
secondary_name:
    .asciz "main_sec_stream"

custom_read:
 bti c
 stp x29,x30,[sp,#-32]!
 str x19,[sp,#16]
 mov x0,#-100
 adr x1,custom_path
 mov x2,#0x8800
 movk x2,#0x8,lsl #16
 mov x3,#0
 mov x8,#56
 svc #0
 tbnz x0,#63,custom_read_default
 mov x19,x0
 add x1,sp,#24
 mov x2,#5
 mov x8,#63
 svc #0
 mov x9,x0
 mov x0,x19
 mov x8,#57
 svc #0
 cmp x9,#4
 b.ne custom_read_default
 ldr w0,[sp,#24]
 cmp w0,#1
 b.lo custom_read_default
 b custom_max
 b.ls custom_read_end
custom_read_default:
 mov w0,#0
custom_read_end:
 ldr x19,[sp,#16]
 ldp x29,x30,[sp],#32
 ret

custom_path: .asciz "/data/x6-owner-features/custom-bitrate-v1/value"

ff_groups:
.incbin "groups.table"

// Custom ceiling and physical lens routing
    .byte 0
    .byte 0
custom_max:
    cmp w0, #0x154
    b.hi .-6204
    cmp w0, #0x140
    mov w9, #0x140
    csel w0, w0, w9, ls
    b .-6216
    cbz w15, .+16
    cmp w0, #0xc
    b.eq .+12
    cbz w0, .+8
    b .-6248708
    b .-6248132
    .byte 30
    .ascii "Front_FreeFrameVideo_8K_1x1_30"
    .byte 29
    .ascii "Back_FreeFrameVideo_8K_1x1_30"
    .incbin "groups.table"

// 8K proxy startup
    .byte 0
    ldp x0, x1, [x19]
    cmp x1, #0xa
    b.ne .+320
    sub sp, sp, #0x70
    stp x2, x3, [sp]
    stp x4, x5, [sp, #0x10]
    stp x6, x7, [sp, #0x20]
    stp x8, x9, [sp, #0x30]
    stp x10, x11, [sp, #0x40]
    mrs x9, nzcv
    str x9, [sp, #0x50]
    adr x2, .+292
    ldr x3, [x2]
    ldr x4, [x0]
    cmp x3, x4
    b.ne .+236
    ldrh w3, [x2, #8]
    ldrh w4, [x0, #8]
    cmp w3, w4
    b.ne .+220
    ldr x2, [sp, #0x98]
    ldp x3, x4, [x2]
    cmp x4, #0x1d
    b.eq .+20
    cmp x4, #0x1e
    b.ne .+196
    adr x5, .+271
    b .+8
    adr x5, .+234
    ldrb w6, [x3], #1
    ldrb w7, [x5], #1
    cmp w6, w7
    b.ne .+168
    subs x4, x4, #1
    b.ne .-20
    ldp w2, w3, [x19, #0x28]
    cmp w3, #0x3e9
    b.ne .+148
    cmp w2, #0x3e8
    mov w6, #0x7530
    ccmp w2, w6, #4, ne
    b.ne .+132
    mov x0, #-0x64
    adr x1, .+233
    mov x2, #0xa0000
    mov x3, #0
    mov x8, #0x38
    svc #0
    tbnz x0, #0x3f, .+64
    mov x7, x0
    add x1, sp, #0x60
    mov x2, #2
    mov x8, #0x3f
    svc #0
    mov x9, x0
    mov x0, x7
    mov x8, #0x39
    svc #0
    cmp x9, #1
    b.ne .+20
    ldrb w10, [sp, #0x60]
    cmp w10, #1
    mov w11, #0x3e8
    csel w6, w11, w6, eq
    str w6, [x19, #0x28]
    ldr x2, [x19, #0x108]
    cmp x2, #0x3c
    ccmp x2, #1, #4, ne
    b.ne .+24
    cmp w6, #0x3e8
    mov x2, #0x3c
    mov x3, #1
    csel x2, x3, x2, eq
    str x2, [x19, #0x108]
    ldr x9, [sp, #0x50]
    msr nzcv, x9
    ldp x2, x3, [sp]
    ldp x4, x5, [sp, #0x10]
    ldp x6, x7, [sp, #0x20]
    ldp x8, x9, [sp, #0x30]
    ldp x10, x11, [sp, #0x40]
    add sp, sp, #0x70
    ldp x0, x1, [x19]
    b .-6293984
    .ascii "sub_streamBack_FreeFrameVideo_8K_1x1_30Front_FreeFrameVideo_8K_1x1_30/data/x6-owner-features/proxy-v1/disabled"
    .byte 0
    .byte 0
    cbz w15, .+16
    b .+6740
    b.eq .+12
    b .+4
    b .-6684
    b .-6254800
    .byte 30
    .ascii "Front_FreeFrameVideo_5K_1x1_30"
    .byte 29
    .ascii "Back_FreeFrameVideo_5K_1x1_30"
    .byte 30
    .ascii "Front_FreeFrameVideo_8K_1x1_30"
    .byte 29
    .ascii "Back_FreeFrameVideo_8K_1x1_30"
    .incbin "groups.table"

// 7K Full proxy startup
    ldp x0, x1, [x19]
    cmp x1, #0xa
    b.ne .+336
    sub sp, sp, #0x70
    stp x2, x3, [sp]
    stp x4, x5, [sp, #0x10]
    stp x6, x7, [sp, #0x20]
    stp x8, x9, [sp, #0x30]
    stp x10, x11, [sp, #0x40]
    mrs x9, nzcv
    str x9, [sp, #0x50]
    adr x2, .+308
    ldr x3, [x2]
    ldr x4, [x0]
    cmp x3, x4
    b.ne .+252
    ldrh w3, [x2, #8]
    ldrh w4, [x0, #8]
    cmp w3, w4
    b.ne .+236
    ldr x2, [sp, #0x98]
    ldp x3, x4, [x2]
    cmp x4, #0x1d
    b.eq .+20
    cmp x4, #0x1e
    b.ne .+212
    adr x5, .+287
    b .+8
    adr x5, .+250
    ldrb w6, [x3], #1
    ldrb w7, [x5], #1
    cmp w6, w7
    b.eq .+20
    cmp w7, #0x38
    b.ne .+176
    cmp w6, #0x35
    b.ne .+168
    subs x4, x4, #1
    b.ne .-36
    ldp w2, w3, [x19, #0x28]
    cmp w3, #0x3e9
    b.ne .+148
    cmp w2, #0x3e8
    mov w6, #0x7530
    ccmp w2, w6, #4, ne
    b.ne .+132
    mov x0, #-0x64
    adr x1, .+233
    mov x2, #0xa0000
    mov x3, #0
    mov x8, #0x38
    svc #0
    tbnz x0, #0x3f, .+64
    mov x7, x0
    add x1, sp, #0x60
    mov x2, #2
    mov x8, #0x3f
    svc #0
    mov x9, x0
    mov x0, x7
    mov x8, #0x39
    svc #0
    cmp x9, #1
    b.ne .+20
    ldrb w10, [sp, #0x60]
    cmp w10, #1
    mov w11, #0x3e8
    csel w6, w11, w6, eq
    str w6, [x19, #0x28]
    ldr x2, [x19, #0x108]
    cmp x2, #0x3c
    ccmp x2, #1, #4, ne
    b.ne .+24
    cmp w6, #0x3e8
    mov x2, #0x3c
    mov x3, #1
    csel x2, x3, x2, eq
    str x2, [x19, #0x108]
    ldr x9, [sp, #0x50]
    msr nzcv, x9
    ldp x2, x3, [sp]
    ldp x4, x5, [sp, #0x10]
    ldp x6, x7, [sp, #0x20]
    ldp x8, x9, [sp, #0x30]
    ldp x10, x11, [sp, #0x40]
    add sp, sp, #0x70
    ldp x0, x1, [x19]
    b .-6300728
    .ascii "sub_streamBack_FreeFrameVideo_8K_1x1_30Front_FreeFrameVideo_8K_1x1_30/data/x6-owner-features/proxy-v1/disabled"
    .byte 0
    .byte 0
    cmp w0, #5
    b.eq .+8
    cmp w0, #0x16
    b .-6748
active_groups:
    .byte 30
    .ascii "Front_FreeFrameVideo_3K_1x1_30"
    .byte 29
    .ascii "Back_FreeFrameVideo_3K_1x1_30"
    .byte 30
    .ascii "Front_FreeFrameVideo_5K_1x1_30"
    .byte 29
    .ascii "Back_FreeFrameVideo_5K_1x1_30"
    .byte 30
    .ascii "Front_FreeFrameVideo_8K_1x1_30"
    .byte 29
    .ascii "Back_FreeFrameVideo_8K_1x1_30"
    .incbin "groups.table"

// 6K Full proxy startup
    .byte 0
    .byte 0
    .byte 0
    ldp x0, x1, [x19]
    cmp x1, #0xa
    b.ne .+344
    sub sp, sp, #0x70
    stp x2, x3, [sp]
    stp x4, x5, [sp, #0x10]
    stp x6, x7, [sp, #0x20]
    stp x8, x9, [sp, #0x30]
    stp x10, x11, [sp, #0x40]
    mrs x9, nzcv
    str x9, [sp, #0x50]
    adr x2, .+316
    ldr x3, [x2]
    ldr x4, [x0]
    cmp x3, x4
    b.ne .+260
    ldrh w3, [x2, #8]
    ldrh w4, [x0, #8]
    cmp w3, w4
    b.ne .+244
    ldr x2, [sp, #0x98]
    ldp x3, x4, [x2]
    cmp x4, #0x1d
    b.eq .+20
    cmp x4, #0x1e
    b.ne .+220
    adr x5, .+295
    b .+8
    adr x5, .+258
    ldrb w6, [x3], #1
    ldrb w7, [x5], #1
    cmp w6, w7
    b.eq .+28
    cmp w7, #0x38
    b.ne .+184
    cmp w6, #0x35
    b.eq .+12
    cmp w6, #0x33
    b.ne .+168
    subs x4, x4, #1
    b.ne .-44
    ldp w2, w3, [x19, #0x28]
    cmp w3, #0x3e9
    b.ne .+148
    cmp w2, #0x3e8
    mov w6, #0x7530
    ccmp w2, w6, #4, ne
    b.ne .+132
    mov x0, #-0x64
    adr x1, .+233
    mov x2, #0xa0000
    mov x3, #0
    mov x8, #0x38
    svc #0
    tbnz x0, #0x3f, .+64
    mov x7, x0
    add x1, sp, #0x60
    mov x2, #2
    mov x8, #0x3f
    svc #0
    mov x9, x0
    mov x0, x7
    mov x8, #0x39
    svc #0
    cmp x9, #1
    b.ne .+20
    ldrb w10, [sp, #0x60]
    cmp w10, #1
    mov w11, #0x3e8
    csel w6, w11, w6, eq
    str w6, [x19, #0x28]
    ldr x2, [x19, #0x108]
    cmp x2, #0x3c
    ccmp x2, #1, #4, ne
    b.ne .+24
    cmp w6, #0x3e8
    mov x2, #0x3c
    mov x3, #1
    csel x2, x3, x2, eq
    str x2, [x19, #0x108]
    ldr x9, [sp, #0x50]
    msr nzcv, x9
    ldp x2, x3, [sp]
    ldp x4, x5, [sp, #0x10]
    ldp x6, x7, [sp, #0x20]
    ldp x8, x9, [sp, #0x30]
    ldp x10, x11, [sp, #0x40]
    add sp, sp, #0x70
    ldp x0, x1, [x19]
    b .-6307536
    .ascii "sub_streamBack_FreeFrameVideo_8K_1x1_30Front_FreeFrameVideo_8K_1x1_30/data/x6-owner-features/proxy-v1/disabled"
    .byte 0
