/// @module CHARACTER_SCRIPT_INTERFACE
/// Extensions to the game's character object.
#include "../common/tw.h"
#include "../common/signature_scanner.h"
#include "../common/log.h"
#include "game_api.h"
#include "tw_types.h"
#include <windows.h>
#include <string>

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
Memory address of the character object in hexadecimal format.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
@usage
local addr = char:GetMemoryAddress()
*/
static int GetMemoryAddress (lua_State* L) { return tw_mem_address(L, "character", CHAR_PTR); }

/***
Current campaign action points remaining for the character.
@function GetActionPoints
@treturn integer action points
@usage
local ap = char:GetActionPoints()
if ap < 20 then
    -- Replenish movement points if army is exhausted:
    char:SetActionPoints(100)
end
*/
static int GetActionPoints  (lua_State* L) { return Props::ActionPoints.get(L); }

/***
Sets the campaign action points for the character.
@function SetActionPoints
@tparam integer value new action points (e.g. 0 to immobilize, 100+ for full movement)
@treturn boolean true on success, false otherwise
@usage
-- Immobilize character for one turn:
char:SetActionPoints(0)

-- Restore full movement range:
char:SetActionPoints(100)
*/
static int SetActionPoints  (lua_State* L) { return Props::ActionPoints.set(L); }

/***
Political influence / gravitas of the character.
Used by the politics and senate simulation to evaluate political power and family standing.
@function GetInfluence
@treturn integer influence value
@usage
local gravitas = char:GetInfluence()
if gravitas < 15 then
    -- Character lacks gravitas to secure ministerial offices
end
*/
static int GetInfluence     (lua_State* L) { return Props::Influence.get(L); }

/***
Sets the political influence / gravitas of the character.
Persisted in savegames. Directly modifies the character's political weight in the faction senate.
@function SetInfluence
@tparam integer value new influence value
@treturn boolean true on success, false otherwise
@usage
-- Elevate character influence to qualify for high political office:
char:SetInfluence(60)
*/
static int SetInfluence     (lua_State* L) { return Props::Influence.set(L); }

