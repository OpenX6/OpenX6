.text
    b load_help
    b refresh_help

load_help:
    adrp x2, ew_language
    ldr w2, [x2, :lo12:ew_language]
    cmp w2, #1
    b.eq simplified
    cmp w2, #2
    b.eq traditional
    adr x0, en
    mov w1, #(en_end - en)
    b new_utf8
simplified:
    adr x0, zh
    mov w1, #(zh_end - zh)
    b new_utf8
traditional:
    adr x0, tw
    mov w1, #(tw_end - tw)
    b new_utf8

// ReInit refreshes the translated PureShot+RAW row. Also refresh its DNG neighbor.
refresh_help:
    stp x19, x30, [sp, #-16]!
    mov x19, x0
    bl set_option03
    bl load_help
    mov x1, x0
    mov x0, x19
    ldp x19, x30, [sp], #16
    b set_option04

en: .ascii "DNGs are saved to your card or drive. The camera album cannot display them."
en_end:
zh: .ascii "DNG 保存在存储卡或所选存储中，无法在机身相册查看。"
zh_end:
tw: .ascii "DNG 會存到記憶卡或所選儲存裝置，無法在機身相簿查看。"
tw_end:
