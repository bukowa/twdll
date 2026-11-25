/// @module twdll.battle
/// Battle functions and hooks for Total War: Attila.
///
/// To use twdll in battles, load it inside `battle_scripted.lua`:
///
///     -- In battle_scripted.lua:
///     local twdll = package.loadlib("twdll", "luaopen_twdll")()
///     twdll.battle.EnableSmeHealthBars(true)
#include "../common/tw.h"
#include "../common/campaign_hooks.h"
#include "game_api.h"
#include "tw_types.h"
#include <MinHook.h>

#include <windows.h>
#include <cstdio>
#include <cstring>

using twdll::TW_Battle;
using twdll::TW_ReinforcementsManager;
using twdll::TW_BattleLandUnitCardStyle;
using twdll::TW_BattleHealthBar;
using twdll::TW_UnitCardUpdateInfo;
using twdll::TW_BattleUIUnit;
using twdll::TW_BattleUnit;
using twdll::TW_BattleEntity;

static TW_Battle* g_battle = nullptr;
static void*      orig_battle_ctor = nullptr;
static void*      orig_battle_dtor = nullptr;
static uintptr_t  battle_ctor_addr = 0;
static uintptr_t  battle_dtor_addr = 0;

typedef int(__thiscall* t_update_information_changed)(TW_BattleLandUnitCardStyle* self, TW_UnitCardUpdateInfo* info);
static t_update_information_changed orig_update_information_changed = nullptr;
static uintptr_t update_card_info_hook_addr = 0;

typedef void(__thiscall* t_battle_health_bar_on_update_pulse)(TW_BattleHealthBar* self, int a2);
static t_battle_health_bar_on_update_pulse orig_battle_health_bar_on_update_pulse = nullptr;
static uintptr_t battle_health_bar_hook_addr = 0;

// Which callers want the update_information_changed hook. The hook stays installed while
// either is on; the BattleHealthBar hook belongs to the SME mode only.
static bool g_sme_health_bars = false;      // EnableSmeHealthBars
static bool g_unit_card_hit_points = false; // EnableUnitCardHitPoints

typedef void(__thiscall* t_calc_hit_points)(TW_BattleUnit* self, uint32_t* current, uint32_t* max);

static void __fastcall HookedBattleHealthBarOnUpdatePulse(TW_BattleHealthBar* self, void* /*edx*/, int a2) {
    if (self && self->m_unit && self->m_unit->m_num_men_initial == 1) {
        self->m_is_using_hit_points = true;
    }
    orig_battle_health_bar_on_update_pulse(self, a2);
}

// The battle model may be owned by the simulation thread while the UI updates. The game's
// own hit-point bar (BattleHealthBar::OnUpdatePulse) skips reading units then; this is the
// same test: BATTLE_UI::s_ui->m_battle_env (+0x118), m_multi_thread_model (+0x64428) and
// m_battle_model_owner (+0x640A8) == 1. The s_ui address comes from that function's first
// instruction (mov eax, [s_ui]).
static bool battle_model_readable_from_ui() {
    if (!g_battle_health_bar_on_update_pulse_addr) return false;
    void** s_ui_var = *reinterpret_cast<void***>(g_battle_health_bar_on_update_pulse_addr + 1);
    if (!s_ui_var || !*s_ui_var) return false;
    const char* env = *reinterpret_cast<const char* const*>(static_cast<const char*>(*s_ui_var) + 0x118);
    if (!env) return false;
    const bool multi_thread = *reinterpret_cast<const bool*>(env + 0x64428);
    const int  owner        = *reinterpret_cast<const int*>(env + 0x640A8);
    return !(multi_thread && owner == 1);
}

