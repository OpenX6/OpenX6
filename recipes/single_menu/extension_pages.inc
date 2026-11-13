// x19 = native page, x20 = row. All entry paths share vr_open's stack frame.
.balign 16
ext_home:
#ifndef OPENX6_LEGACY_EXTENSION_MENU
 adr x1,vnr_title
 adr x2,vnr_open
 mov x0,x19
 bl ext_add_arrow
#endif
 adr x1,ext_title
 adr x2,ext_open
 mov x0,x19
 bl ext_add_arrow
#ifdef OPENX6_STATS_EXTENSION
 b ext_after_stats
#else
 b .Lvisible_stats_row
#endif
ext_open:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 adr x1,ext_title
 bl ext_create_page
 cbz x0,open_done
 mov x19,x0
#ifdef OPENX6_CHOICES
#include "choice_arrows.inc"
#endif
#ifdef OPENX6_STATS_EXTENSION
#ifdef OPENX6_CHOICES
 mov x0,x19
#endif
 adr x1,ext_module_title
 adr x2,ext_open_module
 bl ext_add_arrow
#endif
 b open_done
#ifdef OPENX6_STATS_EXTENSION
ext_open_module:
 bti c
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 adr x1,ext_module_title
 bl ext_create_page
 cbz x0,open_done
 mov x19,x0
 b .Lvisible_stats_row
ext_finish_switch:
 stp x0,x1,[x9]
 b open_done

#endif
#ifdef OPENX6_CHOICES
.global ext_create_page
.hidden ext_create_page
#include "choice_descriptors.inc"
.balign 4
#endif

// x0 = parent, x1 = page title. Return the new native page or null.
ext_create_page:
 stp x29,x30,[sp,#-32]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 mov x19,x0
 mov x20,x1
 ldr x9,[x19,#0xa8]
 cbnz x9,1f
 adrp x1,page_class
 add x1,x1,:lo12:page_class
 mov w2,#0
 mov x3,#0
 mov x4,#0
 mov x5,#0
 mov x6,#0
 bl create_subpage
 ldr x19,[x19,#0xa8]
 cbz x19,1f
 adrp x9,page_class
 add x9,x9,:lo12:page_class
 ldr x10,[x19]
 cmp x9,x10
 b.ne 1f
 mov x0,x20
 bl new_string
 mov x1,x0
 add x0,x19,#0x338
 bl page_title
 mov x0,x19
 b 2f
1: mov x0,#0
2: ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#32
 ret

// x0 = page, x1 = label, x2 = click callback.
ext_add_arrow:
 stp x29,x30,[sp,#-48]!
 mov x29,sp
 stp x19,x20,[sp,#16]
 stp x21,x22,[sp,#32]
 mov x19,x0
 mov x20,x1
 mov x21,x2
 adrp x0,arrow_class
 add x0,x0,:lo12:arrow_class
 mov x1,#0
 bl new_object
 cbz x0,1f
 mov x22,x0
 mov x1,x0
 mov x0,x19
 mov w2,#0
 bl group_add
 mov x0,x20
 bl new_string
 mov x1,x0
 mov x0,x22
 bl arrow_title
 mov x0,x19
 mov x1,x21
 bl new_slot
 add x9,x22,#0x4c0
 stp x0,x1,[x9]
1: ldp x21,x22,[sp,#32]
 ldp x19,x20,[sp,#16]
 ldp x29,x30,[sp],#48
 ret
ext_title: .asciz "Extensions"
