/// @module twdll.world
/// Campaign world singleton and world-level modifiers for Total War: Attila.
#include "../common/tw.h"
#include "../common/campaign_hooks.h"
#include "game_api.h"
#include "tw_types.h"
#include "../common/signature_scanner.h"
#include <MinHook.h>
#include <intrin.h>

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

// ---- Custom campaign keys ---------------------------------------------------
// The engine maps a campaign key to a content flag with hard-coded string compares:
// main_attila(_map) -> 1 (base game), bel_attila(_map) -> 0x20, cha_attila(_map) -> 0x80,
// anything else -> 0. Flag 0 means "DLC not owned": a mod campaign with its own key is
// shown as locked DLC (purchase dialog), is filtered out of multiplayer map lists and
// refused when joining a multiplayer campaign. Patching the final "xor eax,eax" (33 C0)
// into "jmp +0x10" (EB 10) makes unknown keys return 1, the base-game flag. Vanilla
// keys keep their flags, so vanilla and DLC campaigns behave exactly as before.
//
// Unlike other twdll patches this one is NOT rolled back on Lua teardown: the
// ownership checks run while the game moves between Lua states (frontend lobby ->
// campaign load), and rolling back in between would lock the campaign mid-load.
// It stays until AllowCustomCampaignKeys(false) or the process exits.
namespace {
constexpr size_t  kKeyPatchOffset = 4;
constexpr uint8_t kKeyOrig[2]  = {0x33, 0xC0}; // xor eax, eax  -> return 0 (locked)
constexpr uint8_t kKeyPatch[2] = {0xEB, 0x10}; // jmp +0x10     -> return 1 (base game)
} // namespace

// Returns 1 if unknown keys are treated as base game, 0 if not, -1 if the code was not found.
static int campaign_key_patch_state() {
    if (!g_campaign_key_content_tail_addr) return -1;
    const uint8_t* p = reinterpret_cast<const uint8_t*>(g_campaign_key_content_tail_addr + kKeyPatchOffset);
    if (memcmp(p, kKeyPatch, 2) == 0) return 1;
    if (memcmp(p, kKeyOrig, 2) == 0) return 0;
    return -1;
}

static bool set_campaign_key_patch(bool enable) {
    const int state = campaign_key_patch_state();
    if (state < 0) {
        Log("[twdll] AllowCustomCampaignKeys: campaign key check not found (other game build?) - not patched");
        return false;
    }
    if (state == (enable ? 1 : 0)) return true;
    uint8_t* p = reinterpret_cast<uint8_t*>(g_campaign_key_content_tail_addr + kKeyPatchOffset);
    DWORD old_protect = 0;
    if (!VirtualProtect(p, 2, PAGE_EXECUTE_READWRITE, &old_protect)) {
        Log("[twdll] AllowCustomCampaignKeys: VirtualProtect failed (%lu)", GetLastError());
        return false;
    }
    memcpy(p, enable ? kKeyPatch : kKeyOrig, 2);
    VirtualProtect(p, 2, old_protect, &old_protect);
    FlushInstructionCache(GetCurrentProcess(), p, 2);
    Log("[twdll] AllowCustomCampaignKeys: %s at 0x%08X", enable ? "enabled" : "disabled",
        static_cast<unsigned int>(reinterpret_cast<uintptr_t>(p)));
    return true;
}

/***
Makes the game treat campaigns with custom keys as base-game content instead of
unowned DLC. Without it, a mod campaign whose key is not `main_attila`/`main_attila_map`
(or a DLC key) is shown as locked DLC, missing from multiplayer map lists and refused
when joining a multiplayer campaign. Vanilla and DLC campaigns are unaffected.

Call it from the frontend (main menu) script so it is in place before campaign
selection and the multiplayer lobby; every multiplayer player needs it. It stays in
effect until disabled or the game exits (it is not rolled back on Lua teardown).
@function AllowCustomCampaignKeys
@tparam[opt=true] boolean enabled true to allow custom keys, false to restore the vanilla check
@treturn boolean|nil true if custom keys are now allowed, false if not, nil if the check was not found in this game build
@usage
-- In the frontend script:
twdll.world.AllowCustomCampaignKeys()
*/
static int AllowCustomCampaignKeys(lua_State* L) {
    bool enable = true;
    if (l_type(L, 1) == LUA_TBOOLEAN) enable = l_tobool(L, 1);
    set_campaign_key_patch(enable);
    const int state = campaign_key_patch_state();
    if (state < 0) { l_pushnil(L); return 1; }
    l_pushboolean(L, state == 1);
    return 1;
}

