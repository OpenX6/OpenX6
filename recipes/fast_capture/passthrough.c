#include "io.h"
/* Replaces only the strtol() of property insta.iis.binopureshot.passthrough in
 * BinoPureShotNode::Process. A non-zero native property is returned unchanged.
 * Otherwise bypass only when mediad issued a fresh 8K-fast permit for the current
 * capture: interval captures arm it per capture; every other photo capture clears it
 * in mediad before its frames reach this node. (8K normal photos and 8K interval
 * frames carry the same ui mode, so the ui mode cannot be used here.) */
extern long strtol_plt(const char *, char **, int);

struct FastPermit {
    U time;
    I flags, tag;
};

#define FAST_TAG 0x4b384631u

long x6_passthrough(const char *s, char **end, int base, void *request_ref) {
    long native = strtol_plt(s, end, base);
    if (native) return native;
    (void)request_ref;
    struct FastPermit permit = {0, 0, 0};
    if (!transfer("/run/x6-interval-controls/fast8k-capture", &permit, sizeof(permit), 0)) return 0;
    U now = now_ms();
    if (permit.tag != FAST_TAG || permit.flags != 1 || !permit.time || !now ||
        now < permit.time || now - permit.time > 30000) return 0;
    return 1;
}
