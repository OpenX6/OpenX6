#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#define W 3840
static int height=160;
static uint64_t ns(void){return 0;}
static void die(const char *message){fprintf(stderr,"%s\n",message);exit(1);}
#include "levels.h"
#ifdef ALTERNATE_MAGIC
#include "alternate_levels.h"
static size_t symbol_count;
#include "dual_profile_codec.h"
#else
#include "fast3.h"
#endif
#include "tail.h"
int main(int argc,char **argv){
    if(argc!=3)return 2;
    char *end=NULL;long long offset=strtoll(argv[2],&end,10);
    if(!end||*end||offset<0||offset%4096)die("invalid offset");
    FILE *in=fopen(argv[1],"rb");
    if(!in||fseeko(in,offset,SEEK_SET))die("capture seek");
    uint8_t head[4096];
    if(fread(head,1,sizeof head,in)!=sizeof head)die("capture header");
    const uint16_t *points=levels;
    size_t count=sizeof(levels)/sizeof(*levels);
    if(memcmp(head,PROFILE_MAGIC,8)){
#ifdef ALTERNATE_MAGIC
        if(memcmp(head,ALTERNATE_MAGIC,8))die("unknown profile");
        points=alternate_levels;
        count=sizeof(alternate_levels)/sizeof(*alternate_levels);
#else
        die("unknown profile");
#endif
    }
    #ifdef ALTERNATE_MAGIC
    symbol_count=count;
    #endif
    size_t pixels=W*height;
    uint8_t *data=malloc(pixels*4),*low=malloc(pixels/2);
    uint16_t *img=malloc(pixels*sizeof(*img));
    if(!data||!low||!img)die("allocation");
    for(int tile=0;tile<48;tile++){
        uint32_t n,main;memcpy(&n,head+48+4*tile,4);
        if(n<=4||n>pixels*4)die("invalid tile size");
        size_t padded=((size_t)n+4095)&~(size_t)4095;
        if(padded>pixels*4||fread(data,1,padded,in)!=padded)die("truncated tile");
        memcpy(&main,data,4);
        if(main>n-4)die("invalid predictor size");
        unpack_rice(data+4,main,img);
        read_tails(data+4+main,n-4-main,low,pixels);
        for(size_t k=0;k<pixels;k++){
            if(img[k]>=count)die("symbol range");
            img[k]=points[img[k]]+((low[k/2]>>(4*(k%2)))&15);
        }
        if(fwrite(img,sizeof(*img),pixels,stdout)!=pixels)die("Bayer output");
    }
    return fflush(stdout)!=0;
}
