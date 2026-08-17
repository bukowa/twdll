/// @module CHARACTER_SCRIPT_INTERFACE
/// Extensions to the game's character object.
#include "../common/tw.h"
#include "../common/signature_scanner.h"
#include "../common/log.h"
#include "game_api.h"
#include "tw_types.h"
#include <windows.h>

using twdll::TW_Character;
using twdll::TW_CharacterDetails;
using twdll::TW_GeneralBodyguardDetails;
using twdll::TW_CampaignModel;
using twdll::TW_CampaignEnv;
using twdll::TW_GameCore;
using twdll::TW_Databases;

constexpr size_t CHAR_PTR = twdll::TW_PtrOffset<TW_Character>::value;

namespace Props {
    static twdll::Property ActionPoints {&TW_Character::action_points,            CHAR_PTR, "character"};
    static twdll::Property Influence    {&TW_CharacterDetails::political_gravitas, CHAR_PTR, "character", {offsetof(TW_Character, details)}};
}

/***
Returns the memory address of the character object as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemoryAddress (lua_State* L) { return tw_mem_address(L, "character", CHAR_PTR); }

/***
Gets the current action points of the character.
@function GetActionPoints
@treturn integer action points
*/
static int GetActionPoints  (lua_State* L) { return Props::ActionPoints.get(L); }

/***
Sets the action points of the character.
@function SetActionPoints
@tparam integer value new action points
*/
static int SetActionPoints  (lua_State* L) { return Props::ActionPoints.set(L); }

/***
Gets the political influence of the character (m_political_gravitas).
@function GetInfluence
@treturn integer influence
*/
static int GetInfluence     (lua_State* L) { return Props::Influence.get(L); }

/***
Sets the political influence of the character (m_political_gravitas).
@function SetInfluence
@tparam integer value new influence
*/
static int SetInfluence     (lua_State* L) { return Props::Influence.set(L); }

/***
Overrides the default bodyguard unit record for a general so that whenever
the general is recruited into an army (including re-recruitment after being
wounded or disbanded, or through 'Replace this general' in the UI), they
receive this unit type as their default bodyguard. The record is stored directly
in the persistent GENERAL_BODYGUARD_DETAILS struct (serialised with savegames)
and read by the recruitment panel as the pre-selected default choice.
@function SetDefaultBodyGuard
@tparam string unit_key unit record key (e.g. "att_rom_cav_general_guards")
@treturn boolean true if the record was found and applied, false otherwise
*/
static int SetDefaultBodyGuard(lua_State* L) {
    if (!g_record_index) {
        Log("[twdll] SetDefaultBodyGuard: g_record_index not resolved");
        return 0;
    }
    if (!g_campaign_model) {
        Log("[twdll] SetDefaultBodyGuard: campaign model not available");
        return 0;
    }
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        Log("[twdll] SetDefaultBodyGuard: null character");
        return 0;
    }
    size_t key_len = 0;
    const char* key = l_checklstring(L, 2, &key_len);
    if (!key || key_len == 0) {
        Log("[twdll] SetDefaultBodyGuard: unit_key not a string");
        return 0;
    }

    auto* env       = static_cast<TW_CampaignModel*>(g_campaign_model)->m_campaign_env;
    auto* game_core = static_cast<TW_CampaignEnv*>(env)->m_game_core;
    auto* databases = static_cast<TW_GameCore*>(game_core)->m_databases;
    void* units_table = static_cast<TW_Databases*>(databases)->m_main_units_table;
    if (!units_table) {
        Log("[twdll] SetDefaultBodyGuard: main_units_table not loaded");
        return 0;
    }

    struct RecordKey { uint32_t m_len; uint32_t m_pad; const char* m_data; }
        key_string = { static_cast<uint32_t>(key_len), 0, key };

    void* record = g_record_index(units_table, &key_string);
    if (!record) {
        Log("[twdll] SetDefaultBodyGuard: no record for key '%s'", key);
        l_pushboolean(L, 0);
        return 1;
    }

    ch->details.m_initial_general_bodyguard_details.m_unit = record;
    uint16_t num_men = static_cast<uint16_t>(*reinterpret_cast<uint32_t*>(reinterpret_cast<char*>(record) + 0x28));
    ch->details.m_initial_general_bodyguard_details.m_men = num_men;
    ch->details.m_initial_general_bodyguard_details.m_men_in_fully_replenished = num_men;

    Log("[twdll] SetDefaultBodyGuard: character=0x%08X record=0x%08X key='%s'",
        reinterpret_cast<uintptr_t>(ch),
        reinterpret_cast<uintptr_t>(record), key);

    l_pushboolean(L, 1);
    return 1;
}


void push_campaign_political_party(lua_State* L, twdll::TW_CampaignPoliticalParty* party);

