enum { DIRECT=2, MAX_SPARE=32 };
#ifndef SLICE
#define SLICE 8
#endif
typedef struct { Pair p; uint64_t ready_ns,elapsed_ns,first_ns; unsigned stripe;uint64_t cpu,cycles,instructions,pmu_ns; } Item;
static Item live[DIRECT],spare[MAX_SPARE];
static unsigned spare_cap,peak;
static uint64_t live_in,live_out,spare_in,spare_out;
static uint8_t *outbuf[2][MAXSTRIPES*2];
static int sample_kind[SAMPLES],kind_count[2];
static uint64_t last_sample[2];
static uint64_t available_bytes(void){
 FILE *f=fopen("/proc/meminfo","r");char line[128];unsigned long kb=0;
 if(!f)die("meminfo");
 while(fgets(line,sizeof(line),f))if(sscanf(line,"MemAvailable: %lu",&kb)==1)break;
 fclose(f);return (uint64_t)kb*1024;
}
static void *output_buffer(size_t size){
 unsigned char *mem=mmap(NULL,size,PROT_READ|PROT_WRITE,MAP_PRIVATE|MAP_ANONYMOUS,-1,0);
 if(mem==MAP_FAILED)die("output buffer");
 /* Pre-touch the usual stripe payload. Keep worst-case capacity for bright/noisy frames. */
 size_t hot=stripebytes/2;
 for(size_t n=0;n<hot;n+=4096)((volatile unsigned char *)mem)[n]=0;
 return mem;
}
static void init_queues(void){
 for(int kind=0;kind<2;kind++)for(int j=0;j<lanes;j++)outbuf[kind][j]=output_buffer(cap);
 for(int j=0;j<SAMPLES;j++)for(int e=0;e<2;e++)samples[j].raw[e]=alloc_hot(RAW);
 /* Keep space for camera services, four compressed verification samples and traces. */
 uint64_t available=available_bytes(),reserve=700ull*1024*1024;
 if(available<11ull*2*RAW+512ull*1024*1024)fprintf(stderr,"RAM before RAW reserve: %llu bytes\n",(unsigned long long)available);if(available<reserve+4ull*RAW)die("spare memory");
 spare_cap=(available-reserve)/(2*RAW);if(available<11ull*2*RAW+512ull*1024*1024)die("11-pair reserve");spare_cap=11;
 for(unsigned j=0;j<spare_cap;j++)for(int e=0;e<2;e++)spare[j].p.raw[e]=alloc_hot(RAW);
}
static void release_read(int fd){
 struct dma_buf_sync sync={.flags=DMA_BUF_SYNC_END|DMA_BUF_SYNC_READ};
 if(ioctl(fd,DMA_BUF_IOCTL_SYNC,&sync))die("dma end");
}
static void sample_if_due(Pair *p,int kind,uint64_t stop){
 /* Use an owned spare buffer. The card supplies the compressed data at verification. */
 if (!kind || sampled==SAMPLES || (sampled && stop-last_sample[1]<12500000000ull))return;
 Pair *s=&samples[sampled];
 for(int eye=0;eye<2;eye++){
  unsigned char *available=s->raw[eye];s->raw[eye]=p->raw[eye];p->raw[eye]=available;
 }
 for(int j=0;j<lanes;j++){
  s->len[j]=p->len[j];
  s->out[j]=mmap(NULL,(s->len[j]+4095)&~(size_t)4095,PROT_READ|PROT_WRITE,MAP_PRIVATE|MAP_ANONYMOUS,-1,0);
  if(s->out[j]==MAP_FAILED)die("verify buffer");
 }
 memcpy(s->exposure,p->exposure,sizeof(s->exposure));s->req=p->req;sample_kind[sampled++]=kind;kind_count[kind]++;last_sample[kind]=stop;
 printf("sample,%d,%d\n",p->req,kind);
}
static void finish(Item *it,int kind,uint64_t stop){
 Pair *p=&it->p;
 int valid=kind || stop<p->deadline;
 if(valid)sample_if_due(p,kind,stop);
 if(!kind){release_read(p->handles[0]);release_read(p->handles[1]);}
 printf("in,%d,%s,%llu,%llu,%llu,%llu\n",p->req,valid?"ok":"readlate",(unsigned long long)p->ino[0],(unsigned long long)p->ino[1],(unsigned long long)p->entered,(unsigned long long)p->copied);
 printf("route,%d,%d,%llu,%llu,%llu,%llu\n",p->req,kind,(unsigned long long)(kind?p->copied:stop),(unsigned long long)it->elapsed_ns,(unsigned long long)it->ready_ns,(unsigned long long)stop);
 if(valid){
  size_t bytes[2]={0},serialized=4096;
  for(int j=0;j<lanes;j++){bytes[j/stripes]+=p->len[j];serialized+=(p->len[j]+4095)&~(size_t)4095;}
  printf("enc,%d,%llu,%llu,%llu,%zu,%zu\n",p->req,(unsigned long long)p->copied,(unsigned long long)it->first_ns,(unsigned long long)stop,bytes[0],bytes[1]);
  printf("serialized,%d,%zu\n",p->req,serialized);
  enqueue(p);
  for(int j=0;j<lanes;j++)outbuf[kind][j]=p->out[j];
 }
}
static void *schedule(void *arg){
 for(;;){
  pthread_mutex_lock(&mutex);
  while(live_in==live_out && spare_in==spare_out && !finished)pthread_cond_wait(&ready,&mutex);
  if(live_in==live_out && spare_in==spare_out && finished){pthread_mutex_unlock(&mutex);break;}
  int kind=live_in==live_out;
  Item *it=kind?&spare[spare_out%spare_cap]:&live[live_out%DIRECT];
  int slice=kind && !finished?SLICE:lanes;
  pthread_mutex_unlock(&mutex);
  job=&it->p;
  if(!it->stripe)for(int j=0;j<lanes;j++)job->out[j]=outbuf[kind][j];
  stripe_end=it->stripe+slice;if(stripe_end>(unsigned)lanes)stripe_end=lanes;
  if(!it->stripe){it->cpu=it->cycles=it->instructions=it->pmu_ns=0;}
  atomic_store(&next_stripe,it->stripe);
  uint64_t start=ns();if(!it->stripe)it->first_ns=start;
  pthread_barrier_wait(&begin);pthread_barrier_wait(&end);uint64_t stop=ns();
  for(int j=0;j<THREADS;j++){it->cpu+=counters[j].cpu;it->cycles+=counters[j].cycles;it->instructions+=counters[j].instructions;it->pmu_ns+=counters[j].running;}
  it->elapsed_ns+=stop-start;it->stripe=stripe_end;
  if(it->stripe==(unsigned)lanes){
   printf("work,%d,%llu,%llu,%llu,%llu\n",it->p.req,(unsigned long long)it->cpu,(unsigned long long)it->cycles,(unsigned long long)it->instructions,(unsigned long long)it->pmu_ns);
   finish(it,kind,stop);
   pthread_mutex_lock(&mutex);if(kind)spare_out++;else live_out++;pthread_mutex_unlock(&mutex);
  }
 }
 job=NULL;pthread_barrier_wait(&begin);return NULL;
}