// Unit hit points as a fraction of the unit at full strength:
//   men:     (hit points of the living men / their full hit points) x (living men / full unit size)
//   engines: added for artillery, from the game's own UNIT::calculate_engine_hit_points.
// A one-man unit gives its entity's hit points / full hit points (the SME bar).
// Returns a negative value when the unit cannot be read.
static float unit_hit_point_fraction(const TW_BattleUIUnit* ui_unit) {
    TW_BattleUnit* unit = ui_unit->m_unit;
    if (!unit || ui_unit->m_num_men_initial == 0) return -1.0f;

    double men_current = 0.0;
    double men_full = 0.0;
    const int alive = unit->m_men.size();
    if (alive > 0 && !unit->m_men.data()) return -1.0f;
    for (int i = 0; i < alive; ++i) {
        const TW_BattleEntity* ent = unit->m_men[i];
        if (!ent) continue;
        men_current += ent->m_hit_points;
        men_full += ent->m_full_hit_points;
    }
    // Full-strength hit points: the living men's average full hit points x full unit size.
    double current = men_current;
    double maximum = alive > 0 ? men_full / alive * ui_unit->m_num_men_initial : 0.0;

    if (ui_unit->m_num_engines_initial != 0 && g_unit_calc_engine_hit_points_addr) {
        uint32_t engine_current = 0, engine_max = 0;
        reinterpret_cast<t_calc_hit_points>(g_unit_calc_engine_hit_points_addr)(unit, &engine_current, &engine_max);
        current += engine_current;
        maximum += engine_max;
    }
    if (maximum <= 0.0) return alive == 0 ? 0.0f : -1.0f;
    const double fraction = current / maximum;
    return static_cast<float>(fraction > 1.0 ? 1.0 : fraction);
}

static int __fastcall HookedUpdateInformationChanged(TW_BattleLandUnitCardStyle* self, void* /*edx*/, TW_UnitCardUpdateInfo* info) {
    int res = orig_update_information_changed(self, info);

    if (!self || !self->m_unit || !info) return res;
    const TW_BattleUIUnit* ui_unit = self->m_unit;
    if (!g_unit_card_hit_points && !(g_sme_health_bars && ui_unit->m_num_men_initial == 1)) return res;

    if (!battle_model_readable_from_ui()) {
        // Keep the value the card already shows (BattleLandUnitCardStyle cached
        // percent_men_left at +0x38) instead of flicking back to the men count.
        const float shown = *reinterpret_cast<const float*>(reinterpret_cast<const char*>(self) + 0x38);
        if (shown >= 0.0f && shown <= 1.0f) info->percent_men_left = shown;
        return res;
    }
    const float fraction = unit_hit_point_fraction(ui_unit);
    if (fraction >= 0.0f) info->percent_men_left = fraction;
    return res;
}

static void LogBattleCtor(void* ptr) {
    g_battle = static_cast<TW_Battle*>(ptr);
    Log("[twdll] BATTLE ctor hooked — g_battle = 0x%08X",
        static_cast<unsigned int>(reinterpret_cast<uintptr_t>(ptr)));
}

static void LogBattleDtor(void* ptr) {
    Log("[twdll] BATTLE dtor hooked — 0x%08X",
        static_cast<unsigned int>(reinterpret_cast<uintptr_t>(ptr)));
    g_battle = nullptr;
}

__declspec(naked) static void HookedBattleCtor() {
    __asm {
        pushad
        push ecx
        call LogBattleCtor
        add esp, 4
        popad
        jmp dword ptr [orig_battle_ctor]
    }
}

__declspec(naked) static void HookedBattleDtor() {
    __asm {
        pushad
        push ecx
        call LogBattleDtor
        add esp, 4
        popad
        jmp dword ptr [orig_battle_dtor]
    }
}