/***
Overrides the default bodyguard unit record for a general so that whenever
the general is recruited into an army (including re-recruitment after being
wounded or disbanded, or through 'Replace this general' in the UI), they
receive this unit type as their default bodyguard.

Persisted natively in savegames and read by the recruitment panel as the pre-selected default choice.
@function SetDefaultBodyGuard
@tparam string unit_key unit record key from `main_units_tables` (e.g. `"att_rom_cav_general_guards"`, `"att_merc_ger_agathyrsi_warriors"`)
@treturn boolean true if the record was found and applied, false otherwise
@usage
-- Assign an elite bodyguard to the general:
local ok = char:SetDefaultBodyGuard("att_merc_ger_agathyrsi_warriors")
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

    auto* unit_rec = static_cast<const twdll::TW_MainUnitRecord*>(record);
    ch->details.m_initial_general_bodyguard_details.m_unit = record;
    uint16_t num_men = static_cast<uint16_t>(unit_rec->m_num_men);
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
Returns the character's political party (@{CAMPAIGN_POLITICAL_PARTY_SCRIPT_INTERFACE}), or nil if none.
@function GetPoliticalParty
@treturn CAMPAIGN_POLITICAL_PARTY_SCRIPT_INTERFACE|nil party object, or nil if none
@usage
local party = char:GetPoliticalParty()
if party and not party:IsPrimary() then
    -- Character belongs to an opposition house; reassign to the ruling party:
    local ruler_party = faction:GetPrimaryParty()
    char:SetPoliticalParty(ruler_party)
end
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
Sets the character's political party allegiance.

Supports two calling styles:
- **Party userdata**: `char:SetPoliticalParty(party_obj)`
- **Party record key string**: `char:SetPoliticalParty("att_politics_hunni_council")`

Directly updates the character's party pointer in memory and updates family tree representation.
@function SetPoliticalParty
@tparam CAMPAIGN_POLITICAL_PARTY_SCRIPT_INTERFACE|string party party object or party record key string
@treturn boolean true if successfully set, false otherwise
@usage
-- Option 1: Assign via party userdata:
local ruler_party = faction:GetPrimaryParty()
char:SetPoliticalParty(ruler_party)

-- Option 2: Assign via database record key:
char:SetPoliticalParty("att_politics_hunni_council")
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
Returns the @{ARTSET_SCRIPT_INTERFACE} for this character, or nil if none.
Allows inspecting portrait paths, cultural variations, gender, aging, and faction leader flags.
@function GetArtSet
@treturn ARTSET_SCRIPT_INTERFACE|nil character art set interface, or nil if none
@usage
local art_set = char:GetArtSet()
if art_set then
    local path = art_set:GetPortraitPath()
    -- Sample path: "UI/Portraits/Portholes/att_cult_nomadic/att_frontend_faction_leader_huns_0.png"
    if not art_set:IsMale() then
        -- Process female character art set
    end
end
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
Sets the art set for this character, immediately updating all 3D models (campaign map avatar, battle commander, politician panel) and 2D UI portholes/portraits.
@function SetArtSet
@tparam string art_set_key art set ID key from `campaign_character_art_sets_tables` (e.g. `"att_general_nomadic_16"`)
@treturn boolean true on success, false on failure
@usage
-- Swap general appearance and portrait:
char:SetArtSet("att_general_nomadic_16")
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
Automatically calculates and applies all associated trait effect bundles and attribute bonuses.
@function AddTrait
@tparam string trait_key the trait record key from `character_traits_tables` (e.g. `"att_trait_all_personality_brave"`)
@tparam[opt=1] integer points trait level / points to grant (default: 1)
@tparam[opt=false] boolean show_message whether to trigger the on-screen event notification message (default: false)
@treturn boolean true on success, false otherwise
@usage
-- Add 1 point of brave trait silently:
char:AddTrait("att_trait_all_personality_brave", 1, false)

-- Add trait with on-screen notification:
char:AddTrait("att_trait_all_personality_brave", 1, true)
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

    if (!g_add_trait) {
        Log("[twdll] char:AddTrait: g_add_trait not resolved");
        l_pushboolean(L, 0);
        return 1;
    }

    twdll::TW_CAString str{};
    str.m_len = static_cast<uint32_t>(strlen(trait_key));
    str.m_capacity = str.m_len;
    str.m_data = trait_key;

    g_add_trait(ch, &str, points, show_msg ? 1 : 0);
    Log("[twdll] char:AddTrait: added trait '%s' to character 0x%08X", trait_key, reinterpret_cast<uintptr_t>(ch));
    l_pushboolean(L, 1);
    return 1;
}

/***
Removes the specified trait from this character and recalculates active character bonus effects and skill attributes.
@function RemoveTrait
@tparam string trait_key the trait record key to remove
@treturn boolean true if the trait was found and removed, false otherwise
@usage
-- Remove a specific personality trait:
local ok = char:RemoveTrait("att_trait_all_personality_brave")
*/
static int RemoveTrait(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_pushboolean(L, 0);
        return 1;
    }
    const char* trait_key = l_checkstring(L, 2);
    auto* traits = &ch->details.traits;
    if (!traits->m_elements || traits->m_size == 0) {
        l_pushboolean(L, 0);
        return 1;
    }

    for (uint32_t i = 0; i < traits->m_size; ++i) {
        auto& entry = traits->m_elements[i];
        if (entry.m_record && entry.m_record->m_character_trait) {
            const char* key = entry.m_record->m_character_trait->m_key.m_data;
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

                if (g_set_effect_list) {
                    g_set_effect_list(traits);
                }

                Log("[twdll] char:RemoveTrait: removed trait '%s'", trait_key);
                l_pushboolean(L, 1);
                return 1;
            }
        }
    }

    l_pushboolean(L, 0);
    return 1;
}

/***
Returns a table array of all trait record keys currently present on this character.
@function GetTraitList
@treturn table array of trait key strings (e.g. `{"att_trait_all_personality_brave", ...}`)
@usage
-- Sample returned table:
-- {
--     [1] = "att_trait_all_personality_brave",
--     [2] = "att_trait_general_cavalry_commander",
--     [3] = "att_trait_all_political_ambitious"
-- }

local traits = char:GetTraitList()

-- Check if character has a specific trait:
local is_brave = false
for _, trait_key in ipairs(traits) do
    if trait_key == "att_trait_all_personality_brave" then
        is_brave = true
        break
    end
end
*/
static int GetTraitList(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_newtable(L);
        return 1;
    }
    auto* traits = &ch->details.traits;
    l_newtable(L);
    if (!traits->m_elements || traits->m_size == 0) {
        return 1;
    }
    int idx = 1;
    for (uint32_t i = 0; i < traits->m_size; ++i) {
        const auto& entry = traits->m_elements[i];
        if (entry.m_record && entry.m_record->m_character_trait) {
            const char* key = entry.m_record->m_character_trait->m_key.m_data;
            if (key && key[0] != '\0') {
                l_pushinteger(L, idx++);
                l_pushstring(L, key);
                l_settable(L, -3);
            }
        }
    }
    return 1;
}

