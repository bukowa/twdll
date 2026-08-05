/// @module twdll.campaign_ui
/// Campaign UI singleton accessor for Total War: Attila.
#include "common/campaign_hooks.h"
#include "common/lua_api.h"
#include "common/log.h"
#include "game_api.h"
#include "tw_types.h"
#include "common/signature_scanner.h"
#include <MinHook.h>

#include <cstdio>

using twdll::TW_CampaignUi;
using twdll::TW_SettlementCallback;

static TW_CampaignUi* g_campaign_ui = nullptr;
static void* orig_campaign_ui_ctor = nullptr;
static uintptr_t campaign_ui_ctor_addr = 0;

static void LogCampaignUiHook(void* ptr) {
    g_campaign_ui = static_cast<TW_CampaignUi*>(ptr);
    Log("[twdll] CAMPAIGN_UI ctor hooked — g_campaign_ui = 0x%08X", reinterpret_cast<uintptr_t>(ptr));
}

__declspec(naked) static void HookedCampaignUiCtor() {
    __asm {
        pushad
        push ecx
        call LogCampaignUiHook
        add esp, 4
        popad
        jmp dword ptr [orig_campaign_ui_ctor]
    }
}

void install_campaign_ui_hook(uintptr_t base, size_t size) {
    // ---- Explicit, non‑abstracted hook installation -------------------------
    const char* anchor = "data/ui/campaign ui/mp_timer";
    const char* label  = "CAMPAIGN_UI";

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
                                   reinterpret_cast<void*>(HookedCampaignUiCtor),
                                   reinterpret_cast<void**>(&orig_campaign_ui_ctor));
    if (mhs != MH_OK) {
        Log("[twdll] [%s] MH_CreateHook failed (%d)", label, mhs);
        return;
    }
    // Store the address where the hook was installed for later removal
    campaign_ui_ctor_addr = ctor_addr;

    mhs = MH_EnableHook(reinterpret_cast<void*>(ctor_addr));
    if (mhs != MH_OK) {
        Log("[twdll] [%s] MH_EnableHook failed (%d)", label, mhs);
        return;
    }

    Log("[twdll] [%s] hook installed OK", label);
    // ------------------------------------------------------------------------
}

// ---- Settlement max-slot override ------------------------------------------
// CampaignSettlementCallback::Initialize renders the building-slot list with a
// loop bound of m_max_slots (this+0x48), re-read every iteration. Hooking its
// entry and writing m_max_slots before the original runs lets us override the
// slot count on every panel open/refresh. 0 = use the game default (4/6).
static int g_settlement_max_slots_override = 0;
static void* orig_settlement_cb_initialize = nullptr;
static uintptr_t settlement_cb_initialize_addr = 0;

static void ApplySlotOverride(void* callback) {
    if (g_settlement_max_slots_override <= 0) return;
    static_cast<TW_SettlementCallback*>(callback)->m_max_slots = g_settlement_max_slots_override;
}

__declspec(naked) static void HookedSettlementCallbackInitialize() {
    __asm {
        pushad
        push ecx
        call ApplySlotOverride
        add esp, 4
        popad
        jmp dword ptr [orig_settlement_cb_initialize]
    }
}

void install_settlement_slots_hook(uintptr_t base, size_t size) {
    const char* label = "SETTLEMENT_SLOTS";

    // Target resolved by initialize_game_api() signature scan (game_sigs.cpp).
    (void)base;
    (void)size;

    if (!g_settlement_cb_initialize_addr) {
        Log("[twdll] [%s] signature not resolved", label);
        return;
    }
    settlement_cb_initialize_addr = g_settlement_cb_initialize_addr;

    MH_STATUS mhs = MH_CreateHook(reinterpret_cast<void*>(settlement_cb_initialize_addr),
                                  reinterpret_cast<void*>(HookedSettlementCallbackInitialize),
                                  reinterpret_cast<void**>(&orig_settlement_cb_initialize));
    if (mhs != MH_OK) {
        Log("[twdll] [%s] MH_CreateHook failed (%d)", label, mhs);
        return;
    }

    mhs = MH_EnableHook(reinterpret_cast<void*>(settlement_cb_initialize_addr));
    if (mhs != MH_OK) {
        Log("[twdll] [%s] MH_EnableHook failed (%d)", label, mhs);
        return;
    }

    Log("[twdll] [%s] hook installed OK at 0x%08X", label, static_cast<unsigned int>(settlement_cb_initialize_addr));
}

void uninstall_settlement_slots_hook() {
    if (settlement_cb_initialize_addr) {
        MH_DisableHook(reinterpret_cast<void*>(settlement_cb_initialize_addr));
        MH_RemoveHook(reinterpret_cast<void*>(settlement_cb_initialize_addr));
        settlement_cb_initialize_addr = 0;
    }
    orig_settlement_cb_initialize = nullptr;
    g_settlement_max_slots_override = 0;
}

/***
Returns the memory address of the CAMPAIGN_UI singleton as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678"), or nil if not yet initialised
*/
static int GetMemoryAddress(lua_State* L) {
    if (!g_campaign_ui) { l_pushnil(L); return 1; }
    char buf[20];
    snprintf(buf, sizeof(buf), "0x%08X", reinterpret_cast<unsigned int>(g_campaign_ui));
    l_pushstring(L, buf);
    return 1;
}

/***
Overrides the maximum number of building slots shown in the settlement panel.
Clamped to 1..20. Applies from the next panel open/refresh onwards.
Values below 1 are rejected (no-op). Use ClearMaxSlots to restore the default.
@function SetMaxSlots
@tparam number slots max slot count to display
@treturn number the applied value, or nil if rejected
*/
static int SetMaxSlots(lua_State* L) {
    int val = static_cast<int>(l_tointeger(L, 1));
    if (val < 1) { l_pushnil(L); return 1; }
    if (val > 20) val = 20;
    g_settlement_max_slots_override = val;
    Log("[twdll] SetMaxSlots: override = %d", val);
    l_pushinteger(L, val);
    return 1;
}

/***
Returns the currently configured max-slot override, or nil if none is set.
@function GetMaxSlots
@treturn number|nil slots
*/
static int GetMaxSlots(lua_State* L) {
    if (g_settlement_max_slots_override <= 0) { l_pushnil(L); return 1; }
    l_pushinteger(L, g_settlement_max_slots_override);
    return 1;
}

/***
Restores the game default max-slot count (4, or 6 for capitals).
@function ClearMaxSlots
*/
static int ClearMaxSlots(lua_State* L) {
    g_settlement_max_slots_override = 0;
    l_pushnil(L);
    return 1;
}

extern const luaL_Reg campaign_ui_functions[] = {
    {"GetMemoryAddress", GetMemoryAddress},
    {"SetMaxSlots",      SetMaxSlots},
    {"GetMaxSlots",      GetMaxSlots},
    {"ClearMaxSlots",    ClearMaxSlots},
    {nullptr, nullptr}
};

// Uninstall hook and clear global pointer
void uninstall_campaign_ui_hook() {
    if (campaign_ui_ctor_addr) {
        MH_DisableHook(reinterpret_cast<void*>(campaign_ui_ctor_addr));
        MH_RemoveHook(reinterpret_cast<void*>(campaign_ui_ctor_addr));
        campaign_ui_ctor_addr = 0;
    }
    g_campaign_ui = nullptr;
    orig_campaign_ui_ctor = nullptr;
}