static bool install_update_info_hook() {
    if (update_card_info_hook_addr) return true;
    if (!g_update_card_info_addr) {
        Log("[twdll] [BATTLE] update_information_changed signature not resolved");
        return false;
    }
    MH_STATUS mhs = MH_CreateHook(reinterpret_cast<void*>(g_update_card_info_addr),
                                  reinterpret_cast<void*>(HookedUpdateInformationChanged),
                                  reinterpret_cast<void**>(&orig_update_information_changed));
    if (mhs != MH_OK) {
        Log("[twdll] [BATTLE] MH_CreateHook (update_information_changed) failed (%d)", mhs);
        orig_update_information_changed = nullptr;
        return false;
    }
    mhs = MH_EnableHook(reinterpret_cast<void*>(g_update_card_info_addr));
    if (mhs != MH_OK) {
        Log("[twdll] [BATTLE] MH_EnableHook (update_information_changed) failed (%d)", mhs);
        MH_RemoveHook(reinterpret_cast<void*>(g_update_card_info_addr));
        orig_update_information_changed = nullptr;
        return false;
    }
    update_card_info_hook_addr = g_update_card_info_addr;
    Log("[twdll] [BATTLE] update_information_changed hook enabled");
    return true;
}

static void uninstall_update_info_hook() {
    if (update_card_info_hook_addr) {
        MH_DisableHook(reinterpret_cast<void*>(update_card_info_hook_addr));
        MH_RemoveHook(reinterpret_cast<void*>(update_card_info_hook_addr));
        update_card_info_hook_addr = 0;
        Log("[twdll] [BATTLE] update_information_changed hook disabled");
    }
    orig_update_information_changed = nullptr;
}

static bool install_health_bar_hook() {
    if (battle_health_bar_hook_addr) return true;
    if (!g_battle_health_bar_on_update_pulse_addr) {
        Log("[twdll] [BATTLE] BattleHealthBar::OnUpdatePulse signature not resolved");
        return false;
    }
    MH_STATUS mhs = MH_CreateHook(reinterpret_cast<void*>(g_battle_health_bar_on_update_pulse_addr),
                                  reinterpret_cast<void*>(HookedBattleHealthBarOnUpdatePulse),
                                  reinterpret_cast<void**>(&orig_battle_health_bar_on_update_pulse));
    if (mhs != MH_OK) {
        Log("[twdll] [BATTLE] MH_CreateHook (BattleHealthBar::OnUpdatePulse) failed (%d)", mhs);
        orig_battle_health_bar_on_update_pulse = nullptr;
        return false;
    }
    mhs = MH_EnableHook(reinterpret_cast<void*>(g_battle_health_bar_on_update_pulse_addr));
    if (mhs != MH_OK) {
        Log("[twdll] [BATTLE] MH_EnableHook (BattleHealthBar::OnUpdatePulse) failed (%d)", mhs);
        MH_RemoveHook(reinterpret_cast<void*>(g_battle_health_bar_on_update_pulse_addr));
        orig_battle_health_bar_on_update_pulse = nullptr;
        return false;
    }
    battle_health_bar_hook_addr = g_battle_health_bar_on_update_pulse_addr;
    Log("[twdll] [BATTLE] BattleHealthBar::OnUpdatePulse hook enabled");
    return true;
}

static void uninstall_health_bar_hook() {
    if (battle_health_bar_hook_addr) {
        MH_DisableHook(reinterpret_cast<void*>(battle_health_bar_hook_addr));
        MH_RemoveHook(reinterpret_cast<void*>(battle_health_bar_hook_addr));
        battle_health_bar_hook_addr = 0;
        Log("[twdll] [BATTLE] BattleHealthBar::OnUpdatePulse hook disabled");
    }
    orig_battle_health_bar_on_update_pulse = nullptr;
}

// Installs or removes the card hooks so they match the two modes.
static bool sync_card_hooks() {
    bool ok = true;
    if (g_sme_health_bars || g_unit_card_hit_points) ok = install_update_info_hook() && ok;
    else uninstall_update_info_hook();
    if (g_sme_health_bars) ok = install_health_bar_hook() && ok;
    else uninstall_health_bar_hook();
    return ok;
}

static void uninstall_card_hook() {
    g_sme_health_bars = false;
    g_unit_card_hit_points = false;
    uninstall_update_info_hook();
    uninstall_health_bar_hook();
}

