/// @module FACTION_SCRIPT_INTERFACE
/// Extensions to the game's faction object.
#include <windows.h>
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"
#include <vector>

using twdll::TW_Faction;
using twdll::TW_Character;
using twdll::TW_Region;
using twdll::TW_Settlement;
using twdll::TW_CampaignModel;
using twdll::TW_CampaignEnv;
using twdll::TW_GameCore;
using twdll::TW_Databases;
using twdll::TW_CampaignPolitics;
using twdll::TW_CampaignPoliticalParty;
using twdll::TW_PoliticalPartiesMap;
using twdll::TW_PoliticalPartyRecord;

void push_campaign_political_party(lua_State* L, TW_CampaignPoliticalParty* party);
void push_political_party_list(lua_State* L,
                               TW_CampaignPoliticalParty* const* parties,
                               int count);

constexpr size_t FACTION_PTR = twdll::TW_PtrOffset<TW_Faction>::value;

namespace Props {
    static twdll::Property Treasury{&TW_Faction::treasury, FACTION_PTR, "faction"};
}

/***
Memory address of the faction object in hexadecimal format.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
@usage local addr = faction:GetMemoryAddress()
*/
static int GetMemoryAddress    (lua_State* L) { return tw_mem_address(L, "faction", FACTION_PTR); }

/***
Amount of gold in the faction's treasury.
@function GetTreasury
@treturn integer current treasury gold amount
@usage
local gold = faction:GetTreasury()
*/
static int GetTreasury      (lua_State* L) { return Props::Treasury.get(L); }

/***
Sets the amount of gold in the faction's treasury.
Persisted natively in savegames and immediately available for building and recruitment.
@function SetTreasury
@tparam integer value new gold amount
@treturn boolean true on success, false otherwise
@usage
-- Give faction 50,000 gold:
faction:SetTreasury(50000)
*/
static int SetTreasury      (lua_State* L) { return Props::Treasury.set(L); }

/***
Makes the specified region the faction's primary capital / home region.

Works even if the faction currently has no home region (e.g. horde settling): properly assigns
the faction's capital, original home region, and home theatre.
@function SetCapital
@tparam REGION_SCRIPT_INTERFACE region the region to become the new capital
@usage
local region = game:model():world():region_manager():region_by_key("att_reg_scandza_hafn")
faction:SetCapital(region)
*/
static int SetCapital(lua_State* L) {
    auto* faction = twdll::tw_unwrap<TW_Faction>(L, 1);
    auto* region  = twdll::tw_unwrap<TW_Region>(L, 2);
    if (!faction || !region) {
        Log("[twdll] SetCapital: null faction or region");
        return 0;
    }
    faction->m_home_region = region;
    if (!faction->m_original_home_region)
        faction->m_original_home_region = region;
    if (!faction->m_home_theatre && region->m_region_data)
        faction->m_home_theatre = region->m_region_data->m_theatre;
    return 0;
}

