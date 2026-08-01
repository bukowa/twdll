#include "../common/campaign_hooks.h"
#include "../common/log.h"
#include <windows.h>
#include <psapi.h>
#include <MinHook.h>
#include <cstring>

// Forward declarations to the game-specific modules
extern void install_world_hook(uintptr_t base, size_t size);
extern void install_campaign_ui_hook(uintptr_t base, size_t size);
extern void install_model_hook(uintptr_t base, size_t size);
extern void uninstall_world_hook();
extern void uninstall_campaign_ui_hook();
extern void uninstall_model_hook();

static const char* TARGET_MODULE = "empire.retail.dll";

namespace {

// REINFORCEMENTS_MANAGER ctor (sub_102D19D0) at RVA 0x2D1A77:
//   mov eax, [eax+0x13C]   (8B 80 3C 01 00 00)  -- load max units per army from battle setup
//   mov [esi+0x28], eax    (89 46 28)            -- store into manager field
// Patch the load to mov eax, 0x7FFFFFFF (B8 FF FF FF 7F) + nop so the deploy gate
// (m_size >= m_max_num_units_per_army) never blocks reinforcements.
constexpr uintptr_t RVA_REINF_CAP = 0x2D1A77;

void PatchReinforcementCap(uintptr_t base) {
    uintptr_t addr = base + RVA_REINF_CAP;
    const uint8_t expected[6] = { 0x8B, 0x80, 0x3C, 0x01, 0x00, 0x00 };
    const uint8_t patch[6] = { 0xB8, 0xFF, 0xFF, 0xFF, 0x7F, 0x90 };

    if (memcmp(reinterpret_cast<void*>(addr), expected, sizeof(expected)) != 0) {
        Log("[twdll] PatchReinforcementCap: unexpected bytes at RVA 0x%X - skipping", RVA_REINF_CAP);
        return;
    }

    DWORD oldProtect = 0;
    if (!VirtualProtect(reinterpret_cast<void*>(addr), sizeof(patch), PAGE_EXECUTE_READWRITE, &oldProtect)) {
        Log("[twdll] PatchReinforcementCap: VirtualProtect failed (%lu)", GetLastError());
        return;
    }
    memcpy(reinterpret_cast<void*>(addr), patch, sizeof(patch));
    VirtualProtect(reinterpret_cast<void*>(addr), sizeof(patch), oldProtect, &oldProtect);
    Log("[twdll] PatchReinforcementCap: patched RVA 0x%X (unlimited battlefield units)", RVA_REINF_CAP);
}

} // namespace

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

    // PatchReinforcementCap(base);

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
