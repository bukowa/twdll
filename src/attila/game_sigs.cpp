// attila/game_sigs.cpp — Total War: Attila game function signatures.
// Contains byte signatures for all game functions called directly by twdll.
// Consumed by initialize_game_api() defined here; called once from luaopen_twdll.

#include "game_api.h"
#include "../common/signature_scanner.h"
#include "../common/log.h"
#include <windows.h>
#include <psapi.h>

// clang-format off
#define NEW_FACTION_LEADER_SIG  "83 EC ? 53 55 56 57 8B F9 8D 4C 24 ? 68 ? ? ? ? 8B B7 ? ? ? ? E8 ? ? ? ? 8D 44 24"
// clang-format on

FnNewFactionLeader g_new_faction_leader = nullptr;

void initialize_game_api() {
    HMODULE hMod = GetModuleHandleA("empire.retail.dll");
    if (!hMod) { Log("[twdll] initialize_game_api: empire.retail.dll not found"); return; }

    MODULEINFO mi = {};
    if (!GetModuleInformation(GetCurrentProcess(), hMod, &mi, sizeof(mi))) {
        Log("[twdll] initialize_game_api: GetModuleInformation failed (%lu)", GetLastError());
        return;
    }

    uintptr_t base = reinterpret_cast<uintptr_t>(hMod);
    size_t    size = mi.SizeOfImage;

    struct GameSigInfo {
        const char* name;
        void**      target;
        const char* sig;
    };

    GameSigInfo sigs[] = {
        {"FACTION::new_faction_leader", (void**)&g_new_faction_leader, NEW_FACTION_LEADER_SIG},
        {nullptr, nullptr, nullptr}
    };

    for (auto& s : sigs) {
        if (!s.name) break;
        uintptr_t addr = Scanner::find_signature(base, size, s.sig);
        if (!addr) { Log("[twdll] initialize_game_api: signature not found for %s", s.name); continue; }
        *s.target = reinterpret_cast<void*>(addr);
        Log("[twdll] initialize_game_api: %s @ 0x%08X", s.name, addr);
    }
}
