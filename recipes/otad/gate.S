.arch armv8.5-a
.text
.balign 16
.global owner_gate, owner_version_gate, run_verifier, owner_transition

// Patch replaces ldr w1,[sp,#0x250]. Encrypted official path is unchanged.
owner_gate:
    .incbin "load-encoding.bin"
    cmp w1, #1
    b.eq .Lentry_encrypted
    cbnz w1, .Lentry_reject
    mov x0, x21                    // native std::string package path
    bl is_owner_path
    cbz w0, .Lentry_reject
    ldr x0, [x21]
    adr x1, mode_entry
    mov x2, #0
    bl run_verifier
    cbnz w0, .Lentry_reject
    bl owner_transition
    b entry_cleanup
.Lentry_encrypted:
    b entry_encrypted
.Lentry_reject:
    b entry_reject

// Called at the existing main-version-check callsite. No global policy byte
// is changed. Non-owner paths and other DevIds use the original function.
owner_version_gate:
    paciasp
    stp x29, x30, [sp, #-0x40]!
    mov x29, sp
    stp x0, x1, [sp, #0x10]
    str x2, [sp, #0x20]
    cmp w1, #4
    b.ne .Lversion_stock
    mov x0, x21                    // GetPackagePath result, not version string
    bl is_owner_path
    cbz w0, .Lversion_stock
    ldr x0, [x21]
    adr x1, mode_version
    ldr x2, [sp, #0x10]
    ldr x2, [x2]                   // native header version checked against package
    bl run_verifier
    cmp w0, #10
    b.eq .Lversion_downgrade
    cbnz w0, .Lversion_denied
    mov w2, #1                     // owner normal update: retain >= check
    b .Lversion_call
.Lversion_downgrade:
    mov w2, #0                     // verifier allows this specific lower-version package
    b .Lversion_call
.Lversion_stock:
    ldr x2, [sp, #0x20]
.Lversion_call:
    ldp x0, x1, [sp, #0x10]
    bl original_version
    b .Lversion_return
.Lversion_denied:
    mov w0, #-1                    // original caller's nonzero failure branch
.Lversion_return:
    ldp x29, x30, [sp], #0x40
    autiasp
    ret

// Exact C++ length + byte comparison (including NUL); no path prefix aliases.
is_owner_path:
    adr x1, owner_package
    ldr x2, [x0, #8]
    mov x3, #(owner_package_end-owner_package-1)
    cmp x2, x3
    b.ne .Lnot_owner
    ldr x0, [x0]
    add x2, x2, #1
.Lcompare:
    ldrb w3, [x0], #1
    ldrb w4, [x1], #1
    cmp w3, w4
    b.ne .Lnot_owner
    subs x2, x2, #1
    b.ne .Lcompare
    mov w0, #1
    ret
.Lnot_owner:
    mov w0, #0
    ret

// x0=path, x1=mode, x2=optional version. No shell or libc spawn function.
// Linux AArch64 clone(SIGCHLD), execve, wait4. Wait only for this exact child.
run_verifier:
    paciasp
    stp x29, x30, [sp, #-0x80]!
    mov x29, sp
    stp x19, x20, [sp, #0x10]
    adr x9, helper_path
    stp x9, x1, [sp, #0x30]
    stp x0, x2, [sp, #0x40]
    str xzr, [sp, #0x50]
    str wzr, [sp, #0x60]
    mov x0, #17                    // SIGCHLD only: private address space
    mov x1, #0
    mov x2, #0
    mov x3, #0
    mov x4, #0
    mov x8, #220                   // clone
    svc #0
    cmp x0, #0
    b.lt .Lspawn_denied
    b.eq .Lchild
    mov x19, x0
.Lwait:
    mov x0, x19
    add x1, sp, #0x60
    mov x2, #0
    mov x3, #0
    mov x8, #260                   // wait4(pid, &status, 0, NULL)
    svc #0
    cmn x0, #4                     // -EINTR: retry, never treat as success
    b.eq .Lwait
    cmp x0, x19
    b.ne .Lspawn_denied
    ldr w0, [sp, #0x60]
    tst w0, #0xff                  // normal exit only, reject signal/core dump
    b.ne .Lspawn_denied
    ubfx w0, w0, #8, #8
    b .Lspawn_return
.Lchild:
    adr x0, helper_path
    add x1, sp, #0x30
    mov x2, #0                     // empty environment, fixed absolute binary
    mov x8, #221                   // execve
    svc #0
    mov x0, #64
    mov x8, #93                    // exec failure: _exit(64)
    svc #0
    b .Lchild                      // unreachable if Linux exit succeeds
.Lspawn_denied:
    mov w0, #64
.Lspawn_return:
    ldp x19, x20, [sp, #0x10]
    ldp x29, x30, [sp], #0x80
    autiasp
    ret

// Same transition operations as the native encrypted path, but selects the
// existing Integrity state. It does NOT skip native integrity/model checks.
owner_transition:
    paciasp
    stp x29, x30, [sp, #-0x40]!
    mov x29, sp
    stp x19, x20, [sp, #0x10]
    adrp x19, current_got
    ldr x19, [x19, :lo12:current_got]
    ldr x0, [x19]
    cbz x0, .Ltransition_done
    ldr x8, [x0]
    ldr x8, [x8, #0x18]            // current OnExit
    blr x8
    adrp x20, integrity_got
    ldr x20, [x20, :lo12:integrity_got]
    str x20, [x19]
    adrp x0, hook_got
    ldr x0, [x0, :lo12:hook_got]
    ldr x8, [x0, #0x10]
    cbz x8, .Lenter
    ldr w8, [x20, #8]
    str w8, [sp, #0x30]
    add x1, sp, #0x30
    ldr x8, [x0, #0x18]            // native state-change std::function
    blr x8
.Lenter:
    ldr x0, [x19]
    ldr x8, [x0]
    ldr x8, [x8, #0x10]            // Integrity OnEntry launches native pipeline
    blr x8
.Ltransition_done:
    ldp x19, x20, [sp, #0x10]
    ldp x29, x30, [sp], #0x40
    autiasp
    ret

.balign 8
helper_path: .asciz "/usr/local/sbin/x6-owner-update-check"
owner_package: .asciz "/data/x6-owner-update/package.bin"
owner_package_end:
mode_entry: .asciz "entry"
mode_version: .asciz "version"
