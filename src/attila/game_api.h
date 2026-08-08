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
