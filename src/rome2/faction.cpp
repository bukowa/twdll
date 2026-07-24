/// @module twdll.faction
/// Faction properties for Rome 2.
#include "../common/tw.h"
#include <cstddef>

// ── Memory layout ─────────────────────────────────────────────────────────────
#pragma pack(push, 1)
struct TW_Faction {
    char pad_00[0x6C4];
    int  gold;  // 0x6C4
};
#pragma pack(pop)

static_assert(offsetof(TW_Faction, gold) == 0x6C4, "TW_Faction Rome2: gold");

constexpr size_t FACTION_PTR = 0x8;

// ── Accessors ─────────────────────────────────────────────────────────────────
/***
Gets the amount of gold for the faction.
@function GetGold
@tparam userdata faction the faction object (first argument)
@treturn integer amount of gold
*/
static twdll::Property<int, TW_Faction> Gold{&TW_Faction::gold, FACTION_PTR, "faction"};
static int GetGold(lua_State* L) { return Gold.get(L); }

/***
Sets the amount of gold for the faction.
@function SetGold
@tparam userdata faction the faction object (first argument)
@tparam integer value new gold amount
*/
static int SetGold(lua_State* L) { return Gold.set(L); }

/***
Returns the memory address of the real faction object as a hexadecimal string.
@function GetMemoryAddress
@tparam userdata faction the faction object (first argument)
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemAddress(lua_State* L) { return tw_mem_address(L, "faction", FACTION_PTR); }

// ── Lua registration table ────────────────────────────────────────────────────
extern const luaL_Reg faction_functions[] = {
    {"GetMemoryAddress", GetMemAddress},
    {"GetGold",          GetGold},
    {"SetGold",          SetGold},
    {nullptr, nullptr}
};
