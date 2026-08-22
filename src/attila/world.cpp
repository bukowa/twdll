/// @module twdll.world
/// Campaign world singleton and world-level modifiers for Total War: Attila.
#include "../common/tw.h"
#include "../common/campaign_hooks.h"
#include "game_api.h"
#include "tw_types.h"
#include "../common/signature_scanner.h"
#include <MinHook.h>

#include <windows.h>
#include <cstdio>
#include <cstring>

using twdll::TW_World;

static TW_World* g_world = nullptr;
static void* orig_world_ctor = nullptr;
static uintptr_t world_ctor_addr = 0;
static int g_orig_max_units_army = -1;
static int g_orig_max_units_navy = -1;
static int g_orig_max_traits     = -1;


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
    int* p = get_max_units_in_army();
    if (p) {
        l_pushinteger(L, *p);
        return 1;
    }
    l_pushnil(L);
    return 1;
}

/***
Sets the maximum number of units allowed per land army.
Affects recruitment limits, army stacking, and UI capacity.
Pass no arguments or `nil` to restore the vanilla engine default (20).
@function SetMaxUnitsInArmy
@tparam[opt] integer val new maximum unit count (e.g. 40), or omit/nil to restore the default
@usage
-- Allow up to 40 units per land army:
twdll.world.SetMaxUnitsInArmy(40)

-- Restore vanilla default (20):
twdll.world.SetMaxUnitsInArmy()
*/
static int SetMaxUnitsInArmy(lua_State* L) {
    int* p = get_max_units_in_army();
    if (!p) return 0;
    if (g_orig_max_units_army == -1) {
        g_orig_max_units_army = *p;
    }
    int val = g_orig_max_units_army;
    if (l_type(L, 1) > LUA_TNIL) {
        val = static_cast<int>(l_tointeger(L, 1));
        if (val < 1) val = 1;
    }
    *p = val;
    Log("[twdll] SetMaxUnitsInArmy: %d", val);
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
    int* p = get_max_units_in_navy();
    if (p) {
        l_pushinteger(L, *p);
        return 1;
    }
    l_pushnil(L);
    return 1;
}

/***
Sets the maximum number of units allowed per naval fleet.
Affects naval recruitment limits and fleet stacking.
Pass no arguments or `nil` to restore the vanilla engine default (20).
@function SetMaxUnitsInNavy
@tparam[opt] integer val new maximum unit count (e.g. 30), or omit/nil to restore the default
@usage
-- Allow up to 30 ships per naval fleet:
twdll.world.SetMaxUnitsInNavy(30)

-- Restore vanilla default (20):
twdll.world.SetMaxUnitsInNavy()
*/
static int SetMaxUnitsInNavy(lua_State* L) {
    int* p = get_max_units_in_navy();
    if (!p) return 0;
    if (g_orig_max_units_navy == -1) {
        g_orig_max_units_navy = *p;
    }
    int val = g_orig_max_units_navy;
    if (l_type(L, 1) > LUA_TNIL) {
        val = static_cast<int>(l_tointeger(L, 1));
        if (val < 1) val = 1;
    }
    *p = val;
    Log("[twdll] SetMaxUnitsInNavy: %d", val);
    return 0;
}

namespace {
constexpr size_t  kReinfCapInsnLen = 6;
constexpr uint8_t kReinfCapOrig[6] = {0x8B, 0x80, 0x3C, 0x01, 0x00, 0x00};
} // namespace

// Updates or restores the reinforcement cap instruction in REINFORCEMENTS_MANAGER.
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
after this call. Pass no arguments or `nil` to restore the vanilla engine default.
Any value `>= 0` is applied as an absolute cap.
@function SetReinforcementCap
@tparam[opt] integer max_units new cap value (e.g. 40, 80), or omit/nil to restore the default
@usage
-- Allow up to 40 reinforcement units simultaneously in tactical battle:
twdll.world.SetReinforcementCap(40)