// Thrones of Britannia's version of BUILDING_PIECE::kill_all_entities_within_killzone. When a
// wall or tower piece is destroyed (by siege weapons, or a tower collapsing on capture) the game
// kills men around it. ToB runs the same code with two changes, copied here:
//  1. Men standing on a fort wall farther than 7 m (Attila: 10 m) from the piece only stumble.
//     The compare reads a 10.0 constant that 300+ other instructions share, so its address
//     operand is pointed at a twdll float instead.
//  2. The kill area is the 2D collision outline of destruction level index - 1 (clamped at 0),
//     i.e. the piece as it stood, not the outline of the level it is collapsing to. Attila's
//     instructions are 6 bytes too short for ToB's, so the 7-byte index load jumps to a small
//     code cave that does ToB's load and jumps back.
// The 16.5 m radius for men on the ground and the 90 % chance are the same in both games.
namespace {
constexpr uintptr_t kWallRadiusOperandOffset = 0x15;   // m32 of "comiss xmm0, [10.0]"
constexpr size_t    kOutlineLoadSize = 7;              // movzx ecx, byte ptr [ebx+0x159]
float     g_tob_wall_radius = 7.0f;
uint32_t  g_wall_radius_original_operand = 0;
uint8_t   g_outline_original_bytes[kOutlineLoadSize] = {};
uint8_t*  g_outline_cave = nullptr;
bool      g_tob_killzone_on = false;

bool write_code(void* dst, const void* src, size_t n) {
    DWORD old_protect = 0;
    if (!VirtualProtect(dst, n, PAGE_EXECUTE_READWRITE, &old_protect)) {
        Log("[twdll] EnableToBCollapseKillZone: VirtualProtect failed (%lu)", GetLastError());
        return false;
    }
    memcpy(dst, src, n);
    VirtualProtect(dst, n, old_protect, &old_protect);
    FlushInstructionCache(GetCurrentProcess(), dst, n);
    return true;
}

bool install_tob_killzone() {
    if (g_tob_killzone_on) return true;
    uint8_t* load = reinterpret_cast<uint8_t*>(g_killzone_outline_index_addr);
    uint32_t* operand = reinterpret_cast<uint32_t*>(g_killzone_wall_radius_addr + kWallRadiusOperandOffset);

    // The compare must still read the game's 10.0 (guards against a different build).
    const float* current = reinterpret_cast<const float*>(static_cast<uintptr_t>(*operand));
    MEMORY_BASIC_INFORMATION mbi = {};
    if (!VirtualQuery(current, &mbi, sizeof(mbi)) || mbi.State != MEM_COMMIT || *current != 10.0f) {
        Log("[twdll] EnableToBCollapseKillZone: wall radius operand does not read 10.0 - not patched");
        return false;
    }

    // Cave: movzx ecx, byte ptr [ebx+0x159]; sub ecx, 1; adc ecx, 0; jmp back
    // (sub sets CF when the index was 0, adc then brings -1 back to 0: ToB's clamp).
    uint8_t* cave = static_cast<uint8_t*>(VirtualAlloc(nullptr, 32, MEM_COMMIT | MEM_RESERVE, PAGE_EXECUTE_READWRITE));
    if (!cave) {
        Log("[twdll] EnableToBCollapseKillZone: VirtualAlloc failed (%lu)", GetLastError());
        return false;
    }
    const uint8_t body[] = {0x0F, 0xB6, 0x8B, 0x59, 0x01, 0x00, 0x00,   // movzx ecx, byte ptr [ebx+0x159]
                            0x83, 0xE9, 0x01,                           // sub ecx, 1
                            0x83, 0xD1, 0x00};                          // adc ecx, 0
    memcpy(cave, body, sizeof(body));
    uint8_t* jmp_back = cave + sizeof(body);
    jmp_back[0] = 0xE9;
    const int32_t back_rel = static_cast<int32_t>(reinterpret_cast<intptr_t>(load + kOutlineLoadSize) -
                                                  reinterpret_cast<intptr_t>(jmp_back + 5));
    memcpy(jmp_back + 1, &back_rel, 4);
    FlushInstructionCache(GetCurrentProcess(), cave, 32);

    uint8_t jump[kOutlineLoadSize] = {0xE9, 0, 0, 0, 0, 0x90, 0x90};
    const int32_t cave_rel = static_cast<int32_t>(reinterpret_cast<intptr_t>(cave) -
                                                  reinterpret_cast<intptr_t>(load + 5));
    memcpy(jump + 1, &cave_rel, 4);

    memcpy(g_outline_original_bytes, load, kOutlineLoadSize);
    g_wall_radius_original_operand = *operand;
    const uint32_t radius_ptr = static_cast<uint32_t>(reinterpret_cast<uintptr_t>(&g_tob_wall_radius));
    if (!write_code(operand, &radius_ptr, 4)) {
        VirtualFree(cave, 0, MEM_RELEASE);
        return false;
    }
    if (!write_code(load, jump, kOutlineLoadSize)) {
        write_code(operand, &g_wall_radius_original_operand, 4);
        VirtualFree(cave, 0, MEM_RELEASE);
        return false;
    }
    g_outline_cave = cave;
    g_tob_killzone_on = true;
    return true;
}

void remove_tob_killzone() {
    if (!g_tob_killzone_on) return;
    write_code(reinterpret_cast<void*>(g_killzone_outline_index_addr), g_outline_original_bytes, kOutlineLoadSize);
    write_code(reinterpret_cast<void*>(g_killzone_wall_radius_addr + kWallRadiusOperandOffset),
               &g_wall_radius_original_operand, 4);
    // The cave is left allocated: a thread could still be inside it for a few instructions.
    g_tob_killzone_on = false;
    Log("[twdll] EnableToBCollapseKillZone: game kill zone restored");
}
} // namespace