/***
Calculated total loyalty level of the character (0 to 10), taking into account
ruler authority, gravitas difference, marriage, ministerial offices, traits, and direct modifiers.
@function GetLoyalty
@treturn integer total loyalty level clamped between 0 and 10
@usage
local loyalty = char:GetLoyalty()
if loyalty <= 2 then
    -- Character is dangerously disloyal and at imminent risk of civil war or rebellion
end
*/
static int GetLoyalty(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_pushinteger(L, 0);
        return 1;
    }
    if (!g_get_loyalty) {
        l_pushinteger(L, 0);
        return 1;
    }
    l_pushinteger(L, g_get_loyalty(ch));
    return 1;
}

/***
Direct loyalty modifier value applied to this character.
@function GetLoyaltyModifier
@treturn integer loyalty modifier value (between -128 and 127)
@usage
local mod = char:GetLoyaltyModifier()
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
Sets a direct loyalty modifier on the character that directly alters their overall loyalty score.

To permanently zero out loyalty or prime a character for rebellion/defection, pass a strong negative value (e.g. `-100`).
@function SetLoyaltyModifier
@tparam integer value new modifier value (-128 to 127)
@treturn boolean true on success, false otherwise
@usage
-- Prime character for civil war / defection:
char:SetLoyaltyModifier(-100)

-- Reward character with a loyalty boost (+10):
char:SetLoyaltyModifier(10)
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
Returns a key-value dictionary table breakdown of all active loyalty factors contributing to this character's loyalty.
Each key is the database identifier from `loyalty_factors_tables` mapped to its integer point value (positive for loyalty bonuses, negative for grievances and penalties).
@function GetLoyaltyFactorList
@treturn table map of `[db_factor_key] = integer_point_value` for all active non-zero factors
@usage
-- Sample returned table:
-- {
--     ["att_loyalty_factor_leader_authority"] = 2,
--     ["att_loyalty_factor_gravitas_difference"] = -3,
--     ["att_loyalty_factor_office_held"] = 1,
--     ["att_loyalty_factor_direct_modifier"] = -10
-- }

local factors = char:GetLoyaltyFactorList()

-- Check a specific grievance penalty:
local gravitas_penalty = factors["att_loyalty_factor_gravitas_difference"] or 0
if gravitas_penalty < 0 then
    -- Character resents the faction leader having lower gravitas
end

-- Sum total negative grievances:
local total_grievances = 0
for factor_key, points in pairs(factors) do
    if points < 0 then
        total_grievances = total_grievances + points
    end
end
*/
static int GetLoyaltyFactorList(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    l_newtable(L);
    if (!ch) return 1;

    if (!g_get_loyalty_factors) return 1;

    int8_t factors[32]{};
    g_get_loyalty_factors(ch, factors);

    auto* dbs = TW_Databases::get();
    auto* table = dbs ? dbs->loyalty_factors : nullptr;

    for (uint32_t i = 0; i < 32; ++i) {
        if (factors[i] != 0) {
            const char* key = nullptr;
            if (table && table->m_elements && i < table->m_size && table->m_elements[i]) {
                auto* rec = static_cast<const twdll::TW_LoyaltyFactorRecord*>(table->m_elements[i]);
                key = rec->m_key.m_data;
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

Automatic engine side-effects:
- Unassigns any active governorship or minister post held by the character.
- Ends active trade commerce raids and updates trade routes.
- Cancels pending political actions targeting this character.
- Reassigns all units, recalculates senior unit commander, and re-sorts force containers.
- Emits native events (`MILITARY_FORCE_FACTION_CHANGE`, `REPORT_CHARACTER_RENDER_DETAILS_CHANGE`, `CHARACTER_FACTION_CHANGE`).
- Reloads character voiceover culture and lines if the receiving faction is human-controlled.

@function TransferToFaction
@tparam FACTION_SCRIPT_INTERFACE target_faction target faction receiving the character and their force
@tparam[opt] table options optional table `{ replenish_units = false, rebel_region = nil }`:
- `replenish_units` (boolean): if true, restores all units in the force to 100% full soldier capacity.
- `rebel_region` (@{REGION_SCRIPT_INTERFACE}|nil): optional region interface. When provided, binds the army to this region as a provincial rebel force with rebel AI objectives and unrest reduction upon defeat.
@treturn boolean true on success, false otherwise
@usage -- Basic transfer:
char:TransferToFaction(target_faction)
@usage -- Transfer with options:
char:TransferToFaction(target_faction, {
    replenish_units = true,         -- replenish all units to 100% strength (default: false)
    rebel_region = region           -- bind as provincial rebel army in this region (default: nil)
})
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

    if (!g_get_faction_record || !g_reassign_faction) {
        Log("[twdll] char:TransferToFaction: function signatures not resolved");
        l_pushboolean(L, 0);
        return 1;
    }

    void* faction_rec = g_get_faction_record(target_faction);

    void* rebel_region = nullptr;
    int replenish = 0;

    if (l_type(L, 3) == LUA_TTABLE) {
        l_getfield(L, 3, "replenish_units");
        if (l_type(L, -1) == LUA_TNIL) {
            l_pop(L, 1);
            l_getfield(L, 3, "replenish");
        }
        if (l_type(L, -1) != LUA_TNIL && l_type(L, -1) != LUA_TNONE) {
            replenish = l_tobool(L, -1) ? 1 : 0;
        }
        l_pop(L, 1);

        l_getfield(L, 3, "rebel_region");
        if (l_type(L, -1) == LUA_TNIL) {
            l_pop(L, 1);
            l_getfield(L, 3, "region");
        }
        if (l_type(L, -1) == LUA_TUSERDATA) {
            rebel_region = twdll::tw_unwrap<TW_Region>(L, -1);
        }
        l_pop(L, 1);
    }

    __try {
        g_reassign_faction(ch, target_faction, faction_rec, rebel_region, replenish, 0, 0);
        Log("[twdll] char:TransferToFaction: character 0x%08X successfully transferred to faction 0x%08X (replenish=%d, rebel_reg=0x%08X)",
            reinterpret_cast<uintptr_t>(ch), reinterpret_cast<uintptr_t>(target_faction), replenish, reinterpret_cast<uintptr_t>(rebel_region));
        l_pushboolean(L, 1);
    } __except (EXCEPTION_EXECUTE_HANDLER) {
        Log("[twdll] char:TransferToFaction: caught SEH exception 0x%08X in engine call", GetExceptionCode());
        l_pushboolean(L, 0);
    }
    return 1;
}

// Character Name Component Types matching native CHARACTER_NAME slots:
// 0 = Forename, 1 = Family Name, 2 = Clan Name, 3 = Other Name (Nickname/Title)
enum NameType : uint32_t {
    Forename   = 0,
    FamilyName = 1,
    ClanName   = 2,
    OtherName  = 3,
};

// Finds the entry corresponding to 'type' within the character's 4 name slots,
// or initializes the slot if it has not been configured yet.
static twdll::TW_CharacterNameEntry* GetNameEntry(twdll::TW_CharacterName& name, uint32_t type) {
    for (int i = 0; i < 4; ++i) {
        if (name.m_entries[i].m_type == type) return &name.m_entries[i];
    }
    if (type < 4) {
        name.m_entries[type].m_type = type;
        return &name.m_entries[type];
    }
    return nullptr;
}

// Reads the resolved onscreen name string following the engine's rendering precedence:
// 1. In-memory custom wide string (m_custom_string)
// 2. Cached localized string pointer (m_localised_string)
// 3. Raw database localisation key fallback (m_localisation_key)
static std::string ReadNameSlot(const twdll::TW_CharacterNameEntry& entry) {
    if (entry.m_localisation.m_custom_string.m_data && entry.m_localisation.m_custom_string.m_len > 0) {
        return tw_wide_to_utf8(entry.m_localisation.m_custom_string.m_data, entry.m_localisation.m_custom_string.m_len);
    }
    if (entry.m_localisation.m_localised_string) {
        auto* unistr = entry.m_localisation.m_localised_string;
        if (unistr && unistr->m_data && unistr->m_len > 0) {
            return tw_wide_to_utf8(unistr->m_data, unistr->m_len);
        }
    }
    if (entry.m_localisation.m_localisation_key.m_data && entry.m_localisation.m_localisation_key.m_len > 0) {
        return std::string(entry.m_localisation.m_localisation_key.m_data, entry.m_localisation.m_localisation_key.m_len);
    }
    return "";
}

// Sets a custom in-memory text string (UTF-8) for a name slot.
// Clears m_localisation_key and m_localised_string so the UI renders the custom UTF-16 string
// directly across all campaign panels regardless of game language.
static bool SetNameSlot(twdll::TW_CharacterNameEntry& entry, uint32_t type, const char* text, size_t text_len) {
    entry.m_type = type;
    entry.m_localisation.m_localisation_key.m_len = 0;
    entry.m_localisation.m_localisation_key.m_data = nullptr;
    entry.m_localisation.m_localised_string = nullptr;

    if (!text || text_len == 0) {
        entry.m_localisation.m_custom_string.m_len = 0;
        entry.m_localisation.m_custom_string.m_data = nullptr;
        return true;
    }

    std::wstring ws = tw_utf8_to_wide(text, text_len);
    if (ws.empty()) return false;

    wchar_t* wbuf = new wchar_t[ws.length() + 1];
    memcpy(wbuf, ws.c_str(), (ws.length() + 1) * sizeof(wchar_t));

    entry.m_localisation.m_custom_string.m_len = static_cast<uint32_t>(ws.length());
    entry.m_localisation.m_custom_string.m_capacity = static_cast<uint32_t>(ws.length());
    entry.m_localisation.m_custom_string.m_data = wbuf;
    return true;
}

// Reads the raw database localisation key string (e.g. "names_name_12345") assigned to the slot.
static std::string ReadNameKeySlot(const twdll::TW_CharacterNameEntry& entry) {
    if (entry.m_localisation.m_localisation_key.m_data && entry.m_localisation.m_localisation_key.m_len > 0) {
        return std::string(entry.m_localisation.m_localisation_key.m_data, entry.m_localisation.m_localisation_key.m_len);
    }
    return "";
}

// Sets a database localisation key (e.g. "names_name_12345") for a name slot.
// Clears m_custom_string and m_localised_string so the engine dynamically resolves and translates
// the key from the game's names.loc database table based on the player's active language.
static bool SetNameKeySlot(twdll::TW_CharacterNameEntry& entry, uint32_t type, const char* key, size_t key_len) {
    entry.m_type = type;
    entry.m_localisation.m_custom_string.m_len = 0;
    entry.m_localisation.m_custom_string.m_data = nullptr;
    entry.m_localisation.m_localised_string = nullptr;

    if (!key || key_len == 0) {
        entry.m_localisation.m_localisation_key.m_len = 0;
        entry.m_localisation.m_localisation_key.m_data = nullptr;
        return true;
    }

    char* buf = new char[key_len + 1];
    std::memcpy(buf, key, key_len);
    buf[key_len] = '\0';

    entry.m_localisation.m_localisation_key.m_len = static_cast<uint32_t>(key_len);
    entry.m_localisation.m_localisation_key.m_capacity = static_cast<uint32_t>(key_len);
    entry.m_localisation.m_localisation_key.m_data = buf;
    return true;
}

/***
Returns the full formatted onscreen name of the character (combining non-empty forename, clan name, family name, and other name).
Concatenates active name slots in native UI display order: `Forename [ClanName] [FamilyName] [OtherName]`.
@function GetFullName
@treturn string full composite name (e.g. "Witch-king NazgulClan Angmar the Nazgul")
@usage local name = char:GetFullName()
*/
static int GetFullName(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) {
        l_pushstring(L, "");
        return 1;
    }
    static const uint32_t order[4] = {Forename, ClanName, FamilyName, OtherName};
    std::string full;
    for (int i = 0; i < 4; ++i) {
        auto* entry = GetNameEntry(ch->details.m_name, order[i]);
        if (entry) {
            std::string part = ReadNameSlot(*entry);
            if (!part.empty()) {
                if (!full.empty()) full += " ";
                full += part;
            }
        }
    }
    l_pushstring(L, full.c_str());
    return 1;
}

/***
Active onscreen forename of the character.
Returns custom text if set via @{SetForename}, or the translated string from the database if set via @{SetForenameKey}, or the database key as fallback.
@function GetForename
@treturn string forename string
@usage local fn = char:GetForename()
*/
static int GetForename(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushstring(L, ""); return 1; }
    auto* entry = GetNameEntry(ch->details.m_name, Forename);
    l_pushstring(L, entry ? ReadNameSlot(*entry).c_str() : "");
    return 1;
}

/***
Sets the forename of the character as direct custom text (UTF-8).
Clears any existing database localisation key, ensuring the custom string is displayed directly across all UI panels regardless of game language. Persisted natively across turns and save/load.
To use a localized string from `names.loc`, use @{SetForenameKey} instead.
@function SetForename
@tparam string name new custom forename text in UTF-8
@treturn boolean true on success, false otherwise
@usage char:SetForename("Witch-king")
*/
static int SetForename(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushboolean(L, 0); return 1; }
    size_t len = 0;
    const char* text = l_checklstring(L, 2, &len);
    auto* entry = GetNameEntry(ch->details.m_name, Forename);
    bool ok = entry && SetNameSlot(*entry, Forename, text, len);
    l_pushboolean(L, ok ? 1 : 0);
    return 1;
}

