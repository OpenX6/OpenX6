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
    cbz w1,restore
    ldr w10,[x19,#0x140]
    cmp w10,#18
    b.eq check_type
    cmp w10,#19
    b.ne restore
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
    cmp w0,#2
    b.ne restore
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
    mov w2,#(ILOG_RATE & 65535)
    movk w2,#(ILOG_RATE >> 16),lsl #16
    csel w1,w2,w1,eq
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