// EMPIREBATTLE::MANAGER ctor/dtor signatures resolve directly to the function
// entry points, so the hooks install straight onto the resolved addresses.
void install_battle_hook() {
    const char* label = "BATTLE";

    if (!g_battle_ctor_addr || !g_battle_dtor_addr) {
        Log("[twdll] [%s] ctor/dtor signatures not resolved", label);
        return;
    }

    MH_STATUS mhs = MH_CreateHook(reinterpret_cast<void*>(g_battle_ctor_addr),
                                  reinterpret_cast<void*>(HookedBattleCtor),
                                  reinterpret_cast<void**>(&orig_battle_ctor));
    if (mhs == MH_OK) battle_ctor_addr = g_battle_ctor_addr;
    if (mhs != MH_OK) {
        Log("[twdll] [%s] MH_CreateHook (ctor) failed (%d)", label, mhs);
        return;
    }

    mhs = MH_EnableHook(reinterpret_cast<void*>(battle_ctor_addr));
    if (mhs != MH_OK) {
        Log("[twdll] [%s] MH_EnableHook (ctor) failed (%d)", label, mhs);
        return;
    }

    mhs = MH_CreateHook(reinterpret_cast<void*>(g_battle_dtor_addr),
                        reinterpret_cast<void*>(HookedBattleDtor),
                        reinterpret_cast<void**>(&orig_battle_dtor));
    if (mhs == MH_OK) battle_dtor_addr = g_battle_dtor_addr;
    if (mhs != MH_OK) {
        Log("[twdll] [%s] MH_CreateHook (dtor) failed (%d)", label, mhs);
        return;
    }

    mhs = MH_EnableHook(reinterpret_cast<void*>(battle_dtor_addr));
    if (mhs != MH_OK) {
        Log("[twdll] [%s] MH_EnableHook (dtor) failed (%d)", label, mhs);
        return;
    }

    Log("[twdll] [%s] hooks installed OK", label);
}