/***
Database localisation key for the character's forename (e.g. `"names_name_12345"`).
Returns the raw key string if assigned via database or @{SetForenameKey}, or an empty string if direct custom text was assigned via @{SetForename}.
@function GetForenameKey
@treturn string database localisation key string, or empty string if custom text is used
@usage local key = char:GetForenameKey()
*/
static int GetForenameKey(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushstring(L, ""); return 1; }
    auto* entry = GetNameEntry(ch->details.m_name, Forename);
    l_pushstring(L, entry ? ReadNameKeySlot(*entry).c_str() : "");
    return 1;
}

/***
Sets the database localisation key for the character's forename (e.g. `"names_name_12345"`).
Clears any active custom in-memory text, allowing the game engine to translate the name dynamically from localized database files (`names.loc`) based on the player's active language. Persisted natively across turns and save/load.
To assign arbitrary text without editing database files, use @{SetForename} instead.
@function SetForenameKey
@tparam string key database localisation key string
@treturn boolean true on success, false otherwise
@usage char:SetForenameKey("names_name_12345")
*/
static int SetForenameKey(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushboolean(L, 0); return 1; }
    size_t len = 0;
    const char* key = l_checklstring(L, 2, &len);
    auto* entry = GetNameEntry(ch->details.m_name, Forename);
    bool ok = entry && SetNameKeySlot(*entry, Forename, key, len);
    l_pushboolean(L, ok ? 1 : 0);
    return 1;
}

