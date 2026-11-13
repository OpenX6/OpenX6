.text
.balign 16
.global fish_dispatch
fish_dispatch:
    cbz x0, native_single_eye
    cbz x1, native_single_eye
    ldr w9, [x1, #16]
    cmp w9, #3
    b.ne native_single_eye
    b fish_8bit
