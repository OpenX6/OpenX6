#include <arm_neon.h>
static inline uint16x8_t quantize(uint16x8_t x){
uint16x8_t q=vaddq_u16(vrshrq_n_u16(x,2),vdupq_n_u16(0));
q=vminq_u16(q,vaddq_u16(vrshrq_n_u16(x,4),vdupq_n_u16(48)));
q=vminq_u16(q,vaddq_u16(vrshrq_n_u16(x,5),vdupq_n_u16(80)));
q=vminq_u16(q,vaddq_u16(vrshrq_n_u16(x,6),vdupq_n_u16(112)));
q=vminq_u16(q,vaddq_u16(vrshrq_n_u16(x,7),vdupq_n_u16(144)));
return vbslq_u16(vcgeq_u16(x,vdupq_n_u16(13472)),vsubq_u16(vshrq_n_u16(x,4),vdupq_n_u16(592)),q);
}
static inline uint8_t *pack32(uint8_t *out,const uint16x8_t *z,unsigned width){
switch(width){
case 0: return out;
case 1: {
uint8x8_t nib=vorr_u8(vorr_u8(vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],1))),vmovn_u16(vshlq_n_u16(z[2],2))),vmovn_u16(vshlq_n_u16(z[3],3)));
uint16x4_t pairs=vreinterpret_u16_u8(nib);
uint8x8_t bytes=vmovn_u16(vcombine_u16(vorr_u16(pairs,vshr_n_u16(pairs,4)),vdup_n_u16(0)));
vst1_lane_u32((uint32_t *)out,vreinterpret_u32_u8(bytes),0);out+=4;
return out;}
case 2: {
vst1_u8(out,vorr_u8(vorr_u8(vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],2))),vmovn_u16(vshlq_n_u16(z[2],4))),vmovn_u16(vshlq_n_u16(z[3],6))));out+=8;
return out;}
case 3: {
vst1_u8(out,vorr_u8(vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],3))),vmovn_u16(vshlq_n_u16(z[2],6))));out+=8;
uint8x8_t nib=vorr_u8(vmovn_u16(vshrq_n_u16(z[2],2)),vmovn_u16(vshlq_n_u16(z[3],1)));
uint16x4_t pairs=vreinterpret_u16_u8(nib);
uint8x8_t bytes=vmovn_u16(vcombine_u16(vorr_u16(pairs,vshr_n_u16(pairs,4)),vdup_n_u16(0)));
vst1_lane_u32((uint32_t *)out,vreinterpret_u32_u8(bytes),0);out+=4;
return out;}
case 4: {
vst1_u8(out,vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],4))));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(z[2]),vmovn_u16(vshlq_n_u16(z[3],4))));out+=8;
return out;}
case 5: {
vst1_u8(out,vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],5))));out+=8;
vst1_u8(out,vorr_u8(vorr_u8(vmovn_u16(vshrq_n_u16(z[1],3)),vmovn_u16(vshlq_n_u16(z[2],2))),vmovn_u16(vshlq_n_u16(z[3],7))));out+=8;
uint8x8_t nib=vmovn_u16(vshrq_n_u16(z[3],1));
uint16x4_t pairs=vreinterpret_u16_u8(nib);
uint8x8_t bytes=vmovn_u16(vcombine_u16(vorr_u16(pairs,vshr_n_u16(pairs,4)),vdup_n_u16(0)));
vst1_lane_u32((uint32_t *)out,vreinterpret_u32_u8(bytes),0);out+=4;
return out;}
case 6: {
vst1_u8(out,vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],6))));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[1],2)),vmovn_u16(vshlq_n_u16(z[2],4))));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[2],4)),vmovn_u16(vshlq_n_u16(z[3],2))));out+=8;
return out;}
case 7: {
vst1_u8(out,vorr_u8(vmovn_u16(z[0]),vmovn_u16(vshlq_n_u16(z[1],7))));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[1],1)),vmovn_u16(vshlq_n_u16(z[2],6))));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[2],2)),vmovn_u16(vshlq_n_u16(z[3],5))));out+=8;
uint8x8_t nib=vmovn_u16(vshrq_n_u16(z[3],3));
uint16x4_t pairs=vreinterpret_u16_u8(nib);
uint8x8_t bytes=vmovn_u16(vcombine_u16(vorr_u16(pairs,vshr_n_u16(pairs,4)),vdup_n_u16(0)));
vst1_lane_u32((uint32_t *)out,vreinterpret_u32_u8(bytes),0);out+=4;
return out;}
case 8: {
vst1_u8(out,vmovn_u16(z[0]));out+=8;
vst1_u8(out,vmovn_u16(z[1]));out+=8;
vst1_u8(out,vmovn_u16(z[2]));out+=8;
vst1_u8(out,vmovn_u16(z[3]));out+=8;
return out;}
case 9: {
vst1_u8(out,vmovn_u16(z[0]));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[0],8)),vmovn_u16(vshlq_n_u16(z[1],1))));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[1],7)),vmovn_u16(vshlq_n_u16(z[2],2))));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[2],6)),vmovn_u16(vshlq_n_u16(z[3],3))));out+=8;
uint8x8_t nib=vmovn_u16(vshrq_n_u16(z[3],5));
uint16x4_t pairs=vreinterpret_u16_u8(nib);
uint8x8_t bytes=vmovn_u16(vcombine_u16(vorr_u16(pairs,vshr_n_u16(pairs,4)),vdup_n_u16(0)));
vst1_lane_u32((uint32_t *)out,vreinterpret_u32_u8(bytes),0);out+=4;
return out;}
case 10: {
vst1_u8(out,vmovn_u16(z[0]));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[0],8)),vmovn_u16(vshlq_n_u16(z[1],2))));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[1],6)),vmovn_u16(vshlq_n_u16(z[2],4))));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[2],4)),vmovn_u16(vshlq_n_u16(z[3],6))));out+=8;
vst1_u8(out,vmovn_u16(vshrq_n_u16(z[3],2)));out+=8;
return out;}
case 11: {
vst1_u8(out,vmovn_u16(z[0]));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[0],8)),vmovn_u16(vshlq_n_u16(z[1],3))));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[1],5)),vmovn_u16(vshlq_n_u16(z[2],6))));out+=8;
vst1_u8(out,vmovn_u16(vshrq_n_u16(z[2],2)));out+=8;
vst1_u8(out,vorr_u8(vmovn_u16(vshrq_n_u16(z[2],10)),vmovn_u16(vshlq_n_u16(z[3],1))));out+=8;
uint8x8_t nib=vmovn_u16(vshrq_n_u16(z[3],7));
uint16x4_t pairs=vreinterpret_u16_u8(nib);
uint8x8_t bytes=vmovn_u16(vcombine_u16(vorr_u16(pairs,vshr_n_u16(pairs,4)),vdup_n_u16(0)));
vst1_lane_u32((uint32_t *)out,vreinterpret_u32_u8(bytes),0);out+=4;
return out;}
default:die("residual width");return NULL;
}}