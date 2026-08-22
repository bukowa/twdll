#pragma once
#include <cstdint>
#include <windows.h>

extern HMODULE g_empire_module;

using FnNewFactionLeader = void(__thiscall*)(void* faction, void* new_leader, void* old_char, bool heir_coming_of_age);
extern FnNewFactionLeader g_new_faction_leader;

using FnDisbandUnits = void(__cdecl*)(void* units_vector, void* campaign_model);
extern FnDisbandUnits g_disband_units;

// REGION::faction_province_manager(this=REGION*) -> FACTION_PROVINCE_MANAGER*
// Resolved once by initialize_game_api() via FACTION_PROVINCE_MANAGER_SIG.
using FnFactionProvinceManager = void*(__thiscall*)(void* region);
extern FnFactionProvinceManager g_faction_province_manager;

extern void* g_campaign_model;

// CHARACTER_DETAILS::portrait_path
using FnResolvePortraitPath = void*(__thiscall*)(void* details, void* out_str, void* campaign_model);
extern FnResolvePortraitPath g_resolve_portrait_path;

// CA::String::operator=(const char* src)
using FnCaStringAssign = void*(__thiscall*)(void* str, const char* src);
extern FnCaStringAssign g_ca_string_assign;

// CA::UniString::operator=(const wchar_t* src)
using FnCaUniStringAssign = void*(__thiscall*)(void* unistr, const wchar_t* src);
extern FnCaUniStringAssign g_ca_unistring_assign;

// CHARACTER_DETAILS_ART_SET_INFO::update_art_set
using FnUpdateArtSet = int(__thiscall*)(void* art_info, void* details, void* campaign_model);
extern FnUpdateArtSet g_update_art_set;

// ── Technology research ─────────────────────────────────────────────────────
using FnInstantSetResearched = void(__thiscall*)(void* manager, void* record, bool report_to_ui);
extern FnInstantSetResearched g_instant_set_researched;

namespace twdll { struct TW_CampaignTechnology; }

using FnLookupCampaignTech = twdll::TW_CampaignTechnology*(__thiscall*)(void* manager, void* record);
extern FnLookupCampaignTech g_lookup_campaign_tech;

using FnUpdateTechEffects = void(__thiscall*)(void* manager);
extern FnUpdateTechEffects g_update_tech_effects;

// DATABASE_TABLE::record_index
using FnRecordIndex = void*(__thiscall*)(void* table, void* key_string);
extern FnRecordIndex g_record_index;

// UNIT::convert_unit
using FnConvertUnit = void*(__cdecl*)(void* old_unit, void* force, void* target_record);
extern FnConvertUnit g_convert_unit;

namespace twdll { struct TW_CAString; struct TW_CAUniString; struct TW_CampaignEnv; }

// CAMPAIGN_ENV::save_game
using FnSaveGame = bool(__thiscall*)(twdll::TW_CampaignEnv* env, const twdll::TW_CAUniString* name, void* startpos_info, char save_to_cloud, char save_to_cloud_and_disk);
extern FnSaveGame g_save_game;

// CAMPAIGN_ENV::load_game
using FnLoadGame = int(__thiscall*)(twdll::TW_CampaignEnv* env, const twdll::TW_CAUniString* path, char load_from_cloud);
extern FnLoadGame g_load_game;

// CHARACTER::add_trait
using FnAddTrait = int(__thiscall*)(void* ch, const twdll::TW_CAString* trait_str, int points, int show_msg);
extern FnAddTrait g_add_trait;

// CHARACTER_TRAITS::set_effect_list
using FnSetEffectList = void(__thiscall*)(void* traits);
extern FnSetEffectList g_set_effect_list;

// CHARACTER::get_loyalty
using FnGetLoyalty = int(__thiscall*)(void* ch);
extern FnGetLoyalty g_get_loyalty;

// CHARACTER::get_loyalty_factors
using FnGetLoyaltyFactors = void*(__thiscall*)(void* ch, void* factors_buf);
extern FnGetLoyaltyFactors g_get_loyalty_factors;

// FACTION::get_faction_record
using FnGetFactionRecord = void*(__thiscall*)(void* faction);
extern FnGetFactionRecord g_get_faction_record;

// CHARACTER::reassign_faction
using FnReassignFaction = void(__thiscall*)(void* ch, void* target_fac, void* fac_rec, void* rebel_region, int replenish, int bribed, int kill_faction_if_leader);
extern FnReassignFaction g_reassign_faction;

// CHARACTER_RECRUITMENT_POOL::spawn_agent
using FnSpawnAgent = void(__thiscall*)(
    void* recruitment_pool_mgr,
    void* agent_record,
    uint32_t* optional_position,
    void* optional_settlement,
    void* optional_military_force,
    void* script_id_ca_string,
    unsigned int character_type
);
extern FnSpawnAgent g_spawn_agent;

// Address of the `mov eax, [eax+0x13C]` instruction in the REINFORCEMENTS_MANAGER
// ctor, resolved by initialize_game_api().
extern uintptr_t g_reinf_cap_insn_addr;

// EMPIREBATTLE::MANAGER ctor/dtor entry points, resolved by initialize_game_api().
extern uintptr_t g_battle_ctor_addr;
extern uintptr_t g_battle_dtor_addr;

// CampaignSettlementCallback::Initialize entry point, resolved by initialize_game_api().
extern uintptr_t g_settlement_cb_initialize_addr;

// CAI make_occupation_decision entry point, resolved by initialize_game_api().
extern uintptr_t g_make_occupation_decision_addr;

extern const uintptr_t OFFSET_MAX_UNITS_ARMY;
extern const uintptr_t OFFSET_MAX_UNITS_NAVY;

namespace twdll {
    struct TW_TweakerMap;
    struct TW_ITweaker;
}
extern twdll::TW_TweakerMap* g_tweaker_map;
twdll::TW_ITweaker* find_engine_tweaker(const char* name, size_t len);

void refresh_settlements_display();
