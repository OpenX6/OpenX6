/* Minimal libstdc++ (GCC 11, cxx11 ABI, aarch64) layouts used to talk to libbac from C. */
#pragma once
#include <stddef.h>
#include <stdint.h>
#include <string.h>
#include <dlfcn.h>
#include <stdio.h>
#include <stdlib.h>

typedef struct { char *p; size_t len; union { char buf[16]; size_t cap; } u; } cxx_string; /* 32 B */
typedef struct { void *begin, *end, *cap; } cxx_vector;                                      /* 24 B */
typedef struct { void *ptr; void *ctrl; } cxx_shared_ptr;                                      /* 16 B */
typedef struct { unsigned char any[16]; void *manager; void *invoker; } cxx_function;          /* 32 B */

static void *(*cxx_new)(size_t);
static void (*cxx_delete_sized)(void*, size_t);

static void *must_sym(void *h, const char *s) {
  void *f = dlsym(h, s);
  if (!f) { fprintf(stderr, "dlsym %s: %s\n", s, dlerror()); exit(3); }
  return f;
}
static void cxx_init_alloc(void *h) {
  cxx_new = (void*(*)(size_t))must_sym(h, "_Znwm");
  cxx_delete_sized = (void(*)(void*,size_t))must_sym(h, "_ZdlPvm");
}
/* Build a std::string in place. Short strings use the SSO buffer, long ones a heap buffer from
   operator new so libstdc++ could free it consistently. The caller never destroys these. */
static void cxx_string_set(cxx_string *s, const char *v) {
  size_t n = strlen(v);
  if (n < 16) { s->p = s->u.buf; memcpy(s->u.buf, v, n); s->u.buf[n] = 0; s->len = n; return; }
  s->p = (char*)cxx_new(n + 1); memcpy(s->p, v, n + 1); s->len = n; s->u.cap = n;
}