/***
Returns whether campaigns with custom keys are currently treated as base-game content.
@function IsCustomCampaignKeysAllowed
@treturn boolean|nil true if allowed, false if the vanilla check is active, nil if the check was not found
@usage
if twdll.world.IsCustomCampaignKeysAllowed() then ... end
*/
static int IsCustomCampaignKeysAllowed(lua_State* L) {
    const int state = campaign_key_patch_state();
    if (state < 0) { l_pushnil(L); return 1; }
    l_pushboolean(L, state == 1);
    return 1;
}

// ---- Gift Region diplomacy ---------------------------------------------------
// Vanilla offers "Gift Region" only to hordes: the availability check (called with
// the two factions of a negotiation) requires, for each direction, a giver that is
// not a horde and a receiver that is a horde passing a second horde test. The
// giftable-region list builder then keeps only regions on the receiver's own list.
// Patching those conditional jumps opens the option to every faction.
//
//   group kCore (tested in game): direction 1 receiver checks + region list filter
//   group kReverse: the same receiver checks for the other direction
//   group kHordeGivers: lets a horde be the giver (both directions)
//
// Rolled back with the other world patches when the last Lua state closes; the
// campaign script re-applies it on every load.
namespace {
enum GiftGroup : int { kCore = 1, kReverse = 2, kHordeGivers = 4 };

struct GiftPatch {
    uintptr_t* base;
    size_t     offset;
    uint8_t    orig[2];
    uint8_t    patched[2];
    int        group;
    bool       written;   // true if twdll changed these bytes (restore only those)
};

GiftPatch g_gift_patches[] = {
    {&g_gift_region_availability_addr, 0x30, {0x74, 0x37}, {0x90, 0x90}, kCore,        false}, // receiver must be a horde
    {&g_gift_region_availability_addr, 0x3B, {0x74, 0x2C}, {0x90, 0x90}, kCore,        false}, // receiver second horde test
    {&g_gift_region_list_filter_addr,  0x24, {0x74, 0x44}, {0xEB, 0x44}, kCore,        false}, // only receiver-listed regions
    {&g_gift_region_availability_addr, 0x86, {0x74, 0x3D}, {0x90, 0x90}, kReverse,     false},
    {&g_gift_region_availability_addr, 0x91, {0x74, 0x32}, {0x90, 0x90}, kReverse,     false},
    {&g_gift_region_availability_addr, 0x25, {0x75, 0x42}, {0x90, 0x90}, kHordeGivers, false}, // giver must not be a horde
    {&g_gift_region_availability_addr, 0x7B, {0x75, 0x48}, {0x90, 0x90}, kHordeGivers, false},
};

// 1 = patched, 0 = original, -1 = not resolved or unexpected bytes
int gift_patch_state(const GiftPatch& gp) {
    if (!*gp.base) return -1;
    const uint8_t* p = reinterpret_cast<const uint8_t*>(*gp.base + gp.offset);
    if (memcmp(p, gp.patched, 2) == 0) return 1;
    if (memcmp(p, gp.orig, 2) == 0) return 0;
    return -1;
}

bool gift_patch_write(GiftPatch& gp, bool patch) {
    uint8_t* p = reinterpret_cast<uint8_t*>(*gp.base + gp.offset);
    DWORD old_protect = 0;
    if (!VirtualProtect(p, 2, PAGE_EXECUTE_READWRITE, &old_protect)) return false;
    memcpy(p, patch ? gp.patched : gp.orig, 2);
    VirtualProtect(p, 2, old_protect, &old_protect);
    FlushInstructionCache(GetCurrentProcess(), p, 2);
    gp.written = patch;
    return true;
}

// Applies (wanted groups) and removes (other groups) atomically: nothing is written
// unless every site involved has the expected bytes.
bool set_gift_region_groups(int groups) {
    for (const GiftPatch& gp : g_gift_patches) {
        if (gift_patch_state(gp) < 0) {
            Log("[twdll] AllowGiftRegion: code not found or unexpected bytes (other game build?) - nothing patched");
            return false;
        }
    }
    for (GiftPatch& gp : g_gift_patches) {
        const bool want = (groups & gp.group) != 0;
        const int  state = gift_patch_state(gp);
        if (want && state == 0) {
            if (!gift_patch_write(gp, true)) return false;
        } else if (!want && state == 1 && gp.written) {
            // only undo what twdll wrote; a pre-patched process/file is left alone
            if (!gift_patch_write(gp, false)) return false;
        }
    }
    Log("[twdll] AllowGiftRegion: groups = 0x%X", groups);
    return true;
}

int active_gift_region_groups() {
    int active = kCore | kReverse | kHordeGivers;
    for (const GiftPatch& gp : g_gift_patches) {
        if (gift_patch_state(gp) != 1) active &= ~gp.group;
    }
    return active;
}

bool opt_bool(lua_State* L, int table_idx, const char* key) {
    if (l_type(L, table_idx) != LUA_TTABLE) return false;
    l_getfield(L, table_idx, key);
    const bool v = l_type(L, -1) == LUA_TBOOLEAN && l_tobool(L, -1);
    l_pop(L, 1);
    return v;
}
} // namespace