/***
Active onscreen family name (surname) of the character.
Returns custom text if set via @{SetFamilyName}, or the translated string from the database if set via @{SetFamilyNameKey}, or the database key as fallback.
@function GetFamilyName
@treturn string family name string
@usage local fam = char:GetFamilyName()
*/
static int GetFamilyName(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushstring(L, ""); return 1; }
    auto* entry = GetNameEntry(ch->details.m_name, FamilyName);
    l_pushstring(L, entry ? ReadNameSlot(*entry).c_str() : "");
    return 1;
}

/***
Sets the family name (surname) of the character as direct custom text (UTF-8).
Clears any existing database localisation key, ensuring the custom string is displayed directly across all UI panels regardless of game language. Persisted natively across turns and save/load.
To use a localized string from `names.loc`, use @{SetFamilyNameKey} instead.
@function SetFamilyName
@tparam string name new custom family name text in UTF-8
@treturn boolean true on success, false otherwise
@usage char:SetFamilyName("Angmar")
*/
static int SetFamilyName(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushboolean(L, 0); return 1; }
    size_t len = 0;
    const char* text = l_checklstring(L, 2, &len);
    auto* entry = GetNameEntry(ch->details.m_name, FamilyName);
    bool ok = entry && SetNameSlot(*entry, FamilyName, text, len);
    l_pushboolean(L, ok ? 1 : 0);
    return 1;
}

