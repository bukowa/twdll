#include "game_api.h"
#include "../common/game_api.h"
#include "../common/log.h"

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

// FACTION_TECHNOLOGY_MANAGER instant-research path (see game_api.h).
// instant_set_researched_without_updating_effects wrapper:
//   `push esi; push [esp+4+arg_0]; mov esi,ecx; call finder; ...` @ 0x10B9D3D0
#define INSTANT_SET_RESEARCHED_SIG "56 FF 74 24 ? 8B F1 E8 ? ? ? ? 85 C0 74 ? 6A 00 FF 74 24"
// DATABASE_TABLE::record_index(table, &CA::String key) @ 0x10192660.
// The `68 ? ? ? ?` wildcards the `push 0x11AB8010` (error-string address): it is
// an absolute image pointer that ASLR relocates at load, so only the on-disk
// bytes would match a hardcoded sig — it must stay wildcarded.
#define RECORD_INDEX_SIG "83 EC 0C 53 8B D9 56 8B 74 24 ? 56 8D 4B ? E8 ? ? ? ? 85 C0 74 ? 8B 08 8B 43 ? EB ? 57 8B CE E8 ? ? ? ? 8D 4B ? 8B F8 E8 ? ? ? ? 68 ? ? ? ? 8D 4C 24 ? 8B F0 E8 ? ? ? ? 57 8D 44 24 ? 56 50 E8 ? ? ? ? 83 C4 0C 8D 4C 24 ? E8 ? ? ? ? 8B 4B ? 8B C1 5F 3B C8 73 ? 8B 43 ? 5E 5B 8B 04 88 83 C4 0C"
// UNIT::convert_unit @ 0x106FC010: break force link (lea ecx,[ebx+34h]),
// read m_soldiers, clamp vs unit-multiplier(num_men), LIST_POOL::allocate(0x164),
// 4-arg ctor, subsume into force, destroy old. Verified unique.
#define CONVERT_UNIT_SIG "53 8B 5C 24 08 55 56 57 8D 4B 34 E8 ? ? ? ? 8D 4B 30 E8 ? ? ? ? 8D 88 F0 10 00 00 E8 ? ? ? ? 8B 6C 24 1C 8D 48 18 FF 75 28"
// CAMPAIGN_BUILDING_DISPLAY::update_animation @ 0x10B1A790
// Reads force_settlement_refresh tweak @ offset 0x17 from entry (mov ecx, &tweak).
#define UPDATE_ANIMATION_SIG "51 56 8B F1 8B 86 ? ? ? ? 8B 48"
// CHARACTER_DETAILS::portrait_path @ 0x107DC9E0
#define RESOLVE_PORTRAIT_PATH_SIG "83 EC 08 55 8B E9 56 83 BD 54 03 00 00 00 0F 84"

FnNewFactionLeader g_new_faction_leader = nullptr;
FnDisbandUnits     g_disband_units       = nullptr;
FnFactionProvinceManager g_faction_province_manager = nullptr;
FnResolvePortraitPath    g_resolve_portrait_path    = nullptr;
uintptr_t          g_reinf_cap_insn_addr = 0;
uintptr_t          g_battle_ctor_addr    = 0;
uintptr_t          g_battle_dtor_addr    = 0;
uintptr_t          g_settlement_cb_initialize_addr = 0;
static uintptr_t   g_update_animation_addr = 0;
static uintptr_t   g_force_settlement_refresh_addr = 0;

FnInstantSetResearched g_instant_set_researched = nullptr;
FnRecordIndex          g_record_index           = nullptr;
FnConvertUnit          g_convert_unit           = nullptr;

const uintptr_t OFFSET_MAX_UNITS_ARMY = 0x1CC91F0;
const uintptr_t OFFSET_MAX_UNITS_NAVY = 0x1CC91F4;

void refresh_settlements_display() {
    if (!g_force_settlement_refresh_addr && g_update_animation_addr) {
        g_force_settlement_refresh_addr = *reinterpret_cast<uintptr_t*>(g_update_animation_addr + 0x17);
        Log("[twdll] Resolved force_settlement_refresh tweak @ 0x%08X", g_force_settlement_refresh_addr);
    }
    if (g_force_settlement_refresh_addr) {
        *reinterpret_cast<uint8_t*>(g_force_settlement_refresh_addr + 0x48) = 1;
        Log("[twdll] Triggered full settlement display refresh");
    } else {
        Log("[twdll] WARNING: force_settlement_refresh tweak address not resolved");
    }
}

const TW_GameSigInfo g_game_signatures[] = {
    {"CHARACTER_DETAILS::portrait_path", (void**)&g_resolve_portrait_path, RESOLVE_PORTRAIT_PATH_SIG},
    {"FACTION::new_faction_leader",     (void**)&g_new_faction_leader, NEW_FACTION_LEADER_SIG},
    {"UNIT::disband_units",             (void**)&g_disband_units,      DISBAND_UNITS_SIG},
    {"REGION::faction_province_manager", (void**)&g_faction_province_manager, FACTION_PROVINCE_MANAGER_SIG},
    {"REINFORCEMENTS_MANAGER::max_units_load", (void**)&g_reinf_cap_insn_addr, REINF_CAP_SIG},
    {"EMPIREBATTLE::MANAGER::ctor",            (void**)&g_battle_ctor_addr,    BATTLE_CTOR_SIG},
    {"EMPIREBATTLE::MANAGER::dtor",            (void**)&g_battle_dtor_addr,    BATTLE_DTOR_SIG},
    {"CampaignSettlementCallback::Initialize", (void**)&g_settlement_cb_initialize_addr, SETTLEMENT_CB_INITIALIZE_SIG},
    {"FACTION_TECHNOLOGY_MANAGER::instant_set_researched", (void**)&g_instant_set_researched, INSTANT_SET_RESEARCHED_SIG},
    {"DATABASE_TABLE::record_index",                       (void**)&g_record_index,           RECORD_INDEX_SIG},
    {"UNIT::convert_unit",                                 (void**)&g_convert_unit,           CONVERT_UNIT_SIG},
    {"CAMPAIGN_BUILDING_DISPLAY::update_animation",        (void**)&g_update_animation_addr,  UPDATE_ANIMATION_SIG},
    {nullptr, nullptr, nullptr}
};
