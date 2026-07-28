#pragma once
#include <cstdint>

using FnNewFactionLeader = void(__thiscall*)(void* faction, void* new_leader, void* old_char, bool heir_coming_of_age);
extern FnNewFactionLeader g_new_faction_leader;

using FnRemoveUnit = void*(__thiscall*)(void* military_force, void* unit);
extern FnRemoveUnit g_remove_unit;

extern const uintptr_t OFFSET_MAX_UNITS_ARMY;
extern const uintptr_t OFFSET_MAX_UNITS_NAVY;
