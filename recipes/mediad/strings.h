// Keep the historical string order and address instructions with different LLD versions.
// X6_PAGE_STRING uses a relocation with addend 1 to retain ADRP + ADD. Ordinary
// references permit linker relaxation to NOP + ADR. Both address the first byte.
#ifdef __ASSEMBLER__
.section .rodata.x6strings,"a",@progbits
#define X6_STRING(name, value) .global name; .hidden name; name: .asciz value
#define X6_PAGE_STRING(name, value) .global name##_anchor; .hidden name##_anchor; .set name##_anchor, . - 1; .asciz value
#else
#define X6_STRING(name) extern const char name[] __attribute__((visibility("hidden")))
#define X6_PAGE_STRING(name) X6_STRING(name); __asm__(".set " #name ", " #name "_anchor + 1")
#endif
