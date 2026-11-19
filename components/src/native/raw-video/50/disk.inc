#include <sys/uio.h>
#define ALIGN_BYTES 4096
#define OUTPUT_SLOTS 8
struct Header {
 char magic[8];uint32_t bytes,request,w,h,parts,shift;
 uint64_t exposure[2];
 uint32_t sizes[48];
};
_Static_assert(sizeof(struct Header)==240,"header format");
typedef struct {unsigned char *blocks[48];struct Header h;} DiskItem;
static DiskItem items[OUTPUT_SLOTS];
static pthread_mutex_t dlock=PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t changed=PTHREAD_COND_INITIALIZER;
static unsigned prod,cons;static int finished_disk,stream_fd,folder_fd;
static const char *capture_name="capture.x6b";
static size_t aligned(size_t n){return (n+ALIGN_BYTES-1)&~(size_t)(ALIGN_BYTES-1);}

typedef struct { unsigned char *blocks[48]; } Buffers;
static Buffers disk_spare[OUTPUT_SLOTS];
static unsigned nspare, allocated, highwater;
static int new_set_fits(void){
 FILE *f=fopen("/proc/meminfo","r");
 if(!f)return 0;
 char line[160];unsigned long kb=0;
 while(fgets(line,sizeof(line),f))if(sscanf(line,"MemAvailable: %lu kB",&kb)==1)break;
 fclose(f);
 return kb>520000;
}
static Buffers allocate_set(void){
 Buffers b;
 for(int j=0;j<48;j++){
  b.blocks[j]=mmap(NULL,aligned(cap),PROT_READ|PROT_WRITE,MAP_PRIVATE|MAP_ANONYMOUS,-1,0);
  if(b.blocks[j]==MAP_FAILED)die("output buffer");
 }
 allocated++;return b;
}
static void enqueue(Pair *p){
 pthread_mutex_lock(&dlock);
 uint64_t wait_start=ns();unsigned old_depth=prod-cons;
 while(prod-cons==OUTPUT_SLOTS || (!nspare && allocated && cons!=prod && !new_set_fits()))
  pthread_cond_wait(&changed,&dlock);
 uint64_t wait_end=ns();
 printf("wqueue,%d,%u,%llu,%llu\n",p->req,old_depth,(unsigned long long)wait_start,(unsigned long long)wait_end);
 Buffers free_buf=nspare?disk_spare[--nspare]:allocate_set();
 DiskItem *v=&items[prod%OUTPUT_SLOTS];v->h=(struct Header){.magic="X6M50AB2",.bytes=ALIGN_BYTES,.request=p->req,.w=W,.h=W,.parts=24,.shift=13472,.exposure={p->exposure[0],p->exposure[1]}};
 for(int j=0;j<48;j++){
  size_t n=p->len[j];v->h.sizes[j]=n;
  memset(p->out[j]+n,0,aligned(n)-n);
  v->blocks[j]=p->out[j];p->out[j]=free_buf.blocks[j];
 }
 prod++;if(prod-cons>highwater)highwater=prod-cons;
 printf("depth,%d,%u,%u,%llu\n",p->req,prod-cons,allocated,(unsigned long long)ns());
 pthread_cond_broadcast(&changed);pthread_mutex_unlock(&dlock);
}
static void *writer(void *unused){
 unsigned char *header=alloc_hot(ALIGN_BYTES);off_t offset=0;
 for(;;){
  pthread_mutex_lock(&dlock);while(cons==prod&&!finished_disk)pthread_cond_wait(&changed,&dlock);
  if(cons==prod&&finished_disk){pthread_mutex_unlock(&dlock);break;}
  DiskItem *v=&items[cons%OUTPUT_SLOTS];pthread_mutex_unlock(&dlock);
  memcpy(header,&v->h,sizeof(v->h));struct iovec vec[49]={{header,ALIGN_BYTES}};
  size_t total=ALIGN_BYTES;
  for(int j=0;j<48;j++){size_t n=aligned(v->h.sizes[j]);vec[j+1]=(struct iovec){v->blocks[j],n};total+=n;}
  uint64_t begin_write=ns();int idx=0;
  while(idx<49){
   ssize_t wrote=writev(stream_fd,vec+idx,49-idx);
   if(wrote<0&&errno==EINTR)continue;
   if(wrote<=0)die("SSD writev");
   while(idx<49 && (size_t)wrote>=vec[idx].iov_len){wrote-=vec[idx].iov_len;idx++;}
   if(wrote>0){if(wrote%ALIGN_BYTES)die("unaligned partial SSD write");vec[idx].iov_base=(char*)vec[idx].iov_base+wrote;vec[idx].iov_len-=wrote;}
  }
  printf("disk,%u,%lld,%zu,%llu,%llu\n",v->h.request,(long long)offset,total,(unsigned long long)begin_write,(unsigned long long)ns());
  offset+=total;
  pthread_mutex_lock(&dlock);memcpy(disk_spare[nspare++].blocks,v->blocks,sizeof(v->blocks));cons++;pthread_cond_broadcast(&changed);pthread_mutex_unlock(&dlock);
 }
 if(fdatasync(stream_fd)||close(stream_fd))die("SSD flush");
 printf("flushed,%u,%lld,%llu\n",cons,(long long)offset,(unsigned long long)ns());
 return NULL;
}
static void disk_start(pthread_t *thread){
 stream_fd=openat(folder_fd,capture_name,O_WRONLY|O_CREAT|O_EXCL|O_DIRECT,0644);
 if(stream_fd<0)die("open SSD file");

 if(pthread_create(thread,NULL,writer,NULL))die("writer");
}
