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
#include <cstring>

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
@treturn string|nil memory address (e.g. "0x12345678"), or nil if not yet initialised
@usage
local addr = twdll.world.GetMemoryAddress()
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
Gets the total number of factions existing in the current campaign world.
@function GetFactionCount
@treturn integer|nil number of factions, or nil if not yet initialised
@usage
local total_factions = twdll.world.GetFactionCount()
*/
static int GetFactionCount(lua_State* L) { return FactionCount.get(L); }

/***
Gets the maximum number of units allowed per land army (vanilla default: 20).
@function GetMaxUnitsInArmy
@treturn integer maximum unit count
@usage
local max_army_units = twdll.world.GetMaxUnitsInArmy()
*/
static int GetMaxUnitsInArmy(lua_State* L) {
    if (g_empire_module) {
        l_pushinteger(L, tw_read<int>(g_empire_module, OFFSET_MAX_UNITS_ARMY));
        return 1;
    }
    l_pushnil(L);
    return 1;
}

/***
Sets the maximum number of units allowed per land army.
Affects recruitment limits, army stacking, and UI capacity.
@function SetMaxUnitsInArmy
@tparam integer val new maximum unit count (e.g. 40)
@usage
-- Allow up to 40 units per land army:
twdll.world.SetMaxUnitsInArmy(40)
*/
static int SetMaxUnitsInArmy(lua_State* L) {
    int val = static_cast<int>(l_tointeger(L, 1));
    if (g_empire_module) {
        tw_write<int>(g_empire_module, OFFSET_MAX_UNITS_ARMY, val);
        Log("[twdll] SetMaxUnitsInArmy: %d", val);
    }
    return 0;
}

/***
Gets the maximum number of units allowed per naval fleet (vanilla default: 20).
@function GetMaxUnitsInNavy
@treturn integer maximum unit count
@usage
local max_navy_units = twdll.world.GetMaxUnitsInNavy()
*/
static int GetMaxUnitsInNavy(lua_State* L) {
    if (g_empire_module) {
        l_pushinteger(L, tw_read<int>(g_empire_module, OFFSET_MAX_UNITS_NAVY));
        return 1;
    }
    l_pushnil(L);
    return 1;
}

/***
Sets the maximum number of units allowed per naval fleet.
Affects naval recruitment limits and fleet stacking.
@function SetMaxUnitsInNavy
@tparam integer val new maximum unit count (e.g. 30)
@usage
-- Allow up to 30 ships per naval fleet:
twdll.world.SetMaxUnitsInNavy(30)
*/
static int SetMaxUnitsInNavy(lua_State* L) {
    int val = static_cast<int>(l_tointeger(L, 1));
    if (g_empire_module) {
        tw_write<int>(g_empire_module, OFFSET_MAX_UNITS_NAVY, val);
        Log("[twdll] SetMaxUnitsInNavy: %d", val);
    }
    return 0;
}

namespace {
constexpr size_t  kReinfCapInsnLen = 6;
constexpr uint8_t kReinfCapOrig[6] = {0x8B, 0x80, 0x3C, 0x01, 0x00, 0x00};
} // namespace

// REINFORCEMENTS_MANAGER ctor stores `max units per army` from the battle setup
// into the manager field at offset 0x28. Replacing the load instruction
//   mov eax, [eax+0x13C]   (8B 80 3C 01 00 00)
//   mov [esi+0x28], eax    (89 46 28)
// with `mov eax, <imm32>` + nop lets scripts set the cap to any value before a
// battle starts, so the deploy gate (m_size >= m_max_num_units_per_army) never
// blocks reinforcements. It is a permanent code modification and applies to
// battles started after the call. restore_default == true writes the original
// bytes back; otherwise max_units (any uint32, including 0) is the new cap.
bool set_reinforcement_cap(bool restore_default, uint32_t max_units) {
    if (!g_reinf_cap_insn_addr) {
        Log("[twdll] set_reinforcement_cap: signature not resolved");
        return false;
    }
    uint8_t* p = reinterpret_cast<uint8_t*>(g_reinf_cap_insn_addr);

    uint8_t cur[kReinfCapInsnLen];
    memcpy(cur, p, sizeof(cur));
    const bool is_orig  = memcmp(cur, kReinfCapOrig, sizeof(cur)) == 0;
    const bool is_patch = cur[0] == 0xB8 && cur[kReinfCapInsnLen - 1] == 0x90;
    if (!is_orig && !is_patch) {
        Log("[twdll] set_reinforcement_cap: unexpected bytes at 0x%08X - skipping",
            static_cast<unsigned int>(g_reinf_cap_insn_addr));
        return false;
    }

    uint8_t dst[kReinfCapInsnLen];
    if (restore_default) {
        memcpy(dst, kReinfCapOrig, sizeof(dst));           // restore original
    } else {
        dst[0] = 0xB8;                                     // mov eax, imm32
        memcpy(dst + 1, &max_units, sizeof(max_units));
        dst[5] = 0x90;                                     // nop padding
    }
    if (memcmp(cur, dst, sizeof(dst)) == 0)
        return true; // already in the requested state

    DWORD old_protect = 0;
    if (!VirtualProtect(p, sizeof(dst), PAGE_EXECUTE_READWRITE, &old_protect)) {
        Log("[twdll] set_reinforcement_cap: VirtualProtect failed (%lu)", GetLastError());
        return false;
    }
    memcpy(p, dst, sizeof(dst));
    VirtualProtect(p, sizeof(dst), old_protect, &old_protect);
    Log("[twdll] set_reinforcement_cap: %s (max units = %u)",
        restore_default ? "restored" : "patched", max_units);
    return true;
}

