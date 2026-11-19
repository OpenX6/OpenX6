#define _GNU_SOURCE
#include <stdint.h>
#include <stdatomic.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <unistd.h>
#include <fcntl.h>
#include <sys/syscall.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/ioctl.h>
#include <linux/dma-buf.h>
#include <pthread.h>
#include <time.h>
#include <arm_neon.h>


#include "levels.h"
enum { level = 4, raw_shift = 3 };
static uint64_t sample_interval;
#define W 3840
#define RAW (W*W*2U)
#ifndef Q
#define Q 3
#endif
#define MAXSTRIPES 24
#define THREADS 8
#define SAMPLES 0
typedef struct {unsigned char *raw[2],*out[MAXSTRIPES*2];size_t len[MAXSTRIPES*2];int req;uint64_t exposure[2],copied,deadline,entered,ino[2];int handles[2];} Pair;
typedef struct {int lane,part,eye; unsigned char *scratch;} Lane;
static int stripes, lanes, height, pidfd;
static size_t stripebytes,cap;
static Pair samples[SAMPLES];static int sampled;
static Lane pool[MAXSTRIPES*2];static pthread_barrier_t begin,end;
static Pair *job;static uint64_t input=0,output=0;
static pthread_cond_t ready=PTHREAD_COND_INITIALIZER;
static pthread_mutex_t mutex=PTHREAD_MUTEX_INITIALIZER;
static int finished=0;
static void die(const char *s){perror(s);_exit(3);}
static uint64_t ns(void){struct timespec t;clock_gettime(CLOCK_MONOTONIC,&t);return (uint64_t)t.tv_sec*1000000000+t.tv_nsec;}
static void *alloc_hot(size_t n){void *v=mmap(NULL,n,PROT_READ|PROT_WRITE,MAP_PRIVATE|MAP_ANONYMOUS|MAP_POPULATE,-1,0);if(v==MAP_FAILED)die("memory");return v;}

static void allocpair(Pair *p){for(int e=0;e<2;e++)p->raw[e]=alloc_hot(RAW);for(int j=0;j<lanes;j++)p->out[j]=alloc_hot(cap);}
#include "workers_spill.h"
#include "disk.inc"
#include "scheduler.h"
typedef struct {ino_t inode;int fd;unsigned char *map;} Mapping;
static Mapping maps[192];
static int mapped;
static Mapping *acquire(int foreign){
 int fd=syscall(438,pidfd,foreign,0);if(fd<0)return NULL;
 struct stat st;if(fstat(fd,&st)){close(fd);return NULL;}
 int n=0;while(n<mapped && maps[n].inode!=st.st_ino)n++;
 if(n==mapped){
  if(mapped==192)die("map limit");
  char path[80],line[160];size_t bytes=0;snprintf(path,sizeof(path),"/proc/self/fdinfo/%d",fd);
  FILE *f=fopen(path,"r");if(f){while(fgets(line,sizeof(line),f))if(sscanf(line,"size: %zu",&bytes)==1)break;fclose(f);}
  if(bytes!=29675520){close(fd);return NULL;}
  void *addr=mmap(NULL,RAW,PROT_READ,MAP_SHARED,fd,0);if(addr==MAP_FAILED){close(fd);return NULL;}
  maps[n]=(Mapping){st.st_ino,fd,addr};mapped++;
 }else close(fd);
 struct dma_buf_sync sync={.flags=DMA_BUF_SYNC_START|DMA_BUF_SYNC_READ};
 if(ioctl(maps[n].fd,DMA_BUF_IOCTL_SYNC,&sync))return NULL;
 return &maps[n];
}
#include "input.h"
#include "rice_verify.h"
int main(int argc,char **argv){
 if(argc!=4)return 2;
 struct stat card,dir;
 folder_fd=open(argv[2],O_RDONLY|O_DIRECTORY|O_NOFOLLOW);
 if(folder_fd<0||fstat(folder_fd,&dir)||stat(argv[3],&card)||!S_ISBLK(card.st_mode)||dir.st_dev!=card.st_rdev)die("card identity");
 pidfd=syscall(434,atoi(argv[1]),0);if(pidfd<0)die("pidfd");
 stripes=24;lanes=stripes*2;height=W/stripes;stripebytes=RAW/stripes;cap=stripebytes*2+65536;
 sample_interval=18000000000ull;
 init_queues();pthread_t disk_thread;disk_start(&disk_thread);
 pthread_t th[MAXSTRIPES*2],scheduler;pthread_barrier_init(&begin,NULL,THREADS+1);pthread_barrier_init(&end,NULL,THREADS+1);
 for(int j=0;j<THREADS;j++){pool[j]=(Lane){j,j%stripes,j/stripes,NULL};if(pthread_create(&th[j],NULL,worker,&pool[j]))die("worker");}

 if(pthread_create(&scheduler,NULL,schedule,NULL))die("schedule");
 setvbuf(stdout,NULL,_IOLBF,0);puts("ready");
 int req,fd[2];unsigned long long t[2],ino[2];
 unsigned long long e0,e1;
 while(scanf("%d %d %llu %d %llu %llu %llu",&req,&fd[0],&t[0],&fd[1],&t[1],&e0,&e1)==7){
  accept_input(req,fd,t,e0,e1);
 }
 pthread_mutex_lock(&mutex);finished=1;pthread_cond_signal(&ready);pthread_mutex_unlock(&mutex);
 pthread_join(scheduler,NULL);for(int j=0;j<THREADS;j++)pthread_join(th[j],NULL);
 for(int j=0;j<mapped;j++){munmap(maps[j].map,RAW);close(maps[j].fd);}
 printf("drained,%llu\n",(unsigned long long)ns());
 pthread_mutex_lock(&dlock);finished_disk=1;pthread_cond_broadcast(&changed);pthread_mutex_unlock(&dlock);
 pthread_join(disk_thread,NULL);
 stream_fd=openat(folder_fd,capture_name,O_RDONLY|O_DIRECT);
 if(stream_fd<0)die("readback");
 
 verify();close(stream_fd);close(folder_fd);
 return 0;
}
