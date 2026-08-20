#include "tw.h"

extern const char* GAME_NAME;

/// @module twdll.core
/// Utility functions available in all games.

#include <string>

/// Log a message or arbitrary values to twdll.log.
/// Accepts any number of arguments of any type, converting them via Lua tostring.
/// @function Log
/// @tparam any ... values to log
static int script_Log(lua_State* L) {
    std::string full_msg;
    for (int i = 1; l_type(L, i) != LUA_TNONE; ++i) {
        if (i > 1) full_msg += "\t";
        l_getfield(L, LUA_GLOBALSINDEX, "tostring");
        l_pushvalue(L, i);
        if (l_pcall(L, 1, 1, 0) == 0) {
            if (const char* s = l_checklstring(L, -1, nullptr))
                full_msg += s;
            l_pop(L, 1);
        } else {
            l_pop(L, 1);
        }
    }
    Log("%s", full_msg.c_str());
    return 0;
}

/// Returns the game build name.
/// @function GameBuild
/// @treturn string game name (e.g. "Attila")
static int script_GameBuild(lua_State* L) {
    l_pushstring(L, GAME_NAME);
    return 1;
}

#ifndef TWDLL_BUILD_SHA
#define TWDLL_BUILD_SHA "unknown"
#endif

/// Returns the git commit SHA hash from which the DLL was compiled.
/// @function GetBuildSha
/// @treturn string 40-character hexadecimal git commit SHA
/// @usage local sha = twdll.core.GetBuildSha()
static int script_GetBuildSha(lua_State* L) {
    l_pushstring(L, TWDLL_BUILD_SHA);
    return 1;
}

extern const luaL_Reg twdll_core[] = {
    {"Log",         script_Log},
    {"GameBuild",   script_GameBuild},
    {"GetBuildSha", script_GetBuildSha},
    {nullptr, nullptr}
};
