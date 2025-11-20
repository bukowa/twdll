// asmlib_hooks.cpp
#include "asmlib_hooks.h"

void* __cdecl CustomMemmove(void* dest, const void* src, size_t count) {
    return A_memmove(dest, src, count);
}

errno_t __cdecl CustomMemcpy_s(void* dest, size_t dest_size, const void* src, size_t count) {
    A_memcpy(dest, src, count);
    return 0;
}

void* __cdecl CustomMemset(void* s, int c, size_t n) {
    return A_memset(s, c, n);
}

int __cdecl CustomMemcmp(const void* s1, const void* s2, size_t n) {
    return A_memcmp(s1, s2, n);
}
