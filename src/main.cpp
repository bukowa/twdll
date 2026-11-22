#include <windows.h>
#include "common/tw.h"
#include "common/campaign_hooks.h"
#include "common/game_api.h"
#include "attila/security.h"
#include "common/runtime.h"

#include <string>

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
extern const luaL_Reg tweaker_functions[];
extern void register_tweaker_methods(lua_State *L);
extern void uninstall_tweakers();

static bool g_is_initialized = false;
static bool g_api_resolved = false;   // game/Lua function addresses, resolved once per process

// Number of live Lua states that opened the runtime (frontend, campaign, battle...).
// The game creates and destroys these independently and they can overlap, so the
// engine hooks are installed when the first state opens and rolled back only when
// the last one is collected. Tying them to "whichever state loaded first" would
// let one state's teardown silently disable the hooks another state still uses.
static int g_lua_state_count = 0;

static int l_twdll_gc_cleanup(lua_State*) {
    if (g_lua_state_count > 0) --g_lua_state_count;
    Log("[twdll] GC destroying Lua state — %d state(s) still using the runtime", g_lua_state_count);
    if (g_lua_state_count == 0 && g_is_initialized) {
        Log("[twdll] Last Lua state closed — uninstalling campaign hooks and tweakers");
        uninstall_tweakers();
        uninstall_campaign_hooks();
        g_is_initialized = false;
    }
    return 0;
}

// Registers a function table as global <namespace>[_suffix] and leaves it on the stack.
static void register_ns(lua_State* L, const char* suffix, const luaL_Reg* fns) {
    std::string name = TWDLL_NAMESPACE;
    if (suffix) {
        name += "_";
        name += suffix;
    }
    l_register(L, name.c_str(), fns);
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
        runtime_set_module(hModule);
        Log("DllMain() PROCESS_ATTACH: disabling thread attach/detach calls");
        DisableThreadLibraryCalls(hModule);
    }
    return TRUE;
}

extern "C" __declspec(dllexport) int luaopen_twdll(lua_State *L) {
    if (!is_valid_game_host()) {
        Log("[twdll] Execution blocked: host process is not Attila.exe or missing empire.retail.dll");
        return 0;
    }

    Log("[twdll] luaopen_twdll: called (namespace '%s', api %d)", TWDLL_NAMESPACE, TWDLL_API_VERSION);

    std::string refusal;
    if (!runtime_claim_owner(refusal)) {
        l_pushnil(L);
        l_pushstring(L, refusal.c_str());
        return 2;
    }
    runtime_pin_module();

    if (!g_is_initialized) {
        // Signature scanning walks the whole game image once per signature (~0.9 s for
        // ~57 signatures). empire.retail.dll never moves or changes while the process
        // runs and nothing clears the resolved addresses on uninstall, so resolve them
        // once per process; later re-installs (new campaign, post-battle reload) only
        // re-install the hooks.
        if (!g_api_resolved) {
            Log("[twdll] First load: resolving Game API");
            initialize_lua_api();
            initialize_game_api();
            g_api_resolved = true;
        }
        Log("[twdll] Installing hooks");
        install_campaign_hooks();
        g_is_initialized = true;
    }

    // Check if twdll table instance already exists in this Lua state's registry
    l_getfield(L, LUA_REGISTRYINDEX, TWDLL_NAMESPACE);
    if (l_type(L, -1) == LUA_TTABLE) {
        Log("[twdll] luaopen_twdll: returning existing twdll table instance from registry");
        return 1;
    }
    l_settop(L, 0);

    Log("[twdll] luaopen_twdll: registering modules");

    l_createtable(L, 0, 8);

    // Save table instance in Lua registry for future loadlib calls in this session
    l_pushvalue(L, -1);
    l_setfield(L, LUA_REGISTRYINDEX, TWDLL_NAMESPACE);

    l_newuserdata(L, 1);
    l_createtable(L, 0, 1);
    l_pushcclosure(L, l_twdll_gc_cleanup, 0);
    l_setfield(L, -2, "__gc");
    l_setmetatable(L, -2);
    l_setfield(L, -2, "__cleanup_proxy");
    ++g_lua_state_count;
    Log("[twdll] luaopen_twdll: new Lua state, %d state(s) now using the runtime", g_lua_state_count);

    register_ns(L, nullptr, twdll_core);
    l_setfield(L, -2, "core");

    register_ns(L, "character", character_functions);
    l_setfield(L, -2, "character");
    register_character_methods(L);
    register_art_set_methods(L);

    register_ns(L, "unit", unit_functions);
    l_setfield(L, -2, "unit");
    register_unit_methods(L);

    register_ns(L, "faction", faction_functions);
    l_setfield(L, -2, "faction");
    register_faction_methods(L);
    register_political_party_methods(L);
    register_political_party_list_methods(L);

    register_ns(L, "military_force", military_force_functions);
    l_setfield(L, -2, "military_force");
    register_military_force_methods(L);

    register_ns(L, "region", region_functions);
    l_setfield(L, -2, "region");
    register_region_methods(L);
    register_slot_methods(L);
    register_religion_methods(L);
    register_religion_list_methods(L);

    register_ns(L, "model", model_functions);
    l_setfield(L, -2, "model");

    register_ns(L, "world", world_functions);
    l_setfield(L, -2, "world");

    register_ns(L, "battle", battle_functions);
    l_setfield(L, -2, "battle");

    register_ns(L, "campaign_ui", campaign_ui_functions);
    l_setfield(L, -2, "campaign_ui");

    register_ns(L, "cai", cai_functions_export);
    l_setfield(L, -2, "cai");

    register_ns(L, "tweakers", tweaker_functions);
    register_tweaker_methods(L);
    l_setfield(L, -2, "tweakers");

    Log("[twdll] luaopen_twdll: done");
    return 1;
}

