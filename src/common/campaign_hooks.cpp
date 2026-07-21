#include "campaign_hooks.h"
#include "log.h"
#include "signature_scanner.h"

#include <windows.h>
#include <psapi.h>
#include <MinHook.h>

// ── Target Module ─────────────────────────────────────────────────────────────
// Both Rome 2 and Attila use empire.retail.dll for the main campaign logic.
static const char* TARGET_MODULE = "empire.retail.dll";

// ── Global singletons ─────────────────────────────────────────────────────────
void* g_world       = nullptr;
void* g_campaign_ui = nullptr;

static void* orig_world_ctor       = nullptr;
static void* orig_campaign_ui_ctor = nullptr;

static void LogWorldHook(void* ptr) {
    g_world = ptr;
    Log("[twdll] WORLD ctor hooked — g_world = 0x%08X", reinterpret_cast<uintptr_t>(ptr));
}

static void LogCampaignUiHook(void* ptr) {
    g_campaign_ui = ptr;
    Log("[twdll] CAMPAIGN_UI ctor hooked — g_campaign_ui = 0x%08X", reinterpret_cast<uintptr_t>(ptr));
}

// ── Naked Hooks (stack-safe for any signature) ────────────────────────────────
__declspec(naked) void HookedWorldCtor() {
    __asm {
        pushad
        push ecx
        call LogWorldHook
        add esp, 4
        popad
        jmp dword ptr [orig_world_ctor]
    }
}

__declspec(naked) void HookedCampaignUiCtor() {
    __asm {
        pushad
        push ecx
        call LogCampaignUiHook
        add esp, 4
        popad
        jmp dword ptr [orig_campaign_ui_ctor]
    }
}

// ── Anchor descriptor ─────────────────────────────────────────────────────────
struct AnchorHook {
    const char* anchor_string;
    const char* label;
    void*       hook_fn;
    void**      orig_fn;
};

// ── Public init ───────────────────────────────────────────────────────────────
void install_campaign_hooks() {
    Log("[twdll] install_campaign_hooks: starting for %s", TARGET_MODULE);

    HMODULE hMod = GetModuleHandleA(TARGET_MODULE);
    if (!hMod) {
        Log("[twdll] install_campaign_hooks: %s not loaded yet — skipping", TARGET_MODULE);
        return;
    }

    MODULEINFO mi = {};
    if (!GetModuleInformation(GetCurrentProcess(), hMod, &mi, sizeof(mi))) {
        Log("[twdll] install_campaign_hooks: GetModuleInformation failed (%lu)", GetLastError());
        return;
    }

    uintptr_t base = reinterpret_cast<uintptr_t>(hMod);
    size_t    size = mi.SizeOfImage;

    MH_STATUS mhs = MH_Initialize();
    if (mhs != MH_OK && mhs != MH_ERROR_ALREADY_INITIALIZED) {
        Log("[twdll] install_campaign_hooks: MH_Initialize failed (%d)", mhs);
        return;
    }

    AnchorHook anchors[] = {
        {
            "FACTION_ARRAY", "WORLD",
            reinterpret_cast<void*>(HookedWorldCtor),
            reinterpret_cast<void**>(&orig_world_ctor),
        },
        {
            "data/ui/campaign ui/mp_timer", "CAMPAIGN_UI",
            reinterpret_cast<void*>(HookedCampaignUiCtor),
            reinterpret_cast<void**>(&orig_campaign_ui_ctor),
        },
    };

    for (auto& a : anchors) {
        Log("[twdll] Processing anchor '%s' for %s ctor", a.anchor_string, a.label);

        uintptr_t str_addr = Scanner::FindString(base, size, a.anchor_string);
        if (!str_addr) {
            Log("[twdll] [%s] anchor string not found", a.label);
            continue;
        }

        uintptr_t push_addr = Scanner::FindPushRef(base, size, str_addr);
        if (!push_addr) {
            Log("[twdll] [%s] push ref not found", a.label);
            continue;
        }

        uintptr_t ctor_addr = Scanner::FindPrologue(push_addr);
        if (!ctor_addr) {
            Log("[twdll] [%s] prologue not found", a.label);
            continue;
        }

        mhs = MH_CreateHook(reinterpret_cast<void*>(ctor_addr), a.hook_fn, a.orig_fn);
        if (mhs != MH_OK) {
            Log("[twdll] [%s] MH_CreateHook failed (%d)", a.label, mhs);
            continue;
        }

        mhs = MH_EnableHook(reinterpret_cast<void*>(ctor_addr));
        if (mhs != MH_OK) {
            Log("[twdll] [%s] MH_EnableHook failed (%d)", a.label, mhs);
            continue;
        }
        Log("[twdll] [%s] hook installed OK", a.label);
    }
    Log("[twdll] install_campaign_hooks: done");
}
