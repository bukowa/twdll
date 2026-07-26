/// @module FACTION_SCRIPT_INTERFACE
/// Extensions to the game's faction object for Total War: Attila.
#include "../common/tw.h"
#include "game_api.h"
#include <cstddef>
#include <cstdio>

#include "tw_types.h"

constexpr size_t FACTION_PTR = offsetof(twdll::GameScriptInterface<twdll::TW_Faction>, m_wrapped_object);

/***
Gets the amount of gold for the faction.
@function GetGold
@tparam userdata faction the faction object (first argument)
@treturn integer amount of gold
*/
static twdll::Property<int, twdll::TW_Faction> Gold{&twdll::TW_Faction::gold, FACTION_PTR, "faction"};
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

/***
Replaces the faction leader without killing the current one.
The old leader remains in the family tree and on the map.

Behaviour depends on the arguments:
- `old_character` omitted or nil — no succession event is fired; the swap is silent.
- `old_character` provided — fires `faction_succession` event.
  - If the new leader is a regent — fires `faction_succession_regency` instead.
  - If `heir_coming_of_age` is true — fires `faction_succession_heir_comes_of_age` instead.

In all cases: new leader's `m_heir` is set to 0 and political party allegiance is updated.
If the faction has no regions after the swap it is added to the deferred death list.

@function SetFactionLeader
@tparam userdata faction the faction object (self)
@tparam userdata new_character the character to become the new leader
@tparam[opt] userdata old_character the current leader; omit to suppress succession events
@tparam[opt] boolean heir_coming_of_age fire heir_comes_of_age event instead of normal succession (default false)
@usage faction:SetFactionLeader(new_char)              -- silent swap, no event
@usage faction:SetFactionLeader(new_char, old_char)    -- normal succession event
@usage faction:SetFactionLeader(new_char, old_char, true) -- heir coming of age event
*/
static int SetFactionLeader(lua_State* L) {
    auto* faction  = twdll::tw_unwrap<twdll::TW_Faction>(L, 1);
    auto* new_char = twdll::tw_unwrap<twdll::TW_Character>(L, 2);
    auto* old_char = twdll::tw_unwrap<twdll::TW_Character>(L, 3);  // may be null
    bool  heir_coming_of_age = (l_type(L, 4) == 1) && (l_tointeger(L, 4) != 0); // optional, default false

    if (!faction || !new_char) {
        Log("[twdll] SetFactionLeader: null faction or new_character");
        return 0;
    }

    if (!g_new_faction_leader) {
        Log("[twdll] SetFactionLeader: function not resolved");
        return 0;
    }

    Log("[twdll] SetFactionLeader: faction=0x%08X new=0x%08X old=0x%08X heir=%d",
        reinterpret_cast<uintptr_t>(faction), reinterpret_cast<uintptr_t>(new_char),
        reinterpret_cast<uintptr_t>(old_char), (int)heir_coming_of_age);

    g_new_faction_leader(faction, new_char, old_char, heir_coming_of_age);
    return 0;
}

extern const luaL_Reg faction_functions[] = {
    {nullptr, nullptr}
};

// Methods injected into game's FACTION_SCRIPT_INTERFACE metatable.
// Usage: faction:GetGold(), faction:SetGold(1000)
extern const luaL_Reg faction_methods[] = {
    {"GetMemoryAddress",  GetMemAddress},
    {"GetGold",           GetGold},
    {"SetGold",           SetGold},
    {"SetFactionLeader",  SetFactionLeader},
    {nullptr, nullptr}
};

void register_faction_methods(lua_State* L) {
    l_newmetatable(L, "FACTION_SCRIPT_INTERFACE"); // push existing metatable
    l_getfield(L, -1, "__index");                  // push methods table
    if (l_type(L, -1) == 5) {                      // 5 = LUA_TTABLE
        for (const luaL_Reg* f = faction_methods; f->name; ++f) {
            l_pushstring(L, f->name);
            l_pushcclosure(L, f->func, 0);
            l_settable(L, -3);
        }
        Log("[twdll] FACTION_SCRIPT_INTERFACE extended: GetGold, SetGold, GetMemoryAddress, SetFactionLeader");
    } else {
        Log("[twdll] WARNING: FACTION_SCRIPT_INTERFACE __index not found");
    }
    l_pop(L, 2);  // pop __index + metatable
}
