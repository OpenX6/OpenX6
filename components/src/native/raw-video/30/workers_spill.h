#if FAST
#include "fast3.h"
#else
#include "profile.h"
#endif
static uint64_t breakdown[8][2];
static uint64_t times[8][3];
#include "tail.h"
static const unsigned threshold=13472;
static _Atomic unsigned next_stripe;
static unsigned stripe_end;
#include "counter.h"
static void *worker(void *arg){
    Lane *l=arg;int leader=open_pmu();
    uint16_t history[2*W];
    for(;;){
        pthread_barrier_wait(&begin);if(!job)break;
        PerfValues before=perf_read(leader);uint64_t cpu0=thread_ns();
        times[l->lane][0]=ns();
        uint64_t *st=breakdown[l->lane];st[0]=st[1]=0;
        unsigned lane;
        while((lane=atomic_fetch_add_explicit(&next_stripe,1,memory_order_relaxed))<stripe_end){
        const uint16_t *raw=(uint16_t*)job->raw[lane/stripes]+(lane%stripes)*height*W;
        size_t pixels=(size_t)height*W;unsigned tails=0;
        uint8_t *out=job->out[lane];
        uint32_t size=pack_rice(raw,out+4,history,&tails,&st[0],&st[1]);

        
        size_t tailbytes=tails?write_tails(raw,pixels,out+4+size):0;
        size_t total=4+size+tailbytes;
        if(total>cap)die("packet size");
        memcpy(out,&size,4);
        job->len[lane]=total;
        }
        times[l->lane][1]=ns();
        times[l->lane][2]=ns();uint64_t cpu1=thread_ns();PerfValues after=perf_read(leader);
        counters[l->lane]=(Totals){cpu1-cpu0,after.cycles-before.cycles,after.instructions-before.instructions,after.running-before.running};pthread_barrier_wait(&end);
    }
    return NULL;
}