/***
Database localisation key for the character's family name (e.g. `"names_name_12345"`).
Returns the raw key string if assigned via database or @{SetFamilyNameKey}, or an empty string if direct custom text was assigned via @{SetFamilyName}.
@function GetFamilyNameKey
@treturn string database localisation key string, or empty string if custom text is used
@usage local key = char:GetFamilyNameKey()
*/
static int GetFamilyNameKey(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushstring(L, ""); return 1; }
    auto* entry = GetNameEntry(ch->details.m_name, FamilyName);
    l_pushstring(L, entry ? ReadNameKeySlot(*entry).c_str() : "");
    return 1;
}

/***
Sets the database localisation key for the character's family name (e.g. `"names_name_12345"`).
Clears any active custom in-memory text, allowing the game engine to translate the name dynamically from localized database files (`names.loc`) based on the player's active language. Persisted natively across turns and save/load.
To assign arbitrary text without editing database files, use @{SetFamilyName} instead.
@function SetFamilyNameKey
@tparam string key database localisation key string
@treturn boolean true on success, false otherwise
@usage char:SetFamilyNameKey("names_name_12345")
*/
static int SetFamilyNameKey(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushboolean(L, 0); return 1; }
    size_t len = 0;
    const char* key = l_checklstring(L, 2, &len);
    auto* entry = GetNameEntry(ch->details.m_name, FamilyName);
    bool ok = entry && SetNameKeySlot(*entry, FamilyName, key, len);
    l_pushboolean(L, ok ? 1 : 0);
    return 1;
}

