#pragma once

#include <cstddef>

// lua_api.h — Lua abstraction layer.
//
// In BUILD_TESTING_LUA mode: maps l_* macros to real Lua library calls.
// In production mode (DLL): maps l_* macros to game function pointers
//   resolved at runtime via signature scanning.

#ifdef BUILD_TESTING_LUA
// ── Test mode — use real Lua headers ────────────────────────────────────────
extern "C" {
    #include <lua.h>
    #include <lauxlib.h>
    #include <lualib.h>
}
#define l_pushstring    lua_pushstring
#define l_newuserdata   lua_newuserdata
#define l_newmetatable  luaL_newmetatable
#define l_pushcclosure  lua_pushcclosure
#define l_settable      lua_settable
#define l_setmetatable  lua_setmetatable
#define l_setfield      lua_setfield
#define l_checklstring  luaL_checklstring
#define l_register      luaL_register
#define l_createtable   lua_createtable
#define l_touserdata    lua_touserdata
#define l_pushinteger   lua_pushinteger
#define l_tointeger     lua_tointeger
#define l_pushnumber    lua_pushnumber
#define l_tonumber      lua_tonumber
#define l_pushnil       lua_pushnil
#define l_pcall         lua_pcall
#define l_loadstring    luaB_loadstring

#else
// ── Production mode — function pointers resolved from the game binary ────────

// Forward declarations
struct lua_State;
typedef int (*lua_CFunction)(lua_State* L);
typedef struct luaL_Reg {
    const char*   name;
    lua_CFunction func;
} luaL_Reg;
typedef long long lua_Integer;
typedef double    lua_Number;
#define LUA_GLOBALSINDEX (-10002)

// Function pointer typedefs
typedef void        (*lua_pushstring_t)   (lua_State*, const char*);
typedef void*       (*lua_newuserdata_t)  (lua_State*, size_t);
typedef int         (*luaL_newmetatable_t)(lua_State*, const char*);
typedef void        (*lua_pushcclosure_t) (lua_State*, lua_CFunction, int);
typedef void        (*lua_settable_t)     (lua_State*, int);
typedef int         (*lua_setmetatable_t) (lua_State*, int);
typedef void        (*lua_setfield_t)     (lua_State*, int, const char*);
typedef const char* (*luaL_checklstring_t)(lua_State*, int, size_t*);
typedef void        (*luaL_register_t)    (lua_State*, const char*, const luaL_Reg*);
typedef void        (*lua_createtable_t)  (lua_State*, int, int);
typedef void*       (*lua_touserdata_t)   (lua_State*, int);
typedef void        (*lua_pushinteger_t)  (lua_State*, lua_Integer);
typedef lua_Integer (*lua_tointeger_t)    (lua_State*, int);
typedef void        (*lua_pushnumber_t)   (lua_State*, lua_Number);
typedef lua_Number  (*lua_tonumber_t)     (lua_State*, int);
typedef void        (*lua_pushnil_t)      (lua_State*);
typedef int         (*lua_pcall_t)        (lua_State*, int, int, int);
typedef int         (*luaB_loadstring)    (lua_State*);

// Global function pointer instances (defined in lua_api.cpp)
extern lua_pushstring_t    g_game_lua_pushstring;
extern lua_newuserdata_t   g_game_lua_newuserdata;
extern luaL_newmetatable_t g_game_luaL_newmetatable;
extern lua_pushcclosure_t  g_game_lua_pushcclosure;
extern lua_settable_t      g_game_lua_settable;
extern lua_setmetatable_t  g_game_lua_setmetatable;
extern lua_setfield_t      g_game_lua_setfield;
extern luaL_checklstring_t g_game_luaL_checklstring;
extern luaL_register_t     g_game_luaL_register;
extern lua_createtable_t   g_game_lua_createtable;
extern lua_touserdata_t    g_game_lua_touserdata;
extern lua_pushinteger_t   g_game_lua_pushinteger;
extern lua_tointeger_t     g_game_lua_tointeger;
extern lua_pushnumber_t    g_game_lua_pushnumber;
extern lua_tonumber_t      g_game_lua_tonumber;
extern lua_pushnil_t       g_game_lua_pushnil;
extern lua_pcall_t         g_game_lua_pcall;
extern luaB_loadstring     g_game_luaB_loadstring;

// Maps l_* macros to game function pointers
#define l_pushstring    g_game_lua_pushstring
#define l_newuserdata   g_game_lua_newuserdata
#define l_newmetatable  g_game_luaL_newmetatable
#define l_pushcclosure  g_game_lua_pushcclosure
#define l_settable      g_game_lua_settable
#define l_setmetatable  g_game_lua_setmetatable
#define l_setfield      g_game_lua_setfield
#define l_checklstring  g_game_luaL_checklstring
#define l_register      g_game_luaL_register
#define l_createtable   g_game_lua_createtable
#define l_touserdata    g_game_lua_touserdata
#define l_pushinteger   g_game_lua_pushinteger
#define l_tointeger     g_game_lua_tointeger
#define l_pushnumber    g_game_lua_pushnumber
#define l_tonumber      g_game_lua_tonumber
#define l_pushnil       g_game_lua_pushnil
#define l_pcall         g_game_lua_pcall
#define l_loadstring    g_game_luaB_loadstring

// Signature scan descriptor — used by lua_api.cpp + per-game lua_sigs.cpp
struct TW_SignatureInfo {
    const char* function_name;
    void**      target_function_ptr;
    const char* signature;
};

// Initializes all g_game_* pointers via signature scanning.
// Called from DllMain on DLL_PROCESS_ATTACH.
void initialize_lua_api();

#endif // BUILD_TESTING_LUA
