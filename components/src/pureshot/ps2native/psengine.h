#pragma once
#include "cxxabi.h"
#include "stock_tuning.h"
#include <unistd.h>
#include <fcntl.h>
#include <string.h>

/* ---- struct geometry ---- */
#define INPUTFRAME_SZ 0xdb8
#define PSINPUT_SZ    0x180
#define PSOUTPUT_SZ   0x400
#define EYE_W 7760
#define EYE_H 7760
#define EYE_STRIDE 15520            /* bytes, 2 B/px */
#define EYE_BYTES ((size_t)EYE_STRIDE * EYE_H)   /* 120435200 */
#ifndef BAYER_ENUM
#define IN_BAYER 5   /* QUAD_BAYER_GBRG */
#define OUT_BAYER 1  /* standard 2x2 GBRG */
#endif
#define OUT_RAW_TYPE 17

#define Fi8(b,o,v)  (*(uint8_t*)((char*)(b)+(o))=(uint8_t)(v))
#define Fi32(b,o,v) (*(int32_t*)((char*)(b)+(o))=(int32_t)(v))
#define Fi64(b,o,v) (*(int64_t*)((char*)(b)+(o))=(int64_t)(v))
#define Ff32(b,o,v) (*(float*)((char*)(b)+(o))=(float)(v))
#define Fptr(b,o,v) (*(void**)((char*)(b)+(o))=(void*)(v))

/* ---- std::function<int(T&)> hand-crafted ABI (GCC11 libstdc++) ---- */
static long sf_manager(void *dest, const void *src, int op){
  if (op == 2) memcpy(dest, src, 16);       /* __clone_functor */
  else if (op == 1) *(const void**)dest = src; /* __get_functor_ptr */
  return 0;                                  /* __get_type_info / __destroy_functor */
}
struct OutCtx { void *bufs[4]; int w,h,stride,n; };
static struct OutCtx g_out;
static int sf_out_alloc(const void *functor, void *raw){
  (void)functor;
  int w = *(int*)((char*)raw+0x14); if(!w) w=g_out.w;
  int h = *(int*)((char*)raw+0x18); if(!h) h=g_out.h;
  int st= *(int*)((char*)raw+0x1c); if(!st) st=g_out.stride;
  size_t sz=(size_t)st*h;
  void *buf=malloc(sz); memset(buf,0,sz);
  int in_fmt=*(int*)((char*)raw+0x24), in_pat=*(int*)((char*)raw+0x28), in_2c=*(int*)((char*)raw+0x2c);
  /* Fill only the buffer + geometry, exactly like the node's AllocIISCameraBufferToAlgo; leave the
     engine-owned type/pattern words (+0x24/+0x28) untouched -- overwriting them changes the write path. */
  Fptr(raw,0x00,buf); Fptr(raw,0x08,0); Fi32(raw,0x10,-1);
  Fi32(raw,0x14,w); Fi32(raw,0x18,h); Fi32(raw,0x1c,st); Fi32(raw,0x20,0);
  if(g_out.n<4) g_out.bufs[g_out.n]=buf;
  fprintf(stderr,"[out_alloc] call#%d w=%d h=%d stride=%d in_fmt=%d in_pat=%d in_2c=%d buf=%p\n",g_out.n+1,w,h,st,in_fmt,in_pat,in_2c,buf);
  g_out.n++;
  return 0;
}
static int sf_in_release(const void *functor, void *info){ (void)functor;(void)info; return 0; }
static void mk_fn(cxx_function *f, void *invoker, void *ctx){
  memset(f,0,sizeof *f); ((void**)f->any)[0]=ctx; f->manager=(void*)sf_manager; f->invoker=invoker;
}

