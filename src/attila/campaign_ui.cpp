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
#include <string>
#include <windows.h>

using twdll::TW_CampaignUi;
using twdll::TW_SettlementCallback;

static TW_CampaignUi* g_campaign_ui = nullptr;
static void* orig_campaign_ui_ctor = nullptr;
static uintptr_t campaign_ui_ctor_addr = 0;

static uintptr_t g_encyclopedia_url_ptr_addr = 0;
static const char* g_original_encyclopedia_url = nullptr;
static std::string g_custom_encyclopedia_url;

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
    // ---- Dynamic scan for Encyclopedia URL pointer --------------------------
    const char* enc_anchor = "http://Atenc.totalwar.com/#";
    uintptr_t enc_str_addr = Scanner::FindString(base, size, enc_anchor);
    if (enc_str_addr) {
        // Scan memory for the 4-byte pointer referencing this string
        const uint8_t* start = reinterpret_cast<const uint8_t*>(base);
        const uint8_t* end = start + size - sizeof(uint32_t);
        for (const uint8_t* p = start; p <= end; ++p) {
            if (*reinterpret_cast<const uint32_t*>(p) == static_cast<uint32_t>(enc_str_addr)) {
                g_encyclopedia_url_ptr_addr = reinterpret_cast<uintptr_t>(p);
                g_original_encyclopedia_url = reinterpret_cast<const char*>(enc_str_addr);
                Log("[twdll] [ENCYCLOPEDIA_URL] resolved dynamically at 0x%08X -> '%s'",
                    static_cast<unsigned int>(g_encyclopedia_url_ptr_addr),
                    g_original_encyclopedia_url);
                break;
            }
        }
    }
    if (!g_encyclopedia_url_ptr_addr) {
        Log("[twdll] [ENCYCLOPEDIA_URL] pointer reference not resolved via scan");
    }

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
}

// ---- Settlement max-slot override ------------------------------------------
// CampaignSettlementCallback::Initialize renders the building-slot list with a
// loop bound of m_max_slots (this+0x48), re-read every iteration. Hooking its
// entry and writing m_max_slots before the original runs lets us override the
// slot count on every panel open/refresh. 0 = use the game default (4/6).
//
// The major/minor split uses m_is_capital (this+0x44): the main/selected
// settlement card is initialised via SetAsCapital (m_is_capital=1) before
// Initialize, while the other province cards keep m_is_capital=0 (Construct).
static int g_settlement_max_slots_override_major = 0;
static int g_settlement_max_slots_override_minor = 0;
static void* orig_settlement_cb_initialize = nullptr;
static uintptr_t settlement_cb_initialize_addr = 0;

