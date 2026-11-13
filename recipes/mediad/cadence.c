#include "strings.h"
X6_STRING(options_path);
X6_PAGE_STRING(capture_path);

#include "../fast_capture/io.h"
struct State {U owner,period,next,skipped,captures,cap,late;};
extern struct State x6_interval_state __attribute__((visibility("hidden")));
extern long mono_now(void);
struct Options {I period,cap;U time;};
struct Permit {U time;I cap,reserved;};
static int scoped(void*cfg){I*c=cfg;return c&&c[0]==7&&c[1]==0&&c[2]==1&&c[3]==1&&c[22]==17&&c[23]==5&&c[24]==3;}
int x6_interval_capture(void*p,void*cfg,int(*original)(void*,void*),char*ctx){
 struct State*s=&x6_interval_state;U ms=now_ms();
 if(__atomic_load_n((U*)(ctx+0x138),__ATOMIC_ACQUIRE)==0){
  struct Options o={0,0,0};s->owner=(U)ctx;s->period=0;s->cap=0;s->skipped=0;s->captures=0;s->late=0;
  if(scoped(cfg)&&ms&&transfer(options_path,&o,sizeof(o),0)&&o.time&&ms>=o.time&&ms-o.time<=10000){
   if(o.cap>=30&&o.cap<=2000)s->cap=o.cap;
   if(o.period>=4000&&o.period<=120000)s->period=(U)o.period*1000000;
  }
  long now=mono_now();if(now<=0)s->period=0;
  s->next=(U)now+s->period;
 }else if(s->owner==(U)ctx&&s->period){
  if(s->late){long n=mono_now();if(n>0)s->next=(U)n+s->period;else s->period=0;s->late=0;}
  else s->next+=s->period;
 }
 if(s->owner==(U)ctx)s->captures++;
 // Native IntervalTimed + actual per-capture dimensions/format, not a global mode snapshot.
 struct Permit permit={ms,scoped(cfg)&&s->owner==(U)ctx?(I)s->cap:0,0};
 transfer(capture_path,&permit,sizeof(permit),1);
 return original(p,cfg);
}
U x6_interval_wait(char*ctx){
 struct State*s=&x6_interval_state;U old=__atomic_load_n((U*)(ctx+0x1d0),__ATOMIC_ACQUIRE);
 if(s->owner!=(U)ctx||!s->period)return old;
 long n=mono_now();if(n<=0){s->period=0;return old;}
 U now=(U)n;
 if(now>=s->next){s->skipped++;s->late=1;__atomic_store_n((U*)(ctx+0x1d0),1,__ATOMIC_RELEASE);return 1;}
 U ms=(s->next-now+999999UL)/1000000UL;
 __atomic_store_n((U*)(ctx+0x1d0),ms,__ATOMIC_RELEASE);return ms;
}
