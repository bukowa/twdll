// asmlib_hooks.h
#pragma once

#include <windows.h>
#include <cstddef>

extern "C" {
void* __cdecl A_memmove(void* dest, const void* src, size_t count);
void* __cdecl A_memcpy(void* dest, const void* src, size_t count);
void* __cdecl A_memset(void* s, int c, size_t n);
int __cdecl A_memcmp(const void* s1, const void* s2, size_t n);
}

void* __cdecl CustomMemmove(void* dest, const void* src, size_t count);
errno_t __cdecl CustomMemcpy_s(void* dest, size_t dest_size, const void* src, size_t count);
void* __cdecl CustomMemset(void* s, int c, size_t n);
int __cdecl CustomMemcmp(const void* s1, const void* s2, size_t n);
