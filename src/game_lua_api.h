#pragma once

#include <cstdint> // For uintptr_t
#include "signature_scanner.h"

// Forward declaration for lua_State, as we don't want to include full Lua headers
struct lua_State;

// Define the function pointer type for lua_pushstring
typedef const char* (*lua_pushstring_t)(lua_State* L, const char* s);

// Global variable to hold the game's lua_pushstring function address
// This will be initialized at runtime.
extern lua_pushstring_t g_game_lua_pushstring;

// Placeholder for the game's base address (to be filled by the user)
// For demonstration, we'll use a dummy value. In a real scenario, you'd get this
// from GetModuleHandle(NULL) or similar for the game's main executable.
extern uintptr_t g_game_base_address;

// Placeholder for the offset of lua_pushstring within the game's binary
// This value needs to be determined by analyzing the game's executable (e.g., with IDA).
// For demonstration, we'll use a dummy value.
extern uintptr_t LUA_PUSHSTRING_OFFSET;

// Function to initialize the game_lua_api function pointers
void initialize_game_lua_api();

// Struct to hold signature information for a specific function and module
struct SignatureInfo {
    const char* module_name;
    const char* function_name;
    const char* signature_str;
    void** target_function_ptr; // Pointer to the global function pointer (e.g., &g_game_lua_pushstring)
};
