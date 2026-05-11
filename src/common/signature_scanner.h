#pragma once

#include <cstdint> // For uintptr_t
#include <vector>
#include <string>
#include <sstream>

uintptr_t find_signature(uintptr_t base_address, size_t size, const char* signature_str);
