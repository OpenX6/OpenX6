#include <linux/perf_event.h>
typedef struct {uint64_t nr,enabled,running,cycles,instructions;} PerfValues;
typedef struct {uint64_t cpu,cycles,instructions,running;} Totals;
static Totals counters[THREADS] __attribute__((aligned(64)));
static int open_pmu(void){
 struct perf_event_attr a={.type=PERF_TYPE_HARDWARE,.size=sizeof(a),.config=PERF_COUNT_HW_CPU_CYCLES,.disabled=1,.exclude_kernel=1,.exclude_hv=1,.read_format=PERF_FORMAT_GROUP|PERF_FORMAT_TOTAL_TIME_ENABLED|PERF_FORMAT_TOTAL_TIME_RUNNING};
 int fd=syscall(SYS_perf_event_open,&a,0,-1,-1,PERF_FLAG_FD_CLOEXEC);
 if(fd<0)return -1;
 a.config=PERF_COUNT_HW_INSTRUCTIONS;a.disabled=0;
 int member=syscall(SYS_perf_event_open,&a,0,-1,fd,PERF_FLAG_FD_CLOEXEC);
 if(member<0||ioctl(fd,PERF_EVENT_IOC_ENABLE,PERF_IOC_FLAG_GROUP)<0){close(fd);return -1;}
 return fd;
}
static PerfValues perf_read(int fd){PerfValues r={0};if(fd>=0 && (read(fd,&r,sizeof(r))!=sizeof(r)||r.nr!=2))memset(&r,0,sizeof(r));return r;}
static uint64_t thread_ns(void){struct timespec t;clock_gettime(CLOCK_THREAD_CPUTIME_ID,&t);return (uint64_t)t.tv_sec*1000000000+t.tv_nsec;}
