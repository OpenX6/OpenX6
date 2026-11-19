#include <stdint.h>
#include <time.h>
void anchor(uint64_t *p){
    struct timespec t;
    clock_gettime(CLOCK_MONOTONIC,&t);p[0]=t.tv_sec*1000000000ull+t.tv_nsec;
    asm volatile("isb; mrs %0,cntvct_el0":"=r"(p[1]));
    asm volatile("mrs %0,cntfrq_el0":"=r"(p[2]));
    clock_gettime(CLOCK_BOOTTIME,&t);p[3]=t.tv_sec*1000000000ull+t.tv_nsec;
    clock_gettime(CLOCK_MONOTONIC,&t);p[4]=t.tv_sec*1000000000ull+t.tv_nsec;
}
