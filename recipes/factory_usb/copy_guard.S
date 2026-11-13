.text
.global copy_guard
copy_guard:
    cmp w19, #1536
    b.ls valid
    mov w19, #0
    b reject_copy
valid:
    .incbin "payload-address.bin"
    b continue_copy
