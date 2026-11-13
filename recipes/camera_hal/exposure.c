#include <stdint.h>

typedef struct {
    uint32_t addr, addr_type, data_type, op, delay, pad;
    uint32_t data, pad2;
    uint64_t extra;
} Reg;
typedef struct { uint32_t count, pad; Reg *regs; } Info;
_Static_assert(sizeof(Reg)==40, "Pinned native ABI");
_Static_assert(sizeof(Info)==16, "Pinned native ABI");
extern void owner_log(uint64_t, const char *, unsigned, const char *, ...);

// Native SensorNode::CreatePerFrameUpdateResources allocates 50 Reg slots.
// Invoked after the original FES through the three pinned SensorNode call sites.
int fix_exposure(Info *out, uint32_t mode, uint32_t role,
                 uint32_t requested_lines, uint32_t requested_fll) {
    if (mode!=51 || (role!=1 && role!=2)) return 0;
    if (!out || !out->regs || out->count<2 || out->count>46) return -1;
    Reg *r=out->regs;
    uint32_t n=out->count, seen=0;
    Reg *cit_hi=0,*cit_lo=0,*fll_hi=0,*fll_lo=0,*cit_shift=0,*fll_shift=0;
    for (uint32_t i=0;i<n;i++) {
        Reg *x=r+i; unsigned bit=0;
        if (x->addr==0x202) {cit_hi=x;bit=1;}
        else if (x->addr==0x203) {cit_lo=x;bit=2;}
        else if (x->addr==0x340) {fll_hi=x;bit=4;}
        else if (x->addr==0x341) {fll_lo=x;bit=8;}
        else if (x->addr==0x3160) {cit_shift=x;bit=16;}
        else if (x->addr==0x3161) {fll_shift=x;bit=32;}
        if (bit) {
            if ((seen&bit) || x->addr_type!=2 || x->data_type!=1 || x->op || x->data>255) return -2;
            seen|=bit;
        }
    }
    if (seen!=63 || cit_shift->data>9 || fll_shift->data>15) return -3;
    Reg last=r[n-1];
    if (last.addr!=0x104 || last.data || last.addr_type!=2 || last.data_type!=1 || last.op || last.delay) return -4;
    uint32_t cs=cit_shift->data;
    uint64_t cit=(uint64_t)((cit_hi->data<<8)|cit_lo->data)<<cs;
    uint32_t fs=fll_shift->data;
    // The native FES divides BOTH CIT and FLL by CIT_LSHIFT first;
    // FLL_LSHIFT is an additional frame-only shift.
    uint32_t full_shift=cs+fs;
    uint64_t fll=(uint64_t)((fll_hi->data<<8)|fll_lo->data)<<full_shift;
    if (!cit || cit>0x7fffff00ULL) return -5;
    uint64_t phase=cit+32;
    if (phase<1072) phase=1072;
    // Keep both timing counters inside the actual physical frame period.
    uint64_t min_fll=phase+32;
    uint64_t reg_fll=fll>>full_shift;
    if (fll<min_fll) {
        reg_fll=(min_fll+((1ULL<<full_shift)-1))>>full_shift;
        while (reg_fll>65535 && fs<15) {
            fs++;
            full_shift=cs+fs;
            reg_fll=(min_fll+((1ULL<<full_shift)-1))>>full_shift;
        }
        if (reg_fll>65535) return -6;
    }
    uint64_t old_fll=fll;
    fll=reg_fll<<full_shift;
    uint32_t counter=(uint32_t)phase-(role==2?8:0);
    // Commit only after all validation. Hold-off remains the final write.
    fll_hi->data=(uint32_t)(reg_fll>>8);
    fll_lo->data=(uint32_t)(reg_fll&255);
    fll_shift->data=fs;
    r[n+3]=last;
    for (uint32_t i=0;i<4;i++) {
        r[n-1+i]=last;
        r[n-1+i].addr=0x3058+i;
        r[n-1+i].data=(counter>>(24-i*8))&255;
    }
    out->count=n+4;
    owner_log(2,"[ INFO]",4,
       "X6_SYNC_EXPOSURE mode=%u role=%u req=%u cit=%llu cshift=%u phase=%u fll=%llu->%llu fshift=%u reqfll=%u regs=%u",
       mode,role,requested_lines,cit,cit_shift->data,counter,old_fll,fll,fs,requested_fll,out->count);
    return 1;
}
