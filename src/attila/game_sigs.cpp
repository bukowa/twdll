#include "game_api.h"
#include "../common/game_api.h"

// clang-format off
#define NEW_FACTION_LEADER_SIG  "83 EC ? 53 55 56 57 8B F9 8D 4C 24 ? 68 ? ? ? ? 8B B7 ? ? ? ? E8 ? ? ? ? 8D 44 24"
#define DISBAND_UNITS_SIG   "81 EC 98 00 00 00 33 C0"
// clang-format on

FnNewFactionLeader g_new_faction_leader = nullptr;
FnDisbandUnits     g_disband_units       = nullptr;

const uintptr_t OFFSET_MAX_UNITS_ARMY = 0x1CC91F0;
const uintptr_t OFFSET_MAX_UNITS_NAVY = 0x1CC91F4;

const TW_GameSigInfo g_game_signatures[] = {
    {"FACTION::new_faction_leader", (void**)&g_new_faction_leader, NEW_FACTION_LEADER_SIG},
    {"UNIT::disband_units",         (void**)&g_disband_units,      DISBAND_UNITS_SIG},
    {nullptr, nullptr, nullptr}
};