void uninstall_battle_hook() {
    uninstall_card_hook();
    remove_tob_killzone();

    if (battle_ctor_addr) {
        MH_DisableHook(reinterpret_cast<void*>(battle_ctor_addr));
        MH_RemoveHook(reinterpret_cast<void*>(battle_ctor_addr));
        battle_ctor_addr = 0;
    }
    if (battle_dtor_addr) {
        MH_DisableHook(reinterpret_cast<void*>(battle_dtor_addr));
        MH_RemoveHook(reinterpret_cast<void*>(battle_dtor_addr));
        battle_dtor_addr = 0;
    }
    g_battle = nullptr;
    orig_battle_ctor = nullptr;
    orig_battle_dtor = nullptr;
}

/***
Returns live runtime telemetry and reinforcement structure about the current tactical battle.
Returns nil while no battle is active (e.g. on the campaign map).

When inside a battle, returns a table with fields:
- `battle` (string): hexadecimal memory address of the `BATTLE` object.
- `manager` (string|nil): hexadecimal memory address of `REINFORCEMENTS_MANAGER`.
- `cap` (integer|nil): active maximum units per army in battle.
- `size` (integer|nil): number of reinforcing armies currently queued.
@function GetBattleInfo
@treturn table|nil table containing battle state, or nil when not in a tactical battle
@usage
-- Example returned table:
-- {
--     ["battle"] = "0x2A4F8900",
--     ["manager"] = "0x2A4F8B40",
--     ["cap"] = 40,
--     ["size"] = 2
-- }

local info = twdll.battle.GetBattleInfo()
if info then
    if info.size and info.size > 0 then
        -- Reinforcements are queued to enter the battlefield
    end
end
*/
static int GetBattleInfo(lua_State* L) {
    if (!g_battle) {
        l_pushnil(L);
        return 1;
    }

    char buf[20];
    l_createtable(L, 0, 4);

    snprintf(buf, sizeof(buf), "0x%08X", static_cast<unsigned int>(reinterpret_cast<uintptr_t>(g_battle)));
    l_pushstring(L, buf);
    l_setfield(L, -2, "battle");

    TW_ReinforcementsManager* mgr = g_battle->m_reinforcements_manager;
    if (mgr) {
        snprintf(buf, sizeof(buf), "0x%08X", static_cast<unsigned int>(reinterpret_cast<uintptr_t>(mgr)));
        l_pushstring(L, buf);
        l_setfield(L, -2, "manager");

        l_pushinteger(L, mgr->m_max_num_units_per_army);
        l_setfield(L, -2, "cap");

        l_pushinteger(L, mgr->m_reinforcements.m_size);
        l_setfield(L, -2, "size");
    } else {
        l_pushnil(L);
        l_setfield(L, -2, "manager");
        l_pushnil(L);
        l_setfield(L, -2, "cap");
        l_pushnil(L);
        l_setfield(L, -2, "size");
    }
    return 1;
}

/***
Updates health bars on unit cards and 3D banners in real time for 1-man units (SMEs, monsters, single heroes) based on remaining hit points.

@function EnableSmeHealthBars
@tparam[opt=true] boolean enabled whether to enable (true) or disable (false)
@treturn boolean true on success, false otherwise
@usage
-- In battle_scripted.lua:
local twdll = package.loadlib("twdll", "luaopen_twdll")()
twdll.battle.EnableSmeHealthBars(true)
*/
static int EnableSmeHealthBars(lua_State* L) {
    bool enable = true;
    if (l_type(L, 1) != LUA_TNONE && l_type(L, 1) != LUA_TNIL) {
        enable = l_tobool(L, 1);
    }
    g_sme_health_bars = enable;
    const bool ok = sync_card_hooks();
    if (enable && !ok) g_sme_health_bars = false;
    l_pushboolean(L, (ok || !enable) ? 1 : 0);
    return 1;
}