// Restores the Gift Region bytes twdll changed (called from uninstall_world_hook).
static void restore_gift_region() {
    bool any = false;
    for (GiftPatch& gp : g_gift_patches) {
        if (gp.written && gift_patch_state(gp) == 1) {
            gift_patch_write(gp, false);
            any = true;
        }
        gp.written = false;
    }
    if (any) Log("[twdll] AllowGiftRegion: restored vanilla diplomacy checks");
}

/***
Offers the vanilla "Gift Region" diplomacy option between any factions, not only to hordes.
By default it opens the option for the faction proposing the deal (the in-game tested
case). Options add the reverse direction (the other side may give regions) and let
hordes be givers; these two are not tested in game yet. A faction capital still cannot
be gifted. Every multiplayer player must apply the same settings. Rolled back when the
campaign's Lua state is torn down; call it again on every campaign load.
@function AllowGiftRegion
@tparam[opt=true] boolean enabled true to allow, false to restore the vanilla checks
@tparam[opt] table options `{ reverse = bool, horde_givers = bool }`
@treturn boolean|nil true if applied, false if it could not be applied, nil if the code was not found in this game build
@usage
-- In the campaign script:
twdll.world.AllowGiftRegion()
-- Also let the other side offer regions, and hordes give them:
twdll.world.AllowGiftRegion(true, { reverse = true, horde_givers = true })
-- Back to vanilla:
twdll.world.AllowGiftRegion(false)
*/
static int AllowGiftRegion(lua_State* L) {
    bool enable = true;
    if (l_type(L, 1) == LUA_TBOOLEAN) enable = l_tobool(L, 1);
    int groups = 0;
    if (enable) {
        groups = kCore;
        if (opt_bool(L, 2, "reverse")) groups |= kReverse;
        if (opt_bool(L, 2, "horde_givers")) groups |= kHordeGivers;
    }
    if (!g_gift_region_availability_addr || !g_gift_region_list_filter_addr) {
        Log("[twdll] AllowGiftRegion: signatures not resolved");
        l_pushnil(L);
        return 1;
    }
    l_pushboolean(L, set_gift_region_groups(groups));
    return 1;
}

/***
Returns which Gift Region unlocks are active.
@function GetGiftRegionState
@treturn table|nil `{ any = bool, reverse = bool, horde_givers = bool }`, or nil if the code was not found
@usage
local s = twdll.world.GetGiftRegionState()
if s and s.any then ... end
*/
static int GetGiftRegionState(lua_State* L) {
    if (!g_gift_region_availability_addr || !g_gift_region_list_filter_addr) { l_pushnil(L); return 1; }
    const int active = active_gift_region_groups();
    l_createtable(L, 0, 3);
    l_pushboolean(L, (active & kCore) != 0);         l_setfield(L, -2, "any");
    l_pushboolean(L, (active & kReverse) != 0);      l_setfield(L, -2, "reverse");
    l_pushboolean(L, (active & kHordeGivers) != 0);  l_setfield(L, -2, "horde_givers");
    return 1;
}

