/* X6 1.2.36, exact-hash integration only.
 * DetectStreamPresent's GetPadPtr()==NULL path formerly counted every context.
 * ReleasePad keeps the request-pad wrapper but clears its underlying GstPad.
 * Exclude only a null, request-owned, exact-name sub_stream. A live/inactive
 * GstPad follows the original gst_pad_is_active path. Do not consult the UI
 * preference: changing it mid-record must not reintroduce a released source.
 * No writes, calls, new lifetime state, or skipped main/audio drains.
 */
.text
.global x6_proxy_stop_presence
.type x6_proxy_stop_presence,%function
x6_proxy_stop_presence:
    cbnz x0, .Lnonnull
    ldr x9, [x21, #0x58]
    cbz x9, .Lnull
    ldrb w10, [x9, #0x48]
    cmp w10, #1
    b.ne .Lnull
    ldr x10, [x21, #0x70]
    cmp x10, #10
    b.ne .Lnull
    ldr x9, [x21, #0x68]
    cbz x9, .Lnull
    ldr x10, [x9]
    ldr x11, sub_name
    cmp x10, x11
    b.ne .Lnull
    ldrh w10, [x9, #8]
    mov w11, #0x6d61
    cmp w10, w11
    b.ne .Lnull
    b native_inactive
.Lnull:
    b native_null
.Lnonnull:
    b native_nonnull
.balign 8
sub_name:
    .ascii "sub_stre"
.size x6_proxy_stop_presence,.-x6_proxy_stop_presence
