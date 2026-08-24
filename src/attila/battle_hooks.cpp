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

static void __fastcall HookedBattleHealthBarOnUpdatePulse(TW_BattleHealthBar* self, void* /*edx*/, int a2) {
    if (self && self->m_unit && self->m_unit->m_num_men_initial == 1) {
        self->m_is_using_hit_points = true;
    }
    orig_battle_health_bar_on_update_pulse(self, a2);
}

static int __fastcall HookedUpdateInformationChanged(TW_BattleLandUnitCardStyle* self, void* /*edx*/, TW_UnitCardUpdateInfo* info) {
    int res = orig_update_information_changed(self, info);

    if (self && self->m_unit && info) {
        TW_BattleUIUnit* ui_unit = self->m_unit;

        // Single Monster Entity check (Sauron / Single hero / 1-man monster)
        if (ui_unit->m_num_men_initial == 1 && ui_unit->m_unit) {
            TW_BattleUnit* bunit = ui_unit->m_unit;
            if (bunit->m_men.size() >= 1 && bunit->m_men.data() && bunit->m_men[0]) {
                TW_BattleEntity* ent = bunit->m_men[0];
                if (ent->m_full_hit_points > 0) {
                    info->percent_men_left = static_cast<float>(ent->m_hit_points) / static_cast<float>(ent->m_full_hit_points);
                }
            }
        }
    }
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

static bool install_card_hook() {
    bool card_ok = true;
    bool bar_ok = true;

    if (!update_card_info_hook_addr && g_update_card_info_addr) {
        MH_STATUS mhs = MH_CreateHook(reinterpret_cast<void*>(g_update_card_info_addr),
                                      reinterpret_cast<void*>(HookedUpdateInformationChanged),
                                      reinterpret_cast<void**>(&orig_update_information_changed));
        if (mhs == MH_OK) {
            update_card_info_hook_addr = g_update_card_info_addr;
            mhs = MH_EnableHook(reinterpret_cast<void*>(update_card_info_hook_addr));
            if (mhs == MH_OK) {
                Log("[twdll] [BATTLE] update_information_changed hook enabled via Lua API");
            } else {
                Log("[twdll] [BATTLE] MH_EnableHook (update_information_changed) failed (%d)", mhs);
                card_ok = false;
            }
        } else {
            Log("[twdll] [BATTLE] MH_CreateHook (update_information_changed) failed (%d)", mhs);
            card_ok = false;
        }
    }

    if (!battle_health_bar_hook_addr && g_battle_health_bar_on_update_pulse_addr) {
        MH_STATUS mhs = MH_CreateHook(reinterpret_cast<void*>(g_battle_health_bar_on_update_pulse_addr),
                                      reinterpret_cast<void*>(HookedBattleHealthBarOnUpdatePulse),
                                      reinterpret_cast<void**>(&orig_battle_health_bar_on_update_pulse));
        if (mhs == MH_OK) {
            battle_health_bar_hook_addr = g_battle_health_bar_on_update_pulse_addr;
            mhs = MH_EnableHook(reinterpret_cast<void*>(battle_health_bar_hook_addr));
            if (mhs == MH_OK) {
                Log("[twdll] [BATTLE] BattleHealthBar::OnUpdatePulse hook enabled via Lua API");
            } else {
                Log("[twdll] [BATTLE] MH_EnableHook (BattleHealthBar::OnUpdatePulse) failed (%d)", mhs);
                bar_ok = false;
            }
        } else {
            Log("[twdll] [BATTLE] MH_CreateHook (BattleHealthBar::OnUpdatePulse) failed (%d)", mhs);
            bar_ok = false;
        }
    }

    return card_ok && bar_ok;
}

static void uninstall_card_hook() {
    if (update_card_info_hook_addr) {
        MH_DisableHook(reinterpret_cast<void*>(update_card_info_hook_addr));
        MH_RemoveHook(reinterpret_cast<void*>(update_card_info_hook_addr));
        update_card_info_hook_addr = 0;
        Log("[twdll] [BATTLE] update_information_changed hook disabled");
    }
    orig_update_information_changed = nullptr;

    if (battle_health_bar_hook_addr) {
        MH_DisableHook(reinterpret_cast<void*>(battle_health_bar_hook_addr));
        MH_RemoveHook(reinterpret_cast<void*>(battle_health_bar_hook_addr));
        battle_health_bar_hook_addr = 0;
        Log("[twdll] [BATTLE] BattleHealthBar::OnUpdatePulse hook disabled");
    }
    orig_battle_health_bar_on_update_pulse = nullptr;
}

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
    if (enable) {
        bool ok = install_card_hook();
        l_pushboolean(L, ok ? 1 : 0);
    } else {
        uninstall_card_hook();
        l_pushboolean(L, 1);
    }
    return 1;
}

extern const luaL_Reg battle_functions[] = {
    {"GetBattleInfo",        GetBattleInfo},
    {"EnableSmeHealthBars",  EnableSmeHealthBars},
    {nullptr, nullptr}
};
