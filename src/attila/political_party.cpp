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
Database record key string of the political party from `political_parties_tables` (e.g. `"att_politics_hunni_ruler"`).
@function GetKey
@treturn string party record key
@usage
local key = party:GetKey()
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

constexpr size_t PARTY_PTR = twdll::TW_PtrOffset<TW_CampaignPoliticalParty>::value;

namespace Props {
    static twdll::Getter Senators{&TW_CampaignPoliticalParty::m_senators, PARTY_PTR, "political_party"};
    static twdll::Getter Power   {&TW_CampaignPoliticalParty::m_power,    PARTY_PTR, "political_party"};
}

/***
Number of senators currently supporting this political party.
@function GetSenators
@treturn integer number of senators
@usage
local senators = party:GetSenators()
*/
static int GetSenators(lua_State* L) { return Props::Senators.get(L); }

/***
Current political power of the party as a normalized proportion (e.g. `0.70` for 70% senate dominance).
@function GetPower
@treturn number political power proportion (0.0 to 1.0)
@usage
local power_pct = party:GetPower() * 100
if power_pct >= 60.0 then
    -- Ruling party enjoys high political dominance
end
*/
static int GetPower(lua_State* L) { return Props::Power.get(L); }

/***
Checks whether this political party is the faction's primary (ruling) party.
@function IsPrimary
@treturn boolean true if this is the primary ruling party, false otherwise
@usage
if party:IsPrimary() then
    -- Party is the head of the faction
end
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

/***
Sets this party as the faction's primary (ruling) party.

NOTE: This method directly assigns the ruling political party of the faction.
It does not automatically cascade allegiance changes to existing family members
or the faction leader. To avoid visual conflicts in the Clan / Family Tree UI
(where family members belonging to other parties appear under 'Other Houses'),
reassign characters using @{CHARACTER_SCRIPT_INTERFACE:SetPoliticalParty}.
@function SetPrimary
@treturn boolean true if successfully set, false otherwise
@usage
local opposition = faction:GetPoliticalParty("att_politics_hunni_council")
if opposition and not opposition:IsPrimary() then
    opposition:SetPrimary()
end
*/
static int SetPrimary(lua_State* L) {
    auto* party = twdll::tw_unwrap<TW_CampaignPoliticalParty>(L, 1);
    if (!party || !party->m_politics || !party->m_party_record) {
        Log("[twdll] SetPrimary: null party, politics, or party_record");
        l_pushboolean(L, 0);
        return 1;
    }
    auto* politics = static_cast<TW_CampaignPolitics*>(party->m_politics);
    politics->m_primary_party = party->m_party_record;
    Log("[twdll] SetPrimary: party 0x%08X set as primary", reinterpret_cast<uintptr_t>(party->m_party_record));
    l_pushboolean(L, 1);
    return 1;
}

static const luaL_Reg party_methods[] = {
    {"GetKey",     GetKey},
    {"GetSenators", GetSenators},
    {"GetPower",   GetPower},
    {"IsPrimary",  IsPrimary},
    {"SetPrimary", SetPrimary},
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
