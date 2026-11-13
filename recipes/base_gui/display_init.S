.text
    adr x0, version_label
    bl EwNewStringAnsi
    mov x1, x0
    mov x0, x24
    bl WidgetsSetEntryDualText_OnSetStringDesc
    nop
    nop
