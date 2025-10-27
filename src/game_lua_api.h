#pragma once

#include <cstdint>
#include "signature_scanner.h"
#include <vector>

extern "C" {
#include <lua.h>
#include <lauxlib.h>
}

// Define function pointer types for Lua functions
typedef void (*lua_pushstring_t)(lua_State* L, const char* s);
typedef void* (*lua_newuserdata_t)(lua_State* L, size_t size);
typedef int (*luaL_newmetatable_t)(lua_State* L, const char* tname);
typedef void (*lua_pushcclosure_t)(lua_State* L, lua_CFunction fn, int n);
typedef void (*lua_settable_t)(lua_State* L, int idx);
typedef int (*lua_setmetatable_t)(lua_State* L, int objindex);
typedef void (*lua_setfield_t)(lua_State* L, int idx, const char* k);
typedef const char* (*luaL_checklstring_t)(lua_State* L, int arg, size_t *l);
typedef void (*luaL_register_t)(lua_State* L, const char* libname, const luaL_Reg* l);
typedef void (*lua_getfield_t)(lua_State* L, int idx, const char* k);
typedef void (*lua_settop_t)(lua_State* L, int idx);
typedef void (*lua_createtable_t)(lua_State* L, int narr, int nrec);
typedef size_t (*lua_objlen_t)(lua_State* L, int idx);
typedef const char* (*lua_tolstring_t)(lua_State* L, int idx, size_t *len);

// Global variables to hold game's Lua function addresses
extern lua_pushstring_t g_game_lua_pushstring;
extern lua_newuserdata_t g_game_lua_newuserdata;
extern luaL_newmetatable_t g_game_luaL_newmetatable;
extern lua_pushcclosure_t g_game_lua_pushcclosure;
extern lua_settable_t g_game_lua_settable;
extern lua_setmetatable_t g_game_lua_setmetatable;
extern lua_setfield_t g_game_lua_setfield;
extern luaL_checklstring_t g_game_luaL_checklstring;
extern luaL_register_t g_game_luaL_register;
extern lua_getfield_t g_game_lua_getfield;
extern lua_settop_t g_game_lua_settop;
extern lua_createtable_t g_game_lua_createtable;
extern lua_objlen_t g_game_lua_objlen;
extern lua_tolstring_t g_game_lua_tolstring;

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

// New structure
struct ModuleSignature {
    const char* module_name;
    const char* signature_str;
};

struct SignatureInfo {
    const char* function_name;
    void** target_function_ptr;
    std::vector<ModuleSignature> signatures;
};
