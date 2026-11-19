/* Each independent stripe stores 64 consecutive Bayer samples per group.
 * Same-colour left/up predictors, nearest-integer mean. Three header bits
 * select a Rice parameter, with a bounded escape for extreme residuals. */
typedef struct {uint8_t *p;uint64_t pending;unsigned count;} Writer;
static inline void put(Writer *w,unsigned bits,unsigned n){
 w->pending|=(uint64_t)bits<<w->count;w->count+=n;
 if(w->count>=32){uint32_t word=w->pending;memcpy(w->p,&word,4);w->p+=4;w->pending>>=32;w->count-=32;}
}
static inline void flush_group(Writer *w){
 while(w->count){*w->p++=w->pending;w->pending>>=8;w->count=w->count>8?w->count-8:0;}
}
static size_t pack_rice(const uint16_t *raw,uint8_t *out,uint16_t *history,unsigned *tails,uint64_t *pred_ns,uint64_t *pack_ns){
 uint8_t *cursor=out;uint16_t residual[W];unsigned sums[W/64];size_t count=(size_t)height*W;
 memset(history,0,2*W*sizeof(*history));*tails=0;
 for(int y=0;y<height;y++){
  uint64_t start=ns();
  uint16_t *row=history+(y&1)*W;uint16_t left[2]={0,0};
  for(int x=0;x<W;x+=64){
   unsigned sum=0;
   for(int j=0;j<64;j++){
    unsigned r=raw[y*W+x+j];if(r>=16384)die("raw range");
    *tails|=r>=11648?(r&15):0;unsigned v=indices[r],prev=(left[j&1]+row[x+j]+1)>>1;
    int d=(int)v-(int)prev;unsigned z=d>=0?2*d:-2*d-1;
    residual[x+j]=z;sum+=z;row[x+j]=v;left[j&1]=v;
   }
   sums[x/64]=sum;
  }
  uint64_t split=ns();
  for(int x=0;x<W;x+=64){
   unsigned sum=sums[x/64];
   unsigned k=0;if(sum>128){k=32-__builtin_clz((sum-1)>>7);if(k>7)k=7;}
   Writer b={cursor,0,0};put(&b,k,3);unsigned mask=(1u<<k)-1;
   for(int j=0;j<64;j++){
    unsigned z=residual[x+j],q=z>>k;
    if(q>=15){put(&b,z<<15,26);}
    else {put(&b,((z&mask)*2u+1)<<q,q+1+k);}
   }
   flush_group(&b);cursor=b.p;
  }
  uint64_t stop=ns();*pred_ns+=split-start;*pack_ns+=stop-split;
 }
 if((size_t)(cursor-out)>count*26/8+count/64)die("pack range");
 return cursor-out;
}
typedef struct {const uint8_t *p,*end;uint64_t cache;unsigned n;} Reader;
static unsigned getbits(Reader *r,unsigned n){
 while(r->n<n){if(r->p==r->end)die("rice end");r->cache|=(uint64_t)*r->p++<<r->n;r->n+=8;}
 unsigned v=r->cache&((1u<<n)-1);r->cache>>=n;r->n-=n;return v;
}
static void unpack_rice(const uint8_t *src,size_t size,uint16_t *dst){
 Reader b={src,src+size,0,0};
 for(int y=0;y<height;y++)for(int x=0;x<W;x+=64){
  unsigned k=getbits(&b,3);
  for(int j=0;j<64;j++){
   unsigned q=0;while(q<15 && !getbits(&b,1))q++;
   unsigned z=q==15?getbits(&b,11):(q<<k)|getbits(&b,k);
   int n=y*W+x+j;unsigned l=x+j>=2?dst[n-2]:0,u=y>=2?dst[n-2*W]:0;
   int recovered=(int)((l+u+1)/2)+(z&1?-(int)((z+1)/2):(int)(z/2));
   if(recovered<0||(size_t)recovered>=sizeof(levels)/sizeof(*levels))die("rice level");
   dst[n]=recovered;
  }
  if(b.cache!=0)die("rice padding");b.cache=0;b.n=0;
 }
 if(b.p!=b.end)die("rice trailing bytes");
}
