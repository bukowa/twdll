#include "../common/campaign_hooks.h"
#include "../common/log.h"
#include <windows.h>
#include <psapi.h>
#include <MinHook.h>

// Forward declarations to the game-specific modules
extern void install_world_hook(uintptr_t base, size_t size);
extern void install_campaign_ui_hook(uintptr_t base, size_t size);
extern void install_model_hook(uintptr_t base, size_t size);
extern void uninstall_world_hook();
extern void uninstall_campaign_ui_hook();
extern void uninstall_model_hook();

static const char* TARGET_MODULE = "empire.retail.dll";

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

    install_world_hook(base, size);
    install_campaign_ui_hook(base, size);
    install_model_hook(base, size);

    Log("[twdll] install_campaign_hooks: done");
}

void uninstall_campaign_hooks() {
    Log("[twdll] uninstall_campaign_hooks: starting");
    // Disable and remove installed hooks, then clear global pointers.
    uninstall_world_hook();
    uninstall_campaign_ui_hook();
    uninstall_model_hook();
    // Optionally deinitialize MinHook if no longer needed.
    MH_Uninitialize();
    Log("[twdll] uninstall_campaign_hooks: done");
}