static void ApplySlotOverride(void* callback) {
    auto* cb = static_cast<TW_SettlementCallback*>(callback);
    int override = cb->m_is_capital ? g_settlement_max_slots_override_major
                                    : g_settlement_max_slots_override_minor;
    if (override <= 0) return;
    cb->m_max_slots = override;
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
    g_settlement_max_slots_override_major = 0;
    g_settlement_max_slots_override_minor = 0;
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
Overrides the maximum number of building slots shown on the main (capital)
settlement card in the settlement panel. Clamped to 1..20. Applies from the
next panel open/refresh onwards. Values below 1 are rejected (no-op).
@function SetMaxSlotsMajor
@tparam number slots max slot count to display
@treturn number the applied value, or nil if rejected
*/
static int SetMaxSlotsMajor(lua_State* L) {
    int val = static_cast<int>(l_tointeger(L, 1));
    if (val < 1) { l_pushnil(L); return 1; }
    if (val > 20) val = 20;
    g_settlement_max_slots_override_major = val;
    Log("[twdll] SetMaxSlotsMajor: override = %d", val);
    l_pushinteger(L, val);
    return 1;
}

/***
Overrides the maximum number of building slots shown on the minor settlement
cards in the settlement panel. Clamped to 1..20. Applies from the next panel
open/refresh onwards. Values below 1 are rejected (no-op).
@function SetMaxSlotsMinor
@tparam number slots max slot count to display
@treturn number the applied value, or nil if rejected
*/
static int SetMaxSlotsMinor(lua_State* L) {
    int val = static_cast<int>(l_tointeger(L, 1));
    if (val < 1) { l_pushnil(L); return 1; }
    if (val > 20) val = 20;
    g_settlement_max_slots_override_minor = val;
    Log("[twdll] SetMaxSlotsMinor: override = %d", val);
    l_pushinteger(L, val);
    return 1;
}

/***
Returns the currently configured major-slot override, or nil if none is set.
@function GetMaxSlotsMajor
@treturn number|nil slots
*/
static int GetMaxSlotsMajor(lua_State* L) {
    if (g_settlement_max_slots_override_major <= 0) { l_pushnil(L); return 1; }
    l_pushinteger(L, g_settlement_max_slots_override_major);
    return 1;
}

/***
Returns the currently configured minor-slot override, or nil if none is set.
@function GetMaxSlotsMinor
@treturn number|nil slots
*/
static int GetMaxSlotsMinor(lua_State* L) {
    if (g_settlement_max_slots_override_minor <= 0) { l_pushnil(L); return 1; }
    l_pushinteger(L, g_settlement_max_slots_override_minor);
    return 1;
}

/***
Restores the game default max-slot count (4, or 6 for the main settlement card).
@function ClearMaxSlots
*/
static int ClearMaxSlots(lua_State* L) {
    g_settlement_max_slots_override_major = 0;
    g_settlement_max_slots_override_minor = 0;
    l_pushnil(L);
    return 1;
}

/***
Forces an immediate visual refresh of all settlement building models on the campaign map.
@function RefreshSettlements
*/
static int RefreshSettlements(lua_State*) {
    refresh_settlements_display();
    return 0;
}

/***
Sets the base URL prefix for the in-game encyclopedia (defaults to "http://Atenc.totalwar.com/#").
If a custom URL is provided, all in-game encyclopedia links (unit cards, buildings, technologies,
and the main encyclopedia button) will open using this base URL. Passing nil or an empty string
restores the original default URL.
@function SetEncyclopediaUrl
@tparam string|nil url new base encyclopedia URL (e.g. "http://localhost:8080/#")
@treturn string the currently applied base URL
*/
static int SetEncyclopediaUrl(lua_State* L) {
    if (!g_encyclopedia_url_ptr_addr) {
        Log("[twdll] SetEncyclopediaUrl: pointer not resolved");
        l_pushnil(L);
        return 1;
    }
    const char* new_url = nullptr;
    if (l_type(L, 1) == LUA_TSTRING) {
        size_t len = 0;
        new_url = l_checklstring(L, 1, &len);
    }

    DWORD old_prot = 0;
    if (!VirtualProtect(reinterpret_cast<void*>(g_encyclopedia_url_ptr_addr), sizeof(const char*), PAGE_EXECUTE_READWRITE, &old_prot)) {
        Log("[twdll] SetEncyclopediaUrl: VirtualProtect failed");
        l_pushnil(L);
        return 1;
    }

    if (new_url && *new_url != '\0') {
        g_custom_encyclopedia_url = new_url;
        *reinterpret_cast<const char**>(g_encyclopedia_url_ptr_addr) = g_custom_encyclopedia_url.c_str();
        Log("[twdll] SetEncyclopediaUrl: updated to '%s'", g_custom_encyclopedia_url.c_str());
    } else {
        g_custom_encyclopedia_url.clear();
        *reinterpret_cast<const char**>(g_encyclopedia_url_ptr_addr) = g_original_encyclopedia_url;
        Log("[twdll] SetEncyclopediaUrl: restored default '%s'", g_original_encyclopedia_url ? g_original_encyclopedia_url : "null");
    }

    VirtualProtect(reinterpret_cast<void*>(g_encyclopedia_url_ptr_addr), sizeof(const char*), old_prot, &old_prot);

    const char* current = *reinterpret_cast<const char**>(g_encyclopedia_url_ptr_addr);
    if (current) {
        l_pushstring(L, current);
    } else {
        l_pushnil(L);
    }
    return 1;
}

/***
Returns the current base URL prefix for the in-game encyclopedia.
@function GetEncyclopediaUrl
@treturn string current base encyclopedia URL
*/
static int GetEncyclopediaUrl(lua_State* L) {
    if (!g_encyclopedia_url_ptr_addr) {
        l_pushnil(L);
        return 1;
    }
    const char* current = *reinterpret_cast<const char**>(g_encyclopedia_url_ptr_addr);
    if (current) {
        l_pushstring(L, current);
    } else {
        l_pushnil(L);
    }
    return 1;
}

extern const luaL_Reg campaign_ui_functions[] = {
    {"GetMemoryAddress",    GetMemoryAddress},
    {"ClearMaxSlots",       ClearMaxSlots},
    {"SetMaxSlotsMajor",    SetMaxSlotsMajor},
    {"GetMaxSlotsMajor",    GetMaxSlotsMajor},
    {"SetMaxSlotsMinor",    SetMaxSlotsMinor},
    {"GetMaxSlotsMinor",    GetMaxSlotsMinor},
    {"RefreshSettlements",  RefreshSettlements},
    {"SetEncyclopediaUrl",  SetEncyclopediaUrl},
    {"GetEncyclopediaUrl",  GetEncyclopediaUrl},
    {nullptr, nullptr}
};

// Uninstall hook and clear global pointer
void uninstall_campaign_ui_hook() {
    if (g_encyclopedia_url_ptr_addr && g_original_encyclopedia_url) {
        DWORD old_prot = 0;
        if (VirtualProtect(reinterpret_cast<void*>(g_encyclopedia_url_ptr_addr), sizeof(const char*), PAGE_EXECUTE_READWRITE, &old_prot)) {
            *reinterpret_cast<const char**>(g_encyclopedia_url_ptr_addr) = g_original_encyclopedia_url;
            VirtualProtect(reinterpret_cast<void*>(g_encyclopedia_url_ptr_addr), sizeof(const char*), old_prot, &old_prot);
        }
    }
    g_custom_encyclopedia_url.clear();
    g_encyclopedia_url_ptr_addr = 0;
    g_original_encyclopedia_url = nullptr;

    if (campaign_ui_ctor_addr) {
        MH_DisableHook(reinterpret_cast<void*>(campaign_ui_ctor_addr));
        MH_RemoveHook(reinterpret_cast<void*>(campaign_ui_ctor_addr));
        campaign_ui_ctor_addr = 0;
    }
    g_campaign_ui = nullptr;
    orig_campaign_ui_ctor = nullptr;
}
