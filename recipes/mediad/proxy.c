#include "strings.h"
X6_PAGE_STRING(width_key);
X6_PAGE_STRING(release_failed);
X6_STRING(proxy_stream);
X6_PAGE_STRING(height_key);
X6_PAGE_STRING(proxy_disabled);
X6_PAGE_STRING(release_log);
X6_PAGE_STRING(release_ok);

// Version/hash-pinned X6 1.2.36 native StopStreamByContext wrapper.
// No heap allocation, injected library, new service, or direct FD/buffer freeing.
typedef unsigned long U;
extern int native_active(void*,unsigned char);
extern int native_release(void*,void*);
extern void *native_builder(void);
extern void *gst_pad_get_parent_element(void*), *gst_pad_get_current_caps(void*), *gst_caps_get_structure(void*,unsigned);
extern int gst_structure_get_int(void*,const char*,int*);
extern void gst_mini_object_unref(void*),gst_object_unref(void*);
static void *ptr(void*p,U o){return *(void**)((char*)p+o);}
static unsigned word(void*p,U o){return *(unsigned*)((char*)p+o);}
static long sys4(long nr,long a,long b,long c,long d){
 register long x8 __asm__("x8")=nr; register long x0 __asm__("x0")=a;
 register long x1 __asm__("x1")=b;register long x2 __asm__("x2")=c;register long x3 __asm__("x3")=d;
 __asm__ volatile("svc 0":"+r"(x0):"r"(x8),"r"(x1),"r"(x2),"r"(x3):"memory","cc");return x0;
}
static int enabled(void){
 // AArch64 stat: mode+16, uid+24; 128-byte buffer. Root-private, one byte, no symlink.
 long fd=sys4(56,-100,(long)proxy_disabled,02000000|0400000,0);
 if(fd<0)return 0;
 U st[16];unsigned char b[2]={255,255};int ok=0;
 if(sys4(80,fd,(long)st,0,0)==0){unsigned mode=word(st,16);if((mode&0170000)==0100000 && !(mode&0022) && word(st,24)==0) ok=sys4(63,fd,(long)b,2,0)==1 && b[0]==1;}
 sys4(57,fd,0,0,0);return ok;
}
static void note(int ok){
 const char *s=ok?release_ok:release_failed;U len=ok?22:26;
 long fd=sys4(56,-100,(long)release_log,02000000|0400000|02000|0100|1,0600);
 if(fd<0)return;U st[16];
 if(sys4(80,fd,(long)st,0,0)==0 && (word(st,16)&0170000)==0100000 && word(st,24)==0 && st[6]<65536)sys4(64,fd,(long)s,len,0);
 sys4(57,fd,0,0,0);
}
__attribute__((section(".text.entry"))) int x6_proxy_release(void *ctx,unsigned char active){
 const char *name=ctx?ptr(ctx,0x68):0;
 if(active || !ctx || !name || *(U*)((char*)ctx+0x70)!=10 || !enabled())return native_active(ctx,active);
 const char *expect=proxy_stream;for(unsigned i=0;i<10;i++)if(name[i]!=expect[i])return native_active(ctx,active);
 void *obj=ptr(ctx,0x58),*pad=obj?ptr(obj,0x40):0;
 if(!pad)return native_active(ctx,active);
 void *parent=gst_pad_get_parent_element(pad);int width=0,height=0;
 void *caps=gst_pad_get_current_caps(pad);
 if(caps){void*s=gst_caps_get_structure(caps,0);if(s){gst_structure_get_int(s,width_key,&width);gst_structure_get_int(s,height_key,&height);}gst_mini_object_unref(caps);}
 int result=native_active(ctx,active);
 void *builder=native_builder(),*element=builder?ptr(builder,0x110):0;
 if(result==0 && parent && element && ptr(element,8)==parent && width==2048 && height==1024 && ptr(obj,0x40)==pad && word(pad,0x29c)==4 && word(pad,0x290)==3 && *((unsigned char*)obj+0x48)==1){
  int r=native_release(obj,(char*)builder+0x110);note(r==0 && ptr(obj,0x40)==0);
 }
 if(parent)gst_object_unref(parent);
 return result;
}
