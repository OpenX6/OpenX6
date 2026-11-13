#include "paths.h"
.text
.global labels, mask_attach, mask_get, store_selection, read_first, read_again, wire
// The stock object already owns five GC-traced slots in each of its three lists.
labels:
 stp x19,x30,[sp,#-16]!
 mov x19,x28
 mov x9,#0x5340
 add x9,x19,x9
 mov w10,#3
 str w10,[x9,#0x34]
 mov w10,#4
 str w10,[x9,#0x38]
 .macro label offset, slot, text
 adr x0,\text
 bl new_string
 mov x1,x0
 mov x0,#\offset
 add x0,x19,x0
 bl \slot
 .endm
 label 0x5250, string3, extra
 label 0x5250, string4, minimal
 label 0x52c8, string3, extra_desc
 label 0x52c8, string4, min_desc
 ldp x19,x30,[sp],#16
 ldr x0,[sp,#0x170]
 b labels_next
mask_attach:
 mov x7,#0x9b68
 cbz w0,1f
 cmp w0,#2
 b.ne 2f
1: mov x7,#0x8e70
2: b attach_next
mask_get:
 mov x4,#0x9b68
 cbz w1,1f
 cmp w1,#2
 b.ne 2f
1: mov x4,#0x8e70
2: b get_next

// The camera stores a native Low. Keep the video-only extension in one byte.
// Never truncate or replace its inode: the IQ process maps this byte read-only.
store_selection:
 ldr w9,[x19,#0x1558]
 cbz w9,1f
 cmp w9,#2
 b.ne 3f
1: stp x1,x30,[sp,#-32]!
 strb w1,[sp,#16]
 cmp w1,#2
 b.hi 2f
 strb wzr,[sp,#16]
2: mov x0,#-100
 adr x1,pref
 mov x2,#1
 movk x2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,write_error
 mov x10,x0
 add x1,sp,#16
 mov w2,#1
 mov x8,#64
 svc #0
 cmp x0,#1
 b.ne 4f
 mov x0,x10
 mov x8,#83
 svc #0
 cmp x0,#0
 cset x0,eq
4: mov x9,x0
 mov x0,x10
 mov x8,#57
 svc #0
 cmp x9,#1
 b.ne write_error
 ldp x1,x30,[sp],#32
3: stp x21,x22,[sp,#0x60]
 b store_next
write_error:
 ldp x1,x30,[sp],#32
 b store_abort
read_first:
 ldr w0,[x0,#0x44]
 cbnz w0,1f
 bl read_selected
1: b read_first_next
read_again:
 ldr w5,[x0,#0x44]
 cbnz w5,1f
 bl read_selected
 mov w5,w0
1: b read_again_next
read_selected:
 // Old link register has already been saved by the native event handler.
 ldr w9,[x19,#0x1558]
 cbz w9,1f
 cmp w9,#2
 b.ne fallback
1: sub sp,sp,#16
 mov x0,#-100
 adr x1,pref
 mov x2,#0
 mov x8,#56
 svc #0
 tbnz x0,#63,2f
 mov x10,x0
 mov x1,sp
 mov x2,#1
 mov x8,#63
 svc #0
 cmp x0,#1
 ldrb w9,[sp]
 csel w9,w9,wzr,eq
 mov x0,x10
 mov x8,#57
 svc #0
 sub w10,w9,#3
 cmp w10,#1
 csel w0,w9,wzr,ls
 add sp,sp,#16
 ret
2: add sp,sp,#16
fallback:
 mov w0,#0
 ret
wire:
 cmp w2,#2
 csel w2,w2,wzr,ls
 b send_bll
extra: .asciz "Extra Low"
minimal: .asciz "Minimal"
extra_desc: .asciz "Video sharpening strength: 0.30"
min_desc: .asciz "Video sharpening strength: 0"
pref: .asciz SHARP

.balign 4
.global nr_write, nr_read_path
nr_read_path:
 adr x1,noise
 b nr_read_next
nr_write:
 cmp w0,#3
 b.hi nr_invalid
 sub sp,sp,#16
 strb w0,[sp]
 mov x0,#-100
 adr x1,noise
 mov x2,#1
 movk x2,#0xa,lsl #16
 mov x8,#56
 svc #0
 tbnz x0,#63,nr_exit
 mov x10,x0
 mov x1,sp
 mov x2,#1
 mov x8,#64
 svc #0
 cmp x0,#1
 b.ne nr_failed
 mov x0,x10
 mov x8,#83
 svc #0
 b nr_close
nr_failed:
 mov x0,#-1
nr_close:
 mov x9,x0
 mov x0,x10
 mov x8,#57
 svc #0
 cbnz x0,nr_exit
 mov x0,x9
nr_exit:
 add sp,sp,#16
 ret
nr_invalid:
 mov x0,#-1
 ret
noise: .asciz NOISE
