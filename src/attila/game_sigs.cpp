#include "game_api.h"
#include "../common/game_api.h"

// clang-format off
#define NEW_FACTION_LEADER_SIG  "83 EC ? 53 55 56 57 8B F9 8D 4C 24 ? 68 ? ? ? ? 8B B7 ? ? ? ? E8 ? ? ? ? 8D 44 24"
#define DISBAND_UNITS_SIG   "81 EC 98 00 00 00 33 C0"
// REINFORCEMENTS_MANAGER ctor: `mov eax, [eax+0x13C]` (load max units per army
// from the battle setup) followed by `mov [esi+0x28], eax` (store into the
// manager field). Unique in the image; used as the reinforcement cap patch site.
#define REINF_CAP_SIG   "8B 80 3C 01 00 00 89 46 28"
// clang-format on

FnNewFactionLeader g_new_faction_leader = nullptr;
FnDisbandUnits     g_disband_units       = nullptr;
uintptr_t          g_reinf_cap_insn_addr = 0;

const uintptr_t OFFSET_MAX_UNITS_ARMY = 0x1CC91F0;
const uintptr_t OFFSET_MAX_UNITS_NAVY = 0x1CC91F4;

const TW_GameSigInfo g_game_signatures[] = {
    {"FACTION::new_faction_leader",     (void**)&g_new_faction_leader, NEW_FACTION_LEADER_SIG},
    {"UNIT::disband_units",             (void**)&g_disband_units,      DISBAND_UNITS_SIG},
    {"REINFORCEMENTS_MANAGER::max_units_load", (void**)&g_reinf_cap_insn_addr, REINF_CAP_SIG},
    {nullptr, nullptr, nullptr}
};
