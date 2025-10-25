#include <windows.h>

#include "log.h"
#include "unit_script_interface.h"
#include "character_script_interface.h"
#include "battle_unit_script_interface.h"
#include "faction_script_interface.h"
#include "military_force_script_interface.h"
#include "dx_finder.h"

// We still need the extern "C" block for the Lua headers
extern "C"
{
#include <lua.h>
#include <lauxlib.h>
}

/// @module libtwdll
/// The main entry point for the twdll Lua extension library.

// --- Core Functions Exposed to Lua ---

/**
 * Logs a message to the libtwdll.log file.
 * @function Log
 * @tparam string message The message to write to the log.
 */
static int script_Log(lua_State *L)
{
    const char *message = luaL_checkstring(L, 1);
    Log(message); // Call the existing C++ Log function
    return 0;
}

static int findandhook(lua_State *L){
    FindAndHookD3D();
    return 0;
};

// Registration table for the main 'twdll' module
static const struct luaL_Reg twdll_main_functions[] = {
    {"Log", script_Log},
    {"FindSwapChain", findandhook},
    {NULL, NULL}};

/// @function luaopen_libtwdll
/// Initializes all twdll submodules and registers them in Lua.
/// @tparam lua_State L
/// @treturn integer always 1 (returns the main twdll table)
extern "C" __declspec(dllexport) int luaopen_twdll(lua_State *L)
{
    static bool hooks_are_initialized = false;

    // Register the main 'twdll' table with core functions like Log
    luaL_register(L, "twdll", twdll_main_functions);

    // Register the specific interface modules
    luaL_register(L, "twdll_unit", unit_functions);
    luaL_register(L, "twdll_character", character_functions);
    luaL_register(L, "twdll_battle_unit", battle_unit_functions);
    luaL_register(L, "twdll_faction", faction_functions);
    luaL_register(L, "twdll_military_force", military_force_functions);

    if (hooks_are_initialized)
    {
        Log("--- libtwdll modules re-registered. ---");
    }
    else
    {
        Log("--- libtwdll first-time initialization. Placing hooks... ---");
        // TODO: Add any hook initialization here
        hooks_are_initialized = true;
        Log("--- libtwdll modules registered and hooks placed. ---");
    }

    return 0;
}
