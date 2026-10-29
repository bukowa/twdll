#include "tw.h"

extern const char* GAME_NAME;

/// @module twdll.core
/// Utility functions available in all games.

#include <string>

/// Log a message or arbitrary values to `twdll.log`.
/// Accepts any number of arguments of any type, converting them via Lua tostring.
/// Logging is disabled by default; enable it with `twdll.core.EnableLogging(true)`.
/// @function Log
/// @tparam any ... values to log
/// @usage
/// twdll.core.Log("Campaign initialized. Turn:", 1, "Faction:", faction:name())
static int script_Log(lua_State* L) {
    if (!is_logging_enabled()) {
        return 0;
    }

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
    Log(full_msg);
    return 0;
}

/// Enables or disables logging output to `twdll.log`.
/// Logging is disabled by default on initialization.
/// @function EnableLogging
/// @tparam[opt] boolean enabled whether logging to twdll.log is enabled
/// @treturn boolean current enabled state
/// @usage
/// -- Enable logging:
/// twdll.core.EnableLogging(true)
///
/// -- Query current state:
/// local is_logging = twdll.core.EnableLogging()
///
/// -- Disable logging:
/// twdll.core.EnableLogging(false)
static int script_EnableLogging(lua_State* L) {
    if (l_type(L, 1) != LUA_TNONE && l_type(L, 1) != LUA_TNIL) {
        set_logging_enabled(l_tobool(L, 1) != 0);
    }
    l_pushboolean(L, is_logging_enabled() ? 1 : 0);
    return 1;
}

/// Returns the target game build name (e.g. `"Attila"`).
/// @function GameBuild
/// @treturn string game name
/// @usage
/// local game_name = twdll.core.GameBuild()
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
/// @usage
/// local sha = twdll.core.GetBuildSha()
static int script_GetBuildSha(lua_State* L) {
    l_pushstring(L, TWDLL_BUILD_SHA);
    return 1;
}

extern const luaL_Reg twdll_core[] = {
    {"EnableLogging", script_EnableLogging},
    {"Log",           script_Log},
    {"GameBuild",     script_GameBuild},
    {"GetBuildSha",   script_GetBuildSha},
    {nullptr, nullptr}
};
