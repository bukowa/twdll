#include <windows.h>
#include "MinHook.h"
#include "log.h"
#include "unit_script_interface.cpp"
#include "battle_unit_script_interface.h"
#include "character_script_interface.h"

// We still need the extern "C" block for the Lua headers
extern "C" {
    #include <lua.h>
    #include <lauxlib.h>
}


// The main entry point for the DLL, called by Lua
extern "C" __declspec(dllexport) int luaopen_libtwdll(lua_State *L) {
    static bool hooks_are_initialized = false;

    luaL_register(L, "twdll_unit", unit_functions);
    luaL_register(L, "twdll_character", character_functions);
    luaL_register(L, "twdll_battle_unit", battle_unit_functions);
    luaL_register(L, "twdll_battle_unit_stats", battle_unit_stats_functions);

    if (hooks_are_initialized) {
        Log("--- libtwdll already initialized.---");
    }
    else {
        Log("--- libtwdll first-time initialization. Placing hooks... ---");
        hooks_are_initialized = true;
    }

    Log("--- libtwdll first-time initialization. ---");
    return 0;
}