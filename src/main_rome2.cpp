#include <windows.h>
#include <cstdlib>
#include "common/log.h"
#include "common/campaign_hooks.h"
#include "common/lua_api.h"

static bool g_is_initialized = false;

static int Lua_RandomSpacing(lua_State* L) {
    float r = 0.05f + static_cast<float>(rand()) / (static_cast<float>(RAND_MAX / (3.0f - 0.05f)));
    float actual = set_transport_spacing(r);
    Log("[twdll] Lua RandomSpacing: set to %.4ff", actual);
    if (g_game_lua_pushnumber) {
        l_pushnumber(L, actual);
    }
    return 1;
}

static int Lua_SetSpacing(lua_State* L) {
    float val = 0.35f;
    if (g_game_lua_tonumber) {
        double num = g_game_lua_tonumber(L, 1);
        if (num > 0.0) {
            val = static_cast<float>(num);
        }
    }
    float actual = set_transport_spacing(val);
    Log("[twdll] Lua SetSpacing: set to %.4ff", actual);
    if (g_game_lua_pushnumber) {
        l_pushnumber(L, actual);
    }
    return 1;
}

static int Lua_GetSpacing(lua_State* L) {
    if (g_game_lua_pushnumber) {
        l_pushnumber(L, get_transport_spacing());
    }
    return 1;
}

static int Lua_RefreshBattleShips(lua_State* L) {
    float val = 0.35f;
    if (g_game_lua_tonumber) {
        double num = g_game_lua_tonumber(L, 1);
        if (num > 0.0) {
            val = static_cast<float>(num);
        }
    }
    float actual = set_transport_spacing(val);
    Log("[twdll] Lua RefreshBattleShips: updated transport spacing to %.4ff", actual);
    if (g_game_lua_pushnumber) {
        l_pushnumber(L, actual);
    }
    return 1;
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
    Log("DllMain() [Rome2] called, reason=%u (%s)", reason, dll_reason_name(reason));
    if (reason == DLL_PROCESS_ATTACH) {
        Log("DllMain() [Rome2] PROCESS_ATTACH: disabling thread attach/detach calls");
        DisableThreadLibraryCalls(hModule);
    }
    return TRUE;
}

extern "C" __declspec(dllexport) int luaopen_twdll(lua_State *L) {
    Log("[twdll] luaopen_twdll (Rome 2): called");

    if (!g_is_initialized) {
        Log("[twdll] First load (Rome 2): installing transport spacing patch");

        // Pin this DLL in process memory permanently
        HMODULE hSelf = nullptr;
        GetModuleHandleExA(GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS | GET_MODULE_HANDLE_EX_FLAG_PIN, 
                           reinterpret_cast<LPCSTR>(luaopen_twdll), &hSelf);

        initialize_lua_api();
        install_campaign_hooks();
        g_is_initialized = true;
    }

    if (g_game_lua_createtable && g_game_lua_pushcclosure && g_game_lua_setfield) {
        l_createtable(L, 0, 8);

        l_pushcclosure(L, Lua_SetSpacing, 0);
        l_setfield(L, -2, "SetSpacing");

        l_pushcclosure(L, Lua_SetSpacing, 0);
        l_setfield(L, -2, "set_spacing");

        l_pushcclosure(L, Lua_GetSpacing, 0);
        l_setfield(L, -2, "GetSpacing");

        l_pushcclosure(L, Lua_GetSpacing, 0);
        l_setfield(L, -2, "get_spacing");

        l_pushcclosure(L, Lua_RandomSpacing, 0);
        l_setfield(L, -2, "RandomSpacing");

        l_pushcclosure(L, Lua_RandomSpacing, 0);
        l_setfield(L, -2, "random_spacing");

        l_pushcclosure(L, Lua_RefreshBattleShips, 0);
        l_setfield(L, -2, "RefreshBattleShips");

        l_pushcclosure(L, Lua_RefreshBattleShips, 0);
        l_setfield(L, -2, "refresh_battle_ships");

        // Also register into global table twdll
        if (g_game_lua_pushvalue) {
            l_pushvalue(L, -1);
            l_setfield(L, LUA_GLOBALSINDEX, "twdll");
        }

        Log("[twdll] luaopen_twdll (Rome 2): successfully exported Lua functions (SetSpacing, GetSpacing, RandomSpacing, RefreshBattleShips)");
        return 1;
    }

    Log("[twdll] luaopen_twdll (Rome 2): done");
    return 0;
}
