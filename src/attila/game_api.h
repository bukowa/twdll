#pragma once
// attila/game_api.h — Attila game function pointer declarations.
// Definitions and signature scanning live in game_sigs.cpp.

// FACTION::new_faction_leader(FACTION*, CHARACTER* new, CHARACTER* old, bool heir_coming_of_age)
using FnNewFactionLeader = void(__thiscall*)(void* faction, void* new_leader, void* old_char, bool heir_coming_of_age);
extern FnNewFactionLeader g_new_faction_leader;

void initialize_game_api();
