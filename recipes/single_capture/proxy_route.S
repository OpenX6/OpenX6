.text
// The caller keeps the active group string at sp+24. Consult the preference
// only for an exact match in the table derived from the recording profile.
    ldr x9, [sp, #24]
    cbz x9, original
    ldp x10, x11, [x9]
    adr x12, groups
next_group:
    ldrb w13, [x12], #1
    cbz w13, original
    mov x14, x12
    add x12, x12, x13
    cmp x11, x13
    b.ne next_group
    mov x15, x10
compare_name:
    ldrb w16, [x15], #1
    ldrb w17, [x14], #1
    cmp w16, w17
    b.ne next_group
    subs x13, x13, #1
    b.ne compare_name

    sub sp, sp, #80
    stp x0, x1, [sp]
    stp x2, x30, [sp, #16]
    mov x0, #-100
    adr x1, preference
    mov x2, #0x20000
    mov x3, #0
    mov x8, #56
    svc #0
    cmp x0, #0
    b.lt restore
    mov x11, x0
    add x1, sp, #72
    mov x2, #2
    mov x8, #63
    svc #0
    mov x12, x0
    mov x0, x11
    mov x8, #57
    svc #0
    cmp x12, #1
    b.ne restore
    ldrb w9, [sp, #72]
    cmp w9, #1
    b.ne restore

    adr x9, alternate
    mov x10, #39
    stp x9, x10, [sp, #32]
    stp x10, xzr, [sp, #48]
    ldr x0, [sp]
    add x1, sp, #32
    ldr x2, [sp, #16]
    bl native_open_json
    cbz w0, restore
    ldr x30, [sp, #24]
    add sp, sp, #80
    ret
restore:
    ldp x0, x1, [sp]
    ldp x2, x30, [sp, #16]
    add sp, sp, #80
original:
    b native_open_json
preference:
    .asciz "/data/x6-owner-features/proxy-v1/disabled"
alternate:
    .asciz "/usr/share/x6-ff-pano-sole-main-v1.json"
groups:
    .incbin "recording_groups.table"
