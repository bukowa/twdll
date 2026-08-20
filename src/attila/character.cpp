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
using twdll::TW_Faction;
using twdll::TW_Region;

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

    auto* dbs = TW_Databases::get();
    if (!dbs || !dbs->main_units) {
        Log("[twdll] SetDefaultBodyGuard: main_units table not loaded");
        return 0;
    }

    void* record = dbs->main_units->find_record(key, key_len);
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
            auto* found = faction->m_politics.m_political_parties.find_by_record(ch->details.m_political_party);
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
                    auto* party = faction->m_politics.m_political_parties.find_by_key(key);
                    if (party) record = party->m_party_record;
                }
                l_pop(L, 1);
            }
            if (!record) {
                auto* dbs = TW_Databases::get();
                if (dbs && dbs->political_parties) {
                    record = dbs->political_parties->find_record(key, key_len);
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

void push_art_set(lua_State* L, twdll::TW_CharacterDetailsArtSetInfo* art_info);
bool SetCharacterArtSet(twdll::TW_CharacterDetailsArtSetInfo* info, const char* art_set_key);

/***
Returns the ARTSET_SCRIPT_INTERFACE for this character, or nil if none.
@function GetArtSet
@treturn userdata ARTSET_SCRIPT_INTERFACE or nil
*/
static int GetArtSet(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_pushnil(L);
        return 1;
    }
    push_art_set(L, &ch->details.m_art_set_info);
    return 1;
}

/***
Sets the art set for this character, updating all 3D models (campaign, battle, politician) and 2D portraits.
@function SetArtSet
@tparam string art_set_key art set ID key (e.g. "att_huns_general_01")
@treturn boolean true on success, false on failure
*/
static int SetArtSet(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_pushboolean(L, 0);
        return 1;
    }
    const char* key = l_checkstring(L, 2);
    bool ok = SetCharacterArtSet(&ch->details.m_art_set_info, key);
    l_pushboolean(L, ok ? 1 : 0);
    return 1;
}

/***
Adds the specified character trait to this character using the engine's native command queue.
@function AddTrait
@tparam string trait_key the trait record key (e.g. "att_trait_all_personality_brave")
@tparam[opt=false] boolean show_message whether to show the on-screen event message
@treturn boolean true on success, false otherwise
*/
static int AddTrait(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_pushboolean(L, 0);
        return 1;
    }
    const char* trait_key = l_checkstring(L, 2);
    int points = 1;
    bool show_msg = false;
    if (l_type(L, 3) == LUA_TNUMBER) {
        points = static_cast<int>(l_tointeger(L, 3));
        show_msg = (l_type(L, 4) == LUA_TBOOLEAN);
    } else if (l_type(L, 3) == LUA_TBOOLEAN) {
        show_msg = true;
    }

    HMODULE hMod = GetModuleHandleA("empire.retail.dll");
    if (!hMod) {
        l_pushboolean(L, 0);
        return 1;
    }

    twdll::TW_CAString str{};
    str.m_len = static_cast<uint32_t>(strlen(trait_key));
    str.m_pad = str.m_len;
    str.m_data = trait_key;

    using FnAddTrait = int(__thiscall*)(void* ch, const twdll::TW_CAString* trait_str, int points, int show_msg);
    auto fnAddTrait = reinterpret_cast<FnAddTrait>(
        reinterpret_cast<uintptr_t>(hMod) + 0x00797900);

    fnAddTrait(ch, &str, points, show_msg ? 1 : 0);
    Log("[twdll] char:AddTrait: added trait '%s' to character 0x%08X", trait_key, reinterpret_cast<uintptr_t>(ch));
    l_pushboolean(L, 1);
    return 1;
}

/***
Removes the specified trait from this character and recalculates active character bonus effects.
@function RemoveTrait
@tparam string trait_key the trait record key to remove
@treturn boolean true if the trait was found and removed, false otherwise
*/
static int RemoveTrait(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_pushboolean(L, 0);
        return 1;
    }
    const char* trait_key = l_checkstring(L, 2);
    auto* traits = &ch->details.traits;
    if (!traits || !traits->m_elements || traits->m_size == 0) {
        l_pushboolean(L, 0);
        return 1;
    }

    for (uint32_t i = 0; i < traits->m_size; ++i) {
        auto& entry = traits->m_elements[i];
        if (entry.m_record) {
            auto* char_trait_rec = *reinterpret_cast<const char* const*>(entry.m_record);
            if (char_trait_rec) {
                const char* key = *reinterpret_cast<const char* const*>(char_trait_rec + 0x8);
                if (key && strcmp(key, trait_key) == 0) {
                    if (entry.m_level_record && traits->_vptr) {
                        using FnOnRemoveLevel = void(__thiscall*)(void*, void*);
                        auto* vtbl = static_cast<FnOnRemoveLevel*>(traits->_vptr);
                        if (vtbl) {
                            vtbl[0](traits, entry.m_level_record);
                        }
                    }
                    for (uint32_t j = i; j + 1 < traits->m_size; ++j) {
                        traits->m_elements[j] = traits->m_elements[j + 1];
                    }
                    traits->m_size--;

                    HMODULE hMod = GetModuleHandleA("empire.retail.dll");
                    if (hMod) {
                        using FnSetEffectList = void(__thiscall*)(void*);
                        auto fnSetEffectList = reinterpret_cast<FnSetEffectList>(
                            reinterpret_cast<uintptr_t>(hMod) + 0x00728750);
                        fnSetEffectList(traits);
                    }

                    Log("[twdll] char:RemoveTrait: removed trait '%s'", trait_key);
                    l_pushboolean(L, 1);
                    return 1;
                }
            }
        }
    }

    l_pushboolean(L, 0);
    return 1;
}

