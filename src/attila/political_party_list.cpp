/// @module POLITICAL_PARTY_LIST_SCRIPT_INTERFACE
/// List interface for campaign political parties.
#include "../common/tw.h"
#include "tw_types.h"

using twdll::TW_CampaignPoliticalParty;

void push_campaign_political_party(lua_State* L, TW_CampaignPoliticalParty* party);

static const char* kPartyListMetatable = "POLITICAL_PARTY_LIST_SCRIPT_INTERFACE";

struct PartyListUserdata {
    int count;
    TW_CampaignPoliticalParty* items[1];
};

static PartyListUserdata* get_party_list(lua_State* L) {
    return static_cast<PartyListUserdata*>(l_touserdata(L, 1));
}

/***
Number of political parties in this list.
@function num_items
@treturn integer number of parties
@usage
local count = party_list:num_items()
*/
static int PartyListNumItems(lua_State* L) {
    auto* list = get_party_list(L);
    l_pushinteger(L, list ? list->count : 0);
    return 1;
}

/***
Retrieves the political party at a zero-based index (`0` to `num_items() - 1`).
@function item_at
@tparam integer index zero-based list index
@treturn CAMPAIGN_POLITICAL_PARTY_SCRIPT_INTERFACE|nil political party object, or nil when out of bounds
@usage
for i = 0, party_list:num_items() - 1 do
    local party = party_list:item_at(i)
    if party:IsPrimary() then
        -- Found ruling party
    end
end
*/
static int PartyListItemAt(lua_State* L) {
    auto* list = get_party_list(L);
    if (!list || l_type(L, 2) != LUA_TNUMBER) {
        l_pushnil(L);
        return 1;
    }

    lua_Integer index = l_tointeger(L, 2);
    if (index < 0 || index >= list->count) {
        l_pushnil(L);
        return 1;
    }

    push_campaign_political_party(L, list->items[index]);
    return 1;
}

/***
Checks whether this political party list is empty.
@function is_empty
@treturn boolean true when the list has no parties, false otherwise
@usage
if not party_list:is_empty() then
    local first_party = party_list:item_at(0)
end
*/
static int PartyListIsEmpty(lua_State* L) {
    auto* list = get_party_list(L);
    l_pushboolean(L, !list || list->count == 0);
    return 1;
}

static const luaL_Reg party_list_methods[] = {
    {"num_items", PartyListNumItems},
    {"item_at",   PartyListItemAt},
    {"is_empty",  PartyListIsEmpty},
    {nullptr, nullptr}
};

void register_political_party_list_methods(lua_State* L) {
    l_newmetatable(L, kPartyListMetatable);
    l_createtable(L, 0, 3);
    for (const luaL_Reg* f = party_list_methods; f->name; ++f) {
        l_pushstring(L, f->name);
        l_pushcclosure(L, f->func, 0);
        l_settable(L, -3);
    }
    l_setfield(L, -2, "__index");
    l_pop(L, 1);
    Log("[twdll] POLITICAL_PARTY_LIST_SCRIPT_INTERFACE registered");
}

void push_political_party_list(lua_State* L,
                               TW_CampaignPoliticalParty* const* parties,
                               int count) {
    if (count < 0) count = 0;

    const size_t base_size = offsetof(PartyListUserdata, items);
    const size_t item_size = sizeof(TW_CampaignPoliticalParty*);
    const size_t size = base_size + static_cast<size_t>(count) * item_size;
    auto* list = static_cast<PartyListUserdata*>(l_newuserdata(L, size));
    list->count = count;
    for (int i = 0; i < count; ++i) {
        list->items[i] = parties[i];
    }

    l_newmetatable(L, kPartyListMetatable);
    l_setmetatable(L, -2);
}
