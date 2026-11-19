#include <sys/stat.h>
#include <sys/statvfs.h>
#include <sys/mman.h>
#include <sys/uio.h>
#include <fcntl.h>
#include <errno.h>
#include <arm_acle.h>

#define slots 16
#define stripe_cap (((height*W*2+4095)/4096)*4096)
struct Record{
    char magic[8];
    uint32_t request,width,rows,stripes;
    uint64_t hardware,exposure,duration;
    uint32_t sizes[parts],crc[parts],has_crc;
};
struct Slot{struct Record *header; uint8_t *data;};
static struct Slot queue[slots],*current;
static unsigned prod,cons,peak;
static pthread_mutex_t lock=PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t wake=PTHREAD_COND_INITIALIZER;
static pthread_t writer;
static int stopped,fd=-1;
static volatile uint64_t *clockmap;
static FILE *exp_csv, *timing, *bench;
static int dirfd;
static uint64_t first_exp;
static inline unsigned align4k(unsigned n){return (n+4095)&~4095u;}
static uint32_t raw_crc(const uint8_t *p,size_t n){
 uint32_t c=~0u;
 for(size_t i=0;i<n;i+=8){uint64_t v;memcpy(&v,p+i,8);c=__crc32d(c,v);}
 return ~c;
}
static void *write_loop(void *unused){
 for(;;){
  pthread_mutex_lock(&lock);
  while(cons==prod && !stopped)pthread_cond_wait(&wake,&lock);
  if(cons==prod && stopped){pthread_mutex_unlock(&lock);break;}
  struct Slot *s=&queue[cons%slots];
  pthread_mutex_unlock(&lock);
  struct iovec v[1+parts]={{s->header,4096}};
  size_t total=4096;
  for(int i=0;i<parts;i++){
   unsigned len=align4k(s->header->sizes[i]);v[i+1]=(struct iovec){s->data+(size_t)i*stripe_cap,len};total+=len;
  }
  uint64_t a=now();
  int idx=0;
  while(idx<=parts){
   ssize_t n=writev(fd,v+idx,1+parts-idx);
   if(n<0&&errno==EINTR)continue;
   if(n<=0||n%4096)die("direct write");
   while(idx<=parts&&(size_t)n>=v[idx].iov_len){n-=v[idx].iov_len;idx++;}
   if(n){v[idx].iov_base=(char*)v[idx].iov_base+n;v[idx].iov_len-=n;}
  }
  uint64_t done=now();
  struct Record *r=s->header;
  fprintf(bench,"disk,%u,0,%zu,%llu,%llu\n",r->hardware > UINT32_MAX ? r->request : (unsigned)r->hardware,total,(unsigned long long)a,(unsigned long long)done);
  fprintf(exp_csv,getenv("EYE")[0]=='1'?"%u,,%llu\n":"%u,%llu,\n",r->request,(unsigned long long)r->duration);
  fprintf(timing,"%u,%llu,%llu,%llu\n",r->request,(unsigned long long)r->hardware,(unsigned long long)r->exposure,(unsigned long long)r->duration);
  if(cons%8==0) fflush(bench);
  if(ferror(exp_csv)||ferror(timing)||ferror(bench))die("frame metadata");
  pthread_mutex_lock(&lock);cons++;pthread_mutex_unlock(&lock);
 }
 for(unsigned k=0;k<3;k++){
  FILE *f=((FILE *[]){bench,timing,exp_csv})[k];
  if(fflush(f)||fsync(fileno(f))||fclose(f))die("close frame metadata");
 }
 if(fdatasync(fd))die("fdatasync");
 FILE *receipt=fopen("writer-receipt","wx");struct stat sealed;
 if(!receipt||fstat(fd,&sealed)||fprintf(receipt,"%u %u %llu\n",cons,peak,(unsigned long long)sealed.st_size)<1||fclose(receipt))die("write receipt");
 dprintf(2,"durable,%u,%u,%llu\n",cons,peak,(unsigned long long)now());return NULL;
}
static void disk_init(void){
 const char *path=getenv("CARD_RUN");if(!path)die("directory");
 int dir=open(path,O_DIRECTORY|O_NOFOLLOW|O_RDONLY);
 struct stat d,b;struct statvfs v;
 if(dir<0||fstat(dir,&d)||stat(getenv("CARD_DEVICE"),&b)||!S_ISBLK(b.st_mode)||b.st_rdev!=d.st_dev||fstatvfs(dir,&v)||(uint64_t)v.f_bavail*v.f_frsize<6100000000ULL)die("card mount");
 fd=openat(dir,"capture.x6b",O_WRONLY|O_CREAT|O_EXCL|O_DIRECT|O_NOFOLLOW,0600);
 if(fd<0||fsync(dir))die("open file");dirfd=dir;
 FILE **streams[]={&exp_csv,&timing,&bench};
 const char *names[]={"exposure.csv","frames.csv",".writer.log"};
 for(unsigned k=0;k<3;k++){
   int f=openat(dir,names[k],O_WRONLY|O_CREAT|O_EXCL|O_NOFOLLOW,0600);
   if(f<0||!(*streams[k]=fdopen(f,"w")))die("frame metadata");
 }
 fputs("request,cam0_exposure_ns,cam1_exposure_ns\n",exp_csv);
 fputs("request,hardware_frame,exposure_timestamp_ns,exposure_duration_ns\n",timing);
 int meta=open("clockmap",O_RDONLY);
 if(meta<0)die("clock open");
 clockmap=mmap(NULL,16384*32,PROT_READ,MAP_SHARED,meta,0);close(meta);
 if(clockmap==MAP_FAILED)die("clock map");
 for(int i=0;i<slots;i++){
  if(posix_memalign((void**)&queue[i].header,4096,4096))die("header alignment");
  memset(queue[i].header,0,4096);
  queue[i].data=mmap(NULL,(size_t)stripe_cap*parts,PROT_READ|PROT_WRITE,MAP_PRIVATE|MAP_ANONYMOUS,-1,0);
  if(queue[i].data==MAP_FAILED)die("queue allocation");
 }
 if(pthread_create(&writer,NULL,write_loop,NULL))die("writer");
}
static int prepare(unsigned req){

 volatile uint64_t *c=clockmap+(req%16384)*4;
 if(c[0]!=req){dprintf(2,"missing-clock,%u\n",req);return 0;}
 uint64_t hw=c[1],exp=c[2],dur=c[3];
 if(!first_exp){FILE *first=fopen("first-request","wx");if(!first||fprintf(first,"%llu",(unsigned long long)hw)<1||fclose(first))die("first frame");first_exp=exp;dprintf(2,"window,%u,%llu,%llu\n",req,(unsigned long long)hw,(unsigned long long)exp);}
 if(exp<first_exp)return 0;
 pthread_mutex_lock(&lock);
 if(prod-cons==slots){pthread_mutex_unlock(&lock);dprintf(2,"disk-full-queue,%u\n",req);return 0;}
 current=&queue[prod%slots];pthread_mutex_unlock(&lock);
 *current->header=(struct Record){.magic="X6S8B02",.request=req,.width=W,.rows=W,.stripes=parts,.hardware=hw,.exposure=exp,.duration=dur,.has_crc=(prod==0)};
 return 1;
}
static void publish(void){
 pthread_mutex_lock(&lock);prod++;if(prod-cons>peak)peak=prod-cons;
 pthread_cond_signal(&wake);pthread_mutex_unlock(&lock);
}
static void finish_disk(void){
 if(fd<0)return;
 pthread_mutex_lock(&lock);stopped=1;pthread_cond_signal(&wake);pthread_mutex_unlock(&lock);
 pthread_join(writer,NULL);close(fd);
}
