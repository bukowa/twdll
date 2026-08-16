#pragma once
#include <cstdint>

using FnNewFactionLeader = void(__thiscall*)(void* faction, void* new_leader, void* old_char, bool heir_coming_of_age);
extern FnNewFactionLeader g_new_faction_leader;

using FnDisbandUnits = void(__cdecl*)(void* units_vector, void* campaign_model);
extern FnDisbandUnits g_disband_units;

// REGION::faction_province_manager(this=REGION*) -> FACTION_PROVINCE_MANAGER*
// Resolved once by initialize_game_api() via FACTION_PROVINCE_MANAGER_SIG.
using FnFactionProvinceManager = void*(__thiscall*)(void* region);
extern FnFactionProvinceManager g_faction_province_manager;

extern void* g_campaign_model;

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

// Address of the `mov eax, [eax+0x13C]` instruction in the REINFORCEMENTS_MANAGER
// ctor, resolved by initialize_game_api().
extern uintptr_t g_reinf_cap_insn_addr;

// EMPIREBATTLE::MANAGER ctor/dtor entry points, resolved by initialize_game_api().
extern uintptr_t g_battle_ctor_addr;
extern uintptr_t g_battle_dtor_addr;

// CampaignSettlementCallback::Initialize entry point, resolved by initialize_game_api().
extern uintptr_t g_settlement_cb_initialize_addr;

extern const uintptr_t OFFSET_MAX_UNITS_ARMY;
extern const uintptr_t OFFSET_MAX_UNITS_NAVY;

void refresh_settlements_display();
