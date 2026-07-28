#include <windows.h>
#include "common/tw.h"
#include "common/campaign_hooks.h"
#include "common/game_api.h"

extern const luaL_Reg unit_functions[];
extern const luaL_Reg character_functions[];
extern const luaL_Reg battle_unit_functions[];
extern const luaL_Reg faction_functions[];
extern const luaL_Reg military_force_functions[];
extern const luaL_Reg world_functions[];
extern const luaL_Reg campaign_ui_functions[];
extern const luaL_Reg twdll_core[];

extern void register_faction_methods(lua_State *L);
extern void register_military_force_methods(lua_State *L);
extern void register_unit_methods(lua_State *L);

BOOL APIENTRY DllMain(const HMODULE hModule, const DWORD reason, LPVOID) {
    DisableThreadLibraryCalls(hModule);
    if (reason == DLL_PROCESS_ATTACH) {
        init_logger();
        Log("[twdll] DLL_PROCESS_ATTACH — initializing");
        initialize_lua_api();
        initialize_game_api();
        install_campaign_hooks();
    }
    return TRUE;
}

extern "C" __declspec(dllexport) int luaopen_twdll(lua_State *L) {
    Log("[twdll] luaopen_twdll: registering modules");

    l_createtable(L, 0, 8); // Master table

    l_register(L, "twdll", twdll_core);
    l_setfield(L, -2, "core");

    l_register(L, "twdll_unit", unit_functions);
    l_setfield(L, -2, "unit");
    register_unit_methods(L);

    l_register(L, "twdll_faction", faction_functions);
    l_setfield(L, -2, "faction");
    register_faction_methods(L);

    l_register(L, "twdll_military_force", military_force_functions);
    l_setfield(L, -2, "military_force");
    register_military_force_methods(L);

    l_register(L, "twdll_world", world_functions);
    l_setfield(L, -2, "world");

    l_register(L, "twdll_campaign_ui", campaign_ui_functions);
    l_setfield(L, -2, "campaign_ui");

    Log("[twdll] luaopen_twdll: done");
    return 1;
}
