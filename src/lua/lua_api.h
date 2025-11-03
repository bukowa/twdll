#pragma once

#include <cstdint>
#include "signature_scanner.h"
#include <vector>
#include "lua_forward_declarations.h"

void initialize_lua_api();

struct ModuleSignature {
    const char *module_name;
    const char *signature_str;
};

struct SignatureInfo {
    const char *function_name;
    void **target_function_ptr;
    std::vector<ModuleSignature> signatures;
};

extern lua_State *g_game_LuaState;

typedef void (*lua_pushstring_t)(lua_State *L, const char *s);
typedef void *(*lua_newuserdata_t)(lua_State *L, size_t size);
typedef int (*luaL_newmetatable_t)(lua_State *L, const char *tname);
typedef void (*lua_pushcclosure_t)(lua_State *L, lua_CFunction fn, int n);
typedef void (*lua_settable_t)(lua_State *L, int idx);
typedef int (*lua_setmetatable_t)(lua_State *L, int objindex);
typedef void (*lua_setfield_t)(lua_State *L, int idx, const char *k);
typedef const char *(*luaL_checklstring_t)(lua_State *L, int arg, size_t *l);
typedef void (*luaL_register_t)(lua_State *L, const char *libname, const luaL_Reg *l);
typedef void (*lua_createtable_t)(lua_State *L, int narr, int nrec);
typedef void *(*lua_touserdata_t)(lua_State *L, int idx);
typedef void (*lua_pushinteger_t)(lua_State *L, lua_Integer n);
typedef lua_Integer (*lua_tointeger_t)(lua_State *L, int idx);
typedef void (*lua_pushnumber_t)(lua_State *L, lua_Number n);
typedef lua_Number (*lua_tonumber_t)(lua_State *L, int idx);
typedef void (*lua_pushnil_t)(lua_State *L);
typedef int (*lua_pcall_t)(lua_State *L, int nargs, int nresults, int errfunc);
typedef int (*luaB_loadstring)(lua_State *L);

extern lua_pushstring_t g_game_lua_pushstring;
extern lua_newuserdata_t g_game_lua_newuserdata;
extern luaL_newmetatable_t g_game_luaL_newmetatable;
extern lua_pushcclosure_t g_game_lua_pushcclosure;
extern lua_settable_t g_game_lua_settable;
extern lua_setmetatable_t g_game_lua_setmetatable;
extern lua_setfield_t g_game_lua_setfield;
extern luaL_checklstring_t g_game_luaL_checklstring;
extern luaL_register_t g_game_luaL_register;
extern lua_createtable_t g_game_lua_createtable;
extern lua_touserdata_t g_game_lua_touserdata;
extern lua_pushinteger_t g_game_lua_pushinteger;
extern lua_tointeger_t g_game_lua_tointeger;
extern lua_pushnumber_t g_game_lua_pushnumber;
extern lua_tonumber_t g_game_lua_tonumber;
extern lua_pushnil_t g_game_lua_pushnil;
extern lua_pcall_t g_game_lua_pcall;
extern luaB_loadstring g_game_luaB_loadstring;