static void fill_frame(void *F, void *inbuf){
  memset(F,0,INPUTFRAME_SZ);
  Fptr(F,0x00,inbuf); Fptr(F,0x08,0); Fi32(F,0x10,-1);
  Fi32(F,0x14,EYE_W); Fi32(F,0x18,EYE_H); Fi32(F,0x1c,EYE_STRIDE); Fi32(F,0x20,0);
  Fi32(F,0x24,4);                       /* raw_type/format = 4 (node value) */
  Fi32(F,0x28,IN_BAYER);              /* BayerPattern = QUAD_BAYER_GBRG */
  Fi32(F,0x2c,-1);                      /* node value */
  /* No sensor timestamp or camera temperature is recorded in the TIFF IFD. */
  Fi32(F,0x40,50);
  Ff32(F,0x50,64.0f);     /* IMX01F 10-bit sensor pedestal, before the ISP. */
  Ff32(F,0x58,1.0f);
  for (int i=0;i<4;i++){ Ff32(F,0x60+4*i,64); Fi32(F,0x70+4*i,1023); }
  /* The rendered DNG keeps its own ColorMatrix1. This CCM is engine-local. */
  for (int i=0;i<3;i++) Ff32(F,0x8c+16*i,1.0f);
  Ff32(F,0xec,1.0f);
  Ff32(F,0xd00,1.0f);
  /* empty cv::Mat at 0xd58, constructed like OpenCV does (flags=MAGIC_VAL, size.p=&rows, step.p=&step.buf) */
  Fi32(F,0xd58,0x42FF0000); Fptr(F,0xd98,(char*)F+0xd60); Fptr(F,0xda0,(char*)F+0xda8);
  /* Keep LSC size/count/pointer zero. DNG gain maps remain for the RAW developer. */
}
static void fill_input(void *P, void *frame){
  memset(P,0,PSINPUT_SZ);
  cxx_string_set((cxx_string*)P,"IMX01F");              /* name@0x00 */
  Fptr(P,0x20,frame); Fptr(P,0x28,(char*)frame+INPUTFRAME_SZ); Fptr(P,0x30,(char*)frame+INPUTFRAME_SZ); /* frames vec */
  Fi32(P,0x38,EYE_W); Fi32(P,0x3c,EYE_H);
  Fi32(P,0x40,1);                                       /* node value */
  Ff32(P,0x44,0.0f); Ff32(P,0x48,1.0f);                 /* range{0,1} */
  { int *si=malloc(sizeof(int)); si[0]=0;               /* sort_index vec<int> = {0} (node has 1 element) */
    Fptr(P,0x50,si); Fptr(P,0x58,si+1); Fptr(P,0x60,si+1); }
  Fi8(P,0x6c,1);                                        /* tuning_flag */
  Ff32(P,0x70,raw_tune[0]);                                    /* sharpRate */
  Ff32(P,0x74,raw_tune[11]);                                    /* maxWeight */
  for(int i=0;i<3;i++) Ff32(P,0x78+4*i,raw_tune[13+i]); /* darkBoost[3] (tag elems 1..3) */
  Fi8(P,0x88,1);                                        /* addback_flag */
  Fi8(P,0x89,1);                                        /* cac_present */
  for(int i=0;i<17;i++) Ff32(P,0x8c+4*i,raw_tune[24+i]);       /* CacTableR[17] */
  Fi32(P,0xd4,0xffffffff); Fi32(P,0xd8,0xffffffff);     /* ev0,ev1 = NaN (0xffffffff, node value) */
  /* Optional tuning block stays disabled. */
  Ff32(P,0xf4,1.0f);                                    /* scalar 1.0 */
  Fi32(P,0x100,EYE_W); Fi32(P,0x104,EYE_H);             /* width2,height2 */
}
static void fill_output(void *O){
  memset(O,0,PSOUTPUT_SZ);
  Fi32(O,0x10,-1);               /* p010 RawImage fd = -1 (node) */
  Fi32(O,0x24,6); Fi32(O,0x28,3); Fi32(O,0x2c,-1); /* p010 fmt/val/index(-1 disables p010) */
  Ff32(O,0x30,0.0f); Ff32(O,0x34,1.0f); Ff32(O,0x38,2.2f); Ff32(O,0x3c,0.0f);  /* gammaParams */
  Fi32(O,0x128,-1);              /* bayer RawImage fd = -1 */
  Fi32(O,0x13c,4); Fi32(O,0x140,5); Fi32(O,0x144,0); /* bayer fmt=4, pattern=5 (node) */
  void*p50=calloc(1,EYE_BYTES);   /* zeroed; calloc keeps it unresident unless the engine writes it */ Fptr(O,0x50,p50);
}
