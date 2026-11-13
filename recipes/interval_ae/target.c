#include "../fast_capture/io.h"

extern U ae_cache[] __attribute__((visibility("hidden")));
extern void *get_common(unsigned);
extern void *get_algo(unsigned);

struct Permit { U time; I cap, reserved; };

static int auto_single(I cid) {
    char *algo = get_algo(cid);
    char *common = get_common(cid);
    if (!algo || !common) return 0;
    I mode = (unsigned char)algo[0x1c4f];
    return (mode == 0 || mode == 2) && *(unsigned short *)(common + 0xaa) == 1;
}

float ae_prepare(I cid, I fps, I kind, I mode, void *prev, I reason, float value) {
    (void)fps;
    (void)mode;
    (void)prev;
    if (cid >= 2) return value;
    U now = now_ms();
    if (kind == 0 && reason == 2 && now) {
        __atomic_store_n(&ae_cache[cid], now, __ATOMIC_RELEASE);
        return value;
    }
    if (kind != 1) return value;

    // Invalidate the previous still's limit even if the next request is manual or HDR.
    ae_cache[2 + cid] = 0;
    ae_cache[4 + cid] = 0;
    if (reason != 2 || !auto_single(cid) || !now) return value;
    struct Permit permit = {0, 0, 0};
    U preview = __atomic_load_n(&ae_cache[cid], __ATOMIC_ACQUIRE);
    if (!preview || now < preview || now - preview > 2000 ||
        !transfer("/run/x6-interval-controls/capture", &permit, sizeof(permit), 0) ||
        permit.reserved || permit.cap < 30 || permit.cap > 2000 ||
        !permit.time || now < permit.time || now - permit.time > 1500 ||
        ae_cache[6 + cid] == permit.time) return value;

    // Use the converged exposure target for a permitted still.
    char *algo = get_algo(cid);
    float target = *(float *)(algo + 0xefc);
    if (!(target > 0 && target < 100000000)) return value;
    ae_cache[6 + cid] = permit.time;
    ae_cache[2 + cid] = permit.cap;
    ae_cache[4 + cid] = now;
    return target;
}

float ae_cap(I cid, I kind, void *algo, float native) {
    (void)algo;
    if (cid >= 2 || kind != 1 || !auto_single(cid)) return native;
    U now = now_ms(), accepted = ae_cache[4 + cid], cap = ae_cache[2 + cid];
    return accepted && now >= accepted && now - accepted <= 500 && cap >= 30 && cap <= 2000
        ? (float)cap : native;
}