// ---- Custom battle default campaign map ---------------------------------------
// When the custom battle screen opens, the engine pre-selects the campaign map whose
// campaign_map_playable_areas mapname is the hard-coded string "main_attila_map".
// The selection code pushes that string's address; SetCustomBattleDefaultMap
// repoints the pushed address to a string owned by this (pinned) DLL. If no row has
// the requested mapname the game keeps its normal fallback.
// Not rolled back on Lua teardown (like AllowCustomCampaignKeys): the frontend Lua state
// closes soon after the main menu script runs, and the custom battle screen is built again
// when a custom battle ends, so a teardown restore made the screen's minimap
// (BattleMapLoader::OnCreate -> BattleGeneratorMap::Initialise) fall back to
// main_attila_map while its campaign selection came back as the chosen map. The string
// lives in this pinned DLL, so it stays valid until the game exits.
namespace {
constexpr size_t kDefaultMapImmOffset = 0x0B;    // imm32 of "push offset main_attila_map"
const char*      g_vanilla_default_map = nullptr; // the game's own string, read on first use
char             g_custom_default_map[64] = {};
bool             g_default_map_written = false;

uint32_t* default_map_imm() {
    return g_custom_battle_default_map_addr
        ? reinterpret_cast<uint32_t*>(g_custom_battle_default_map_addr + kDefaultMapImmOffset)
        : nullptr;
}

bool write_default_map_imm(const char* str) {
    uint32_t* imm = default_map_imm();
    DWORD old_protect = 0;
    if (!VirtualProtect(imm, 4, PAGE_EXECUTE_READWRITE, &old_protect)) return false;
    *imm = static_cast<uint32_t>(reinterpret_cast<uintptr_t>(str));
    VirtualProtect(imm, 4, old_protect, &old_protect);
    FlushInstructionCache(GetCurrentProcess(), imm, 4);
    return true;
}

// Reads the string the selection code currently uses; null if unreadable.
const char* current_default_map() {
    uint32_t* imm = default_map_imm();
    if (!imm) return nullptr;
    const char* s = reinterpret_cast<const char*>(static_cast<uintptr_t>(*imm));
    if (s == g_custom_default_map) return s;
    MEMORY_BASIC_INFORMATION mbi = {};
    if (!VirtualQuery(s, &mbi, sizeof(mbi)) || mbi.State != MEM_COMMIT) return nullptr;
    return s;
}
} // namespace

static void restore_custom_battle_default_map() {
    if (g_default_map_written && g_vanilla_default_map) {
        write_default_map_imm(g_vanilla_default_map);
        Log("[twdll] SetCustomBattleDefaultMap: restored '%s'", g_vanilla_default_map);
    }
    g_default_map_written = false;
}

