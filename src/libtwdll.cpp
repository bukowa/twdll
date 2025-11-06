#include <filesystem>
#include <windows.h>
#include <string>
#include <libloaderapi.h>
#include <winnt.h>

#include "MinHook.h"
#include "imgui.h" // --- ADDED: Needed for our render function ---
#include "dx_finder.h" // --- ADDED: The new hook class ---
#include "lua/lua_api.h"
#include "log.h"
// #include "dx_finder.h" // --- REMOVED: Old global functions are gone ---
#include "module.h"
#include "unit_script_interface.h"
#include "character_script_interface.h"
#include "battle_unit_script_interface.h"
#include "faction_script_interface.h"
#include "military_force_script_interface.h"
#include "game/g_campaign.h" // --- ADDED: For campaign global addresses ---
#include "ui.h" // --- ADDED: Include the new UI header ---
#include "game/game_context.h"


BOOL APIENTRY DllMain(HMODULE hModule, DWORD ul_reason_for_call, LPVOID lpReserved) {
    DisableThreadLibraryCalls(hModule);

    if (ul_reason_for_call == DLL_PROCESS_ATTACH) {
        init_logger();
    }

    switch (ul_reason_for_call) {
    case DLL_PROCESS_ATTACH:
        spdlog::info("DLL process attach START");
        module_initialize(hModule);
#ifndef BUILD_TESTING_LUA
        initialize_lua_api();
#endif
        spdlog::info("DLL process attach FINISH");
        break;
    case DLL_PROCESS_DETACH:
        spdlog::info("--- DllMain: DETACH received, calling Uninstall... ---");
        ImGuiD3D11Hook::Uninstall(); // --- CHANGED: Use the new class method ---
        spdlog::info("--- DllMain: Uninstall complete. ---");
        break;
    case DLL_THREAD_ATTACH:
    case DLL_THREAD_DETACH:
        break;
    }
    return TRUE;
};

// --- Lua GC Hook for Standalone Game ---
// This is the C function that Lua's garbage collector will call.
static int script_Cleanup(lua_State *L) {
    spdlog::info("--- Lua GC: __gc metamethod called. Cleaning up hooks... ---");
    ImGuiD3D11Hook::Uninstall(); // --- CHANGED: Use the new class method ---
    spdlog::info("--- Lua GC: Cleanup complete. ---");
    return 0;
}

// This function creates the special 'cleanup' object in Lua. No changes needed here.
static void CreateCleanupObject(lua_State *L) {
    l_newuserdata(L, 1);
    l_newmetatable(L, "twdll_cleanup_metatable");
    l_pushstring(L, "__gc");
    l_pushcclosure(L, script_Cleanup, 0);
    l_settable(L, -3);
    l_setmetatable(L, -2);
    l_setfield(L, LUA_GLOBALSINDEX, "_twdll_cleanup_trigger");
    spdlog::info("--- Lua GC: Cleanup object created and registered. ---");
}

/// @module libtwdll
/// The main entry point for the twdll Lua extension library.

// --- Core Functions Exposed to Lua ---

// --- CHANGED: This function is now much simpler ---
static int findandhook(lua_State *L) {
    // The class handles checking if it's already initialized.
    // We just need to call Install and give it our render function.
    ImGuiD3D11Hook::Install(RenderMyUI);
    return 0;
};

// Registration table for the main 'twdll' module
static const struct luaL_Reg twdll_main_functions[] = {{"FindSwapChain", findandhook},
                                                       {NULL, NULL}};

/// @function luaopen_libtwdll
/// Initializes all twdll submodules and registers them in Lua.
/// @tparam lua_State L
/// @treturn integer always 1 (returns the main twdll table)
extern "C" __declspec(dllexport) int luaopen_twdll(lua_State *L) {
    spdlog::info("luaopen_twdll START");

    // --- CONTEXT SWITCHING PLACEHOLDER ---
    // This is a placeholder. In the future, we will inspect the lua_State (L)
    // to determine if we are in the frontend, campaign, or battle.
    // For now, we'll just set it to Campaign so we can see our globals.
    Game::SetCurrentContext(Game::Context::Campaign);
    spdlog::info("Game context set to Campaign (placeholder)");
    // --- END PLACEHOLDER ---

    l_register(L, "twdll", twdll_main_functions);

    // Register the specific interface modules
    l_register(L, "twdll_unit", unit_functions);
    l_register(L, "twdll_character", character_functions);
    l_register(L, "twdll_battle_unit", battle_unit_functions);
    l_register(L, "twdll_faction", faction_functions);
    l_register(L, "twdll_military_force", military_force_functions);

    l_register(L, "battle_unit", battle_unit_functions);
    spdlog::info("--- libtwdll modules registered. ---");
    CreateCleanupObject(L);
    spdlog::info("luaopen_twdll FINISH");
    return 1;
}