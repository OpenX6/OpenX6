#include "blocks.h"
static inline uint8_t *pack16(uint8_t *out,const uint16x8_t *z,unsigned width){
switch(width){
case 0: {
return out;}
case 1: {
uint8x8_t tail=vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],1)));
uint64_t t=vget_lane_u64(vreinterpret_u64_u8(tail),0);
t=(t|(t>>6))&0x000f000f000f000fULL;
t=(t|(t>>12))&0x000000ff000000ffULL;
uint16_t u=(uint16_t)(t|(t>>24));memcpy(out,&u,2);out+=2;
return out;}
case 2: {
uint8x8_t tail=vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],2)));
uint16x4_t a=vreinterpret_u16_u8(tail);
uint8x8_t b=vmovn_u16(vcombine_u16(vorr_u16(a,vshr_n_u16(a,4)),vdup_n_u16(0)));
vst1_lane_u32((uint32_t*)out,vreinterpret_u32_u8(b),0);out+=4;
return out;}
case 3: {
uint8x8_t tail=vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],3)));
uint64_t t=vget_lane_u64(vreinterpret_u64_u8(tail),0);
t=(t&0x003f003f003f003fULL)|((t>>2)&0x0fc00fc00fc00fc0ULL);
t=(t&0x00000fff00000fffULL)|((t>>4)&0x00fff00000fff000ULL);
t=(t&0x0000000000ffffffULL)|((t>>8)&0x0000ffffff000000ULL);
memcpy(out,&t,6);out+=6;
return out;}
case 4: {
vst1_u8(out,vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],4))));out+=8;
return out;}
case 5: {
vst1_u8(out,vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],5))));out+=8;
uint8x8_t tail=vmovn_u16(vshrq_n_u16(z[1],3));
uint64_t t=vget_lane_u64(vreinterpret_u64_u8(tail),0);
t=(t|(t>>6))&0x000f000f000f000fULL;
t=(t|(t>>12))&0x000000ff000000ffULL;
uint16_t u=(uint16_t)(t|(t>>24));memcpy(out,&u,2);out+=2;
return out;}
case 6: {
vst1_u8(out,vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],6))));out+=8;
uint8x8_t tail=vmovn_u16(vshrq_n_u16(z[1],2));
uint16x4_t a=vreinterpret_u16_u8(tail);
uint8x8_t b=vmovn_u16(vcombine_u16(vorr_u16(a,vshr_n_u16(a,4)),vdup_n_u16(0)));
vst1_lane_u32((uint32_t*)out,vreinterpret_u32_u8(b),0);out+=4;
return out;}
case 7: {
vst1_u8(out,vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],7))));out+=8;
uint8x8_t tail=vmovn_u16(vshrq_n_u16(z[1],1));
uint64_t t=vget_lane_u64(vreinterpret_u64_u8(tail),0);
t=(t&0x003f003f003f003fULL)|((t>>2)&0x0fc00fc00fc00fc0ULL);
t=(t&0x00000fff00000fffULL)|((t>>4)&0x00fff00000fff000ULL);
t=(t&0x0000000000ffffffULL)|((t>>8)&0x0000ffffff000000ULL);
memcpy(out,&t,6);out+=6;
return out;}
case 8: {
vst1_u8(out,vmovn_u16(z[0]));out+=8;
vst1_u8(out,vmovn_u16(z[1]));out+=8;
return out;}
case 9: {
vst1_u8(out,vmovn_u16(z[0]));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[0],8)),vmovn_u16(vshlq_n_u16(z[1],1))));out+=8;
uint8x8_t tail=vmovn_u16(vshrq_n_u16(z[1],7));
uint64_t t=vget_lane_u64(vreinterpret_u64_u8(tail),0);
t=(t|(t>>6))&0x000f000f000f000fULL;
t=(t|(t>>12))&0x000000ff000000ffULL;
uint16_t u=(uint16_t)(t|(t>>24));memcpy(out,&u,2);out+=2;
return out;}
case 10: {
vst1_u8(out,vmovn_u16(z[0]));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[0],8)),vmovn_u16(vshlq_n_u16(z[1],2))));out+=8;
uint8x8_t tail=vmovn_u16(vshrq_n_u16(z[1],6));
uint16x4_t a=vreinterpret_u16_u8(tail);
uint8x8_t b=vmovn_u16(vcombine_u16(vorr_u16(a,vshr_n_u16(a,4)),vdup_n_u16(0)));
vst1_lane_u32((uint32_t*)out,vreinterpret_u32_u8(b),0);out+=4;
return out;}
case 11: {
vst1_u8(out,vmovn_u16(z[0]));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[0],8)),vmovn_u16(vshlq_n_u16(z[1],3))));out+=8;
uint8x8_t tail=vmovn_u16(vshrq_n_u16(z[1],5));
uint64_t t=vget_lane_u64(vreinterpret_u64_u8(tail),0);
t=(t&0x003f003f003f003fULL)|((t>>2)&0x0fc00fc00fc00fc0ULL);
t=(t&0x00000fff00000fffULL)|((t>>4)&0x00fff00000fff000ULL);
t=(t&0x0000000000ffffffULL)|((t>>8)&0x0000ffffff000000ULL);
memcpy(out,&t,6);out+=6;
return out;}
default:die("width16");return NULL;}}


