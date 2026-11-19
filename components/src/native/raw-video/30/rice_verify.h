static void verify(void){
    uint16_t *decoded=alloc_hot(stripebytes);
    size_t pixels=(size_t)W*height;
    uint8_t *low=alloc_hot(pixels/2);
    for(int t=0;t<sampled;t++)for(int eye=0;eye<2;eye++){
        Pair *p=&samples[t];uint64_t bad=0,bright=0,bright_bad=0,sse=0,tailcount=0;
        unsigned lo=16383,hi=0,peak=0;
        for(int part=0;part<stripes;part++){
            int lane=eye*stripes+part;uint32_t n;memcpy(&n,p->out[lane],4);
            if(n+4>p->len[lane])die("size");
            unpack_rice(p->out[lane]+4,n,decoded);
            size_t tails=p->len[lane]-4-n;
            read_tails(p->out[lane]+4+n,tails,low,pixels);
            const uint16_t *raw=(const uint16_t*)p->raw[eye]+part*pixels;
            for(int y=0;y<height;y++)for(int x=0;x<W;x++){
                size_t i=y*W+x,j=((y&1)*2+(x&1))*pixels/4+(y/2)*(W/2)+x/2;
                unsigned index=decoded[i];
                if(index>=sizeof(levels)/sizeof(*levels))die("decoded index");
                unsigned v=levels[index],ref=raw[i];
                if(tails){unsigned nib=(low[i/2]>>(4*(i&1)))&15;v+=nib;tailcount+=nib!=0;}
                bad+=v!=reference[ref];bright+=ref>=threshold;bright_bad+=ref>=threshold && v!=ref;
                unsigned d=abs((int)v-(int)ref);if(d>peak)peak=d;sse+=d*d;
                if(ref<lo)lo=ref;if(ref>hi)hi=ref;
            }
            
        }
        printf("highlight,%d,%d,%llu,%llu,%llu\n",p->req,eye,(unsigned long long)bright,(unsigned long long)bright_bad,(unsigned long long)tailcount);
        printf("error,%d,%d,%u,%llu\n",p->req,eye,peak,(unsigned long long)sse);
        printf("check,%d,%d,%llu,%u,%u\n",p->req,eye,(unsigned long long)bad,lo,hi);
        if(bad||bright_bad||peak>0)die("quality verification");
    }
    
}
