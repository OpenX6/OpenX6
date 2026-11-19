#include "blocks.h"
/* Two 32-pixel blocks share one width byte. Each block packs four samples
 * into each of eight parallel lanes. A final half byte is paired by lane. */
static size_t pack_rice(const uint16_t *raw,uint8_t *out,uint16_t *history,unsigned *tails,uint64_t *pt,uint64_t *bt){
    uint8_t *dst=out;
    uint16x8_t any=vdupq_n_u16(0);
    memset(history,0,2*W*sizeof(*history));
    for(int y=0;y<height;y++){
        uint16_t *up=history+(y%2)*W;
        uint16x8_t left=vdupq_n_u16(0);
        for(int x=0;x<W;x+=64){
            uint8_t *header=dst++;
            unsigned widths=0;
            for(int half=0;half<2;half++){
                uint16x8_t z[4];
                for(int j=0;j<4;j++){
                    int col=x+32*half+8*j;
                    uint16x8_t a=vld1q_u16(raw+y*W+col);
                    any=vorrq_u16(any,vandq_u16(a,vcgeq_u16(a,vdupq_n_u16(13472))));
                    uint16x8_t v=quantize(a);
                    uint16x8_t pred=vrhaddq_u16(vextq_u16(left,v,6),vld1q_u16(up+col));
                    left=v;vst1q_u16(up+col,v);
                    int16x8_t delta=vreinterpretq_s16_u16(vsubq_u16(v,pred));
                    z[j]=vreinterpretq_u16_s16(veorq_s16(vshlq_n_s16(delta,1),vshrq_n_s16(delta,15)));
                }
                unsigned top=vmaxvq_u16(vmaxq_u16(vmaxq_u16(z[0],z[1]),vmaxq_u16(z[2],z[3])));
                unsigned bits=top?32-__builtin_clz(top):0;
                widths|=bits<<(4*half);dst=pack32(dst,z,bits);
            }
            *header=widths;
        }
    }
    *tails=vmaxvq_u16(vandq_u16(any,vdupq_n_u16(15)));
    return dst-out;
}
static void unpack_rice(const uint8_t *p,size_t size,uint16_t *dst){
    const uint8_t *end=p+size;
    for(int y=0;y<height;y++)for(int x=0;x<W;x+=64){
        if(p>=end)die("header");unsigned h=*p++;
        for(int half=0;half<2;half++){
            unsigned b=(h>>(4*half))&15;
            if(b>11 || end-p<4*b)die("block length");
            uint64_t words[8]={0};
            for(int lane=0;lane<8;lane++){
                for(unsigned k=0;k<b/2;k++)words[lane]|=(uint64_t)p[k*8+lane]<<(8*k);
                if(b&1)words[lane]|=(uint64_t)((p[(b/2)*8+lane/2]>>(4*(lane&1)))&15)<<(8*(b/2));
            }
            for(unsigned j=0;j<32;j++){
                unsigned v=(words[j%8]>>(b*(j/8)))&((1u<<b)-1);
                int col=x+half*32+j,i=y*W+col;
                int restored=(((col>=2?dst[i-2]:0)+(y>=2?dst[i-2*W]:0)+1)>>1)+(v&1?-(int)((v+1)/2):(int)(v/2));
                if(restored<0 || restored>=(int)(symbol_count))die("bad symbol");
                dst[i]=restored;
            }
            p+=4*b;
        }
    }
    if(p!=end)die("trailing bytes");
}