/***
Returns a list of all trait keys currently present on this character.
@function GetTraitList
@treturn table array of trait key strings (e.g. {"att_trait_all_personality_brave", ...})
*/
static int GetTraitList(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_newtable(L);
        return 1;
    }
    auto* traits = &ch->details.traits;
    l_newtable(L);
    if (!traits || !traits->m_elements || traits->m_size == 0) {
        return 1;
    }
    int idx = 1;
    for (uint32_t i = 0; i < traits->m_size; ++i) {
        const auto& entry = traits->m_elements[i];
        if (entry.m_record) {
            auto* char_trait_rec = *reinterpret_cast<const char* const*>(entry.m_record);
            if (char_trait_rec) {
                const char* key = *reinterpret_cast<const char* const*>(char_trait_rec + 0x8);
                if (key && key[0] != '\0') {
                    l_pushinteger(L, idx++);
                    l_pushstring(L, key);
                    l_settable(L, -3);
                }
            }
        }
    }
    return 1;
}

/***
Gets the current calculated total loyalty of the character (0 to 10), taking into account
ruler authority, gravitas difference, marriage, ministerial offices, traits, and direct modifiers.
@function GetLoyalty
@treturn integer total loyalty level clamped between 0 and 10
*/
static int GetLoyalty(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_pushinteger(L, 0);
        return 1;
    }
    HMODULE hMod = GetModuleHandleA("empire.retail.dll");
    if (!hMod) {
        l_pushinteger(L, 0);
        return 1;
    }
    using FnGetLoyalty = int(__thiscall*)(void*);
    auto fnGetLoyalty = reinterpret_cast<FnGetLoyalty>(
        reinterpret_cast<uintptr_t>(hMod) + 0x007C5920);
    l_pushinteger(L, fnGetLoyalty(ch));
    return 1;
}

/***
Gets the direct loyalty modifier value (Factor 26 at offset +0x4C8) for this character.
@function GetLoyaltyModifier
@treturn integer loyalty modifier value
*/
static int GetLoyaltyModifier(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_pushinteger(L, 0);
        return 1;
    }
    l_pushinteger(L, static_cast<int>(ch->details.m_loyalty_modifier));
    return 1;
}

/***
Sets the direct loyalty modifier value (Factor 26 at offset +0x4C8) for this character.
This value is directly added to the character's 32 loyalty factor sum during loyalty evaluation.
To permanently zero out or prime a character for rebellion/defection, call:
  char:SetLoyaltyModifier(-100) or char:SetLoyaltyModifier(-char:GetLoyalty())
@function SetLoyaltyModifier
@tparam integer value new modifier value (-128 to 127)
@treturn boolean true on success, false otherwise
*/
static int SetLoyaltyModifier(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_pushboolean(L, 0);
        return 1;
    }
    int val = static_cast<int>(l_tointeger(L, 2));
    if (val < -128) val = -128;
    if (val > 127) val = 127;
    ch->details.m_loyalty_modifier = static_cast<int8_t>(val);

    Log("[twdll] char:SetLoyaltyModifier: character 0x%08X modifier set to %d",
        reinterpret_cast<uintptr_t>(ch), val);
    l_pushboolean(L, 1);
    return 1;
}

/***
Returns a table breakdown of all active loyalty factors contributing to this character's loyalty.
Each key is the genuine database key from loyalty_factors_tables and its integer point value.
@function GetLoyaltyFactorList
@treturn table map of db_factor_key -> integer point value for all non-zero factors
*/
static int GetLoyaltyFactorList(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    l_newtable(L);
    if (!ch) return 1;

    HMODULE hMod = GetModuleHandleA("empire.retail.dll");
    if (!hMod) return 1;

    int8_t factors[32]{};
    using FnGetFactors = void*(__thiscall*)(void*, void*);
    auto fnGetFactors = reinterpret_cast<FnGetFactors>(
        reinterpret_cast<uintptr_t>(hMod) + 0x007C5A00);

    fnGetFactors(ch, factors);

    auto* dbs = TW_Databases::get();
    void** factor_records = nullptr;
    uint32_t factor_count = 0;
    if (dbs && dbs->loyalty_factors) {
        factor_records = *reinterpret_cast<void***>(reinterpret_cast<uintptr_t>(dbs->loyalty_factors) + 0x0C);
        factor_count = *reinterpret_cast<uint32_t*>(reinterpret_cast<uintptr_t>(dbs->loyalty_factors) + 0x08);
    }

    for (int i = 0; i < 32; ++i) {
        if (factors[i] != 0) {
            const char* key = nullptr;
            if (factor_records && static_cast<uint32_t>(i) < factor_count && factor_records[i]) {
                key = *reinterpret_cast<const char* const*>(reinterpret_cast<uintptr_t>(factor_records[i]) + 0x08);
            }
            if (key && key[0] != '\0') {
                l_pushstring(L, key);
            } else {
                char fallback[32];
                snprintf(fallback, sizeof(fallback), "factor_%d", i);
                l_pushstring(L, fallback);
            }
            l_pushinteger(L, static_cast<int>(factors[i]));
            l_settable(L, -3);
        }
    }
    return 1;
}

