#include "game_api.h"
#include "../common/game_api.h"

// clang-format off
#define NEW_FACTION_LEADER_SIG  "83 EC ? 53 55 56 57 8B F9 8D 4C 24 ? 68 ? ? ? ? 8B B7 ? ? ? ? E8 ? ? ? ? 8D 44 24"
#define DISBAND_UNITS_SIG   "81 EC 98 00 00 00 33 C0"
// REGION::faction_province_manager entry: mov esi,ecx (this=REGION*) then two
// CONST_SAFE_PTR::deref calls (province link +0x350, owning faction +0x120) and a
// PROVINCE sub-call, with a fallback that reads [province+0x5C]->[0]. Verified unique.
#define FACTION_PROVINCE_MANAGER_SIG "53 56 8B F1 57 8D 8E ? ? ? ? E8 ? ? ? ? 8D 8E"
// REINFORCEMENTS_MANAGER ctor: `mov eax, [eax+0x13C]` (load max units per army
// from the battle setup) followed by `mov [esi+0x28], eax` (store into the
// manager field). Unique in the image; used as the reinforcement cap patch site.
#define REINF_CAP_SIG   "8B 80 3C 01 00 00 89 46 28"
// EMPIREBATTLE::MANAGER ctor/dtor entry points. Verified unique in the image;
// both signatures start at the function entry, so they can be hooked directly.
#define BATTLE_CTOR_SIG "83 EC 0C 53 55 8B 6C 24 ? 56 57 8B F9 8B 45"
#define BATTLE_DTOR_SIG "51 53 55 56 57 8B F9 8B B7 ? ? ? ? 85 F6"
// CampaignSettlementCallback::Initialize entry point (thiscall, this in ecx).
// The building-slot render loop reads m_max_slots at this+0x48 on every
// iteration (cmp [esi+48h], ... @ 0x113E2513 / 0x113E274C), so hooking here
// lets twdll override the slot count before the panel renders. Verified unique.
#define SETTLEMENT_CB_INITIALIZE_SIG "83 EC 74 53 55 56 8B F1 33 DB"
// clang-format on

FnNewFactionLeader g_new_faction_leader = nullptr;
FnDisbandUnits     g_disband_units       = nullptr;
FnFactionProvinceManager g_faction_province_manager = nullptr;
uintptr_t          g_reinf_cap_insn_addr = 0;
uintptr_t          g_battle_ctor_addr    = 0;
uintptr_t          g_battle_dtor_addr    = 0;
uintptr_t          g_settlement_cb_initialize_addr = 0;

const uintptr_t OFFSET_MAX_UNITS_ARMY = 0x1CC91F0;
const uintptr_t OFFSET_MAX_UNITS_NAVY = 0x1CC91F4;

const TW_GameSigInfo g_game_signatures[] = {
    {"FACTION::new_faction_leader",     (void**)&g_new_faction_leader, NEW_FACTION_LEADER_SIG},
    {"UNIT::disband_units",             (void**)&g_disband_units,      DISBAND_UNITS_SIG},
    {"REGION::faction_province_manager", (void**)&g_faction_province_manager, FACTION_PROVINCE_MANAGER_SIG},
    {"REINFORCEMENTS_MANAGER::max_units_load", (void**)&g_reinf_cap_insn_addr, REINF_CAP_SIG},
    {"EMPIREBATTLE::MANAGER::ctor",            (void**)&g_battle_ctor_addr,    BATTLE_CTOR_SIG},
    {"EMPIREBATTLE::MANAGER::dtor",            (void**)&g_battle_dtor_addr,    BATTLE_DTOR_SIG},
    {"CampaignSettlementCallback::Initialize", (void**)&g_settlement_cb_initialize_addr, SETTLEMENT_CB_INITIALIZE_SIG},
    {nullptr, nullptr, nullptr}
};
