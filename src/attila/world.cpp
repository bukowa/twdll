/// @module twdll.world
/// Campaign world singleton and world-level modifiers for Total War: Attila.
#include "../common/tw.h"
#include "../common/campaign_hooks.h"
#include "game_api.h"
#include <windows.h>
#include <cstddef>
#include <cstdio>

// ── Memory layout ─────────────────────────────────────────────────────────────
#pragma pack(push, 1)
struct TW_World {
    char pad_00[0x50];
    int  faction_count;  // 0x50 — needs verification
};
#pragma pack(pop)

static_assert(offsetof(TW_World, faction_count) == 0x50, "TW_World Attila: faction_count");

// ── Accessors ─────────────────────────────────────────────────────────────────

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

// ── Lua registration table ────────────────────────────────────────────────────
extern const luaL_Reg world_functions[] = {
    {"GetMemoryAddress",   GetMemoryAddress},
    {"GetFactionCount",    GetFactionCount},
    {"SetMaxUnitsInArmy",  SetMaxUnitsInArmy},
    {"SetMaxUnitsInNavy",  SetMaxUnitsInNavy},
    {nullptr, nullptr}
};