/***
Returns the character's campaign political party, or nil if none.
@function GetPoliticalParty
@treturn userdata CAMPAIGN_POLITICAL_PARTY or nil
*/
static int GetPoliticalParty(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch || !ch->details.m_political_party) {
        l_pushnil(L);
        return 1;
    }

    l_getfield(L, 1, "faction");
    l_pushvalue(L, 1);
    if (l_pcall(L, 1, 1, 0) == 0) {
        auto* faction = twdll::tw_unwrap<twdll::TW_Faction>(L, -1);
        if (faction) {
            twdll::TW_CampaignPoliticalParty* found = nullptr;
            const auto& map = faction->m_politics.m_political_parties;
            for (int i = 0; i < map.m_size; ++i) {
                char* bucket = reinterpret_cast<char*>(map.m_elements) + i * 12;
                char* fake   = bucket + 4;
                char* node   = *reinterpret_cast<char**>(bucket);
                while (node && node != fake) {
                    auto* party = reinterpret_cast<twdll::TW_CampaignPoliticalParty*>(node + 0xC);
                    if (party->m_party_record == ch->details.m_political_party) {
                        found = party;
                        break;
                    }
                    node = *reinterpret_cast<char**>(node + 4);
                }
                if (found) break;
            }
            l_pop(L, 1);
            if (found) {
                push_campaign_political_party(L, found);
                return 1;
            }
        } else {
            l_pop(L, 1);
        }
    }

    l_pushnil(L);
    return 1;
}

/***
Sets the character's political party allegiance. Accepts either a
CAMPAIGN_POLITICAL_PARTY userdata or a party record key string.
@function SetPoliticalParty
@tparam userdata|string party CAMPAIGN_POLITICAL_PARTY object or party record key (e.g. "att_political_party_romans_1")
@treturn boolean true if successfully set, false otherwise
*/
static int SetPoliticalParty(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        Log("[twdll] SetPoliticalParty: null character");
        l_pushboolean(L, 0);
        return 1;
    }

    void* record = nullptr;
    if (l_type(L, 2) == LUA_TUSERDATA) {
        auto* party = twdll::tw_unwrap<twdll::TW_CampaignPoliticalParty>(L, 2);
        if (party && party->m_party_record) {
            record = party->m_party_record;
        }
    } else if (l_type(L, 2) == LUA_TSTRING) {
        size_t key_len = 0;
        const char* key = l_checklstring(L, 2, &key_len);
        if (key && key_len > 0) {
            l_getfield(L, 1, "faction");
            l_pushvalue(L, 1);
            if (l_pcall(L, 1, 1, 0) == 0) {
                auto* faction = twdll::tw_unwrap<twdll::TW_Faction>(L, -1);
                if (faction) {
                    const auto& map = faction->m_politics.m_political_parties;
                    for (int i = 0; i < map.m_size; ++i) {
                        char* bucket = reinterpret_cast<char*>(map.m_elements) + i * 12;
                        char* fake   = bucket + 4;
                        char* node   = *reinterpret_cast<char**>(bucket);
                        while (node && node != fake) {
                            auto* party = reinterpret_cast<twdll::TW_CampaignPoliticalParty*>(node + 0xC);
                            auto* rec = static_cast<twdll::TW_PoliticalPartyRecord*>(party->m_party_record);
                            if (rec && rec->m_key.m_data && std::strcmp(rec->m_key.m_data, key) == 0) {
                                record = rec;
                                break;
                            }
                            node = *reinterpret_cast<char**>(node + 4);
                        }
                        if (record) break;
                    }
                }
                l_pop(L, 1);
            }
            if (!record && g_record_index && g_campaign_model) {
                auto* env = static_cast<TW_CampaignModel*>(g_campaign_model)->m_campaign_env;
                auto* game_core = static_cast<TW_CampaignEnv*>(env)->m_game_core;
                auto* databases = static_cast<TW_GameCore*>(game_core)->m_databases;
                void* parties_table = *reinterpret_cast<void**>(reinterpret_cast<char*>(databases) + 0xF18);
                if (parties_table) {
                    struct RecordKey { uint32_t m_len; uint32_t m_pad; const char* m_data; }
                        key_string = { static_cast<uint32_t>(key_len), 0, key };
                    record = g_record_index(parties_table, &key_string);
                }
            }
        }
    }

    if (!record) {
        Log("[twdll] SetPoliticalParty: failed to resolve political party");
        l_pushboolean(L, 0);
        return 1;
    }

    ch->details.m_political_party = record;
    Log("[twdll] SetPoliticalParty: character 0x%08X party set to 0x%08X",
        reinterpret_cast<uintptr_t>(ch), reinterpret_cast<uintptr_t>(record));
    l_pushboolean(L, 1);
    return 1;
}

extern const luaL_Reg character_functions[] = {
    {nullptr, nullptr}
};

static const luaL_Reg character_methods[] = {
    {"GetMemoryAddress",      GetMemoryAddress},
    {"GetActionPoints",       GetActionPoints},
    {"SetActionPoints",       SetActionPoints},
    {"GetInfluence",          GetInfluence},
    {"SetInfluence",          SetInfluence},
    {"SetDefaultBodyGuard",   SetDefaultBodyGuard},
    {"GetPoliticalParty",     GetPoliticalParty},
    {"SetPoliticalParty",     SetPoliticalParty},
    {nullptr, nullptr}
};

void register_character_methods(lua_State* L) {
    l_newmetatable(L, "CHARACTER_SCRIPT_INTERFACE");
    l_getfield(L, -1, "__index");
    if (l_type(L, -1) == LUA_TTABLE) {
        for (const luaL_Reg* f = character_methods; f->name; ++f) {
            l_pushstring(L, f->name);
            l_pushcclosure(L, f->func, 0);
            l_settable(L, -3);
        }
        Log("[twdll] CHARACTER_SCRIPT_INTERFACE extended");
    } else {
        Log("[twdll] WARNING: CHARACTER_SCRIPT_INTERFACE __index not found");
    }
    l_pop(L, 2);
}