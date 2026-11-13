#include "../fast_capture/io.h"
I tl_rate(const void *cfg,I original){
 const char *c=cfg;
 if(!cfg || *(const I*)(c+8)!=1 || *(const I*)(c+12)!=7680 || *(const I*)(c+16)!=7680 || *(const unsigned short*)(c+48)>1 || original!=180000000) return original;
 struct { U time; I enabled,tag;} p;
 if(!transfer("/run/x6-interval-controls/tl-options",&p,16,0)) return original;
 U n=now_ms();
 if(!n || p.time>n || n-p.time>5000 || p.enabled!=1 || p.tag!=0x544c3136) return original;
 return 245000000;
}