/***
Active onscreen clan name of the character.
Returns custom text if set via @{SetClanName}, or the translated string from the database if set via @{SetClanNameKey}, or the database key as fallback.
@function GetClanName
@treturn string clan name string
@usage local clan = char:GetClanName()
*/
static int GetClanName(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushstring(L, ""); return 1; }
    auto* entry = GetNameEntry(ch->details.m_name, ClanName);
    l_pushstring(L, entry ? ReadNameSlot(*entry).c_str() : "");
    return 1;
}

/***
Sets the clan name of the character as direct custom text (UTF-8).
Clears any existing database localisation key, ensuring the custom string is displayed directly across all UI panels regardless of game language. Persisted natively across turns and save/load.
To use a localized string from `names.loc`, use @{SetClanNameKey} instead.
@function SetClanName
@tparam string name new custom clan name text in UTF-8
@treturn boolean true on success, false otherwise
@usage char:SetClanName("NazgulClan")
*/
static int SetClanName(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushboolean(L, 0); return 1; }
    size_t len = 0;
    const char* text = l_checklstring(L, 2, &len);
    auto* entry = GetNameEntry(ch->details.m_name, ClanName);
    bool ok = entry && SetNameSlot(*entry, ClanName, text, len);
    l_pushboolean(L, ok ? 1 : 0);
    return 1;
}

/***
Database localisation key for the character's clan name (e.g. `"names_name_12345"`).
Returns the raw key string if assigned via database or @{SetClanNameKey}, or an empty string if direct custom text was assigned via @{SetClanName}.
@function GetClanNameKey
@treturn string database localisation key string, or empty string if custom text is used
@usage local key = char:GetClanNameKey()
*/
static int GetClanNameKey(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushstring(L, ""); return 1; }
    auto* entry = GetNameEntry(ch->details.m_name, ClanName);
    l_pushstring(L, entry ? ReadNameKeySlot(*entry).c_str() : "");
    return 1;
}

/***
Sets the database localisation key for the character's clan name (e.g. `"names_name_12345"`).
Clears any active custom in-memory text, allowing the game engine to translate the name dynamically from localized database files (`names.loc`) based on the player's active language. Persisted natively across turns and save/load.
To assign arbitrary text without editing database files, use @{SetClanName} instead.
@function SetClanNameKey
@tparam string key database localisation key string
@treturn boolean true on success, false otherwise
@usage char:SetClanNameKey("names_name_12345")
*/
static int SetClanNameKey(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushboolean(L, 0); return 1; }
    size_t len = 0;
    const char* key = l_checklstring(L, 2, &len);
    auto* entry = GetNameEntry(ch->details.m_name, ClanName);
    bool ok = entry && SetNameKeySlot(*entry, ClanName, key, len);
    l_pushboolean(L, ok ? 1 : 0);
    return 1;
}

/***
Active onscreen other name (title / nickname) of the character.
Returns custom text if set via @{SetOtherName}, or the translated string from the database if set via @{SetOtherNameKey}, or the database key as fallback.
@function GetOtherName
@treturn string other name string
@usage local on = char:GetOtherName()
*/
static int GetOtherName(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushstring(L, ""); return 1; }
    auto* entry = GetNameEntry(ch->details.m_name, OtherName);
    l_pushstring(L, entry ? ReadNameSlot(*entry).c_str() : "");
    return 1;
}

/***
Sets the other name (title / nickname) of the character as direct custom text (UTF-8).
Clears any existing database localisation key, ensuring the custom string is displayed directly across all UI panels regardless of game language. Persisted natively across turns and save/load.
To use a localized string from `names_titles_tables` / `names.loc`, use @{SetOtherNameKey} instead.
@function SetOtherName
@tparam string name new other name custom text (e.g. "the Nazgul")
@treturn boolean true on success, false otherwise
@usage char:SetOtherName("the Nazgul")
*/
static int SetOtherName(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushboolean(L, 0); return 1; }
    size_t len = 0;
    const char* text = l_checklstring(L, 2, &len);
    auto* entry = GetNameEntry(ch->details.m_name, OtherName);
    bool ok = entry && SetNameSlot(*entry, OtherName, text, len);
    l_pushboolean(L, ok ? 1 : 0);
    return 1;
}