-- Restore vanilla behavior:
twdll.world.SetReinforcementCap()
*/
static int SetReinforcementCap(lua_State* L) {
    if (l_type(L, 1) == LUA_TNIL || l_type(L, 1) == LUA_TNONE) {
        set_reinforcement_cap(true, 0);
        return 0;
    }
    int v = static_cast<int>(l_tointeger(L, 1));
    if (v < 0) {
        set_reinforcement_cap(true, 0);
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
    auto* tweaker = find_engine_tweaker("max_traits", 10);
    if (tweaker) {
        l_pushinteger(L, static_cast<lua_Integer>(tweaker->raw_value()));
        return 1;
    }
    l_pushnil(L);
    return 1;
}

/***
Sets the maximum number of traits a character can hold simultaneously.
Prevents new traits from being discarded when a character exceeds 10 traits.
Pass no arguments or `nil` to restore the vanilla engine default (10).
@function SetMaxTraits
@tparam[opt] integer val new maximum trait count (e.g. 20, 30, 50), or omit/nil to restore the default
@usage
-- Expand character trait limit to 30:
twdll.world.SetMaxTraits(30)

-- Restore vanilla default (10):
twdll.world.SetMaxTraits()
*/
static int SetMaxTraits(lua_State* L) {
    auto* tweaker = find_engine_tweaker("max_traits", 10);
    if (!tweaker) return 0;
    if (g_orig_max_traits == -1) {
        g_orig_max_traits = static_cast<int>(tweaker->raw_value());
    }
    int val = g_orig_max_traits;
    if (l_type(L, 1) > LUA_TNIL) {
        val = static_cast<int>(l_tointeger(L, 1));
        if (val < 1) val = 1;
    }
    tweaker->set_raw_value(static_cast<uint32_t>(val));
    Log("[twdll] SetMaxTraits: %d", val);
    return 0;
}

/***
Saves the active campaign game to disk.
@function SaveGame
@tparam string filename save game filename (with or without `.save` extension)
@treturn boolean `true` if the save succeeded, `false` otherwise
@usage
local success = twdll.world.SaveGame("my_checkpoint")
*/
static int SaveGame(lua_State* L) {
    if (!g_save_game) {
        Log("[twdll] SaveGame: save_game signature not resolved");
        l_pushboolean(L, 0);
        return 1;
    }
    if (!g_campaign_model) {
        Log("[twdll] SaveGame: campaign model not available");
        l_pushboolean(L, 0);
        return 1;
    }
    auto* cm = static_cast<twdll::TW_CampaignModel*>(g_campaign_model);
    if (!cm->m_campaign_env) {
        Log("[twdll] SaveGame: campaign env not available");
        l_pushboolean(L, 0);
        return 1;
    }
    size_t len = 0;
    const char* name = l_checklstring(L, 1, &len);
    if (!name || len == 0) {
        Log("[twdll] SaveGame: invalid filename argument");
        l_pushboolean(L, 0);
        return 1;
    }
    std::wstring wname = tw_utf8_to_wide(name, len);
    twdll::TW_CAUniString unistr{
        static_cast<uint32_t>(wname.length()),
        static_cast<uint32_t>(wname.length()),
        wname.c_str()
    };
    auto* env = static_cast<twdll::TW_CampaignEnv*>(cm->m_campaign_env);
    bool ok = g_save_game(env, &unistr, nullptr, 0, 0);
    Log("[twdll] SaveGame('%s'): result = %d", name, ok ? 1 : 0);
    l_pushboolean(L, ok ? 1 : 0);
    return 1;
}

/***
Requests the engine to load a saved game from disk at the end of the current tick.
@function LoadGame
@tparam string filename save game filename (e.g. `"my_checkpoint"` or `"tests.save"`)
@treturn boolean `true` if the load request was successfully dispatched
@usage
twdll.world.LoadGame("tests.save")
*/
static int LoadGame(lua_State* L) {
    if (!g_campaign_model) {
        Log("[twdll] LoadGame: campaign model not available");
        l_pushboolean(L, 0);
        return 1;
    }
    auto* cm = static_cast<twdll::TW_CampaignModel*>(g_campaign_model);
    if (!cm->m_campaign_env) {
        Log("[twdll] LoadGame: campaign env not available");
        l_pushboolean(L, 0);
        return 1;
    }
    size_t len = 0;
    const char* name = l_checklstring(L, 1, &len);
    if (!name || len == 0) {
        Log("[twdll] LoadGame: invalid filename argument");
        l_pushboolean(L, 0);
        return 1;
    }
    std::wstring wname = tw_utf8_to_wide(name, len);
    twdll::TW_CAUniString unistr{
        static_cast<uint32_t>(wname.length()),
        static_cast<uint32_t>(wname.length()),
        wname.c_str()
    };
    auto* env = static_cast<twdll::TW_CampaignEnv*>(cm->m_campaign_env);
    if (g_load_game) {
        g_load_game(env, &unistr, 0);
    }
    Log("[twdll] LoadGame('%s'): scheduled load", name);
    l_pushboolean(L, 1);
    return 1;
}

/***
Requests the engine to cleanly exit the active campaign and return to the main menu.
@function ExitToMainMenu
@treturn boolean `true` if the exit request was dispatched
@usage
twdll.world.ExitToMainMenu()
*/
static int ExitToMainMenu(lua_State* L) {
    if (!g_campaign_model) {
        Log("[twdll] ExitToMainMenu: campaign model not available");
        l_pushboolean(L, 0);
        return 1;
    }
    auto* cm = static_cast<twdll::TW_CampaignModel*>(g_campaign_model);
    if (!cm->m_campaign_env) {
        Log("[twdll] ExitToMainMenu: campaign env not available");
        l_pushboolean(L, 0);
        return 1;
    }
    auto* env = static_cast<twdll::TW_CampaignEnv*>(cm->m_campaign_env);
    env->m_quit_to_main_menu = true;
    Log("[twdll] ExitToMainMenu: requested exit to main menu");
    l_pushboolean(L, 1);
    return 1;
}

/***
Requests the engine to cleanly exit the game process to Windows.
@function ExitGame
@treturn boolean `true` if the exit request was dispatched
@usage
twdll.world.ExitGame()
*/
static int ExitGame(lua_State* L) {
    if (!g_campaign_model) {
        Log("[twdll] ExitGame: campaign model not available");
        l_pushboolean(L, 0);
        return 1;
    }
    auto* cm = static_cast<twdll::TW_CampaignModel*>(g_campaign_model);
    if (!cm->m_campaign_env) {
        Log("[twdll] ExitGame: campaign env not available");
        l_pushboolean(L, 0);
        return 1;
    }
    auto* env = static_cast<twdll::TW_CampaignEnv*>(cm->m_campaign_env);
    env->m_quit_to_windows = true;
    Log("[twdll] ExitGame: requested exit to windows");
    l_pushboolean(L, 1);
    return 1;
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
    {"SaveGame",            SaveGame},
    {"LoadGame",            LoadGame},
    {"ExitToMainMenu",      ExitToMainMenu},
    {"ExitGame",            ExitGame},
    {nullptr, nullptr}
};

// Uninstall hook and clear global pointers / restore engine defaults
void uninstall_world_hook() {
    if (g_orig_max_units_army != -1) {
        int* p = get_max_units_in_army();
        if (p) *p = g_orig_max_units_army;
        Log("[twdll] Restored max_units_army to %d", g_orig_max_units_army);
        g_orig_max_units_army = -1;
    }
    if (g_orig_max_units_navy != -1) {
        int* p = get_max_units_in_navy();
        if (p) *p = g_orig_max_units_navy;
        Log("[twdll] Restored max_units_navy to %d", g_orig_max_units_navy);
        g_orig_max_units_navy = -1;
    }
    if (g_orig_max_traits != -1) {
        auto* tweaker = find_engine_tweaker("max_traits", 10);
        if (tweaker) {
            tweaker->set_raw_value(static_cast<uint32_t>(g_orig_max_traits));
            Log("[twdll] Restored max_traits tweaker to %d", g_orig_max_traits);
        }
        g_orig_max_traits = -1;
    }
    set_reinforcement_cap(true, 0);

    if (world_ctor_addr) {
        MH_DisableHook(reinterpret_cast<void*>(world_ctor_addr));
        MH_RemoveHook(reinterpret_cast<void*>(world_ctor_addr));
        world_ctor_addr = 0;
    }
    g_world = nullptr;
    orig_world_ctor = nullptr;
}
