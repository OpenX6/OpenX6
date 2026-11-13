.text
profile:
 cmp w0,#2
#ifdef CUSTOM
 b.eq high
 cmp w0,#3
 b.ne native
high:
#else
 b.ne native
#endif
 mov w0,#1
native:
 cmp w0,#1
 b continuation
