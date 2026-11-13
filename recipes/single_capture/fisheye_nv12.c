/* Standard-colour panorama LCD path. The existing P010 handler is an independent tail target. */
#include <stddef.h>
#include <stdint.h>

typedef struct {
    uint8_t reserved0[8];
    uint32_t width, height, format;
    uint8_t reserved1[12];
    uint32_t pixels, rows, pitch, chroma_descriptor;
    uint8_t reserved2[192];
    uint64_t luma_size, chroma_size;
    uint8_t reserved3[60];
    int fd;
    uint8_t reserved4[16];
    uint8_t *luma, *chroma;
} Image;
_Static_assert(offsetof(Image, fd)==316 && offsetof(Image,luma)==336, "CHI buffer ABI");
#ifdef CPU_TEST
extern int dma_access(int, unsigned long);
#else
static int dma_access(int fd, unsigned long flags) {
    register long nr __asm__("x8")=29;
    register long a __asm__("x0")=fd;
    register long b __asm__("x1")=0x40086200;
    register long c __asm__("x2")=(long)&flags;
    __asm__ volatile("svc 0" : "+r"(a) : "r"(nr),"r"(b),"r"(c) : "memory","cc");
    return (int)a;
}
#endif
static int matches(const Image *p, unsigned w, unsigned h) {
    return p && p->format==3 && p->width==w && p->height==h &&
           p->pixels==w && p->rows==h && p->pitch==w &&
           p->luma_size==(uint64_t)w*h && p->chroma_size==p->luma_size/2 &&
           p->fd>=0 && p->luma && p->chroma &&
           (uintptr_t)p->chroma-(uintptr_t)p->luma==p->luma_size;
}
int fish_8bit(Image *out, const Image *in, unsigned front) {
    if (!matches(out,640,800) || !matches(in,2048,1024) || out->fd==in->fd ||
        out->luma==in->luma || front>1) return 0;
    if (dma_access(in->fd,1)) return 0;
    if (dma_access(out->fd,2)) { dma_access(in->fd,5); return 0; }
    /* Identical pixel centres and eye order to the tested P010 branch. Keep interleaved chroma pairs together. */
    uint16_t column[640];
    for (unsigned x=0;x<640;x++)
        column[x]=(front ? 0 : 1024)+(x*1024+512)/640;
    for (unsigned y=0;y<640;y++) {
        unsigned row=(y*1024+512)/640;
        const uint8_t *src=in->luma+row*2048;
        uint8_t *dst=out->luma+y*640;
        for (unsigned x=0;x<640;x++) dst[x]=src[column[x]];
        if (!(y&1)) {
            src=in->chroma+(row/2)*2048;dst=out->chroma+(y/2)*640;
            for (unsigned x=0;x<640;x+=2) {
                unsigned pair=column[x]&~1u;
                dst[x]=src[pair];dst[x+1]=src[pair+1];
            }
        }
    }
    for (unsigned i=640*640;i<640*800;i++) out->luma[i]=16;
    for (unsigned i=640*320;i<640*400;i++) out->chroma[i]=128;
    int end_out=dma_access(out->fd,6),end_in=dma_access(in->fd,5);
    return !end_out && !end_in;
}

#define PIXEL uint8_t
#define BLACK 16
#define NEUTRAL 128
#include "linear_sample.h"
int linear_nv12(Image *out, Image *in, unsigned front) {
    if (!(matches(out,640,800) && matches(in,2048,1024)) || out->fd==in->fd || out->luma==in->luma || front>1) return 0;
    if (dma_access(in->fd,1)) return 0;
    if (dma_access(out->fd,2)) { dma_access(in->fd,5); return 0; }
    sample_linear(out->luma,out->chroma,in->luma,in->chroma,front ? 0 : 1024);
    int a=dma_access(out->fd,6), b=dma_access(in->fd,5);
    return !a && !b;
}
