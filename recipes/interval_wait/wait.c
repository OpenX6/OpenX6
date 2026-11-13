#include "../fast_capture/io.h"

I wait_budget(I original) {
    if (!original) return 0;
    struct { U time; I cap, reserved; } capture;
    if (!transfer("/run/x6-interval-controls/capture", &capture, sizeof(capture), 0) ||
        capture.reserved) return original;
    U now = now_ms();
    if (!now || !capture.time || now < capture.time || now - capture.time > 1500) return original;
    I cap = capture.cap;
    if (cap != 125 && cap != 250 && cap != 500 && cap != 1000 && cap != 2000) return original;
    // The native acquisition path budgets twice the exposure, in milliseconds.
    I desired = 1000 + cap * 2;
    return original > desired ? original : desired;
}
