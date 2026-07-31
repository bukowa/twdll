/// @module twdll.world
/// Campaign world singleton and world-level modifiers for Total War: Attila.
#include "common/tw.h"
#include "common/campaign_hooks.h"
#include "game_api.h"
#include "tw_types.h"
#include "common/signature_scanner.h"
#include <MinHook.h>

#include <windows.h>
#include <cstdio>

using twdll::TW_World;

static TW_World* g_world = nullptr;
static void* orig_world_ctor = nullptr;
static uintptr_t world_ctor_addr = 0;

static void LogWorldHook(void* ptr) {
    g_world = static_cast<TW_World*>(ptr);
    Log("[twdll] WORLD ctor hooked — g_world = 0x%08X", reinterpret_cast<uintptr_t>(ptr));
}

__declspec(naked) static void HookedWorldCtor() {
    __asm {
        pushad
        push ecx
        call LogWorldHook
        add esp, 4
        popad
        jmp dword ptr [orig_world_ctor]
    }
}

void install_world_hook(uintptr_t base, size_t size) {
    // ---- Explicit, non‑abstracted hook installation -------------------------
    const char* anchor = "FACTION_ARRAY";
    const char* label  = "WORLD";

    Log("[twdll] Processing anchor '%s' for %s ctor", anchor, label);

    uintptr_t str_addr = Scanner::FindString(base, size, anchor);
    if (!str_addr) {
        Log("[twdll] [%s] anchor string not found", label);
        return;
    }

    uintptr_t push_addr = Scanner::FindPushRef(base, size, str_addr);
    if (!push_addr) {
        Log("[twdll] [%s] push ref not found", label);
        return;
    }

    uintptr_t ctor_addr = Scanner::FindPrologue(push_addr);
    if (!ctor_addr) {
        Log("[twdll] [%s] prologue not found", label);
        return;
    }

    MH_STATUS mhs = MH_CreateHook(reinterpret_cast<void*>(ctor_addr),
                                   reinterpret_cast<void*>(HookedWorldCtor),
                                   reinterpret_cast<void**>(&orig_world_ctor));
    if (mhs == MH_OK) {
        world_ctor_addr = ctor_addr;
    }
    if (mhs != MH_OK) {
        Log("[twdll] [%s] MH_CreateHook failed (%d)", label, mhs);
        return;
    }

    mhs = MH_EnableHook(reinterpret_cast<void*>(ctor_addr));
    if (mhs != MH_OK) {
        Log("[twdll] [%s] MH_EnableHook failed (%d)", label, mhs);
        return;
    }

    Log("[twdll] [%s] hook installed OK", label);
    // ------------------------------------------------------------------------
}

/***
Returns the memory address of the WORLD singleton as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678"), or nil if not yet initialised
*/
static int GetMemoryAddress(lua_State* L) {
    if (!g_world) { l_pushnil(L); return 1; }
    char buf[20];
    snprintf(buf, sizeof(buf), "0x%08X", reinterpret_cast<unsigned int>(g_world));
    l_pushstring(L, buf);
    return 1;
}

static twdll::GlobalGetter<int, TW_World> FactionCount{&TW_World::faction_count, &g_world};

/***
Gets the number of factions in the current campaign.
@function GetFactionCount
@treturn integer number of factions, or nil if not yet initialised
*/
static int GetFactionCount(lua_State* L) { return FactionCount.get(L); }

/***
Gets the maximum number of units allowed in an army.
@function GetMaxUnitsInArmy
@treturn integer maximum unit count
*/
static int GetMaxUnitsInArmy(lua_State* L) {
    if (HMODULE hMod = GetModuleHandleA("empire.retail.dll")) {
        l_pushinteger(L, *reinterpret_cast<int*>(reinterpret_cast<uintptr_t>(hMod) + OFFSET_MAX_UNITS_ARMY));
        return 1;
    }
    l_pushnil(L);
    return 1;
}

/***
Sets the maximum number of units allowed in an army.
@function SetMaxUnitsInArmy
@tparam integer val maximum unit count
*/
static int SetMaxUnitsInArmy(lua_State* L) {
    int val = static_cast<int>(l_tointeger(L, 1));
    if (HMODULE hMod = GetModuleHandleA("empire.retail.dll")) {
        *reinterpret_cast<int*>(reinterpret_cast<uintptr_t>(hMod) + OFFSET_MAX_UNITS_ARMY) = val;
        Log("[twdll] SetMaxUnitsInArmy: %d", val);
    }
    return 0;
}

/***
Gets the maximum number of units allowed in a navy.
@function GetMaxUnitsInNavy
@treturn integer maximum unit count
*/
static int GetMaxUnitsInNavy(lua_State* L) {
    if (HMODULE hMod = GetModuleHandleA("empire.retail.dll")) {
        l_pushinteger(L, *reinterpret_cast<int*>(reinterpret_cast<uintptr_t>(hMod) + OFFSET_MAX_UNITS_NAVY));
        return 1;
    }
    l_pushnil(L);
    return 1;
}

/***
Sets the maximum number of units allowed in a navy.
@function SetMaxUnitsInNavy
@tparam integer val maximum unit count
*/
static int SetMaxUnitsInNavy(lua_State* L) {
    int val = static_cast<int>(l_tointeger(L, 1));
    if (HMODULE hMod = GetModuleHandleA("empire.retail.dll")) {
        *reinterpret_cast<int*>(reinterpret_cast<uintptr_t>(hMod) + OFFSET_MAX_UNITS_NAVY) = val;
        Log("[twdll] SetMaxUnitsInNavy: %d", val);
    }
    return 0;
}

extern const luaL_Reg world_functions[] = {
    {"GetMemoryAddress",   GetMemoryAddress},
    {"GetFactionCount",    GetFactionCount},
    {"GetMaxUnitsInArmy",  GetMaxUnitsInArmy},
    {"SetMaxUnitsInArmy",  SetMaxUnitsInArmy},
    {"GetMaxUnitsInNavy",  GetMaxUnitsInNavy},
    {"SetMaxUnitsInNavy",  SetMaxUnitsInNavy},
    {nullptr, nullptr}
};

// Uninstall hook and clear global pointers
void uninstall_world_hook() {
    if (world_ctor_addr) {
        MH_DisableHook(reinterpret_cast<void*>(world_ctor_addr));
        MH_RemoveHook(reinterpret_cast<void*>(world_ctor_addr));
        world_ctor_addr = 0;
    }
    g_world = nullptr;
    orig_world_ctor = nullptr;
}