/***
Sets which campaign map the custom battle screen selects when it opens. The game
pre-selects the `campaign_map_playable_areas` row whose `mapname` is `"main_attila_map"`;
this changes that name. Call it from the frontend (main menu) script. If no
custom-battle row has the given mapname, the game falls back to its normal behaviour.
A custom mapname (not main/bel/cha) also needs `AllowCustomCampaignKeys()`, otherwise
the map is shown as locked DLC. It stays in effect until it is changed or the game exits
(not rolled back on Lua teardown, because the screen is rebuilt after a custom battle
while no Lua state is loaded).
@function SetCustomBattleDefaultMap
@tparam[opt] string mapname `mapname` of a `campaign_map_playable_areas` row (max 63 characters), or nil to restore `"main_attila_map"`
@treturn string|nil the mapname now selected by default, or nil if the code was not found in this game build
@usage
twdll.world.SetCustomBattleDefaultMap("rom_third_age_map")
*/
static int SetCustomBattleDefaultMap(lua_State* L) {
    const char* cur = current_default_map();
    if (!cur) {
        Log("[twdll] SetCustomBattleDefaultMap: code not found (other game build?)");
        l_pushnil(L);
        return 1;
    }
    if (!g_vanilla_default_map) {
        if (cur == g_custom_default_map || strcmp(cur, "main_attila_map") != 0) {
            Log("[twdll] SetCustomBattleDefaultMap: unexpected default '%.63s' - not patched", cur);
            l_pushnil(L);
            return 1;
        }
        g_vanilla_default_map = cur;
    }

    size_t len = 0;
    const char* want = l_type(L, 1) == LUA_TSTRING ? l_checklstring(L, 1, &len) : nullptr;
    if (!want || len == 0 || strcmp(want, g_vanilla_default_map) == 0) {
        restore_custom_battle_default_map();
    } else if (len >= sizeof(g_custom_default_map)) {
        Log("[twdll] SetCustomBattleDefaultMap: mapname too long (%u)", static_cast<unsigned>(len));
    } else {
        // Point the code at the vanilla string while the buffer changes, then at the buffer.
        if (g_default_map_written) write_default_map_imm(g_vanilla_default_map);
        memcpy(g_custom_default_map, want, len + 1);
        if (write_default_map_imm(g_custom_default_map)) {
            g_default_map_written = true;
            Log("[twdll] SetCustomBattleDefaultMap: '%s'", g_custom_default_map);
        }
    }
    l_pushstring(L, current_default_map());
    return 1;
}

// ---- Governor protection on diplomacy region trades ----------------------------------
// A province governor lives in a settlement as the general of its garrison army. When the
// settlement changes owner, SETTLEMENT::handle_garrison_army_and_navy_pre_change_hands
// destroys that garrison army and calls CHARACTER::kill_instant(KILLED_IN_ACTION) on its
// general, so gifting or trading the region the governor lives in kills him.
// While enabled, every kill_instant call made from that change-hands step during an
// accepted diplomacy region trade runs with the character's immortal flag set, so the
// engine records DEATH_TYPE_CONVALESCING instead; the flag is put back to its previous
// value right after. Captures, sacks, occupation decisions and script transfers are not
// affected. Both detours are removed on Lua teardown.
namespace {
using FnRegionTradeApply = void(__fastcall*)(void* self, void* edx);
using FnKillInstant      = void(__fastcall*)(twdll::TW_Character* self, void* edx, int death_type);
// Length of handle_garrison_army_and_navy_pre_change_hands; both of its kill_instant
// calls return inside it.
constexpr uintptr_t kPreChangeHandsSize = 0x91;

FnRegionTradeApply g_orig_region_trade_apply = nullptr;
FnKillInstant      g_orig_kill_instant = nullptr;
bool               g_governor_hooks_installed = false;
int                g_region_trade_depth = 0;

void __fastcall hooked_region_trade_apply(void* self, void* edx) {
    ++g_region_trade_depth;
    g_orig_region_trade_apply(self, edx);
    --g_region_trade_depth;
}

void __fastcall hooked_kill_instant(twdll::TW_Character* self, void* edx, int death_type) {
    const uintptr_t ret = reinterpret_cast<uintptr_t>(_ReturnAddress());
    if (g_region_trade_depth > 0 && self != nullptr &&
        ret - g_settlement_pre_change_hands_addr < kPreChangeHandsSize) {
        const bool was_immortal = self->details.m_is_immortal;
        self->details.m_is_immortal = true;
        g_orig_kill_instant(self, edx, death_type);
        self->details.m_is_immortal = was_immortal;
        Log("[twdll] ProtectGovernorsOnRegionTrade: kept garrison general 0x%08X alive (immortal flag restored to %d)",
            reinterpret_cast<uintptr_t>(self), was_immortal ? 1 : 0);
        return;
    }
    g_orig_kill_instant(self, edx, death_type);
}

void remove_governor_hooks() {
    if (!g_governor_hooks_installed) return;
    void* trade = reinterpret_cast<void*>(g_region_trade_apply_addr);
    void* kill  = reinterpret_cast<void*>(g_character_kill_instant_addr);
    MH_DisableHook(trade);
    MH_DisableHook(kill);
    MH_RemoveHook(trade);
    MH_RemoveHook(kill);
    g_orig_region_trade_apply = nullptr;
    g_orig_kill_instant = nullptr;
    g_region_trade_depth = 0;
    g_governor_hooks_installed = false;
    Log("[twdll] ProtectGovernorsOnRegionTrade: hooks removed");
}

bool install_governor_hooks() {
    if (g_governor_hooks_installed) return true;
    void* trade = reinterpret_cast<void*>(g_region_trade_apply_addr);
    void* kill  = reinterpret_cast<void*>(g_character_kill_instant_addr);
    const bool trade_created = MH_CreateHook(trade, reinterpret_cast<void*>(&hooked_region_trade_apply),
                                             reinterpret_cast<void**>(&g_orig_region_trade_apply)) == MH_OK;
    const bool kill_created  = MH_CreateHook(kill, reinterpret_cast<void*>(&hooked_kill_instant),
                                             reinterpret_cast<void**>(&g_orig_kill_instant)) == MH_OK;
    if (trade_created && kill_created &&
        MH_EnableHook(trade) == MH_OK && MH_EnableHook(kill) == MH_OK) {
        g_governor_hooks_installed = true;
        Log("[twdll] ProtectGovernorsOnRegionTrade: enabled");
        return true;
    }
    if (trade_created) { MH_DisableHook(trade); MH_RemoveHook(trade); }
    if (kill_created)  { MH_DisableHook(kill);  MH_RemoveHook(kill); }
    g_orig_region_trade_apply = nullptr;
    g_orig_kill_instant = nullptr;
    Log("[twdll] ProtectGovernorsOnRegionTrade: hook install failed - vanilla behaviour kept");
    return false;
}
} // namespace

