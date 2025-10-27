#pragma once

// Dummy declarations for Lua types to avoid including Lua headers directly
struct lua_State;
typedef struct lua_State lua_State;
typedef int (*lua_CFunction)(lua_State *L);
typedef struct luaL_Reg luaL_Reg;
struct luaL_Reg {
    const char *name;
    lua_CFunction func;
};

typedef long long lua_Integer;
typedef double lua_Number;

#define LUA_GLOBALSINDEX (-10002)
