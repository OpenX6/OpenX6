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
int framed_p010(struct chi *out, struct chi *in, unsigned front, unsigned span) {
    if (!valid(out)||!valid(in)||out->fd==in->fd||out->w!=640||out->h!=800||
        in->w!=2048||in->h!=1024) return 0;
    if (dma(in->fd,1)) return 0;
    if (dma(out->fd,2)) {dma(in->fd,5); return 0;}

if (span==1056) {
    // Ten standby LCD pixels on either side lie outside the available source.
    const unsigned margin=10, inside=640-2*margin, eye=front?0:1024;
    unsigned table[620];
    for(unsigned x=0;x<inside;x++) table[x]=eye+(x*1024+512)/inside;
    for(unsigned y=0;y<800;y++) {
        int image=y>=margin && y<640-margin;
        unsigned row=image ? ((y-margin)*1024+512)/inside : 0;
        u16 *dy=out->y+y*640, *duv=out->uv+(y/2)*640;
        const u16 *sy=in->y+row*2048, *suv=in->uv+(row/2)*2048;
        for(unsigned x=0;x<640;x++){
            int valid=image && x>=margin && x<640-margin;
            unsigned sx=valid?table[x-margin]:0;
            dy[x]=valid?sy[sx]:(64u<<6);
            if(!(y&1) && !(x&1)) {
                unsigned pair=sx&~1u;
                duv[x]=valid?suv[pair]:(512u<<6);
                duv[x+1]=valid?suv[pair+1]:(512u<<6);
            }
        }
    }
    int a=dma(out->fd,6), b=dma(in->fd,5);
    return !a && !b;
}

    unsigned pad=(1024-span)/2;
    unsigned side=front?0:1024;
    unsigned short tx[640];
    for(unsigned x=0;x<640;x++) tx[x]=side+(pad+(x*span+span/2)/640);
    for(unsigned y=0;y<800;y++){
        int blank=y>=640;
        unsigned src=blank?0:(pad+(y*span+span/2)/640);
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