/* Two 32-pixel blocks share one width byte. Each block packs four samples
 * into each of eight parallel lanes. A final half byte is paired by lane. */
static size_t pack_rice(const uint8_t *raw,uint8_t *out,uint16_t *history,unsigned *tails,uint64_t *pt,uint64_t *bt){
    uint8_t *dst=out;
    uint16x8_t any=vdupq_n_u16(0);
    memset(history,0,2*W*sizeof(*history));
    for(int y=0;y<height;y++){
        uint16_t *up=history+(y%2)*W;
        uint16x8_t left=vdupq_n_u16(0);
        for(int x=0;x<W;x+=32){
            uint8_t *header=dst++;

                uint16x8_t z[4];
                for(int j=0;j<4;j++){
                    int col=x+8*j;
                    uint16x8_t a=read8(raw+((size_t)y*W+col)*5/4);
                    any=vorrq_u16(any,vandq_u16(a,vcgeq_u16(a,vdupq_n_u16(1024))));
                    uint16x8_t v=quantize(a);
                    uint16x8_t pred=vrhaddq_u16(vextq_u16(left,v,6),vld1q_u16(up+col));
                    left=v;vst1q_u16(up+col,v);
                    int16x8_t delta=vreinterpretq_s16_u16(vsubq_u16(v,pred));
                    z[j]=vreinterpretq_u16_s16(veorq_s16(vshlq_n_s16(delta,1),vshrq_n_s16(delta,15)));
                }
                unsigned top0=vmaxvq_u16(vmaxq_u16(z[0],z[1])),top1=vmaxvq_u16(vmaxq_u16(z[2],z[3]));
                unsigned b0=top0?32-__builtin_clz(top0):0,b1=top1?32-__builtin_clz(top1):0;
                *header=b0|(b1<<4);
                if(b0==b1)dst=pack32(dst,z,b0);
                else{dst=pack16(dst,z,b0);dst=pack16(dst,z+2,b1);}
        }
    }
    *tails=vmaxvq_u16(vandq_u16(any,vdupq_n_u16(15)));
    return dst-out;
}

static void unpack_rice(const uint8_t *p,size_t size,uint16_t *dst){
 const uint8_t *end=p+size;
 for(int y=0;y<height;y++)for(int x=0;x<W;x+=32){
  if(p>=end)die("header");unsigned h=*p++,b0=h&15,b1=h>>4;
  if(b0>11||b1>11)die("width");
  for(unsigned half=0;half<(b0==b1?1:2);half++){
   unsigned b=half?b1:b0,count=b0==b1?32:16,bytes=count*b/8;
   if((size_t)(end-p)<bytes)die("block length");
   uint64_t words[8]={0};unsigned full=(count/8)*b/8,rem=(count/8)*b%8;
   for(unsigned lane=0;lane<8;lane++){
    for(unsigned k=0;k<full;k++)words[lane]|=(uint64_t)p[k*8+lane]<<(8*k);
    unsigned bit=lane*rem,t=0;for(unsigned k=0;k<rem;k++)t|=((p[full*8+(bit+k)/8]>>((bit+k)%8))&1)<<k;
    words[lane]|=(uint64_t)t<<(8*full);
   }
   for(unsigned j=0;j<count;j++){
    unsigned v=(words[j%8]>>(b*(j/8)))&((1u<<b)-1);int col=x+half*16+j,i=y*W+col;
    int restored=(((col>=2?dst[i-2]:0)+(y>=2?dst[i-2*W]:0)+1)>>1)+(v&1?-(int)((v+1)/2):(int)(v/2));
    if(restored<0||restored>1023)die("symbol");dst[i]=restored;
   }
   p+=bytes;
  }
 }
 if(p!=end)die("trailing bytes");
}
