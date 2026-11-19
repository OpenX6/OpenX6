#define _GNU_SOURCE
#include <math.h>

#include <arm_neon.h>
#include <stdatomic.h>
#include <pthread.h>
#include <dlfcn.h>
#include <time.h>
#include <string.h>
#include <stdint.h>
#include <stdio.h>
#include <unistd.h>
#include <stdlib.h>

#define W 7680
enum {parts=24,height=W/parts,threads=8};
static void die(const char *why){dprintf(2,"codec failure %s\n",why);_exit(91);}
#include "levels.h"
static inline uint16x8_t read8(const uint8_t *raw){
    const uint8x8_t hi={0,1,2,3,5,6,7,8}, lo={4,4,4,4,9,9,9,9};
    const int8x8_t shifts={0,-2,-4,-6,0,-2,-4,-6};
    uint8x16_t bytes=vld1q_u8(raw); // external allocation includes padding beyond the last row
    uint8x8_t low=vand_u8(vshl_u8(vqtbl1_u8(bytes,lo),shifts),vdup_n_u8(3));
    return vorrq_u16(vshll_n_u8(vqtbl1_u8(bytes,hi),2),vmovl_u8(low));
}
#include "packed.h"
static pthread_once_t once=PTHREAD_ONCE_INIT;
static pthread_barrier_t begin,end;
static atomic_uint next;
static size_t sizes[parts];
static const uint8_t *input;
static int testing;
static pthread_mutex_t callback=PTHREAD_MUTEX_INITIALIZER;
static __thread FILE *sink;
static __thread int id;
static uint64_t now(void){struct timespec t;clock_gettime(CLOCK_MONOTONIC,&t);return (uint64_t)t.tv_sec*1000000000+t.tv_nsec;}
#include "disk.h"
static void *worker(void *unused){
    uint16_t previous[2*W]; 

    for(;;){
        pthread_barrier_wait(&begin);
        for(unsigned part;(part=atomic_fetch_add(&next,1))<parts;){
            const uint8_t *src=input+(size_t)part*height*W*5/4;
            uint8_t *out=current->data+(size_t)part*stripe_cap;
            unsigned tails;uint64_t pt=0,bt=0;
            uint32_t n=pack_rice(src,out+4,previous,&tails,&pt,&bt);
            if(tails)die("unexpected low-nibble tail for RAW10");
            memcpy(out,&n,4);sizes[part]=n+4;current->header->sizes[part]=n+4;
            memset(out+n+4,0,align4k(n+4)-n-4);
            if(current->header->has_crc)current->header->crc[part]=raw_crc(src,(size_t)height*W*5/4);
            if(testing && part==1){
                uint16_t *decoded=malloc((size_t)height*W*2);
                if(!decoded)die("verify allocation");
                unpack_rice(out+4,n,decoded);
                for(size_t i=0;i<(size_t)height*W;i++){
                    const uint8_t *q=src+(i/4)*5;
                    unsigned dn=((unsigned)q[i%4]<<2)|((q[4]>>(2*(i%4)))&3);
                    if(decoded[i]!=dn)die("decode mismatch");
                }
                free(decoded);
                dprintf(2,"verified %d RAW10 sites\n",height*W);
            }
        }
        pthread_barrier_wait(&end);
    }
    return NULL;
}
static void meter(const unsigned char *b, unsigned id){
    static int enabled = -1;
    if (enabled < 0) enabled = getenv("X5_RAW_AUTO_EXPOSURE") != NULL;
    if (!enabled || id % 3) return;
    unsigned hist[256]={0}, count=0;
    for(unsigned y=576;y<7104;y+=96) for(unsigned x=576;x<7104;x+=96){
        double dx=x-3840.,dy=y-3840.;
        if(dx*dx+dy*dy>3200.*3200.)continue;
        // Green sites in each row of GRBG. Use high bytes only for metering.
        unsigned v=(b[y*9600+x/4*5]+b[(y+1)*9600+x/4*5+1])/2;
        hist[v]++;count++;
    }
    unsigned sum=0, total=0;
    for(unsigned i=0;i<256;i++){
        unsigned prev=sum;sum+=hist[i];
        unsigned lo=prev>count/5?prev:count/5, hi=sum<count*4/5?sum:count*4/5;
        if(hi>lo)total+=(hi-lo)*(i>16?i-16:0);
    }
    FILE *f = fopen("ae-meter.tmp", "w");
    if (!f) die("opening RAW meter");
    int result = fprintf(f, "%u %.4f\n", id + 1, (double)total/(count*4/5-count/5));
    if (fclose(f) || result < 0 || rename("ae-meter.tmp","ae-meter")) die("publishing RAW meter");
}