/***
Instantly completes research of the given technology for the faction, using the
game's own internal path for finishing a technology.

This triggers all native engine completion mechanics:
- Fires research completion campaign events.
- Grants technology-related campaign achievements.
- Applies unit upgrades and building unlocks immediately.
- Completes parent prerequisites automatically.

Note: this differs from the game's built-in `cm:unlock_technology`, which only
makes a technology selectable in the UI and never actually finishes research.
@function InstantlyResearchTechnology
@tparam string technology_key the technology record key from `technologies_tables` (e.g. `"att_tech_military_barracks"`, `"att_hunnic_military_combat_at_distance"`)
@treturn boolean true if the technology was found and completed, false otherwise
@usage
local ok = faction:InstantlyResearchTechnology("att_hunnic_military_combat_at_distance")
if ok then
    twdll.core.Log("Technology researched instantly!")
end
*/
static int InstantlyResearchTechnology(lua_State* L) {
    if (!g_instant_set_researched || !g_record_index) {
        Log("[twdll] InstantlyResearchTechnology: signatures not resolved");
        return 0;
    }
    if (!g_campaign_model) {
        Log("[twdll] InstantlyResearchTechnology: campaign model not available");
        return 0;
    }

    size_t key_len = 0;
    const char* key = l_checklstring(L, 2, &key_len);
    if (!key) {
        Log("[twdll] InstantlyResearchTechnology: technology key not a string");
        return 0;
    }

    auto* faction = twdll::tw_unwrap<TW_Faction>(L, 1);
    if (!faction) {
        Log("[twdll] InstantlyResearchTechnology: null faction");
        return 0;
    }

    auto* dbs = TW_Databases::get();
    if (!dbs || !dbs->technologies) {
        Log("[twdll] InstantlyResearchTechnology: technologies table not loaded");
        return 0;
    }

    void* record = dbs->technologies->find_record(key, key_len);
    if (!record) {
        Log("[twdll] InstantlyResearchTechnology: no record for key '%s'", key);
        l_pushboolean(L, 0);
        return 1;
    }

    void* manager = faction->m_faction_technology_manager;
    if (!manager) {
        Log("[twdll] InstantlyResearchTechnology: no technology manager");
        l_pushboolean(L, 0);
        return 1;
    }

    Log("[twdll] InstantlyResearchTechnology: faction=0x%08X manager=0x%08X record=0x%08X key='%s'",
        reinterpret_cast<uintptr_t>(faction),
        reinterpret_cast<uintptr_t>(manager),
        reinterpret_cast<uintptr_t>(record), key);

    g_instant_set_researched(manager, record, /*report_to_ui*/ true);
    l_pushboolean(L, 1);
    return 1;
}

/***
Sets a new leader for the faction.

Supports three operational modes:
1. **Silent swap** (`faction:SetFactionLeader(new_char)`): changes the faction leader immediately without firing succession events or modifying political stability.
2. **Standard succession** (`faction:SetFactionLeader(new_char, old_char)`): triggers the standard `faction_succession` (or `faction_succession_regency`) campaign event.
3. **Heir coming of age** (`faction:SetFactionLeader(new_char, old_char, true)`): triggers the `faction_succession_heir_comes_of_age` campaign event.

Automatically resets the new leader's heir status and aligns political party leadership.
@function SetFactionLeader
@tparam CHARACTER_SCRIPT_INTERFACE new_character the character to become the new leader
@tparam[opt] CHARACTER_SCRIPT_INTERFACE old_character the outgoing leader (triggers succession event if provided)
@tparam[opt=false] boolean heir_coming_of_age fire the heir-comes-of-age event variant (default: false)
@usage
-- Mode 1: Silent swap without event popup:
faction:SetFactionLeader(new_general)

-- Mode 2: Standard succession with in-game succession event:
local old_leader = faction:faction_leader()
faction:SetFactionLeader(new_general, old_leader)

-- Mode 3: Heir coming of age succession:
faction:SetFactionLeader(heir_general, old_leader, true)
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

/***
List interface (@{POLITICAL_PARTY_LIST_SCRIPT_INTERFACE}) for the faction's campaign political parties.
Use `num_items()` and zero-based `item_at(index)` to iterate the parties.
@function GetPoliticalPartyList
@treturn POLITICAL_PARTY_LIST_SCRIPT_INTERFACE list interface for the faction's campaign political parties
@usage
local party_list = faction:GetPoliticalPartyList()
for i = 0, party_list:num_items() - 1 do
    local party = party_list:item_at(i)
    twdll.core.Log(string.format("Party [%s]: Senators=%d, Power=%.1f%%, Primary=%s",
        party:GetKey(), party:GetSenators(), party:GetPower() * 100, tostring(party:IsPrimary())))
end
*/
static int GetPoliticalPartyList(lua_State* L) {
    auto* faction = twdll::tw_unwrap<TW_Faction>(L, 1);
    if (!faction) {
        Log("[twdll] GetPoliticalPartyList: null faction");
        l_pushnil(L);
        return 1;
    }

    const auto& map = faction->m_politics.m_political_parties;
    std::vector<TW_CampaignPoliticalParty*> parties;
    parties.reserve(map.m_count);
    map.for_each([&](TW_CampaignPoliticalParty* party) {
        parties.push_back(party);
    });

    push_political_party_list(L, parties.data(), static_cast<int>(parties.size()));
    return 1;
}

