/// @module twdll_faction
/// Faction properties for Total War: Attila.
#include "../tw.h"
#include <cstddef>

// ── Memory layout ─────────────────────────────────────────────────────────────
// NOTE: Layout currently matches Rome 2. Verify before changing offsets.
#pragma pack(push, 1)
struct TW_Faction {
    char pad_00[0x6C4];
    int  gold;  // 0x6C4
};
#pragma pack(pop)

static_assert(offsetof(TW_Faction, gold) == 0x6C4, "TW_Faction Attila: gold");

constexpr size_t FACTION_PTR = 0x8;

// ── Accessors ─────────────────────────────────────────────────────────────────
/***
Gets the amount of gold for the faction.
@function GetGold
@tparam userdata faction the faction object (first argument)
@treturn integer amount of gold
*/
/***
Sets the amount of gold for the faction.
@function SetGold
@tparam userdata faction the faction object (first argument)
@tparam integer value new gold amount
*/
TW_PROP(Gold, TW_Faction, gold, FACTION_PTR, "faction")

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
