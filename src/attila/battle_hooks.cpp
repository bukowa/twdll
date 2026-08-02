/// @module twdll.battle
/// Battle lifecycle hooks for Total War: Attila.
#include "common/tw.h"
#include "common/campaign_hooks.h"
#include "game_api.h"
#include "tw_types.h"
#include <MinHook.h>

#include <windows.h>
#include <cstdio>

using twdll::TW_Battle;
using twdll::TW_ReinforcementsManager;

// Tentative stride of a REINFORCEMENT_ARMY element inside
// REINFORCEMENTS_MANAGER::m_reinforcements (32-bit layout).
static constexpr uintptr_t kReinforcementArmyStride = 0x28;

static TW_Battle* g_battle = nullptr;
static void*      orig_battle_ctor = nullptr;
static void*      orig_battle_dtor = nullptr;
static uintptr_t  battle_ctor_addr = 0;
static uintptr_t  battle_dtor_addr = 0;

static void LogBattleCtor(void* ptr) {
    g_battle = static_cast<TW_Battle*>(ptr);
    Log("[twdll] BATTLE ctor hooked — g_battle = 0x%08X",
        static_cast<unsigned int>(reinterpret_cast<uintptr_t>(ptr)));
}

static void LogBattleDtor(void* ptr) {
    Log("[twdll] BATTLE dtor hooked — 0x%08X",
        static_cast<unsigned int>(reinterpret_cast<uintptr_t>(ptr)));

    TW_Battle* battle = static_cast<TW_Battle*>(ptr);
    if (battle && battle->m_reinforcements_manager) {
        TW_ReinforcementsManager* mgr = battle->m_reinforcements_manager;
        Log("[twdll]  reinforcements manager = 0x%08X, cap = %d, size = %d, elements = 0x%08X",
            static_cast<unsigned int>(reinterpret_cast<uintptr_t>(mgr)),
            mgr->m_max_num_units_per_army,
            mgr->m_reinforcements.m_size,
            static_cast<unsigned int>(reinterpret_cast<uintptr_t>(mgr->m_reinforcements.m_elements)));

        if (mgr->m_reinforcements.m_elements && mgr->m_reinforcements.m_size > 0) {
            uintptr_t base = reinterpret_cast<uintptr_t>(mgr->m_reinforcements.m_elements);
            const int n = mgr->m_reinforcements.m_size > 8 ? 8 : mgr->m_reinforcements.m_size;
            for (int i = 0; i < n; ++i) {
                Log("[twdll]  reinforcement[%d] @ 0x%08X", i,
                    static_cast<unsigned int>(base + static_cast<uintptr_t>(i) * kReinforcementArmyStride));
            }
        }
    } else {
        Log("[twdll]  reinforcements manager = null");
    }

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
Returns live information about the current battle. Returns nil while no battle
is active (e.g. in the campaign). Inside a battle, returns a table with the
battle object address, the reinforcements manager address, the reinforcement
cap and the number of reinforcement armies currently waiting.
@function GetBattleInfo
@treturn[opt] table battle info, or nil when no battle is active
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

extern const luaL_Reg battle_functions[] = {
    {"GetBattleInfo", GetBattleInfo},
    {nullptr, nullptr}
};
