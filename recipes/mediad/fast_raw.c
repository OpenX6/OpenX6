#include "strings.h"
X6_PAGE_STRING(options_path);
X6_PAGE_STRING(fast16_path);
X6_PAGE_STRING(dng_preference);
X6_STRING(fast8_path);
X6_PAGE_STRING(capture_path);
X6_PAGE_STRING(node_path);

#include "../fast_capture/io.h"
/* Interval scheduling for the 1.2.36 media service, including 16K Fast RAW.
 * 16K native (no fresh fast16k permit): unchanged policy (fixed grid, 4 s minimum, FrameDone
 *   serial, 1 ms wait floor, level-0 DNG only with the owner's fast-dng flag).
 * 8K Interval RAW Only: unchanged (Shutter Max permit, 100 ms timer floor, lossless level-0 DNG,
 *   optional per-capture passthrough permit for the IIS node, and (Minimum timing only)
 *   GrabDone-driven scheduling with at most 3 frames in post-processing).
 * 16K Fast RAW: a fresh per-series permit /run/x6-interval-controls/fast16k {u64 time_ms;
 *   u32 enabled; u32 tag "F16K"} with (enabled & 0xff) == 1 gives the 16K series the same fast
 *   path: 100 ms period/timer floor, the per-capture node passthrough permit (unprocessed
 *   passthrough DNGs, converted later in camera by pureshot-batch), GrabDone overlap with at most
 *   LIMIT16 frames in post-processing (bits 8..15 of enabled override it, clamped to 1..3) and
 *   level-0 DNG for the series. Each scope reads only its own permit file and tag. */
struct State {U owner,period,next,skipped,captures,cap,late,scope,overlap,passthrough,pending,deferred,last_ms,grabs,dones,deferrals,token,cleared,limit;};
/* limit (in-flight bound of this series) is appended; the first 18 fields keep their offsets. */
_Static_assert(__builtin_offsetof(struct State,limit)==144&&sizeof(struct State)<=4096,"State must stay in the 0x8f5000 page");
extern struct State x6_interval_state __attribute__((visibility("hidden")));
extern long mono_now(void);
extern int TIFFSetField(void*,unsigned,...);
extern unsigned TIFFNumberOfStrips(void*);
struct Options {I period,cap;U time;};
struct Permit {U time;I cap,reserved;};
struct Fast {U time;I enabled,tag;};
#define FAST_TAG 0x4b384631u
#define FAST16_TAG 0x4b363146u
#define LIMIT 3
#define LIMIT16 2
/* c[21] (+84) is the self-timer countdown in seconds and c[28] (+112) the interval in ms:
 * owner settings, not part of the mode identity (v2 required c[21]==3, so 8K only matched
 * with a 3 s countdown). */
static U scope_of(const void*cfg){
 const I*c=cfg;
 if(!c||c[0]!=7||c[1]!=0||c[2]!=1||c[3]!=1||c[23]!=5||c[24]!=3)return 0;
 if(c[22]==17)return 16;
 if(c[22]==0)return 8;
 return 0;
}
static int stopping(const char*ctx){
 return __atomic_load_n((const unsigned char*)(ctx+0x1b8),__ATOMIC_ACQUIRE)||__atomic_load_n((const unsigned char*)(ctx+0x1ba),__ATOMIC_ACQUIRE);
}
/* 8K always runs the fast scheduling path; 16K only with this series' fast16k permit. */
static int fast_scope(const struct State*s){return s->scope==8||(s->scope==16&&s->passthrough);}
int x6_interval_capture(void*p,void*cfg,int(*original)(void*,void*),char*ctx){
 struct State*s=&x6_interval_state;U ms=now_ms();U sc=scope_of(cfg);
 if(__atomic_load_n((U*)(ctx+0x138),__ATOMIC_ACQUIRE)==0){
  struct Options o={0,0,0};struct Fast f={0,0,0};
  s->owner=(U)ctx;s->period=0;s->cap=0;s->skipped=0;s->captures=0;s->late=0;s->scope=sc;s->overlap=0;s->passthrough=0;
  s->pending=0;s->deferred=0;s->grabs=0;s->dones=0;s->deferrals=0;s->limit=sc==8?LIMIT:0;
  /* The scope's own fast permit is read first: the 16K period floor depends on it. */
  if((sc==8||sc==16)&&ms&&transfer(sc==8?fast8_path:fast16_path,&f,sizeof(f),0)&&f.tag==(sc==8?FAST_TAG:FAST16_TAG)&&f.time&&ms>=f.time&&ms-f.time<=10000){
   if(sc==8){if(f.enabled==1)s->passthrough=1;}
   else if((f.enabled&0xff)==1){U l=(f.enabled>>8)&0xff;s->passthrough=1;s->limit=!l?LIMIT16:l>3?3:l;}
  }
  int fast=fast_scope(s);
  if(sc&&ms&&transfer(options_path,&o,sizeof(o),0)&&o.time&&ms>=o.time&&ms-o.time<=10000){
   if(o.cap>=30&&o.cap<=2000)s->cap=o.cap;
   if(o.period>=(fast?100u:4000u)&&o.period<=120000)s->period=(U)o.period*1000000;
  }
  long now=mono_now();if(now<=0)s->period=0;
  s->next=(U)now+s->period;
  s->overlap=(fast&&s->period)?1:0;
 }else if(s->owner==(U)ctx&&s->period){
  if(s->late){long n=mono_now();if(n>0)s->next=(U)n+s->period;else{s->period=0;s->overlap=0;}s->late=0;}
  else s->next+=s->period;
 }
 int mine=s->owner==(U)ctx;
 if(mine){s->captures++;s->last_ms=ms;}
 struct Permit permit={ms,sc&&mine?(I)s->cap:0,0};
 transfer(capture_path,&permit,sizeof(permit),1);
 /* passthrough is only ever set for an 8K or 16K series; the capture must be in that scope. */
 struct Fast node={ms,(sc==s->scope&&mine&&s->passthrough)?1:0,FAST_TAG};
 transfer(node_path,&node,sizeof(node),1);
 __atomic_store_n(&s->token,1,__ATOMIC_RELEASE);
 int r=original(p,cfg);
 __atomic_store_n(&s->token,0,__ATOMIC_RELEASE);
 return r;
}
/* Every photo capture enters the pipeline capture method. Only the interval hook above
 * arms the one-shot token; any other capture (normal photo, burst, AEB...) clears the
 * per-capture passthrough permit before its frames reach the IIS node. */
