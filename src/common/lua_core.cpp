#include "tw.h"

extern const char* GAME_NAME;

/// @module twdll.core
/// Utility functions available in all games.

/// Log a message to twdll.log.
/// @function Log
/// @tparam string msg message to log
static int script_Log(lua_State* L) {
    if (const char* msg = l_checklstring(L, 1, nullptr))
        Log(msg);
    return 0;
}

/// Returns the game build name.
/// @function GameBuild
/// @treturn string game name (e.g. "Attila")
static int script_GameBuild(lua_State* L) {
    l_pushstring(L, GAME_NAME);
    return 1;
}

extern const luaL_Reg twdll_core[] = {
    {"Log",       script_Log},
    {"GameBuild", script_GameBuild},
    {nullptr, nullptr}
};
