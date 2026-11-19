/* An empty tail represents all zeros. Otherwise one bitmap bit per
 * 128 raw samples locates the blocks containing protected low nibbles. */
static size_t write_tails(const uint16_t *raw,size_t pixels,uint8_t *out){
 size_t groups=pixels/128,header=(groups+7)/8;
 memset(out,0,header);uint8_t *next=out+header;
 for(size_t g=0;g<groups;g++){
  unsigned any=0;uint8_t block[64];
  for(unsigned j=0;j<64;j++){
   unsigned a=raw[g*128+2*j],b=raw[g*128+2*j+1];
   block[j]=(a>=13472?(a&15):0)|((b>=13472?(b&15):0)<<4);any|=block[j];
  }
  if(any){out[g>>3]|=1u<<(g&7);memcpy(next,block,64);next+=64;}
 }
 return next-out;
}
static void read_tails(const uint8_t *src,size_t n,uint8_t *dst,size_t pixels){
 memset(dst,0,pixels/2);
 if(!n)return;
 size_t groups=pixels/128,offset=(groups+7)/8;
 if(offset>n)die("tail header");
 for(size_t g=0;g<groups;g++)if(src[g>>3]&(1u<<(g&7))){
  if(offset+64>n)die("tail block");
  memcpy(dst+g*64,src+offset,64);offset+=64;
 }
 if(offset!=n)die("tail trailing bytes");
}
