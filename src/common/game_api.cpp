#include "game_api.h"
#include "signature_scanner.h"
#include "log.h"
#include <windows.h>
#include <psapi.h>

extern const char* GAME_MODULE_NAME;

void initialize_game_api() {
    HMODULE hMod = GetModuleHandleA(GAME_MODULE_NAME);
    if (!hMod) { Log("[twdll] initialize_game_api: %s not found", GAME_MODULE_NAME); return; }
    g_empire_module = hMod;

    MODULEINFO mi = {};
    if (!GetModuleInformation(GetCurrentProcess(), hMod, &mi, sizeof(mi))) {
        Log("[twdll] initialize_game_api: GetModuleInformation failed (%lu)", GetLastError());
        return;
    }

    uintptr_t base = reinterpret_cast<uintptr_t>(hMod);
    size_t    size = mi.SizeOfImage;

    for (const TW_GameSigInfo* s = g_game_signatures; s->name; ++s) {
        uintptr_t addr = Scanner::find_signature(base, size, s->sig);
        if (!addr) { Log("[twdll] initialize_game_api: not found — %s", s->name); continue; }
        *s->target = reinterpret_cast<void*>(addr);
        Log("[twdll] initialize_game_api: %s @ 0x%08X", s->name, addr);
    }
}
