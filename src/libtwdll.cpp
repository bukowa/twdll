#include <windows.h>

#include "log.h"
#include "unit_script_interface.h"
#include "character_script_interface.h"
#include "battle_unit_script_interface.h"
#include "faction_script_interface.h"
#include "military_force_script_interface.h"
#include "dx_finder.h"
#include "MinHook.h" // <<< ADDED THIS LINE

// --- DllMain ---
// This function is the entry point for the DLL.
BOOL APIENTRY DllMain(HMODULE hModule, DWORD ul_reason_for_call, LPVOID lpReserved) {
    switch (ul_reason_for_call) {
        case DLL_PROCESS_ATTACH:
            // This is called when the DLL is first loaded into the process.
            // We don't need to do anything here, as hooks are initialized via Lua.
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


// We still need the extern "C" block for the Lua headers
extern "C"
{
#include <lua.h>
#include <lauxlib.h>
}

// --- Lua GC Hook for Standalone Game ---
// This is the C function that Lua's garbage collector will call.
static int script_Cleanup(lua_State* L) {
    Log("--- Lua GC: __gc metamethod called. Cleaning up hooks... ---");
    CleanupHooks();
    Log("--- Lua GC: Cleanup complete. ---");
    return 0;
}

// This function creates the special 'cleanup' object in Lua.
static void CreateCleanupObject(lua_State* L) {
    // Create a new userdata object. It's just a placeholder.
    lua_newuserdata(L, 1);

    // Create a metatable for our userdata.
    luaL_newmetatable(L, "twdll_cleanup_metatable");

    // Set the __gc field of the metatable to our C cleanup function.
    lua_pushstring(L, "__gc");
    lua_pushcfunction(L, script_Cleanup);
    lua_settable(L, -3);

    // Assign the metatable to our userdata object.
    lua_setmetatable(L, -2);

    // Store the userdata in a global variable so it doesn't get collected early.
    lua_setglobal(L, "_twdll_cleanup_trigger");
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
        CreateCleanupObject(L); // <<< ADDED THIS LINE
        hooks_are_initialized = true;
        Log("--- libtwdll modules registered and hooks placed. ---");
    }

    return 1; // <<< MODIFIED THIS LINE: Return the 'twdll' table
}