void x6_capture_gate(void){
 struct State*s=&x6_interval_state;
 if(__atomic_exchange_n(&s->token,0,__ATOMIC_ACQ_REL))return;
 struct Fast node={now_ms(),0,FAST_TAG};s->cleared++;
 transfer(node_path,&node,sizeof(node),1);
}
U x6_interval_wait(char*ctx){
 struct State*s=&x6_interval_state;U old=__atomic_load_n((U*)(ctx+0x1d0),__ATOMIC_ACQUIRE);
 if(s->owner!=(U)ctx||!s->period)return old;
 long n=mono_now();if(n<=0){s->period=0;s->overlap=0;return old;}
 U now=(U)n,floor=fast_scope(s)?100:1,ms;
 if(now>=s->next){s->skipped++;s->late=1;ms=floor;}
 else{ms=(s->next-now+999999UL)/1000000UL;if(ms<floor)ms=floor;}
 __atomic_store_n((U*)(ctx+0x1d0),ms,__ATOMIC_RELEASE);return ms;
}
/* Native GrabDone strategy read: 1 schedules the next capture now. */
U x6_interval_grab(char*ctx,const I*flag){
 I orig=__atomic_load_n(flag,__ATOMIC_ACQUIRE);struct State*s=&x6_interval_state;
 if(s->owner!=(U)ctx||!s->overlap)return orig;
 s->grabs++;s->pending++;
 if(s->pending<=s->limit)return 1;
 s->deferred=1;s->deferrals++;return 0;
}
/* Native FrameDone strategy read: 0 takes the serial path that schedules (or completes stop). */
U x6_interval_done(char*ctx,const I*flag){
 I orig=__atomic_load_n(flag,__ATOMIC_ACQUIRE);struct State*s=&x6_interval_state;
 if(s->owner!=(U)ctx||!s->overlap)return orig;
 s->dones++;if(s->pending)s->pending--;
 if(s->deferred){
  if(stopping(ctx))s->deferred=0;
  else if(s->pending<s->limit){s->deferred=0;return 0;}
 }
 return 1;
}
static int fast_enabled(void){
 long fd=sc(56,-100,(long)dng_preference,0x88800,0);if(fd<0)return 0;
 U st[16];unsigned char b[2];int ok=0;
 if(sc(80,fd,(long)st,0,0)==0&&(*(I*)((char*)st+16)&0170022)==0100000&&*(I*)((char*)st+24)==0&&*(I*)((char*)st+20)==1&&st[6]==1)ok=sc(63,fd,(long)b,2,0)==1&&b[0]==1;
 sc(57,fd,0,0,0);return ok;
}
static int recent(U scope){
 struct State*s=&x6_interval_state;U ms=now_ms();
 return s->scope==scope&&s->last_ms&&ms>=s->last_ms&&ms-s->last_ms<=30000;
}
/* Only called at the validated DNG Deflate writer, after its TIFF field setup.
 * 120MP: level 0 with the owner's fast-dng flag, or when the last interval series was a 16K
 * Fast RAW (passthrough) series that captured within 30 s. 8K interval: level 0 when the series
 * runs 8K Fast RAW (per-series permit) or with the flag. */
unsigned x6_fast_dng(void*t,const unsigned short*meta){
 struct State*s=&x6_interval_state;
 if(t&&meta&&meta[6]==16&&meta[7]==8&&((meta[4]==15520&&meta[5]==7760&&(fast_enabled()||(s->passthrough&&recent(16))))||(meta[4]==7776&&meta[5]==3888&&recent(8)&&(s->passthrough||fast_enabled()))))TIFFSetField(t,65557,0);
 return TIFFNumberOfStrips(t);
}
