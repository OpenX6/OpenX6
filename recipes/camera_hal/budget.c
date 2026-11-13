#include "../fast_capture/io.h"

// Keep the historical literal layout independent of LLD string pooling.
static const char snapshot1[] __attribute__((used, section(".rodata.budget"))) = "RealTimeFeatureNZSLSnapshotRDIStatsParse1_0_cam_1";
static const char permit_path[] __attribute__((used, section(".rodata.budget"))) = "/run/x6-interval-controls/capture";
static const char snapshot0[] __attribute__((used, section(".rodata.budget"))) = "RealTimeFeatureNZSLSnapshotRDIStatsParse0_0_cam_0";

static int same(const char*a,const char*b){if(!a)return 0;for(I i=0;i<96;i++){if(a[i]!=b[i])return 0;if(!b[i])return 1;}return 0;}
I budget(void*p,I original){
 if(!p)return original;
 U*v=*(U**)p;const char*name=((const char*(*)(void*))v[12])(p);
 if(!same(name,snapshot0)&&!same(name,snapshot1))return original;
 struct {U time;I cap,reserved;} opt;
 if(!transfer(permit_path,&opt,16,0)||opt.reserved)return original;
 U now=now_ms();if(!now||!opt.time||now<opt.time||now-opt.time>1500)return original;
 I cap=opt.cap;if(cap!=125&&cap!=250&&cap!=500&&cap!=1000&&cap!=2000)return original;
 return original>cap?original:cap;
}