/***
Keeps province governors alive when the region they live in changes hands through an
accepted diplomacy deal: the vanilla Gift Region option, and regions offered or demanded
in any other deal. The engine normally kills the general of the settlement's garrison
(the governor living there) on every change of owner. While this is on, it records him
as convalescing (wounded) instead, like an immortal character, and his own immortality
setting is left exactly as it was (a mortal governor stays mortal). Captures, sacks,
occupation decisions and `cm:transfer_region_to_faction` are not affected. Deals run on
every machine, so in multiplayer every player must have it on. Removed when the
campaign's Lua state is torn down; call it again on every campaign load.
@function ProtectGovernorsOnRegionTrade
@tparam[opt=true] boolean enabled true to protect governors, false for vanilla behaviour
@treturn boolean|nil true if protection is now on, false if it is off, nil if the code was not found in this game build
@usage
-- In the campaign script:
twdll.world.ProtectGovernorsOnRegionTrade()
-- Back to vanilla:
twdll.world.ProtectGovernorsOnRegionTrade(false)
*/
static int ProtectGovernorsOnRegionTrade(lua_State* L) {
    bool enable = true;
    if (l_type(L, 1) == LUA_TBOOLEAN) enable = l_tobool(L, 1);
    if (!g_region_trade_apply_addr || !g_settlement_pre_change_hands_addr || !g_character_kill_instant_addr) {
        Log("[twdll] ProtectGovernorsOnRegionTrade: signatures not resolved");
        l_pushnil(L);
        return 1;
    }
    if (enable) install_governor_hooks();
    else remove_governor_hooks();
    l_pushboolean(L, g_governor_hooks_installed);
    return 1;
}

extern const luaL_Reg world_functions[] = {
    {"ProtectGovernorsOnRegionTrade", ProtectGovernorsOnRegionTrade},
    {"SetCustomBattleDefaultMap",   SetCustomBattleDefaultMap},
    {"AllowGiftRegion",             AllowGiftRegion},
    {"GetGiftRegionState",          GetGiftRegionState},
    {"AllowCustomCampaignKeys",     AllowCustomCampaignKeys},
    {"IsCustomCampaignKeysAllowed", IsCustomCampaignKeysAllowed},
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
    restore_gift_region();
    // custom battle default map: kept on purpose, see SetCustomBattleDefaultMap
    remove_governor_hooks();

    if (world_ctor_addr) {
        MH_DisableHook(reinterpret_cast<void*>(world_ctor_addr));
        MH_RemoveHook(reinterpret_cast<void*>(world_ctor_addr));
        world_ctor_addr = 0;
    }
    g_world = nullptr;
    orig_world_ctor = nullptr;
}
