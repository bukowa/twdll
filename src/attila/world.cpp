/// @module twdll.world
/// Campaign world singleton and world-level modifiers for Total War: Attila.
#include "../common/tw.h"
#include "../common/campaign_hooks.h"
#include "game_api.h"
#include "tw_types.h"
#include <windows.h>
#include <cstdio>

using twdll::TW_World;

static TW_World* g_world = nullptr;
static void* orig_world_ctor = nullptr;

static void LogWorldHook(void* ptr) {
    g_world = static_cast<TW_World*>(ptr);
    Log("[twdll] WORLD ctor hooked — g_world = 0x%08X", reinterpret_cast<uintptr_t>(ptr));
}

__declspec(naked) static void HookedWorldCtor() {
    __asm {
        pushad
        push ecx
        call LogWorldHook
        add esp, 4
        popad
        jmp dword ptr [orig_world_ctor]
    }
}

static void install_world_hook(uintptr_t base, size_t size) {
    install_singleton_hook(base, size, "FACTION_ARRAY", "WORLD",
                           reinterpret_cast<void*>(HookedWorldCtor),
                           &orig_world_ctor);
}

/***
Returns the memory address of the WORLD singleton as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678"), or nil if not yet initialised
*/
static int GetMemoryAddress(lua_State* L) {
    if (!g_world) { l_pushnil(L); return 1; }
    char buf[20];
    snprintf(buf, sizeof(buf), "0x%08X", reinterpret_cast<unsigned int>(g_world));
    l_pushstring(L, buf);
    return 1;
}

static twdll::GlobalGetter<int, TW_World> FactionCount{&TW_World::faction_count, &g_world};

/***
Gets the number of factions in the current campaign.
@function GetFactionCount
@treturn integer number of factions, or nil if not yet initialised
*/
static int GetFactionCount(lua_State* L) { return FactionCount.get(L); }

/***
Sets the maximum number of units allowed in an army.
@function SetMaxUnitsInArmy
@tparam integer val maximum unit count
*/
static int SetMaxUnitsInArmy(lua_State* L) {
    int val = static_cast<int>(l_tointeger(L, 1));
    if (HMODULE hMod = GetModuleHandleA("empire.retail.dll")) {
        *reinterpret_cast<int*>(reinterpret_cast<uintptr_t>(hMod) + OFFSET_MAX_UNITS_ARMY) = val;
        Log("[twdll] SetMaxUnitsInArmy: %d", val);
    }
    return 0;
}

/***
Sets the maximum number of units allowed in a navy.
@function SetMaxUnitsInNavy
@tparam integer val maximum unit count
*/
static int SetMaxUnitsInNavy(lua_State* L) {
    int val = static_cast<int>(l_tointeger(L, 1));
    if (HMODULE hMod = GetModuleHandleA("empire.retail.dll")) {
        *reinterpret_cast<int*>(reinterpret_cast<uintptr_t>(hMod) + OFFSET_MAX_UNITS_NAVY) = val;
        Log("[twdll] SetMaxUnitsInNavy: %d", val);
    }
    return 0;
}

extern const luaL_Reg world_functions[] = {
    {"GetMemoryAddress",   GetMemoryAddress},
    {"GetFactionCount",    GetFactionCount},
    {"SetMaxUnitsInArmy",  SetMaxUnitsInArmy},
    {"SetMaxUnitsInNavy",  SetMaxUnitsInNavy},
    {nullptr, nullptr}
};
