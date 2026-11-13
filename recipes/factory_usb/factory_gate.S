// AArch64 hook for FactoryApiServer::CmdPacketProcessed.
// x0=this, x1=packet, w2=len, x8=std::vector<uint8_t> return storage.
// No globals, no libc socket ABI, and no persistent unlocked state.
// The offline builder resolves three external branch relocations only.
.text
.balign 16
.global owner_factory_gate
owner_factory_gate:
    paciasp
    sub sp, sp, #0xf00
    stp x29,x30,[sp]
    stp x19,x20,[sp,#16]
    stp x21,x22,[sp,#32]
    stp x23,x24,[sp,#48]
    stp x25,x26,[sp,#64]
    stp x27,x28,[sp,#80]
    mov x29,sp
    mov x19,x8
    mov x20,x0
    mov x21,x1
    mov w22,w2
    mov x23,#-1
    cbz w22,denied
    cmp w22,#1536
    b.hi denied
    // Frame the exact bytes for the root-only Unix broker.
    str w22,[sp,#0x100]
    add x0,sp,#0x104
    mov x1,x21
    mov w2,w22
    bl owner_memcpy
    mov x0,#1                  // AF_UNIX
    mov x1,#1
    movk x1,#8,lsl #16         // SOCK_CLOEXEC
    mov x2,#0
    mov x8,#198                // socket
    svc #0
    tbnz x0,#63,denied
    mov x23,x0
    // SO_RCVTIMEO and SO_SNDTIMEO: two seconds, fail closed.
    mov x9,#2
    stp x9,xzr,[sp,#0xc0]
    mov x1,#1
    mov x2,#20
    add x3,sp,#0xc0
    mov x4,#16
    mov x8,#208
    svc #0
    cbnz x0,denied
    mov x0,x23
    mov x1,#1
    mov x2,#21
    add x3,sp,#0xc0
    mov x4,#16
    mov x8,#208
    svc #0
    cbnz x0,denied
    mov x0,x23
    adr x1,sockaddr
    mov x2,#(sockaddr_end-sockaddr)
    mov x8,#203                // connect
    svc #0
    cbnz x0,denied
    mov x0,x23
    add x1,sp,#0x100
    add w2,w22,#4
    bl send_all
    cbnz x0,denied
    mov x0,x23
    add x1,sp,#0x800
    mov x2,#5
    bl read_all
    cbnz x0,denied
    ldrb w24,[sp,#0x800]
    add x9,sp,#0x801
    ldr w25,[x9]
    cmp w24,#1
    b.hi denied
    cbz w25,denied
    cmp w25,#1536
    b.hi denied
    mov x0,x23
    add x1,sp,#0x810
    mov w2,w25
    bl read_all
    cbnz x0,denied
    mov x0,x23
    mov x8,#57                 // close
    svc #0
    mov x23,#-1
    add x21,sp,#0x810
    cbz w24,reply_vector
    // Authenticated input only. Original parser, dispatcher, SYS handler and
    // original return vector remain in use; this is not a substitute shell.
    mov x0,x20
    mov x1,x21
    mov w2,w25
    mov x8,x19
    bl original_trampoline
    b finished
denied:
    tbnz x23,#63,no_fd
    mov x0,x23
    mov x8,#57
    svc #0
no_fd:
    adr x21,error_response
    mov w25,#(error_end-error_response)
reply_vector:
    mov w0,w25
    bl owner_alloc
    mov x26,x0
    add x9,x0,x25
    stp x0,x9,[x19]
    str x9,[x19,#16]
    mov x1,x21
    mov w2,w25
    bl owner_memcpy
finished:
    ldp x29,x30,[sp]
    ldp x19,x20,[sp,#16]
    ldp x21,x22,[sp,#32]
    ldp x23,x24,[sp,#48]
    ldp x25,x26,[sp,#64]
    ldp x27,x28,[sp,#80]
    add sp,sp,#0xf00
    autiasp
    ret
original_trampoline:
    .incbin "native-prologue.bin"
    b owner_original_resume
send_all:
    mov x13,#206               // sendto(MSG_NOSIGNAL)
    mov x3,#0x4000
    b transfer
read_all:
    mov x13,#63                // read
    mov x3,#0
transfer:
    mov x12,x0
    mov x9,x1
    mov x10,x2
    mov x11,#64                // bounded partial I/O and EINTR retries
io_loop:
    cbz x10,io_success
    subs x11,x11,#1
    b.mi io_fail
    mov x0,x12
    mov x1,x9
    mov x2,x10
    mov x4,#0
    mov x5,#0
    mov x8,x13
    svc #0
    cmn x0,#4
    b.eq io_loop
    cmp x0,#0
    b.le io_fail
    cmp x0,x10
    b.hi io_fail
    add x9,x9,x0
    sub x10,x10,x0
    b io_loop
io_fail:
    mov x0,#-1
    ret
io_success:
    mov x0,#0
    ret
.balign 8
sockaddr:
    .short 1
    .asciz "/run/x6-owner/auth.sock"
sockaddr_end:
error_response:
    .ascii "{\"cmd\":\"OWNER\",\"result\":-1,\"reason\":\"owner authorization unavailable\"}"
error_end:
