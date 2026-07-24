/// @module twdll.world
/// Campaign WORLD singleton accessor for Total War: Rome 2.
/// The pointer is captured at runtime when the game constructs the WORLD object.
#include "../common/tw.h"
#include "../common/campaign_hooks.h"
#include <cstddef>
#include <cstdio>

// ── Memory layout ─────────────────────────────────────────────────────────────
#pragma pack(push, 1)
struct TW_World {
    char pad_00[0x50];
    int  faction_count;  // 0x50 — TODO: verify for Rome 2
};
#pragma pack(pop)

static_assert(offsetof(TW_World, faction_count) == 0x50, "TW_World Rome2: faction_count");

// ── Accessors (global singleton — not Lua userdata) ───────────────────────────

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

// ── Lua registration table ────────────────────────────────────────────────────
extern const luaL_Reg world_functions[] = {
    {"GetMemoryAddress", GetMemoryAddress},
    {"GetFactionCount",  GetFactionCount},
    {nullptr, nullptr}
};
