// main.cpp — DLL entry point and Lua module registration.
// Game-agnostic: all game-specific code lives in src/rome2/ or src/attila/.

#include <windows.h>
#include "common/tw.h"

// Forward-declare Lua registration tables from per-game translation units.
extern const luaL_Reg unit_functions[];
extern const luaL_Reg character_functions[];
extern const luaL_Reg battle_unit_functions[];
extern const luaL_Reg faction_functions[];
extern const luaL_Reg military_force_functions[];
extern const luaL_Reg world_functions[];
extern const luaL_Reg campaign_ui_functions[];

// Core module functions (from src/lua/lua_core.cpp)
extern const luaL_Reg twdll_core[];

// Game-specific hooks (compiled in only for the relevant game)
#include "common/campaign_hooks.h"

// ── DLL entry ─────────────────────────────────────────────────────────────────

BOOL APIENTRY DllMain(HMODULE hModule, DWORD reason, LPVOID) {
    DisableThreadLibraryCalls(hModule);
    if (reason == DLL_PROCESS_ATTACH) {
        init_logger();
        Log("[twdll] DLL_PROCESS_ATTACH — initializing");
        initialize_lua_api();
        install_campaign_hooks();
    }
    return TRUE;
}

// ── Lua module entry point ────────────────────────────────────────────────────

extern "C" __declspec(dllexport) int luaopen_twdll(lua_State* L) {
    Log("[twdll] luaopen_twdll: registering modules");

    l_createtable(L, 0, 8); // Master table

    l_register(L, "twdll", twdll_core);
    l_setfield(L, -2, "core");

    l_register(L, "twdll_unit", unit_functions);
    l_setfield(L, -2, "unit");

    l_register(L, "twdll_character", character_functions);
    l_setfield(L, -2, "character");

    l_register(L, "twdll_battle_unit", battle_unit_functions);
    l_setfield(L, -2, "battle_unit");

    l_register(L, "twdll_faction", faction_functions);
    l_setfield(L, -2, "faction");

    l_register(L, "twdll_military_force", military_force_functions);
    l_setfield(L, -2, "military_force");

    l_register(L, "twdll_world", world_functions);
    l_setfield(L, -2, "world");

    l_register(L, "twdll_campaign_ui", campaign_ui_functions);
    l_setfield(L, -2, "campaign_ui");

    Log("[twdll] luaopen_twdll: done");
    return 1;
}
