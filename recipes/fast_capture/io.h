typedef unsigned long U;
typedef unsigned I;

static long sc(long nr, long a, long b, long c, long d) {
    register long x8 __asm__("x8") = nr;
    register long x0 __asm__("x0") = a, x1 __asm__("x1") = b;
    register long x2 __asm__("x2") = c, x3 __asm__("x3") = d;
    __asm__ volatile("svc 0" : "+r"(x0)
                     : "r"(x8), "r"(x1), "r"(x2), "r"(x3) : "memory", "cc");
    return x0;
}

static U now_ms(void) {
    long t[2];
    if (sc(113, 1, (long)t, 0, 0)) return 0;  // clock_gettime(CLOCK_MONOTONIC)
    return (U)t[0] * 1000 + (U)t[1] / 1000000;
}

// Runtime files are precreated by the root coordinator. Never create or follow links.
static int transfer(const char *path, void *buf, I size, int write) {
    long fd = sc(56, -100, (long)path, write ? 0x88801 : 0x88800, 0);  // openat
    if (fd < 0) return 0;
    U st[16];
    char *fields = (char *)st;
    int ok = sc(80, fd, (long)st, 0, 0) == 0  // fstat: regular, protected, root-owned, one link
          && (*(I *)(fields + 16) & 0170022) == 0100000
          && *(I *)(fields + 24) == 0 && *(I *)(fields + 20) == 1 && st[6] == size;
    if (ok) ok = sc(write ? 64 : 63, fd, (long)buf, size, 0) == size;
    sc(57, fd, 0, 0, 0);  // close
    return ok;
}
