#pragma once

// This is the single, unified header for the Lua abstraction layer.
// It handles switching between the real Lua library (for testing) and
// the game's function pointers (for production).

#ifdef BUILD_TESTING_LUA
    // === TEST MODE ===
    // Include real Lua headers and map abstractions to standard Lua functions.
    extern "C" {
        #include <lua.h>
        #include <lauxlib.h>
        #include <lualib.h>
    }

    // Map abstractions to standard Lua functions.
    // It is assumed the test environment provides any non-standard functions needed,
    // like luaB_loadstring, as per previous user feedback.
    #define l_pushstring        lua_pushstring
    #define l_newuserdata       lua_newuserdata
    #define l_newmetatable      luaL_newmetatable
    #define l_pushcclosure      lua_pushcclosure
    #define l_settable          lua_settable
    #define l_setmetatable      lua_setmetatable
    #define l_setfield          lua_setfield
    #define l_checklstring      luaL_checklstring
    #define l_register          luaL_register
    #define l_createtable       lua_createtable
    #define l_touserdata        lua_touserdata
    #define l_pushinteger       lua_pushinteger
    #define l_tointeger         lua_tointeger
    #define l_pushnumber        lua_pushnumber
    #define l_tonumber          lua_tonumber
    #define l_pushnil           lua_pushnil
    #define l_pcall             lua_pcall
    #define l_loadstring        luaB_loadstring

#else
    // === PRODUCTION MODE (DLL) ===

    // 1. Forward-declare Lua types
    struct lua_State;
    typedef int (*lua_CFunction)(lua_State *L);
    typedef struct luaL_Reg {
        const char *name;
        lua_CFunction func;
    } luaL_Reg;
    typedef long long lua_Integer;
    typedef double lua_Number;
    #define LUA_GLOBALSINDEX (-10002)

    // 2. Typedefs for all game function pointers (from lua_api.h)
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

    void initialize_lua_api();

    // 3. Extern declarations for the global function pointers (from lua_api.h)
    extern lua_State *g_game_LuaState;
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

    // 4. Map abstractions to the game's function pointers.
    #define l_pushstring        g_game_lua_pushstring
    #define l_newuserdata       g_game_lua_newuserdata
    #define l_newmetatable      g_game_luaL_newmetatable
    #define l_pushcclosure      g_game_lua_pushcclosure
    #define l_settable          g_game_lua_settable
    #define l_setmetatable      g_game_lua_setmetatable
    #define l_setfield          g_game_lua_setfield
    #define l_checklstring      g_game_luaL_checklstring
    #define l_register          g_game_luaL_register
    #define l_createtable       g_game_lua_createtable
    #define l_touserdata        g_game_lua_touserdata
    #define l_pushinteger       g_game_lua_pushinteger
    #define l_tointeger         g_game_lua_tointeger
    #define l_pushnumber        g_game_lua_pushnumber
    #define l_tonumber          g_game_lua_tonumber
    #define l_pushnil           g_game_lua_pushnil
    #define l_pcall             g_game_lua_pcall
    #define l_loadstring        g_game_luaB_loadstring

#endif