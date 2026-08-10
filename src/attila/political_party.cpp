/// @module CAMPAIGN_POLITICAL_PARTY_SCRIPT_INTERFACE
/// Political party userdata handed out by faction:GetPoliticalParties().
#include "common/tw.h"
#include "game_api.h"
#include "tw_types.h"

using twdll::TW_CampaignPoliticalParty;
using twdll::TW_PoliticalPartyRecord;
using twdll::TW_CampaignPolitics;

static const char* kPartyMetatable = "CAMPAIGN_POLITICAL_PARTY_SCRIPT_INTERFACE";

/***
Returns the party's record key string (e.g. "att_political_party_romans_1").
@function GetKey
@treturn string party record key
*/
static int GetKey(lua_State* L) {
    auto* party = twdll::tw_unwrap<TW_CampaignPoliticalParty>(L, 1);
    if (!party || !party->m_party_record) {
        Log("[twdll] GetKey: null party");
        l_pushnil(L);
        return 1;
    }
    auto* record = static_cast<TW_PoliticalPartyRecord*>(party->m_party_record);
    l_pushstring(L, record->m_key.m_data ? record->m_key.m_data : "");
    return 1;
}

/***
Returns the number of senators currently held by the party.
@function GetSenators
@treturn integer number of senators
*/
static int GetSenators(lua_State* L) {
    auto* party = twdll::tw_unwrap<TW_CampaignPoliticalParty>(L, 1);
    if (!party) {
        Log("[twdll] GetSenators: null party");
        l_pushnil(L);
        return 1;
    }
    l_pushinteger(L, party->m_senators);
    return 1;
}

/***
Returns the current political power of the party as a float.
@function GetPower
@treturn number political power
*/
static int GetPower(lua_State* L) {
    auto* party = twdll::tw_unwrap<TW_CampaignPoliticalParty>(L, 1);
    if (!party) {
        Log("[twdll] GetPower: null party");
        l_pushnil(L);
        return 1;
    }
    l_pushnumber(L, party->m_power);
    return 1;
}

/***
Returns whether this party is the faction's primary (leading) party.
@function IsPrimary
@treturn boolean true if this is the primary party
*/
static int IsPrimary(lua_State* L) {
    auto* party = twdll::tw_unwrap<TW_CampaignPoliticalParty>(L, 1);
    if (!party || !party->m_politics) {
        Log("[twdll] IsPrimary: null party");
        l_pushboolean(L, 0);
        return 1;
    }
    auto* politics = static_cast<TW_CampaignPolitics*>(party->m_politics);
    l_pushboolean(L, party->m_party_record == politics->m_primary_party);
    return 1;
}

static const luaL_Reg party_methods[] = {
    {"GetKey",     GetKey},
    {"GetSenators", GetSenators},
    {"GetPower",   GetPower},
    {"IsPrimary",  IsPrimary},
    {nullptr, nullptr}
};

void register_political_party_methods(lua_State* L) {
    l_newmetatable(L, kPartyMetatable);
    l_createtable(L, 0, 3);
    for (const luaL_Reg* f = party_methods; f->name; ++f) {
        l_pushstring(L, f->name);
        l_pushcclosure(L, f->func, 0);
        l_settable(L, -3);
    }
    l_setfield(L, -2, "__index");
    l_pop(L, 1);
    Log("[twdll] CAMPAIGN_POLITICAL_PARTY_SCRIPT_INTERFACE registered");
}

void push_campaign_political_party(lua_State* L, TW_CampaignPoliticalParty* party) {
    twdll::tw_push_wrapped<TW_CampaignPoliticalParty>(L, party);
    if (party) {
        l_newmetatable(L, kPartyMetatable);
        l_setmetatable(L, -2);
    }
}
