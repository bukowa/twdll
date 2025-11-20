// hooks.h
#pragma once

#include <cstdint>

struct Hook {
    const char* name;
    const char* module_name; // e.g., "Rome2.exe" or "empire.retail.dll"
    bool is_immediate;      // True to hook on attach, false to wait for module
    uintptr_t ida_address;
    void* replacement_function;
    void** original_function; // To store the original function pointer
};
