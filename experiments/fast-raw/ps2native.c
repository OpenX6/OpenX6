/* Offline validation candidate for 1.2.36 120 MP Fast RAW.
   Legacy DNGs have no verified engine lux index. Request the base lux band;
   exposure and gain remain available to the native model selector. */
#include "psengine.h"
#include <sys/stat.h>
#include <math.h>

#define DNG_W (2*EYE_W)
#define DNG_H EYE_H

typedef struct { uint8_t *b; size_t n; } Buf;
static uint16_t rd16(const uint8_t *p){ return (uint16_t)(p[0] | p[1]<<8); }
static uint32_t rd32(const uint8_t *p){ return (uint32_t)p[0] | p[1]<<8 | p[2]<<16 | (uint32_t)p[3]<<24; }
static void wr16(uint8_t *p, uint16_t v){ p[0]=(uint8_t)v; p[1]=(uint8_t)(v>>8); }
static void wr32(uint8_t *p, uint32_t v){ p[0]=(uint8_t)v; p[1]=(uint8_t)(v>>8); p[2]=(uint8_t)(v>>16); p[3]=(uint8_t)(v>>24); }
static int tsz(int t){ static const int s[] = {0,1,1,2,4,8,1,1,2,4,8,4,8,4}; return (t > 0 && t < 14) ? s[t] : 0; }
static void invalid_tag(void){ fprintf(stderr, "Invalid TIFF field or range\n"); exit(20); }
static uint32_t ifd_find(Buf *f, uint32_t ifd, uint16_t tag){
  if ((uint64_t)ifd+2 > f->n) invalid_tag();
  uint16_t n = rd16(f->b + ifd);
  if ((uint64_t)ifd+2+12u*n+4 > f->n) invalid_tag();
  for (int i = 0; i < n; i++){ uint32_t e = ifd + 2 + 12*i; if (rd16(f->b + e) == tag) return e; }
  return 0;
}
static uint8_t *ent_data(Buf *f, uint32_t e){
  int t = rd16(f->b + e + 2); uint32_t c = rd32(f->b + e + 4);
  uint64_t size = (uint64_t)tsz(t) * c;
  uint32_t offset = size <= 4 ? e+8 : rd32(f->b+e+8);
  if (!size || offset+size > f->n) invalid_tag();
  return f->b+offset;
}
static uint32_t ent_u(Buf *f, uint32_t e, int k){
  int t = rd16(f->b + e + 2); uint8_t *d = ent_data(f, e);
  if ((t != 3 && t != 4 && !(t == 13 && rd16(f->b+e) == 34665)) || (uint32_t)k >= rd32(f->b+e+4)) invalid_tag();
  return t == 3 ? rd16(d + 2*k) : rd32(d + 4*k);
}
static double ent_rat(Buf *f, uint32_t e, int k){
  if (rd16(f->b+e+2) != 5 || (uint32_t)k >= rd32(f->b+e+4)) invalid_tag();
  uint8_t *d = ent_data(f, e); uint32_t a = rd32(d + 8*k), b = rd32(d + 8*k + 4);
  return b ? (double)a / b : 0.0;
}
static uint32_t need(Buf *f, uint32_t ifd, uint16_t tag){
  uint32_t e = ifd_find(f, ifd, tag); if (!e){ fprintf(stderr, "DNG: missing tag %u\n", tag); exit(20); } return e;
}