/***
Database localisation key for the character's other name/title (e.g. `"names_titles_the_great"`).
Returns the raw key string if assigned via database or @{SetOtherNameKey}, or an empty string if direct custom text was assigned via @{SetOtherName}.
@function GetOtherNameKey
@treturn string database localisation key string, or empty string if custom text is used
@usage local key = char:GetOtherNameKey()
*/
static int GetOtherNameKey(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushstring(L, ""); return 1; }
    auto* entry = GetNameEntry(ch->details.m_name, OtherName);
    l_pushstring(L, entry ? ReadNameKeySlot(*entry).c_str() : "");
    return 1;
}

/***
Sets the database localisation key for the character's other name/title (e.g. `"names_titles_the_great"`).
Clears any active custom in-memory text, allowing the game engine to translate the title dynamically from localized database files (`names.loc`) based on the player's active language. Persisted natively across turns and save/load.
To assign arbitrary text without editing database files, use @{SetOtherName} instead.
@function SetOtherNameKey
@tparam string key database localisation key string
@treturn boolean true on success, false otherwise
@usage char:SetOtherNameKey("names_titles_the_great")
*/
static int SetOtherNameKey(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushboolean(L, 0); return 1; }
    size_t len = 0;
    const char* key = l_checklstring(L, 2, &len);
    auto* entry = GetNameEntry(ch->details.m_name, OtherName);
    bool ok = entry && SetNameKeySlot(*entry, OtherName, key, len);
    l_pushboolean(L, ok ? 1 : 0);
    return 1;
}

/***
Checks whether the character is flagged as immortal (will be wounded instead of dying).
@function IsImmortal
@treturn boolean true if immortal, false otherwise
@usage local immortal = char:IsImmortal()
*/
static int IsImmortal(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushboolean(L, 0); return 1; }
    l_pushboolean(L, ch->details.m_is_immortal ? 1 : 0);
    return 1;
}

/***
Sets the immortality flag of the character.
@function SetImmortal
@tparam boolean immortal true to make immortal, false to make mortal
@treturn boolean true on success, false otherwise
@usage char:SetImmortal(true)
*/
static int SetImmortal(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushboolean(L, 0); return 1; }
    ch->details.m_is_immortal = l_tobool(L, 2);
    l_pushboolean(L, 1);
    return 1;
}

/***
Turns remaining until resurrection for a wounded immortal character.
@function GetResurrectionTurns
@treturn integer turns to resurrection
@usage local turns = char:GetResurrectionTurns()
*/
static int GetResurrectionTurns(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushinteger(L, 0); return 1; }
    l_pushinteger(L, static_cast<int>(ch->details.m_turns_to_resurrection));
    return 1;
}

/***
Sets the turns remaining until resurrection for a wounded immortal character.
@function SetResurrectionTurns
@tparam integer turns resurrection countdown turns (0 for healthy/ready)
@treturn boolean true on success, false otherwise
@usage char:SetResurrectionTurns(3)
*/
static int SetResurrectionTurns(lua_State* L) {
    auto* ch = twdll::tw_unwrap<TW_Character>(L, 1);
    if (!ch) { l_pushboolean(L, 0); return 1; }
    int turns = static_cast<int>(l_tointeger(L, 2));
    if (turns < 0) turns = 0;
    ch->details.m_turns_to_resurrection = static_cast<uint32_t>(turns);
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
    {"GetFullName",           GetFullName},
    {"GetForename",           GetForename},
    {"SetForename",           SetForename},
    {"GetForenameKey",        GetForenameKey},
    {"SetForenameKey",        SetForenameKey},
    {"GetFamilyName",         GetFamilyName},
    {"SetFamilyName",         SetFamilyName},
    {"GetFamilyNameKey",      GetFamilyNameKey},
    {"SetFamilyNameKey",      SetFamilyNameKey},
    {"GetClanName",           GetClanName},
    {"SetClanName",           SetClanName},
    {"GetClanNameKey",        GetClanNameKey},
    {"SetClanNameKey",        SetClanNameKey},
    {"GetOtherName",          GetOtherName},
    {"SetOtherName",          SetOtherName},
    {"GetOtherNameKey",       GetOtherNameKey},
    {"SetOtherNameKey",       SetOtherNameKey},
    {"IsImmortal",            IsImmortal},
    {"SetImmortal",           SetImmortal},
    {"GetResurrectionTurns",  GetResurrectionTurns},
    {"SetResurrectionTurns",  SetResurrectionTurns},
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