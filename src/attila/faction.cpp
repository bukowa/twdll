/// @module FACTION_SCRIPT_INTERFACE
/// Extensions to the game's faction object.
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"

using twdll::TW_Faction;
using twdll::TW_Character;

constexpr size_t FACTION_PTR = twdll::TW_PtrOffset<TW_Faction>::value;

namespace Props {
    static twdll::Property<int, TW_Faction> Gold{&TW_Faction::gold, FACTION_PTR, "faction"};
}

/***
Returns the memory address of the faction object as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemoryAddress    (lua_State* L) { return tw_mem_address(L, "faction", FACTION_PTR); }

/***
Gets the amount of gold for the faction.
@function GetGold
@treturn integer amount of gold
*/
static int GetGold          (lua_State* L) { return Props::Gold.get(L); }

/***
Sets the amount of gold for the faction.
@function SetGold
@tparam integer value new amount of gold
*/
static int SetGold          (lua_State* L) { return Props::Gold.set(L); }

/***
Sets a new leader for the faction.
If `old_character` is provided, the game fires a succession event. If `heir_coming_of_age`
is true, fires `faction_succession_heir_comes_of_age` instead of the default succession event.
@function SetFactionLeader
@tparam userdata new_character the character to become the new leader
@tparam[opt] userdata old_character the outgoing leader (triggers succession event if provided)
@tparam[opt] boolean heir_coming_of_age fire the heir-comes-of-age event variant (default false)
*/
static int SetFactionLeader(lua_State* L) {
    auto* faction  = twdll::tw_unwrap<TW_Faction>(L, 1);
    auto* new_char = twdll::tw_unwrap<TW_Character>(L, 2);
    auto* old_char = twdll::tw_unwrap<TW_Character>(L, 3);  // may be null
    const bool  heir_coming_of_age = (l_type(L, 4) == LUA_TBOOLEAN) && (l_tointeger(L, 4) != 0);

    if (!faction || !new_char) {
        Log("[twdll] SetFactionLeader: null faction or new_character");
        return 0;
    }
    if (!g_new_faction_leader) {
        Log("[twdll] SetFactionLeader: function not resolved");
        return 0;
    }

    Log("[twdll] SetFactionLeader: faction=0x%08X new=0x%08X old=0x%08X heir=%d",
        faction, new_char,
        old_char, heir_coming_of_age);

    g_new_faction_leader(faction, new_char, old_char, heir_coming_of_age);
    return 0;
}

extern const luaL_Reg faction_functions[] = {
    {nullptr, nullptr}
};

static const luaL_Reg faction_methods[] = {
    {"GetMemoryAddress",  GetMemoryAddress},
    {"GetGold",           GetGold},
    {"SetGold",           SetGold},
    {"SetFactionLeader",  SetFactionLeader},
    {nullptr, nullptr}
};

void register_faction_methods(lua_State* L) {
    l_newmetatable(L, "FACTION_SCRIPT_INTERFACE");
    l_getfield(L, -1, "__index");
    if (l_type(L, -1) == LUA_TTABLE) {
        for (const luaL_Reg* f = faction_methods; f->name; ++f) {
            l_pushstring(L, f->name);
            l_pushcclosure(L, f->func, 0);
            l_settable(L, -3);
        }
        Log("[twdll] FACTION_SCRIPT_INTERFACE extended");
    } else {
        Log("[twdll] WARNING: FACTION_SCRIPT_INTERFACE __index not found");
    }
    l_pop(L, 2);
}