/***
Retrieves the political party in this faction matching the given record key.
@function GetPoliticalParty
@tparam string party_key the party record key from `political_parties_tables` (e.g. `"att_politics_hunni_ruler"`, `"att_politics_hunni_council"`)
@treturn CAMPAIGN_POLITICAL_PARTY_SCRIPT_INTERFACE|nil political party object, or nil if not found
@usage
local party = faction:GetPoliticalParty("att_politics_hunni_ruler")
if party then
    twdll.core.Log("Found party with senators:", party:GetSenators())
end
*/
static int GetPoliticalParty(lua_State* L) {
    auto* faction = twdll::tw_unwrap<TW_Faction>(L, 1);
    const char* key = l_checklstring(L, 2, nullptr);
    if (!faction || !key) {
        Log("[twdll] GetPoliticalParty: null faction or key");
        l_pushnil(L);
        return 1;
    }
    auto* party = faction->m_politics.m_political_parties.find_by_key(key);
    if (!party) {
        l_pushnil(L);
        return 1;
    }
    push_campaign_political_party(L, party);
    return 1;
}

/***
Primary (leading / ruling) political party of the faction.
@function GetPrimaryParty
@treturn CAMPAIGN_POLITICAL_PARTY_SCRIPT_INTERFACE|nil primary political party object, or nil if none
@usage
local ruler_party = faction:GetPrimaryParty()
if ruler_party then
    twdll.core.Log("Ruling party key:", ruler_party:GetKey(), "Senators:", ruler_party:GetSenators())
end
*/
static int GetPrimaryParty(lua_State* L) {
    auto* faction = twdll::tw_unwrap<TW_Faction>(L, 1);
    if (!faction) {
        Log("[twdll] GetPrimaryParty: null faction");
        l_pushnil(L);
        return 1;
    }
    auto* party = faction->m_politics.m_political_parties.find_by_record(faction->m_politics.m_primary_party);
    if (!party) {
        l_pushnil(L);
        return 1;
    }
    push_campaign_political_party(L, party);
    return 1;
}

/***
Checks whether the faction participates in the campaign politics system and has political parties.
@function HasPoliticalParties
@treturn boolean true if the faction has at least one political party, false otherwise
@usage
if faction:HasPoliticalParties() then
    local parties = faction:GetPoliticalPartyList()
    twdll.core.Log("Faction has politics with party count:", parties:num_items())
end
*/
static int HasPoliticalParties(lua_State* L) {
    auto* faction = twdll::tw_unwrap<TW_Faction>(L, 1);
    if (!faction) {
        Log("[twdll] HasPoliticalParties: null faction");
        l_pushboolean(L, 0);
        return 1;
    }
    l_pushboolean(L, faction->m_politics.m_political_parties.m_count > 0);
    return 1;
}

