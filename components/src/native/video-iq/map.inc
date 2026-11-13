// Map a pre-created, stable preference inode once for this library instance.
.macro map_pref name, slot
 mov x0,#-100
 adr x1,\name
 mov x2,#0xa0000
 mov x8,#56
 svc #0
 tbnz x0,#63,9f
 mov x4,x0
 mov x0,#0
 mov x1,#1
 mov x2,#1
 mov x3,#1
 mov x5,#0
 mov x8,#222
 svc #0
 tbnz x0,#63,8f
 adrp x9,\slot
 str x0,[x9,:lo12:\slot]
8:
 mov x0,x4
 mov x8,#57
 svc #0
9:
.endm
