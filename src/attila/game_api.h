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

// CHARACTER_DETAILS::portrait_path (sub_107DC9E0)
// (this=CHARACTER_DETAILS*, out_str=void*, campaign_model=void*) -> out_str
using FnResolvePortraitPath = void*(__thiscall*)(void* details, void* out_str, void* campaign_model);
extern FnResolvePortraitPath g_resolve_portrait_path;

// CA::String::operator=(const char* src) (sub_100DBD40)
using FnCaStringAssign = void*(__thiscall*)(void* str, const char* src);
extern FnCaStringAssign g_ca_string_assign;

// CHARACTER_DETAILS_ART_SET_INFO::update_art_set (sub_107F83D0)
// (this=CHARACTER_DETAILS_ART_SET_INFO*, details=CHARACTER_DETAILS*, campaign_model=CAMPAIGN_MODEL*)
using FnUpdateArtSet = int(__thiscall*)(void* art_info, void* details, void* campaign_model);
extern FnUpdateArtSet g_update_art_set;

// ── Technology research (instantly_research_technology) ──────────────────────
// 32-bit analogs of the 64-bit FACTION_TECHNOLOGY_MANAGER instant-research
// path (research/ structures_faction_substructs.md + plan_6_features_twdll.md):
//   sub_10B9D3D0 = instant_set_researched_without_updating_effects wrapper:
//     (this=manager, record, report_to_ui) -> lookup_tech_for_record ->
//     sub_10B9D400 (set_tech_as_researched + event + parent recursion) ->
//     update_effect_list + update_availabilities. This is exactly the game's
//     native per-tech instant completion, so it fires events, achievements,
//     unit upgrades and refreshes effects — no manual field writes.
using FnInstantSetResearched = void(__thiscall*)(void* manager, void* record, bool report_to_ui);
extern FnInstantSetResearched g_instant_set_researched;

// sub_10192660 = DATABASE_TABLE::record_index(table, &CA::String key) -> record.
using FnRecordIndex = void*(__thiscall*)(void* table, void* key_string);
extern FnRecordIndex g_record_index;

// UNIT::convert_unit (sub_106FC010): replace old_unit with a fresh UNIT built
// from target_record inside the same force, preserving men/xp/stats, then
// destroy old_unit. Returns the new UNIT*.
using FnConvertUnit = void*(__cdecl*)(void* old_unit, void* force, void* target_record);
extern FnConvertUnit g_convert_unit;

namespace twdll { struct TW_CAString; }

// CHARACTER::add_trait (sub_10797900)
using FnAddTrait = int(__thiscall*)(void* ch, const twdll::TW_CAString* trait_str, int points, int show_msg);
extern FnAddTrait g_add_trait;

// CHARACTER_TRAITS::set_effect_list (sub_10728750)
using FnSetEffectList = void(__thiscall*)(void* traits);
extern FnSetEffectList g_set_effect_list;

// CHARACTER::get_loyalty (sub_107C5920)
using FnGetLoyalty = int(__thiscall*)(void* ch);
extern FnGetLoyalty g_get_loyalty;

// CHARACTER::get_loyalty_factors (sub_107C5A00)
using FnGetLoyaltyFactors = void*(__thiscall*)(void* ch, void* factors_buf);
extern FnGetLoyaltyFactors g_get_loyalty_factors;

// FACTION::get_faction_record (sub_106FF720)
using FnGetFactionRecord = void*(__thiscall*)(void* faction);
extern FnGetFactionRecord g_get_faction_record;

// CHARACTER::reassign_faction (sub_107E6400)
using FnReassignFaction = void(__thiscall*)(void* ch, void* target_fac, void* fac_rec, void* rebel_region, int replenish, int bribed, int kill_faction_if_leader);
extern FnReassignFaction g_reassign_faction;

// CHARACTER_RECRUITMENT_POOL::spawn_agent (sub_107F2CF0)
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
extern const uintptr_t OFFSET_MAX_TRAITS;

void refresh_settlements_display();