/***
Sets the reinforcement cap (maximum concurrent units per army in tactical battles) for battles started
after this call. Pass `-1` to restore the vanilla engine default.
Any value `>= 0` is applied as an absolute cap.
@function SetReinforcementCap
@tparam integer max_units new cap value (e.g. 40, 80), or -1 to restore the default
@usage
-- Allow up to 40 reinforcement units simultaneously in tactical battle:
twdll.world.SetReinforcementCap(40)

-- Restore vanilla behavior:
twdll.world.SetReinforcementCap(-1)
*/
static int SetReinforcementCap(lua_State* L) {
    if (l_type(L, 1) == LUA_TNIL || l_type(L, 1) == LUA_TNONE) {
        return 0;
    }
    // lua_Integer is 32-bit in Attila's Lua; -1 means restore default.
    int v = static_cast<int>(l_tointeger(L, 1));
    if (v == -1) {
        set_reinforcement_cap(true, 0);
    } else if (v < 0) {
        Log("[twdll] SetReinforcementCap: value must be >= 0 or -1 (default), got %d", v);
    } else {
        set_reinforcement_cap(false, static_cast<uint32_t>(v));
    }
    return 0;
}

/***
Returns the currently applied reinforcement cap override, or nil if the game default is in effect.
@function GetReinforcementCap
@treturn integer|nil current cap value, or nil if default
@usage
local cap = twdll.world.GetReinforcementCap()
if cap then
    -- An override is currently active
end
*/
static int GetReinforcementCap(lua_State* L) {
    if (!g_reinf_cap_insn_addr) { l_pushnil(L); return 1; }
    const uint8_t* p = reinterpret_cast<const uint8_t*>(g_reinf_cap_insn_addr);
    if (p[0] == 0xB8 && p[kReinfCapInsnLen - 1] == 0x90) {
        int v;
        memcpy(&v, p + 1, sizeof(v));
        l_pushinteger(L, v);
        return 1;
    }
    l_pushnil(L);
    return 1;
}

/***
Gets the maximum number of traits a character can hold simultaneously (vanilla default is 10).
@function GetMaxTraits
@treturn integer maximum trait count
@usage
local max_traits = twdll.world.GetMaxTraits()
*/
static int GetMaxTraits(lua_State* L) {
    if (g_empire_module) {
        l_pushinteger(L, tw_read<int>(g_empire_module, OFFSET_MAX_TRAITS));
        return 1;
    }
    l_pushnil(L);
    return 1;
}

/***
Sets the maximum number of traits a character can hold simultaneously.
Prevents new traits from being discarded when a character exceeds 10 traits.
@function SetMaxTraits
@tparam integer val new maximum trait count (e.g. 20, 30, 50)
@usage
-- Expand character trait limit to 30:
twdll.world.SetMaxTraits(30)
*/
static int SetMaxTraits(lua_State* L) {
    int val = static_cast<int>(l_tointeger(L, 1));
    if (val < 1) val = 1;
    if (g_empire_module) {
        tw_write<int>(g_empire_module, OFFSET_MAX_TRAITS, val);
        Log("[twdll] SetMaxTraits: %d", val);
    }
    return 0;
}

extern const luaL_Reg world_functions[] = {
    {"GetMemoryAddress",    GetMemoryAddress},
    {"GetFactionCount",     GetFactionCount},
    {"GetMaxUnitsInArmy",   GetMaxUnitsInArmy},
    {"SetMaxUnitsInArmy",   SetMaxUnitsInArmy},
    {"GetMaxUnitsInNavy",   GetMaxUnitsInNavy},
    {"SetMaxUnitsInNavy",   SetMaxUnitsInNavy},
    {"GetReinforcementCap", GetReinforcementCap},
    {"SetReinforcementCap", SetReinforcementCap},
    {"GetMaxTraits",        GetMaxTraits},
    {"SetMaxTraits",        SetMaxTraits},
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