static void write_cover(const uint8_t *src){
 unsigned char rgb[176*120*3]={0},lut[1024];
 for(int k=0;k<1024;k++)lut[k]=k<=64?0:(unsigned char)(255*pow((k-64)/959.0,0.45)+0.5);
 for(int y=0;y<120;y++)for(int x=0;x<120;x++){
   unsigned yy=64*y+32, xx=64*x+32;
   const uint8_t *top=src+(size_t)yy*9600+xx/4*5,*bot=top+9600;
   unsigned a=4*top[0]+(top[4]&3),b=4*top[1]+((top[4]>>2)&3);
   unsigned c=4*bot[0]+(bot[4]&3),d=4*bot[1]+((bot[4]>>2)&3);
   unsigned char *out=rgb+3*(y*176+x+28);
   out[0]=lut[c];out[1]=lut[(a+d)/2];out[2]=lut[b];
 }
 int f=openat(dirfd,"cover.rgb",O_WRONLY|O_CREAT|O_EXCL|O_NOFOLLOW,0600);
 if(f<0||write(f,rgb,sizeof(rgb))!=sizeof(rgb)||close(f))die("cover");
}
unsigned sleep(unsigned seconds){
 static unsigned (*original)(unsigned);
 if(!original)original=dlsym(RTLD_NEXT,"sleep");
 if(seconds!=86401)return original(seconds);
 while(access("stop",F_OK)!=0)usleep(50000);
 return 0;
}
static void init(void){
    disk_init();
    pthread_barrier_init(&begin,NULL,threads+1);pthread_barrier_init(&end,NULL,threads+1);
    for(int i=0;i<threads;i++){pthread_t thread;if(pthread_create(&thread,NULL,worker,NULL))die("pthread");}
}
FILE *fopen(const char *path,const char *mode){
    static FILE *(*orig)(const char*,const char*);
    if(!orig)orig=dlsym(RTLD_NEXT,"fopen");
    if(strncmp(path,"/data/misc/camera/",18)||mode[0]!='w')return orig(path,mode);
    id=-1;sscanf(strrchr(path,'/')+1,"preview_w[7680]_h[7680]_id[%d]",&id);
    sink=orig("/dev/null",mode);return sink;
}
size_t fwrite(const void *buf,size_t size,size_t count,FILE *f){
    static size_t (*orig)(const void*,size_t,size_t,FILE*);
    if(!orig)orig=dlsym(RTLD_NEXT,"fwrite");
    if(!sink||f!=sink)return orig(buf,size,count,f);
    if(id<0)return count;
    if(size*count!=92160000)die("mapped size");
    pthread_mutex_lock(&callback);pthread_once(&once,init);
    if(!prepare(id+1)){pthread_mutex_unlock(&callback);return count;}
    static int first=1;
    testing=first && getenv("CHECK_STRIPE")!=NULL;first=0;
    meter(buf,id);
    if(first_exp && !prod) write_cover(buf);input=buf;atomic_store(&next,0);
    pthread_barrier_wait(&begin);pthread_barrier_wait(&end);
    publish();pthread_mutex_unlock(&callback);return count;
}
int fclose(FILE *f){
    static int (*orig)(FILE*);
    if(!orig)orig=dlsym(RTLD_NEXT,"fclose");
    if(sink && f==sink)sink=NULL;
    return orig(f);
}
void exit(int status){static atomic_int once_exit;if(!atomic_exchange(&once_exit,1))finish_disk();fflush(NULL);_exit(status);}
typedef int (*mainfn)(int,char**,char**);
static mainfn actual_main;
static int run_main(int argc,char **argv,char **envp){int rc=actual_main(argc,argv,envp);exit(rc);}
int __libc_start_main(mainfn m,int argc,char **argv,void *init,void *fini,void *rtld_fini,void *sp){
 typedef int (*startfn)(mainfn,int,char**,void*,void*,void*,void*);
 actual_main=m;return ((startfn)dlsym(RTLD_NEXT,"__libc_start_main"))(run_main,argc,argv,init,fini,rtld_fini,sp);
}
