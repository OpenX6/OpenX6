.text
.macro addr reg,sym
 adrp \reg,\sym
 add \reg,\reg,:lo12:\sym
.endm
.global make_rate_ref,show_rate,after_save,style_key
make_rate_ref:
 stp x8,x30,[sp,#-16]!
 addr x0,settings
 addr x1,settings_class
 bl get_auto
 addr x1,rate_get
 addr x2,rate_set
 ldp x8,x30,[sp],#16
 b new_ref

show_rate:
 bti c
 stp x19,x30,[sp,#-16]!
 mov x19,x0
 addr x0,settings
 addr x1,settings_class
 bl get_auto
 cbz x0,unknown
 bl rate_get
 cmp w0,#3
 b.eq custom
 cmp w0,#2
 b.eq max_rate
 cmp w0,#1
 b.hi unknown
 addr x1,std_label
 addr x2,high_label
 csel x0,x1,x2,lo
 bl localize
 b draw
custom:
 bl custom_label
 b draw
max_rate:
 adr x0,max_text
 b literal
unknown:
 adr x0,missing
literal:
 bl new_string
draw:
 mov x1,x0
 add x0,x19,#0x610
 ldp x19,x30,[sp],#16
 b set_subtitle

// A second save while Custom is selected changes the number but not the enum.
after_save:
 stp x29,x30,[sp,#-48]!
 str x0,[sp,#40]
 bl refresh_list
 add x8,sp,#16
 bl make_rate_ref
 add x0,sp,#16
 mov w1,#0
 bl notify
 ldr x0,[sp,#40]
 addr x1,save_slot
 bl confirm_key
 ldp x29,x30,[sp],#48
 ret
style_key:
 stp x19,x30,[sp,#-16]!
 mov x19,x0
 // Use the native rounded key background with an accent for the confirmation key.
 mov w1,#0x3434
 movk w1,#0xff34,lsl #16
 cmp w21,#11
 b.ne 1f
 mov w1,#0x7030
 movk w1,#0xff22,lsl #16
1:
 bl set_background_color
 addr x0,key_bitmap
 addr x1,bitmap_class
 bl load_resource
 mov x1,x0
 mov x0,x19
 bl set_bitmap
 mov x1,#144
 orr x1,x1,x1,lsl #32
 mov x0,x19
 bl set_width
 ldp x19,x30,[sp],#16
 ret
max_text: .asciz "Max"
missing: .asciz "--"

// Find this page's OK key by its click slot. The timer holds its EW receiver
// until the one-shot returns, including when the editor is closed immediately.
.balign 4
confirm_key:
 stp x19,x30,[sp,#-32]!
 stp x20,x21,[sp,#16]
 ldr x19,[x0,#0x60]
 addr x11,button_class
 mov x12,x1
1:
 cbz x19,3f
 ldr x9,[x19]
 cmp x9,x11
 b.ne 2f
 ldr x9,[x19,#0x2f0]
 cmp x9,x12
 b.eq 4f
2:
 ldr x19,[x19,#0x20]
 b 1b
4:
 ldr x9,[x19,#0xad8]
 cbz x9,3f
 ldrh w10,[x9]
 cmp w10,#79 // O of OK; repeated taps do not stack timers
 b.ne 3f
 addr x0,image_class
 mov x1,#0
 bl new_object
 cbz x0,3f
 mov x21,x0
 addr x0,confirm_bitmap
 addr x1,bitmap_class
 bl load_resource
 cbz x0,3f
 mov x1,x0
 mov x0,x21
 bl set_image
 mov x0,x21
 mov x1,#52
 movk x1,#24,lsl #32
 mov x2,#92
 movk x2,#64,lsl #32
 bl bounds
 addr x0,timer_class
 mov x1,#0
 bl new_object
 cbz x0,3f
 mov x20,x0
 bl lock_object
 mov x0,x21
 adr x1,reset_key
 bl new_slot
 stp x0,x1,[x20,#0x20]
 mov w9,#1400
 stp wzr,w9,[x20,#0x3c] // one shot, initial delay in ms
 mov x0,x20
 mov w1,#1
 bl timer_enabled
 mov x0,x19
 mov x1,x21
 mov w2,#0
 bl group_add
 mov x1,#0
 mov x0,x19
 bl set_key_text
3:
 ldp x20,x21,[sp,#16]
 ldp x19,x30,[sp],#32
 ret
reset_key:
 bti c
 stp x19,x30,[sp,#-32]!
 stp x0,x1,[sp,#16]
 ldr x19,[x0,#0x30]
 cbz x19,5f
 mov x1,x0
 mov x0,x19
 bl group_remove
 adr x0,ok_text
 bl new_string
 mov x1,x0
 mov x0,x19
 bl set_key_text
5:
 ldr x0,[sp,#24]
 ldp x19,x30,[sp],#32
 b unlock_object
ok_text: .asciz "OK"

.balign 4
.global rate_icon
// Reuse the native icon container so it follows the card's layout and hit area.
rate_icon:
 stp x19,x30,[sp,#-64]!
 stp x20,x21,[sp,#16]
 stp x22,x23,[sp,#32]
 str x24,[sp,#48]
 str xzr,[x22,#0x968]
 str xzr,[x19,#0x5f8]
 add x0,x19,#0x550
 mov x1,#0
 bl set_image
 add x19,x19,#0x480
 ldr w20,[x19,#0x58]
 ldr w9,[x19,#0x50]
 sub w20,w20,w9
 asr w20,w20,#1
 ldr w21,[x19,#0x5c]
 ldr w9,[x19,#0x54]
 sub w21,w21,w9
 asr w21,w21,#1
 adr x22,slider_shapes
 mov w23,#6
1:
 addr x0,rect_class
 mov x1,#0
 bl new_object
 cbz x0,2f
 mov x24,x0
 mov w1,#-1
 bl rect_color
 ldrsb w9,[x22]
 ldrsb w10,[x22,#1]
 ldrsb w11,[x22,#2]
 ldrsb w12,[x22,#3]
 add w9,w9,w20
 add w10,w10,w21
 add w11,w11,w20
 add w12,w12,w21
 orr x1,x9,x10,lsl #32
 orr x2,x11,x12,lsl #32
 mov x0,x24
 bl bounds
 mov x0,x19
 mov x1,x24
 mov w2,#0
 bl group_add
 add x22,x22,#4
 subs w23,w23,#1
 b.ne 1b
2:
 ldr x24,[sp,#48]
 ldp x22,x23,[sp,#32]
 ldp x20,x21,[sp,#16]
 ldp x19,x30,[sp],#64
 b icon_next
slider_shapes:
 .byte -16,-12,16,-9, -9,-15,-4,-6
 .byte -16,-1,16,2, 4,-4,9,5
 .byte -16,10,16,13, -6,7,-1,16

// Match the first two cards to the spacing and rounded background of Customize Button.
.global insert_mod_row,insert_quick_row
.balign 4
insert_mod_row:
 stp x19,x30,[sp,#-32]!
 stp x1,x2,[sp,#16]
 bl add_behind
 ldr x0,[sp,#16]
 mov w1,#1
 bl set_row_type
 ldr x0,[sp,#24]
 mov w1,#16
 ldp x19,x30,[sp],#32
 b set_row_top_padding

// Place the shortcut immediately before the native Display Settings entry.
insert_quick_row:
 stp x0,x30,[sp,#-32]!
 str x1,[sp,#16]
 mov x0,x1
 mov w1,#3
 bl set_row_type
 ldr x1,[sp,#16]
 ldp x0,x30,[sp],#32
 mov x2,#0x3e68
 add x2,x0,x2
 b add_behind

.global after_interval_save
after_interval_save:
 stp x0,x30,[sp,#-16]!
 bl interval_relabel
 ldp x0,x30,[sp],#16
 addr x1,interval_slot
 b confirm_key
