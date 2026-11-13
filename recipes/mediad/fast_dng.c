#include "strings.h"
X6_STRING(dng_preference);

typedef unsigned long U;
extern int TIFFSetField(void*,unsigned,...);
extern unsigned TIFFNumberOfStrips(void*);
static long sc(long nr,long a,long b,long c,long d){register long x8 __asm__("x8")=nr,x0 __asm__("x0")=a,x1 __asm__("x1")=b,x2 __asm__("x2")=c,x3 __asm__("x3")=d;__asm__ volatile("svc 0":"+r"(x0):"r"(x8),"r"(x1),"r"(x2),"r"(x3):"memory","cc");return x0;}
static int enabled(void){
 long fd=sc(56,-100,(long)dng_preference,0x88800,0);if(fd<0)return 0;
 U st[16];unsigned char b[2];int ok=0;
 if(sc(80,fd,(long)st,0,0)==0 && (*(unsigned*)((char*)st+16)&0170022)==0100000 && *(unsigned*)((char*)st+24)==0 && *(unsigned*)((char*)st+20)==1 && st[6]==1)ok=sc(63,fd,(long)b,2,0)==1 && b[0]==1;
 sc(57,fd,0,0,0);return ok;
}
// Only called at the validated DNG Deflate writer, after its TIFF field setup.
unsigned x6_fast_dng(void*t,const unsigned short*meta){
 if(t && meta && meta[4]==15520 && meta[5]==7760 && meta[6]==16 && meta[7]==8 && enabled())TIFFSetField(t,65557,0);
 return TIFFNumberOfStrips(t);
}
