// The native root receives display-oriented coordinates for each contact.
// Only capture a press starting on the displayed SoC row. The other controls
// retain the original handler, including taps in the same place on a menu.
.balign 4
thermal_hit:
 bti c
 stp x0,x1,[sp,#-64]!
 stp x2,x3,[sp,#16]
 str x30,[sp,#32]                  // PAC-signed by the original entry
 adrp x12,thermal_contact
 add x12,x12,:lo12:thermal_contact
 ldr w13,[x12]
 cbnz w1,thermal_down
 cbz w13,thermal_pass
 sub w13,w13,#1
 cmp w13,w2
 b.ne thermal_pass
 str wzr,[x12]
 ldr x13,[x12,#8]
 sub w14,w3,w13
 add w14,w14,#10
 cmp w14,#20
 b.hi thermal_used
 asr x15,x13,#32
 asr x14,x3,#32
 sub w14,w14,w15
 add w14,w14,#10
 cmp w14,#20
 b.hi thermal_used
 bl thermal_on_row
 cbz w0,thermal_used
 mov x0,#-100
 adr x1,thermal_path
 mov x2,#0
 mov x8,#35
 svc #0
 cmn x0,#2
 b.ne thermal_used
 mov x0,#-100
 adr x1,thermal_path
 mov x2,#0xc1                        // O_WRONLY|O_CREAT|O_EXCL
 mov x3,#384
 mov x8,#56
 svc #0
 tbnz x0,#63,thermal_used
 mov x8,#57
 svc #0
 b thermal_used
thermal_down:
 cbnz w13,thermal_pass
 bl thermal_on_row
 cbz w0,thermal_pass
 ldp x2,x3,[sp,#16]
 adrp x12,thermal_contact
 add x12,x12,:lo12:thermal_contact
 add w13,w2,#1
 str w13,[x12]
 str x3,[x12,#8]
thermal_used:
 ldr x30,[sp,#32]
 add sp,sp,#64
 mov w0,#1
 autiasp
 ret
thermal_pass:
 ldr x30,[sp,#32]
 ldp x2,x3,[sp,#16]
 ldp x0,x1,[sp],#64
 stp x29,x30,[sp,#-0x90]!           // intercepted native instruction
 b thermal_hit_resume

thermal_on_row:
 stp x19,x30,[sp,#-16]!
 mov x19,x3
 bl thermal_capture_page
 cbz w0,thermal_no
 adrp x9,stats_globals
 add x9,x9,:lo12:stats_globals
 ldr w10,[x9,#8]
 cbz w10,thermal_no
 ldr x9,[x9,#32]
 cbz x9,thermal_no
 add x9,x9,#0xe0
 ldr x12,[x9,#0x80]                // displayed UTF-16, owned by GUI thread
 cbz x12,thermal_no
 ldp w13,w14,[x9,#0x50]
 sub w11,w19,w13
 cmp w11,#137
 b.hi thermal_no
 asr x15,x19,#32
 sub w15,w15,w14
 tbnz w15,#31,thermal_no
 mov w13,#0                         // find the actual SoC line
 mov w14,#0
thermal_line:
 ldrh w10,[x12]
 cmp w10,#83                        // S
 b.ne thermal_skip
 ldrh w10,[x12,#2]
 cmp w10,#111                       // o
 b.ne thermal_skip
 ldrh w10,[x12,#4]
 cmp w10,#67                        // C
 b.eq thermal_found
thermal_skip:
 ldrh w10,[x12],#2
 cbz w10,thermal_no
 add w14,w14,#1
 cmp w14,#220
 b.hs thermal_no
 cmp w10,#10
 b.ne thermal_skip
 add w13,w13,#31                    // native small font line height
 b thermal_line
thermal_found:
 sub w15,w15,w13
 cmp w15,#30
 cset w0,ls
 b thermal_row_done
thermal_no:
 mov w0,#0
thermal_row_done:
 ldp x19,x30,[sp],#16
 ret

// At first launch the focus manager may still have a null page. Use the
// application's mounted active page in that case; the caller checks its class.
thermal_focus:
 ldr x0,[x0,#0x28]
 cbnz x0,thermal_focus_done
 stp x29,x30,[sp,#-16]!
 adrp x0,thermal_app_holder
 add x0,x0,:lo12:thermal_app_holder
 adrp x1,thermal_app_class
 add x1,x1,:lo12:thermal_app_class
 bl get_auto
 cbz x0,thermal_focus_return
 ldr x0,[x0,#0x7e8]
 cbz x0,thermal_focus_return
 ldr x9,[x0,#0x30]
 cmp x9,#0
 csel x0,x0,xzr,ne
thermal_focus_return:
 ldp x29,x30,[sp],#16
thermal_focus_done:
 ret
thermal_path: .asciz "/run/x6-stats-overlay/thermal-expanded"
