.text
.balign 16
.global x6_passthrough_entry
x6_passthrough_entry:
 bti c
 mov x3,x20
 b x6_passthrough