static uint32_t adler(const uint8_t *d, size_t n){
  uint32_t a = 1, b = 0;
  while (n){ size_t k = n < 5552 ? n : 5552; n -= k; while (k--){ a += *d++; b += a; } a %= 65521; b %= 65521; }
  return b << 16 | a;
}
/* zlib stream -> raw. Stored blocks are parsed directly; anything else goes through libz. */
static int (*z_uncompress)(uint8_t*, unsigned long*, const uint8_t*, unsigned long);
static void inflate_strip(const uint8_t *s, size_t n, uint8_t *out, size_t want){
  size_t p = 2, o = 0; int ok = n > 6 && s[0] == 0x78;
  while (ok){
    if (p + 5 > n || ((s[p] >> 1) & 3)){ ok = 0; break; }
    uint16_t len = rd16(s + p + 1), nlen = rd16(s + p + 3);
    if ((uint16_t)(len ^ nlen) != 0xFFFF || p + 5 + len > n || o + len > want){ ok = 0; break; }
    memcpy(out + o, s + p + 5, len); o += len; int fin = s[p] & 1; p += 5 + len;
    if (fin) break;
  }
  if (ok && o == want && p + 4 <= n){
    uint32_t ad = (uint32_t)s[p]<<24 | s[p+1]<<16 | s[p+2]<<8 | s[p+3];
    if (ad == adler(out, o)) return;
  }
  if (!z_uncompress){
    void *z = dlopen("libz.so.1", RTLD_NOW); if (!z) z = dlopen("libz.so", RTLD_NOW);
    if (z) z_uncompress = (int(*)(uint8_t*,unsigned long*,const uint8_t*,unsigned long))dlsym(z, "uncompress");
    if (!z_uncompress){ fprintf(stderr, "DNG: strip is not a stored stream and libz is unavailable\n"); exit(21); }
  }
  unsigned long ol = want;
  if (z_uncompress(out, &ol, s, n) != 0 || ol != want){ fprintf(stderr, "DNG: inflate failed\n"); exit(22); }
}
/* camera-exact stored stream (libtiff->zlib, Z_NO_FLUSH then Z_FINISH): 65535-byte non-final
   blocks while >=32768 bytes remain, then the tail as the final block (empty if already flushed). */
static size_t stored_deflate(const uint8_t *d, size_t L, uint8_t *o){
  size_t q = 0, pos = 0, rem = L; o[q++] = 0x78; o[q++] = 0x01;
  while (rem >= 32768){
    uint16_t blk = rem > 65535 ? 65535 : (uint16_t)rem;
    o[q++] = 0; wr16(o + q, blk); wr16(o + q + 2, (uint16_t)~blk); q += 4;
    memcpy(o + q, d + pos, blk); q += blk; pos += blk; rem -= blk;
  }
  o[q++] = 1; wr16(o + q, (uint16_t)rem); wr16(o + q + 2, (uint16_t)~rem); q += 4;
  memcpy(o + q, d + pos, rem); q += rem;
  uint32_t ad = adler(d, L); o[q++] = ad >> 24; o[q++] = ad >> 16; o[q++] = ad >> 8; o[q++] = ad;
  return q;
}

static size_t stored_size(size_t L){
  size_t q = 2, rem = L;
  while (rem >= 32768){ size_t blk = rem > 65535 ? 65535 : rem; q += 5 + blk; rem -= blk; }
  return q + 5 + rem + 4;
}

