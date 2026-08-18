#include <windows.h>
#include "common/tw.h"
#include "common/campaign_hooks.h"
#include "common/game_api.h"

extern const luaL_Reg unit_functions[];
extern const luaL_Reg character_functions[];
extern const luaL_Reg battle_unit_functions[];
extern const luaL_Reg faction_functions[];
extern const luaL_Reg military_force_functions[];
extern const luaL_Reg region_functions[];
extern const luaL_Reg model_functions[];
extern const luaL_Reg world_functions[];
extern const luaL_Reg battle_functions[];
extern const luaL_Reg campaign_ui_functions[];
extern const luaL_Reg cai_functions_export[];
extern const luaL_Reg twdll_core[];

extern void register_character_methods(lua_State *L);
extern void register_art_set_methods(lua_State *L);
extern void register_faction_methods(lua_State *L);
extern void register_unit_methods(lua_State *L);
extern void register_military_force_methods(lua_State *L);
extern void register_region_methods(lua_State *L);
extern void register_slot_methods(lua_State *L);
extern void register_political_party_methods(lua_State *L);
extern void register_political_party_list_methods(lua_State *L);
extern void register_religion_methods(lua_State *L);
extern void register_religion_list_methods(lua_State *L);

static bool g_is_initialized = false;

static int l_twdll_gc_cleanup(lua_State* L) {
    if (g_is_initialized) {
        Log("[twdll] GC destroying Lua state — uninstalling campaign hooks");
        uninstall_campaign_hooks();
        g_is_initialized = false;
    }
    return 0;
}

static const char* dll_reason_name(DWORD reason) {
    switch (reason) {
        case DLL_PROCESS_DETACH: return "DLL_PROCESS_DETACH";
        case DLL_PROCESS_ATTACH: return "DLL_PROCESS_ATTACH";
        case DLL_THREAD_ATTACH:  return "DLL_THREAD_ATTACH";
        case DLL_THREAD_DETACH:  return "DLL_THREAD_DETACH";
        default:                 return "UNKNOWN";
    }
}

BOOL APIENTRY DllMain(const HMODULE hModule, const DWORD reason, LPVOID) {
    Log("DllMain() called, reason=%u (%s)", reason, dll_reason_name(reason));
    if (reason == DLL_PROCESS_ATTACH) {
        Log("DllMain() PROCESS_ATTACH: disabling thread attach/detach calls");
        DisableThreadLibraryCalls(hModule);
    }
    return TRUE;
}

extern "C" __declspec(dllexport) int luaopen_twdll(lua_State *L) {
    Log("[twdll] luaopen_twdll: called");

    if (!g_is_initialized) {
        Log("[twdll] First load: initializing Game API and hooks");
        initialize_lua_api();
        initialize_game_api();
        install_campaign_hooks();
        g_is_initialized = true;
    }

    Log("[twdll] luaopen_twdll: registering modules");

    l_createtable(L, 0, 8);

    l_newuserdata(L, 1);
    l_createtable(L, 0, 1);
    l_pushcclosure(L, l_twdll_gc_cleanup, 0);
    l_setfield(L, -2, "__gc");
    l_setmetatable(L, -2);
    l_setfield(L, -2, "__cleanup_proxy");

    l_register(L, "twdll", twdll_core);
    l_setfield(L, -2, "core");

    l_register(L, "twdll_character", character_functions);
    l_setfield(L, -2, "character");
    register_character_methods(L);
    register_art_set_methods(L);

    l_register(L, "twdll_unit", unit_functions);
    l_setfield(L, -2, "unit");
    register_unit_methods(L);

    l_register(L, "twdll_faction", faction_functions);
    l_setfield(L, -2, "faction");
    register_faction_methods(L);
    register_political_party_methods(L);
    register_political_party_list_methods(L);

    l_register(L, "twdll_military_force", military_force_functions);
    l_setfield(L, -2, "military_force");
    register_military_force_methods(L);

    l_register(L, "twdll_region", region_functions);
    l_setfield(L, -2, "region");
    register_region_methods(L);
    register_slot_methods(L);
    register_religion_methods(L);
    register_religion_list_methods(L);

    l_register(L, "twdll_model", model_functions);
    l_setfield(L, -2, "model");

    l_register(L, "twdll_world", world_functions);
    l_setfield(L, -2, "world");

    l_register(L, "twdll_battle", battle_functions);
    l_setfield(L, -2, "battle");

    l_register(L, "twdll_campaign_ui", campaign_ui_functions);
    l_setfield(L, -2, "campaign_ui");

    l_register(L, "twdll_cai", cai_functions_export);
    l_setfield(L, -2, "cai");

    Log("[twdll] luaopen_twdll: done");
    return 1;
}

