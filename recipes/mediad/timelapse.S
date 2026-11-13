.text
.macro saveall
 sub sp,sp,#448
 stp x0,x1,[sp,#0]
 stp x2,x3,[sp,#16]
 stp x4,x5,[sp,#32]
 stp x6,x7,[sp,#48]
 stp x8,x9,[sp,#64]
 stp x10,x11,[sp,#80]
 stp x12,x13,[sp,#96]
 stp x14,x15,[sp,#112]
 stp x16,x17,[sp,#128]
 stp x18,x30,[sp,#144]
 mrs x9,nzcv
 str x9,[sp,#160]
 stp q0,q1,[sp,#192]
 stp q2,q3,[sp,#224]
 stp q4,q5,[sp,#256]
 stp q6,q7,[sp,#288]
 stp q8,q9,[sp,#320]
 stp q10,q11,[sp,#352]
 stp q12,q13,[sp,#384]
 stp q14,q15,[sp,#416]
.endm
.macro restoreall
 ldp q0,q1,[sp,#192]
 ldp q2,q3,[sp,#224]
 ldp q4,q5,[sp,#256]
 ldp q6,q7,[sp,#288]
 ldp q8,q9,[sp,#320]
 ldp q10,q11,[sp,#352]
 ldp q12,q13,[sp,#384]
 ldp q14,q15,[sp,#416]
 ldr x9,[sp,#160]
 msr nzcv,x9
 ldp x0,x1,[sp,#0]
 ldp x2,x3,[sp,#16]
 ldp x4,x5,[sp,#32]
 ldp x6,x7,[sp,#48]
 ldp x8,x9,[sp,#64]
 ldp x10,x11,[sp,#80]
 ldp x12,x13,[sp,#96]
 ldp x14,x15,[sp,#112]
 ldp x16,x17,[sp,#128]
 ldp x18,x30,[sp,#144]
 add sp,sp,#448
.endm

.balign 16
.global tl_rate_entry
tl_rate_entry:
 bti c
 saveall
 mov x0,x20
 mov w1,w25
 bl tl_rate
 str x0,[sp,#16]
 restoreall
 ret