static Buf load_file(const char *path){
  Buf f = {0}; int fd = open(path, O_RDONLY); if (fd < 0){ perror(path); exit(23); }
  struct stat st; if (fstat(fd,&st) || !S_ISREG(st.st_mode) || st.st_size < 8 || st.st_size > (512LL<<20)) invalid_tag(); f.n = (size_t)st.st_size; f.b = malloc(f.n);
  size_t got = 0; ssize_t r; while (got < f.n && (r = read(fd, f.b + got, f.n - got)) > 0) got += (size_t)r;
  close(fd); if (got != f.n){ fprintf(stderr, "%s: short read\n", path); exit(24); }
  return f;
}
typedef struct { uint32_t ifd, rps, nst, offs_e, cnts_e; } DngGeo;
static DngGeo dng_geo(Buf *f){
  DngGeo g; if (f->n < 8 || f->b[0] != 'I' || f->b[1] != 'I' || rd16(f->b + 2) != 42){ fprintf(stderr, "DNG: not little-endian TIFF\n"); exit(25); }
  g.ifd = rd32(f->b + 4);
  if (ent_u(f, need(f, g.ifd, 256), 0) != DNG_W || ent_u(f, need(f, g.ifd, 257), 0) != DNG_H ||
      ent_u(f, need(f, g.ifd, 258), 0) != 16 || ent_u(f, need(f, g.ifd, 259), 0) != 8){
    fprintf(stderr, "DNG: expected 15520x7760 16-bit Deflate CFA\n"); exit(26);
  }
  g.rps = ent_u(f, need(f, g.ifd, 278), 0);
  g.offs_e = need(f, g.ifd, 273); g.cnts_e = need(f, g.ifd, 279); g.nst = rd32(f->b + g.offs_e + 4);
  if (!g.rps || g.rps > DNG_H || rd32(f->b+g.cnts_e+4) != g.nst) invalid_tag();
  if (g.nst != (DNG_H + g.rps - 1) / g.rps){ fprintf(stderr, "DNG: strip count mismatch\n"); exit(27); }
  return g;
}
static uint16_t *dng_pixels(Buf *f, DngGeo *g){
  uint16_t *px = malloc((size_t)DNG_W * DNG_H * 2);
  for (uint32_t s = 0; s < g->nst; s++){
    uint32_t rows = (s + 1) * g->rps > DNG_H ? DNG_H - s * g->rps : g->rps;
    uint32_t off = ent_u(f, g->offs_e, s), cnt = ent_u(f, g->cnts_e, s);
    if ((size_t)off + cnt > f->n){ fprintf(stderr, "DNG: strip %u out of file\n", s); exit(28); }
    inflate_strip(f->b + off, cnt, (uint8_t*)(px + (size_t)s * g->rps * DNG_W), (size_t)rows * DNG_W * 2);
  }
  return px;
}

