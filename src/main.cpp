#include <windows.h>
#include "log.h"
#include "lua/lua_api.h"

// Expose the simple script interfaces
#include "unit.h"
#include "character.h"
#include "battle_unit.h"
#include "faction.h"
#include "military_force.h"

BOOL APIENTRY DllMain(HMODULE hModule, DWORD ul_reason_for_call, LPVOID lpReserved) {
    DisableThreadLibraryCalls(hModule);

    if (ul_reason_for_call == DLL_PROCESS_ATTACH) {
        init_logger();
        Log("--- DLL_PROCESS_ATTACH: libtwdll initialized ---");
#ifndef BUILD_TESTING_LUA
        initialize_lua_api();
#endif
    }
    return TRUE;
};

// --- Example: Global Flag Modification ---
// This demonstrates how to directly modify global variables in the game's memory.

#ifdef STEAM_BUILD
const char* TARGET_MODULE = "empire.retail.dll";
const size_t OFFSET_MAX_UNITS_ARMY = 0x193FC68;
const size_t OFFSET_MAX_UNITS_NAVY = 0x193FC6C;
#else
const char* TARGET_MODULE = "Rome2.dll";
const size_t OFFSET_MAX_UNITS_ARMY = 0x18F835C;
const size_t OFFSET_MAX_UNITS_NAVY = 0x18F8360;
#endif

static int script_set_max_units_in_army(lua_State* L) {
    int max_units = (int)l_tointeger(L, 1);
    
    HMODULE hModule = GetModuleHandleA(TARGET_MODULE);
    if (!hModule) {
        Log("Failed to get module handle for %s to set max army units", TARGET_MODULE);
        return 0;
    }
    
    int* p_max_army = (int*)((uintptr_t)hModule + OFFSET_MAX_UNITS_ARMY);
    *p_max_army = max_units;
    Log("Set max units in army to: %d", max_units);
    
    return 0;
}

static int script_set_max_units_in_navy(lua_State* L) {
    int max_units = (int)l_tointeger(L, 1);
    
    HMODULE hModule = GetModuleHandleA(TARGET_MODULE);
    if (!hModule) {
        Log("Failed to get module handle for %s to set max navy units", TARGET_MODULE);
        return 0;
    }
    
    int* p_max_navy = (int*)((uintptr_t)hModule + OFFSET_MAX_UNITS_NAVY);
    *p_max_navy = max_units;
    Log("Set max units in navy to: %d", max_units);
    
    return 0;
}

// --- Core Functions Exposed to Lua ---

/**
 * Logs a message to the twdll.log file.
 * @function Log
 * @tparam string message The message to write to the log.
 */
static int script_Log(lua_State *L) {
    const char *message = l_checklstring(L, 1, nullptr);
    if (message) {
        Log(message);
    }
    return 0;
}

// Registration table for the main 'twdll' module
static const struct luaL_Reg twdll_main_functions[] = {
    {"Log", script_Log},
    {"set_max_units_in_army", script_set_max_units_in_army},
    {"set_max_units_in_navy", script_set_max_units_in_navy},
    {NULL, NULL}
};

/// @function luaopen_twdll
/// Initializes all twdll submodules and registers them in Lua.
/// @tparam lua_State L
/// @treturn integer always 1 (returns the main twdll table)
extern "C" __declspec(dllexport) int luaopen_twdll(lua_State *L) {
    Log("--- luaopen_twdll called. Registering Lua modules... ---");

    // Register the main 'twdll' table with core functions
    l_register(L, "twdll", twdll_main_functions);

    // Register the specific interface modules (defined in their respective headers/cpps)
    l_register(L, "twdll_unit", unit_functions);
    l_register(L, "twdll_character", character_functions);
    l_register(L, "twdll_battle_unit", battle_unit_functions);
    l_register(L, "twdll_faction", faction_functions);
    l_register(L, "twdll_military_force", military_force_functions);

    Log("--- libtwdll modules successfully registered. ---");
    return 1;
}
