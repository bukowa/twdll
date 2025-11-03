#include <filesystem>
#include <windows.h>
#include <string>
#include <libloaderapi.h>
#include "MinHook.h"
#include "game_lua_api.h"
#include "log.h"
#include "dx_finder.h"
#include "module.h"
#include "unit_script_interface.h"
#include "character_script_interface.h"
#include "battle_unit_script_interface.h"
#include "faction_script_interface.h"
#include "military_force_script_interface.h"
#include "performance_test.h"

// --- DllMain ---
// This function is the entry point for the DLL.
BOOL APIENTRY DllMain(HMODULE hModule, DWORD ul_reason_for_call, LPVOID lpReserved) {
    switch (ul_reason_for_call) {
    case DLL_PROCESS_ATTACH:
        Log("DLL process attach START");
        // module_initialize(hModule); // Calling the snake_case version of the function
        // This is called when the DLL is first loaded into the process.
        initialize_game_lua_api(); // Initialize our game Lua API function pointers
        Log("DLL process attach FINISH");
        break;
    case DLL_PROCESS_DETACH:
        // This is called when the DLL is being unloaded.
        // THIS IS THE FIX: We must disable our hooks to prevent a crash.
        Log("--- DllMain: DETACH received, calling CleanupHooks... ---");
        CleanupHooks(); // <<< MODIFIED THIS LINE
        Log("--- DllMain: Cleanup complete. ---");
        break;
    case DLL_THREAD_ATTACH:
    case DLL_THREAD_DETACH:
        // These are called for thread creation/destruction, not relevant for us.
        break;
    }
    return TRUE;
}

// --- Lua GC Hook for Standalone Game ---
// This is the C function that Lua's garbage collector will call.
static int script_Cleanup(lua_State *L) {
    Log("--- Lua GC: __gc metamethod called. Cleaning up hooks... ---");
    CleanupHooks();
    Log("--- Lua GC: Cleanup complete. ---");
    return 0;
}

// This function creates the special 'cleanup' object in Lua.
static void CreateCleanupObject(lua_State *L) {
    // Create a new userdata object. It's just a placeholder.
    g_game_lua_newuserdata(L, 1);

    // Create a metatable for our userdata.
    g_game_luaL_newmetatable(L, "twdll_cleanup_metatable");

    // Set the __gc field of the metatable to our C cleanup function.
    g_game_lua_pushstring(L, "__gc");
    g_game_lua_pushcclosure(L, script_Cleanup, 0);
    g_game_lua_settable(L, -3);

    // Assign the metatable to our userdata object.
    g_game_lua_setmetatable(L, -2);

    // Store the userdata in a global variable so it doesn't get collected early.
    g_game_lua_setfield(L, LUA_GLOBALSINDEX, "_twdll_cleanup_trigger");
    Log("--- Lua GC: Cleanup object created and registered. ---");
}

/// @module libtwdll
/// The main entry point for the twdll Lua extension library.

// --- Core Functions Exposed to Lua ---

/**
 * Logs a message to the libtwdll.log file.
 * @function Log
 * @tparam string message The message to write to the log.
 */
static int script_Log(lua_State *L) {
    const char *message = g_game_luaL_checklstring(L, 1, NULL);
    Log(message); // Call the existing C++ Log function
    return 0;
}

extern std::atomic<bool> g_isHookInitialized;

static int findandhook(lua_State *L) {
    if (!g_isHookInitialized) {
        FindAndHookD3D();
    } else {
        Log("D3D hooks already active, skipping re-initialization.");
    }
    return 0;
};

// Registration table for the main 'twdll' module
static const struct luaL_Reg twdll_main_functions[] = {
    {"Log", script_Log}, {"FindSwapChain", findandhook}, {NULL, NULL}};

/// @function luaopen_libtwdll
/// Initializes all twdll submodules and registers them in Lua.
/// @tparam lua_State L
/// @treturn integer always 1 (returns the main twdll table)
extern "C" __declspec(dllexport) int luaopen_twdll(lua_State *L) {
    g_game_LuaState = L;
    register_performance_test_functions(g_game_LuaState);
    Log("luaopen_twdll START");
    // Register the main 'twdll' table with core functions like Log
    g_game_luaL_register(L, "twdll", twdll_main_functions);

    // Register the specific interface modules
    g_game_luaL_register(L, "twdll_unit", unit_functions);
    g_game_luaL_register(L, "twdll_character", character_functions);
    g_game_luaL_register(L, "twdll_battle_unit", battle_unit_functions);
    g_game_luaL_register(L, "twdll_faction", faction_functions);
    g_game_luaL_register(L, "twdll_military_force", military_force_functions);
    
    g_game_luaL_register(L, "battle_unit", battle_unit_functions);
    Log("--- libtwdll modules registered. ---");
    CreateCleanupObject(L); // Always create cleanup object for the current Lua state
    Log("luaopen_twdll FINISH");
    return 1; // <<< MODIFIED THIS LINE: Return the 'twdll' table
}
