#pragma once

#include <cstdint> // For uintptr_t
#include <vector>
#include <string>
#include <sstream>

namespace Scanner {

// Finds a sequence of bytes according to the signature mask (e.g. "55 8B EC ?? ??")
uintptr_t find_signature(uintptr_t base_address, size_t size, const char* signature_str);

// Find the first occurrence of an ASCII string anywhere in the mapped image.
uintptr_t FindString(uintptr_t base, size_t size, const char* needle);

// Scan .text (or any segment) for `push <target_addr>` (opcode 68 XX XX XX XX).
uintptr_t FindPushRef(uintptr_t text_base, size_t text_size, uintptr_t target_addr);

// Walk backwards from `from_addr` looking for the start of the function prologue.
uintptr_t FindPrologue(uintptr_t from_addr, size_t max_back = 0x5000);

}