int main(int argc, char **argv){
  /* --caps: lets the daemon tell this build (per-frame --dsp/--hmx) from older ones, which print usage and exit 1 */
  if (argc == 2 && !strcmp(argv[1], "--caps")){ printf("ps2native caps: dsp hmx\n"); return 0; }
  /* --dsp=N / --hmx=N (anywhere after OUT): the AI accelerator (HTP) DSP-core / HMX power levels, written to
     PsInitConfig+0x38/+0x3c. PureshotEngine::Init copies the whole PsInitConfig to engine+0xb40, so these land on
     engine+0xb78/+0xb7c = the override ApplyDspPowerLevel uses when >= 0 (-1 = the engine chooses; its dual
     single-frame JDD path asks for 5/0 = NOM, the level native captures run at). DSPCoreClockLevel -> voltage
     corner (libbva_camera): 0 off,1 min,2 svs2,3 svs,4 svs+,5 nom,6 nom+,7 turbo,8 turbo+,9 l2,10 l3,11 max.
     The level only changes speed and heat, never the output bytes (unit 5, 2026-11-04: levels 1/5/7/11 identical). */
  int dsp_level = -1, hmx_level = -1;
  { int w = 3;
    for (int r = 3; r < argc; r++){
      if (!strncmp(argv[r], "--dsp=", 6)){ dsp_level = atoi(argv[r] + 6); continue; }
      if (!strncmp(argv[r], "--hmx=", 6)){ hmx_level = atoi(argv[r] + 6); continue; }
      fprintf(stderr, "unsupported option: %s\n", argv[r]); return 3;
    }
    if (argc >= 3) argc = w; }
  if (dsp_level < -1 || dsp_level > 11 || hmx_level < -1 || hmx_level > 13){
    fprintf(stderr, "invalid --dsp=%d / --hmx=%d\n", dsp_level, hmx_level); return 3; }
  if (argc < 3){ fprintf(stderr, "usage: %s IN.dng OUT.dng [--dsp=N] [--hmx=N] | --caps\n", argv[0]); return 1; }
  struct stat ost;
  if (stat(argv[2], &ost) == 0){ fprintf(stderr, "%s exists; not overwriting\n", argv[2]); return 13; }


  setenv("ADSP_LIBRARY_PATH", "/usr/lib;/system/lib/rfsa/adsp;/system/vendor/lib/rfsa/adsp;/dsp;/usr/share/dsp-libs", 1);
  setenv("MODEL_PATH", "/vendor/etc/camera/models/", 1);
  setenv("QNN_LIB_PATH", "/usr/share/dsp-libs", 1);
  setenv("MALLOC_ARENA_MAX", "6", 1);

  Buf in = load_file(argv[1]); DngGeo g = dng_geo(&in);
  uint32_t wl = ent_u(&in, need(&in, g.ifd, 50717), 0);
  if (rd32(in.b+need(&in,g.ifd,50717)+4) != 1) invalid_tag();
  if (wl != 1023){ fprintf(stderr, "WhiteLevel=%u: not a passthrough DNG (expected 1023); refusing\n", wl); return 3; }
  for (uint32_t s = 0; s < g.nst; s++){             /* level-0 strips have content-independent sizes: check before the 12 s engine run */
    uint32_t rows = (s + 1) * g.rps > DNG_H ? DNG_H - s * g.rps : g.rps;
    if (stored_size((size_t)rows * DNG_W * 2) != ent_u(&in, g.cnts_e, s)){
      fprintf(stderr, "DNG: strip %u is not a level-0 stream (enable fast-dng before shooting passthrough)\n", s); return 9; }
  }
  uint16_t *px = dng_pixels(&in, &g);
  uint16_t *eye[2] = { malloc(EYE_BYTES), malloc(EYE_BYTES) };      /* eye0 = right half, eye1 = left half */
  for (int y = 0; y < EYE_H; y++){
    memcpy(eye[1] + (size_t)y * EYE_W, px + (size_t)y * DNG_W, EYE_W * 2);
    memcpy(eye[0] + (size_t)y * EYE_W, px + (size_t)y * DNG_W + EYE_W, EYE_W * 2);
  }
  /* capture parameters the DNG carries */
  uint32_t exif = ent_u(&in, need(&in, g.ifd, 34665), 0);
  double iso = ent_u(&in, need(&in, exif, 34855), 0), et = ent_rat(&in, need(&in, exif, 33434), 0);
  uint32_t asn = need(&in, g.ifd, 50728);
  double n0 = ent_rat(&in, asn, 0), n1 = ent_rat(&in, asn, 1), n2 = ent_rat(&in, asn, 2);
  double be = 0; uint32_t bee = ifd_find(&in, g.ifd, 50730); int have_be = 0;
  if (bee && rd16(in.b + bee + 2) == 10){ uint8_t *d = ent_data(&in, bee); int32_t a = (int32_t)rd32(d), b = (int32_t)rd32(d + 4); if (b){ be = (double)a / b; have_be = 1; } }
  fprintf(stderr, "[dng] ISO=%.0f exposure=%.6fs AsShotNeutral=%.6f/%.6f/%.6f BaselineExposure=%.9f strips=%u x %u rows\n", iso, et, n0, n1, n2, be, g.nst, g.rps);

  if (et > 120 || !isfinite(iso) || iso <= 0 || !isfinite(et) || et <= 0 ||
      !isfinite(n0) || n0 <= 0 || !isfinite(n1) || n1 != 1 || !isfinite(n2) || n2 <= 0 ||
      !have_be || !isfinite(be) || be < -16 || be > 16) {
    fprintf(stderr,"Missing or invalid shot metadata; refusing conversion\n"); return 20;
  }
  void *h = dlopen("libbac.so", RTLD_NOW | RTLD_GLOBAL);
  if (!h){ fprintf(stderr, "dlopen libbac: %s\n", dlerror()); return 2; }
  cxx_init_alloc(h);
  void (*ctor)(void*, const void*) = must_sym(h, "_ZN3bac12pureshot_trd15PureshotManagerC1ERKN4base12pureshot_trd14PsCreateConfigE");
  int  (*init)(void*, const void*) = must_sym(h, "_ZN3bac12pureshot_trd15PureshotManager4InitERKN4base12pureshot_trd12PsInitConfigE");
  void (*reg_in)(void*, void*)     = must_sym(h, "_ZN3bac12pureshot_trd15PureshotManager14RegInputCbFuncERSt8functionIFiRN4base12pureshot_trd17ReleaseBufferInfoEEE");
  void (*reg_out)(void*, void*)    = must_sym(h, "_ZN3bac12pureshot_trd15PureshotManager20RegOutputAllocCbFuncERSt8functionIFiRN4base8RawImageEEE");
  int  (*pdual)(void*, void*, void*, void*, void*) = must_sym(h, "_ZN3bac12pureshot_trd15PureshotManager11ProcessDualERN4base12pureshot_trd13PureshotInputES5_RNS3_14PureshotOutputES7_");
  int  (*uninit)(void*) = must_sym(h, "_ZN3bac12pureshot_trd15PureshotManager6UnInitEv");
  void (*dtor)(void*)   = must_sym(h, "_ZN3bac12pureshot_trd15PureshotManagerD1Ev");
  typedef struct { int32_t one, algo; cxx_string dsp, model; } PsCreateConfig;
  typedef struct { int32_t w, h, res_class, sensor_type, num_frames; float scale[7]; int32_t mode30; uint8_t f34, p[3]; int32_t v38, v3c; } PsInitConfig;
  PsCreateConfig cc; memset(&cc, 0, sizeof cc); cc.one = 1; cc.algo = 0;
  cxx_string_set(&cc.dsp, "/usr/share/dsp-libs"); cxx_string_set(&cc.model, "/vendor/etc/camera/models");
  void *mgr = cxx_new(0x58); memset(mgr, 0, 0x58); ctor(mgr, &cc);
  PsInitConfig ic; memset(&ic, 0, sizeof ic);
  ic.w = EYE_W; ic.h = EYE_H; ic.res_class = 2; ic.sensor_type = 2; ic.num_frames = 1; ic.scale[0] = 1.0f; ic.mode30 = 3;
  ic.v38 = dsp_level; ic.v3c = hmx_level;   /* was 1/0: forced the minimum voltage corner (7.6 s vs 5.3 s at NOM) */
  fprintf(stderr, "[power] dsp=%d hmx=%d\n", dsp_level, hmx_level);
  if (init(mgr, &ic)){ fprintf(stderr, "engine Init failed\n"); return 7; }
  cxx_function fin, fout; mk_fn(&fin, (void*)sf_in_release, 0); mk_fn(&fout, (void*)sf_out_alloc, &g_out);
  reg_in(mgr, &fin); reg_out(mgr, &fout);

  void *F[2], *P[2], *O[2];
  for (int e = 0; e < 2; e++){
    F[e] = malloc(INPUTFRAME_SZ); fill_frame(F[e], eye[e]);
    P[e] = malloc(PSINPUT_SZ);    fill_input(P[e], F[e]);
    O[e] = malloc(PSOUTPUT_SZ);   fill_output(O[e]);
    /* Per-shot estimate; EXIF ISO rounds the hardware gain. */
    double sensor_gain = iso / 100.0;
    float adrc = (float)exp2(be + 1.0);
    Ff32(F[e], 0x54, (float)sensor_gain);
    Fi64(F[e], 0x48, (int64_t)llround(et * 1e9));
    Ff32(F[e], 0x80, (float)(1.0/n0)); Ff32(F[e], 0x84, 1); Ff32(F[e], 0x88, (float)(1.0/n2));
    Ff32(F[e], 0x38, adrc); Ff32(F[e], 0xe8, adrc);
    for (int j=0;j<3;j++){
      Fi64(F[e],0xc0+8*j,(int64_t)llround(et * 1e9));
      Ff32(F[e],0xd8+4*j,(float)sensor_gain);
      Ff32(F[e],0xb0+4*j,(float)(et*1e9*sensor_gain));
    }
    /* Piecewise linear noise add-back from the official IMX01F snapshot ATM curve. */
    float noise = raw_tune[20];
    for(int j=1;j<4;j++){
      if (sensor_gain <= raw_tune[16+j-1]) break;
      if (sensor_gain >= raw_tune[16+j]) { noise=raw_tune[20+j]; continue; }
      double f=(sensor_gain-raw_tune[16+j-1])/(raw_tune[16+j]-raw_tune[16+j-1]);
      noise=(float)(raw_tune[20+j-1]+f*(raw_tune[20+j]-raw_tune[20+j-1])); break;
    }
    Ff32(P[e], 0x84, noise);
  }
  g_out.w = EYE_W; g_out.h = EYE_H; g_out.stride = EYE_STRIDE; g_out.n = 0;
  int rc = pdual(mgr, P[0], P[1], O[0], O[1]);
  if (rc || g_out.n < 2 || !g_out.bufs[0] || !g_out.bufs[1]){ fprintf(stderr, "ProcessDual rc=%d outputs=%d\n", rc, g_out.n); return 8; }
  fprintf(stderr, "[engine] ProcessDual rc=0\n");

  /* assemble native pixels: left = eye1, right = eye0 */
  for (int y = 0; y < EYE_H; y++){
    memcpy(px + (size_t)y * DNG_W,         (uint16_t*)g_out.bufs[1] + (size_t)y * EYE_W, EYE_W * 2);
    memcpy(px + (size_t)y * DNG_W + EYE_W, (uint16_t*)g_out.bufs[0] + (size_t)y * EYE_W, EYE_W * 2);
  }
  /* write strips in place (stored streams have content-independent size) */
  uint8_t *tmp = malloc((size_t)g.rps * DNG_W * 2 + 65536);
  for (uint32_t s = 0; s < g.nst; s++){
    uint32_t rows = (s + 1) * g.rps > DNG_H ? DNG_H - s * g.rps : g.rps;
    size_t L = (size_t)rows * DNG_W * 2, q = stored_deflate((uint8_t*)(px + (size_t)s * g.rps * DNG_W), L, tmp);
    uint32_t off = ent_u(&in, g.offs_e, s), cnt = ent_u(&in, g.cnts_e, s);
    if (q != cnt){ fprintf(stderr, "DNG: strip %u size %zu != %u (input not level-0? enable fast-dng)\n", s, q, cnt); return 9; }
    memcpy(in.b + off, tmp, q);
  }
  { uint32_t e = need(&in, g.ifd, 50717); int t = rd16(in.b + e + 2);
    if (t == 3) wr16(in.b + e + 8, 16383); else wr32(in.b + e + 8, 16383); }
  { uint32_t e = need(&in, g.ifd, 50714); int t = rd16(in.b + e + 2); uint32_t c = rd32(in.b + e + 4); uint8_t *d = ent_data(&in, e);
    if (t != 3 && t != 4 && t != 5) invalid_tag();
    for (uint32_t k = 0; k < c; k++){ if (t == 3) wr16(d + 2*k, 0); else if (t == 4) wr32(d + 4*k, 0); else { wr32(d + 8*k, 0); wr32(d + 8*k + 4, 1); } } }
  char tmpname[4096]; if (snprintf(tmpname,sizeof tmpname,"%s.tmp",argv[2]) >= (int)sizeof tmpname) return 10;
  int fd = open(tmpname, O_WRONLY | O_CREAT | O_EXCL, 0644); if (fd < 0){ perror(tmpname); return 10; }
  free(px); free(eye[0]); free(eye[1]);
  size_t w = 0; ssize_t r; while (w < in.n && (r = write(fd, in.b + w, in.n - w)) > 0) w += (size_t)r;
  if (w != in.n || fsync(fd) != 0){ perror("write"); close(fd); unlink(tmpname); return 11; }
  close(fd);
  if (rename(tmpname, argv[2]) != 0){ perror("rename"); unlink(tmpname); return 12; }
  { FILE *st = fopen("/proc/self/status", "r"); char l[256]; long hwm = -1;
    while (st && fgets(l, sizeof l, st)) if (!strncmp(l, "VmHWM:", 6)) hwm = atol(l + 6);
    if (st) fclose(st);
    fprintf(stderr, "[done] wrote %s (%zu bytes) peak RSS %ld MiB\n", argv[2], in.n, hwm / 1024); }
  uninit(mgr); dtor(mgr);
  return 0;
}