/***
Creates and spawns a new agent character on the campaign map for this faction in the current tick.

Supports two calling styles:
- **Settlement**: `faction:CreateAgent(agent_key, settlement)` — spawns the agent on the campaign map adjacent to the specified settlement.
- **Map coordinates**: `faction:CreateAgent(agent_key, x, y)` — spawns the agent at or adjacent to the specified map coordinates.

@function CreateAgent
@tparam string agent_key database key of the agent (e.g. `"champion"`, `"spy"`, `"dignitary"`, `"priest"`)
@tparam[opt] userdata|number settlement_or_x `SETTLEMENT_SCRIPT_INTERFACE` object or map X coordinate (defaults to faction capital)
@tparam[opt] number y map Y coordinate (required when `settlement_or_x` is an X coordinate)
@treturn boolean true on success, false otherwise
@usage
-- Example 1: Spawn a champion next to a settlement
local capital = faction:home_region():settlement()
faction:CreateAgent("champion", capital)

-- Example 2: Spawn a dignitary at specific map coordinates (x, y)
faction:CreateAgent("dignitary", 516, 381)
*/
static int CreateAgent(lua_State* L) {
    auto* faction = twdll::tw_unwrap<TW_Faction>(L, 1);
    if (!faction) {
        Log("[twdll] faction:CreateAgent: null faction");
        l_pushboolean(L, 0);
        return 1;
    }

    if (l_type(L, 2) != LUA_TSTRING) {
        Log("[twdll] faction:CreateAgent: agent_key must be a string");
        l_pushboolean(L, 0);
        return 1;
    }
    const char* agent_key = l_checkstring(L, 2);

    void* pool_mgr = faction->m_character_recruitment_pool;
    if (!pool_mgr) {
        Log("[twdll] faction:CreateAgent: recruitment pool manager is null");
        l_pushboolean(L, 0);
        return 1;
    }

    auto* dbs = TW_Databases::get();
    if (!dbs || !dbs->agents) {
        Log("[twdll] faction:CreateAgent: agents database table is null");
        l_pushboolean(L, 0);
        return 1;
    }

    void* agent_rec = dbs->agents->find_record(agent_key);
    if (!agent_rec) {
        Log("[twdll] faction:CreateAgent: agent record '%s' not found in database", agent_key);
        l_pushboolean(L, 0);
        return 1;
    }

    void* optional_settlement = nullptr;
    uint32_t logical_pos = 0;
    uint32_t* p_optional_position = nullptr;

    if (l_type(L, 3) == LUA_TNUMBER && l_type(L, 4) == LUA_TNUMBER) {
        uint16_t x = static_cast<uint16_t>(l_tonumber(L, 3));
        uint16_t y = static_cast<uint16_t>(l_tonumber(L, 4));
        logical_pos = (static_cast<uint32_t>(y) << 16) | static_cast<uint32_t>(x);
        p_optional_position = &logical_pos;
    } else if (l_type(L, 3) == LUA_TUSERDATA) {
        optional_settlement = twdll::tw_unwrap<TW_Settlement>(L, 3);
    }

    if (!optional_settlement && !p_optional_position) {
        optional_settlement = faction->m_home_region;
    }

    HMODULE hMod = GetModuleHandleA("empire.retail.dll");
    if (!hMod) {
        l_pushboolean(L, 0);
        return 1;
    }

    using FnSpawnAgent = void(__thiscall*)(
        void* recruitment_pool_mgr,
        void* agent_record,
        uint32_t* optional_position,
        void* optional_settlement,
        void* optional_military_force,
        void* script_id_ca_string,
        unsigned int character_type
    );
    auto fnSpawnAgent = reinterpret_cast<FnSpawnAgent>(
        reinterpret_cast<uintptr_t>(hMod) + 0x007F2CF0);

    struct { uint32_t len; uint32_t cap; const char* buf; } empty_id = { 0, 0, "" };

    __try {
        fnSpawnAgent(pool_mgr, agent_rec, p_optional_position, optional_settlement, nullptr, &empty_id, 3);
        Log("[twdll] faction:CreateAgent: agent '%s' spawned successfully for faction 0x%08X",
            agent_key, reinterpret_cast<uintptr_t>(faction));
        l_pushboolean(L, 1);
    } __except (EXCEPTION_EXECUTE_HANDLER) {
        Log("[twdll] faction:CreateAgent: caught SEH exception 0x%08X in engine spawn call", GetExceptionCode());
        l_pushboolean(L, 0);
    }
    return 1;
}

extern const luaL_Reg faction_functions[] = {
    {nullptr, nullptr}
};

static const luaL_Reg faction_methods[] = {
    {"GetMemoryAddress",  GetMemoryAddress},
    {"GetTreasury",       GetTreasury},
    {"SetTreasury",       SetTreasury},
    {"SetFactionLeader",  SetFactionLeader},
    {"SetCapital",        SetCapital},
    {"InstantlyResearchTechnology", InstantlyResearchTechnology},
    {"GetPoliticalPartyList", GetPoliticalPartyList},
    {"GetPoliticalParties",   GetPoliticalPartyList},
    {"political_party_list",  GetPoliticalPartyList},
    {"GetPoliticalParty", GetPoliticalParty},
    {"GetPrimaryParty",   GetPrimaryParty},
    {"HasPoliticalParties", HasPoliticalParties},
    {"CreateAgent",       CreateAgent},
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
