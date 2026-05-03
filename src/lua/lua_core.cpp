// lua_core.cpp — Core twdll module functions (Log, memory tweaks, etc.)
#include <windows.h>
#include "tw.h"

// Forward-declare game-specific constants and signatures
extern const char* GAME_MODULE_NAME;
extern const uintptr_t OFFSET_MAX_UNITS_ARMY;
extern const uintptr_t OFFSET_MAX_UNITS_NAVY;

static int script_Log(lua_State* L) {
    if (const char* msg = l_checklstring(L, 1, nullptr))
        Log(msg);
    return 0;
}

static int script_SetMaxUnitsInArmy(lua_State* L) {
    int val = static_cast<int>(l_tointeger(L, 1));
    if (OFFSET_MAX_UNITS_ARMY > 0) {
        if (HMODULE hMod = GetModuleHandleA(GAME_MODULE_NAME)) {
            int* p = (int*)((uintptr_t)hMod + OFFSET_MAX_UNITS_ARMY);
            *p = val;
            Log("[twdll] set_max_units_in_army: %d", val);
        }
    } else {
        Log("[twdll] set_max_units_in_army: stub (no offset for this game)");
    }
    return 0;
}

static int script_SetMaxUnitsInNavy(lua_State* L) {
    int val = static_cast<int>(l_tointeger(L, 1));
    if (OFFSET_MAX_UNITS_NAVY > 0) {
        if (HMODULE hMod = GetModuleHandleA(GAME_MODULE_NAME)) {
            int* p = (int*)((uintptr_t)hMod + OFFSET_MAX_UNITS_NAVY);
            *p = val;
            Log("[twdll] set_max_units_in_navy: %d", val);
        }
    } else {
        Log("[twdll] set_max_units_in_navy: stub (no offset for this game)");
    }
    return 0;
}

extern const char* GAME_NAME;

static int script_GameBuild(lua_State* L) {
    l_pushstring(L, GAME_NAME);
    return 1;
}

extern const luaL_Reg twdll_core[] = {
    {"Log",                  script_Log},
    {"GameBuild",            script_GameBuild},
    {"set_max_units_in_army",script_SetMaxUnitsInArmy},
    {"set_max_units_in_navy",script_SetMaxUnitsInNavy},
    {nullptr, nullptr}
};