/***
Makes the health bar on battle unit cards show the unit's remaining hit points instead of
the share of men still alive. The number on the card (men, or artillery pieces) is unchanged.

The bar is the unit's hit points as a share of the unit at full strength: the living men's
hit points over their full hit points, times living men over full unit size, plus the
artillery pieces' hit points for artillery units. Wounded men lower the bar as well as dead
ones; a unit that starts the battle under strength starts below a full bar, as before. Mount
hit points are not counted (the game's own hit-point bar leaves them out too). The bar's
colour and its "Health" tooltip percentage follow the new value.

Display only: nothing in the battle simulation changes, so it is safe in multiplayer.
While the battle model is busy on the simulation thread the card keeps the value it shows.
The hook is removed when the last Lua state using twdll closes.
@function EnableUnitCardHitPoints
@tparam[opt=true] boolean enabled true to show hit points, false for the vanilla men bar
@treturn boolean|nil true when the requested state is in place, false if the hook could not
be installed, nil if the needed game functions were not found
@usage
-- In battle_scripted.lua:
local twdll = package.loadlib("twdll", "luaopen_twdll")()
twdll.battle.EnableUnitCardHitPoints(true)
*/
static int EnableUnitCardHitPoints(lua_State* L) {
    bool enable = true;
    if (l_type(L, 1) != LUA_TNONE && l_type(L, 1) != LUA_TNIL) {
        enable = l_tobool(L, 1);
    }
    if (enable && (!g_update_card_info_addr || !g_battle_health_bar_on_update_pulse_addr || !g_unit_calc_engine_hit_points_addr)) {
        Log("[twdll] EnableUnitCardHitPoints: game functions not found - vanilla unit card bars kept");
        l_pushnil(L);
        return 1;
    }
    g_unit_card_hit_points = enable;
    const bool ok = sync_card_hooks();
    if (enable && !ok) g_unit_card_hit_points = false;
    Log("[twdll] EnableUnitCardHitPoints(%d): unit card bars show %s", enable ? 1 : 0,
        g_unit_card_hit_points ? "hit points" : "men alive (vanilla)");
    l_pushboolean(L, (ok || !enable) ? 1 : 0);
    return 1;
}

/***
Makes the men killed around a destroyed wall or tower piece follow Thrones of Britannia instead
of Attila. When a piece is destroyed (by siege weapons, or a tower collapsing after its capture
point is taken), the game kills men nearby. ToB changed two things in that code, and this applies
them: men standing on a fort wall farther than 7 m from the piece only stumble (Attila: 10 m), and
the kill area is the outline of the piece as it stood instead of the destruction level it falls
to. Men on the piece itself (90 % chance) and men on the ground within 16.5 m still die, as in
both games.

Changes the battle simulation: in multiplayer every player must use the same setting.
Restored when the last Lua state using twdll closes.
@function EnableToBCollapseKillZone
@tparam[opt=true] boolean enabled true for ToB's rules, false for Attila's
@treturn boolean|nil true when the requested state is in place, false if patching failed, nil if
the code was not found in this game build
@usage
-- In battle_scripted.lua:
twdll.battle.EnableToBCollapseKillZone(true)
*/
static int EnableToBCollapseKillZone(lua_State* L) {
    bool enable = true;
    if (l_type(L, 1) != LUA_TNONE && l_type(L, 1) != LUA_TNIL) {
        enable = l_tobool(L, 1);
    }
    if (!g_killzone_outline_index_addr || !g_killzone_wall_radius_addr) {
        Log("[twdll] EnableToBCollapseKillZone: code not found - Attila kill zone kept");
        l_pushnil(L);
        return 1;
    }
    bool ok = true;
    if (enable) {
        ok = install_tob_killzone();
        Log("[twdll] EnableToBCollapseKillZone: %s", ok ? "ToB rules (7 m wall radius, standing outline)" : "patch failed - Attila kill zone kept");
    } else {
        remove_tob_killzone();
    }
    l_pushboolean(L, ok ? 1 : 0);
    return 1;
}

extern const luaL_Reg battle_functions[] = {
    {"GetBattleInfo",        GetBattleInfo},
    {"EnableSmeHealthBars",  EnableSmeHealthBars},
    {"EnableUnitCardHitPoints", EnableUnitCardHitPoints},
    {"EnableToBCollapseKillZone", EnableToBCollapseKillZone},
    {nullptr, nullptr}
};
