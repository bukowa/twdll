/// @module FACTION_SCRIPT_INTERFACE
/// Extensions to the game's faction object.
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"

using twdll::TW_Faction;
using twdll::TW_Character;
using twdll::TW_Region;
using twdll::TW_CampaignModel;
using twdll::TW_CampaignEnv;
using twdll::TW_GameCore;
using twdll::TW_Databases;

constexpr size_t FACTION_PTR = twdll::TW_PtrOffset<TW_Faction>::value;

namespace Props {
    static twdll::Property<int, TW_Faction> Treasury{&TW_Faction::treasury, FACTION_PTR, "faction"};
}

/***
Returns the memory address of the faction object as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemoryAddress    (lua_State* L) { return tw_mem_address(L, "faction", FACTION_PTR); }

/***
Gets the amount of gold (treasury) for the faction.
@function GetTreasury
@treturn integer amount of gold
*/
static int GetTreasury      (lua_State* L) { return Props::Treasury.get(L); }

/***
Sets the amount of gold (treasury) for the faction.
@function SetTreasury
@tparam integer value new amount of gold
*/
static int SetTreasury      (lua_State* L) { return Props::Treasury.set(L); }

/***
Makes the given region the faction's capital.
Works even if the faction currently has no home region: the fields are
set exactly as the engine does in FACTION::attach_to_region, including the
original home region and home theatre when they are not yet assigned.
@function SetCapital
@tparam userdata region the region to become the new capital
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
game's own internal path for finishing a technology. This is exactly what the
engine does when a tech completes, so events, achievements and unit upgrades
fire normally, and parent prerequisites are completed automatically.
Note: this differs from the game's built-in `cm:unlock_technology`, which only
makes a technology selectable and never actually finishes the research.
@function InstantlyResearchTechnology
@tparam string technology_key the technology record key (e.g. "att_tech_military_barracks")
@treturn boolean true if the technology was found and completed
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

    // Resolve the game's database list through the campaign model, the same
    // way the game itself looks up records (the typed chain below is verified
    // in tw_types.h against the 64-bit reference layout):
    //   env = cm->m_campaign_env; core = env->m_game_core; dbs = core->m_databases
    auto* env = static_cast<TW_CampaignModel*>(g_campaign_model)->m_campaign_env;
    if (!env) {
        Log("[twdll] InstantlyResearchTechnology: campaign env not resolved");
        return 0;
    }
    auto* game_core = static_cast<TW_CampaignEnv*>(env)->m_game_core;
    if (!game_core) {
        Log("[twdll] InstantlyResearchTechnology: game core not resolved");
        return 0;
    }
    auto* databases = static_cast<TW_GameCore*>(game_core)->m_databases;
    if (!databases) {
        Log("[twdll] InstantlyResearchTechnology: databases not resolved");
        return 0;
    }

    // technologies_table is the lazy-loader cache field on the databases object.
    void* tech_table = static_cast<TW_Databases*>(databases)->m_technologies_table;
    if (!tech_table) {
        Log("[twdll] InstantlyResearchTechnology: technologies_table not loaded");
        return 0;
    }

    // 32-bit CA::String layout verified via disasm: m_len @+0 (ctor stores
    // strlen @ sub_100DB440), m_data @+8 (hash base @ sub_100C76D0, compare
    // @ sub_100D9D90). record_index only reads the key (hash + compare, never
    // frees it), so no ctor/dtor, no heap, no leak. The +4 pad is unread by
    // the lookup path and matches the default ctor's 0.
    struct RecordKey {
        uint32_t    m_len;
        uint32_t    m_pad;
        const char* m_data;
    } key_string = { static_cast<uint32_t>(key_len), 0, key };

    void* record = g_record_index(tech_table, &key_string);
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
    {"GetTreasury",       GetTreasury},
    {"SetTreasury",       SetTreasury},
    {"SetFactionLeader",  SetFactionLeader},
    {"SetCapital",        SetCapital},
    {"InstantlyResearchTechnology", InstantlyResearchTechnology},
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
