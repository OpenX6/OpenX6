/* Operate on the native LCD render node's already-owned P010 buffers. */
typedef unsigned int u32;
typedef unsigned short u16;
typedef unsigned long u64;
struct chi {
    unsigned char pad0[8];
    u32 w,h,fmt;
    unsigned char pad1[12];
    u32 pixels, rows, pitch, uv_header;
    unsigned char pad2[240-48];
    u64 y_bytes;
    unsigned char pad3[316-248];
    int fd;
    unsigned char pad4[336-320];
    u16 *y,*uv;
};
_Static_assert(__builtin_offsetof(struct chi, fd)==316, "fd");
_Static_assert(__builtin_offsetof(struct chi, y)==336, "CPU address");
static int dma(int fd, u64 flags) {
    register long nr __asm__("x8")=29;
    register long a __asm__("x0")=fd;
    register long b __asm__("x1")=0x40086200;
    register long c __asm__("x2")=(long)&flags;
    __asm__ volatile ("svc 0" : "+r"(a) : "r"(nr),"r"(b),"r"(c) : "memory","cc");
    return a;
}
static int valid(struct chi *p) {
    return p && p->fmt==22 && p->fd>=0 && p->y && p->uv &&
        p->pitch==p->w*2 && p->rows==p->h && p->pixels==p->w &&
        p->y_bytes==(u64)p->pitch*p->h && (u64)p->uv-(u64)p->y==p->y_bytes;
}
int native_single_eye(struct chi *out, struct chi *in, unsigned front) {
    if (!valid(out)||!valid(in)||out->fd==in->fd||out->w!=640||out->h!=800||
        in->w!=2048||in->h!=1024) return 0;
    if (dma(in->fd,1)) return 0;
    if (dma(out->fd,2)) {dma(in->fd,5); return 0;}
    unsigned side=front?0:1024;
    unsigned short tx[640];
    for(unsigned x=0;x<640;x++) tx[x]=side+(x*1024+512)/640;
    for(unsigned y=0;y<800;y++){
        int blank=y>=640;
        unsigned src=blank?0:(y*1024+512)/640;
        u16 *dst=out->y+y*640,*rd=in->y+src*2048;
        if(blank){for(unsigned x=0;x<640;x++) dst[x]=64u<<6;}
        else {for(unsigned x=0;x<640;x++) dst[x]=rd[tx[x]];}
        if(y%2==0){
            dst=out->uv+(y/2)*640;rd=in->uv+(src/2)*2048;
            if(blank){for(unsigned x=0;x<640;x++) dst[x]=512u<<6;}
            else {for(unsigned x=0;x<640;x+=2){unsigned t=tx[x]&~1u;dst[x]=rd[t];dst[x+1]=rd[t+1];}}
        }
    }
    int end_out=dma(out->fd,6),end_in=dma(in->fd,5);
    return !end_out && !end_in;
}

#define PIXEL u16
#define BLACK 4096
#define NEUTRAL 32768
#include "linear_sample.h"
int linear_p010(struct chi *out, struct chi *in, unsigned front) {
    if (!(valid(out) && valid(in) && out->w==640 && out->h==800 && in->w==2048 && in->h==1024) || out->fd==in->fd || out->y==in->y || front>1) return 0;
    if (dma(in->fd,1)) return 0;
    if (dma(out->fd,2)) { dma(in->fd,5); return 0; }
    sample_linear(out->y,out->uv,in->y,in->uv,front ? 0 : 1024);
    int a=dma(out->fd,6), b=dma(in->fd,5);
    return !a && !b;
}