/***
Instantly transfers the character and their commanded military force to another faction in the current tick.
@function TransferToFaction
@tparam userdata target_faction target FACTION_SCRIPT_INTERFACE
@tparam[opt] userdata|boolean region_or_replenish REGION_SCRIPT_INTERFACE or boolean replenish_units if omitting region
@tparam[opt=false] boolean replenish_units whether to instantly replenish all units in the force to 100% (defaults to false)
@tparam[opt=false] boolean bribed whether the character was bribed (defaults to false)
@treturn boolean true on success, false otherwise
*/
static int TransferToFaction(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_pushboolean(L, 0);
        return 1;
    }

    TW_Faction* target_faction = nullptr;
    if (l_type(L, 2) == LUA_TUSERDATA) {
        target_faction = twdll::tw_unwrap<TW_Faction>(L, 2);
    }

    if (!target_faction) {
        Log("[twdll] char:TransferToFaction: target faction could not be resolved");
        l_pushboolean(L, 0);
        return 1;
    }

    HMODULE hMod = GetModuleHandleA("empire.retail.dll");
    if (!hMod) {
        l_pushboolean(L, 0);
        return 1;
    }

    using FnGetFactionRecord = void*(__thiscall*)(void* faction);
    auto fnGetFactionRecord = reinterpret_cast<FnGetFactionRecord>(
        reinterpret_cast<uintptr_t>(hMod) + 0x006FF720);

    void* faction_rec = fnGetFactionRecord(target_faction);

    void* rebel_region = nullptr;
    int replenish = 0;
    int bribed = 0;

    auto get_bool = [](lua_State* state, int idx) -> int {
        if (l_type(state, idx) == LUA_TBOOLEAN) {
            auto* pState = reinterpret_cast<uintptr_t*>(state);
            auto* pVal = reinterpret_cast<uint32_t*>(pState[3] + 8 * (idx - 1));
            return (pVal[0] != 0) ? 1 : 0;
        }
        return 0;
    };

    if (l_type(L, 3) == LUA_TBOOLEAN) {
        replenish = get_bool(L, 3);
        if (l_type(L, 4) == LUA_TBOOLEAN) {
            bribed = get_bool(L, 4);
        }
    } else if (l_type(L, 3) == LUA_TUSERDATA) {
        rebel_region = twdll::tw_unwrap<TW_Region>(L, 3);
        if (l_type(L, 4) == LUA_TBOOLEAN) {
            replenish = get_bool(L, 4);
        }
        if (l_type(L, 5) == LUA_TBOOLEAN) {
            bribed = get_bool(L, 5);
        }
    }

    using FnReassignFaction = void(__thiscall*)(void* ch, void* target_fac, void* fac_rec, void* rebel_region, int replenish, int bribed, int flags);
    auto fnReassignFaction = reinterpret_cast<FnReassignFaction>(
        reinterpret_cast<uintptr_t>(hMod) + 0x007E6400);

    __try {
        fnReassignFaction(ch, target_faction, faction_rec, rebel_region, replenish, bribed, 0);
        Log("[twdll] char:TransferToFaction: character 0x%08X successfully transferred to faction 0x%08X (replenish=%d, bribed=%d)",
            reinterpret_cast<uintptr_t>(ch), reinterpret_cast<uintptr_t>(target_faction), replenish, bribed);
        l_pushboolean(L, 1);
    } __except (EXCEPTION_EXECUTE_HANDLER) {
        Log("[twdll] char:TransferToFaction: caught SEH exception 0x%08X in engine call", GetExceptionCode());
        l_pushboolean(L, 0);
    }
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
    {"GetArtSet",             GetArtSet},
    {"SetArtSet",             SetArtSet},
    {"AddTrait",              AddTrait},
    {"RemoveTrait",           RemoveTrait},
    {"GetTraitList",          GetTraitList},
    {"GetLoyalty",            GetLoyalty},
    {"GetLoyaltyModifier",    GetLoyaltyModifier},
    {"SetLoyaltyModifier",    SetLoyaltyModifier},
    {"GetLoyaltyFactorList",  GetLoyaltyFactorList},
    {"TransferToFaction",     TransferToFaction},
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