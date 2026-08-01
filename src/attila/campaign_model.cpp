/// CAMPAIGN_MODEL singleton hook — provides the live CAMPAIGN_MODEL pointer.
#include "../common/tw.h"
#include "../common/signature_scanner.h"
#include <MinHook.h>

#include <windows.h>

static void* orig_model_ctor_1 = nullptr;
static void* orig_model_ctor_2 = nullptr;
static uintptr_t model_ctor_1_addr = 0;
static uintptr_t model_ctor_2_addr = 0;

void* g_campaign_model = nullptr;

static void LogModelHook(void* ptr) {
    g_campaign_model = ptr;
    Log("[twdll] CAMPAIGN_MODEL ctor hooked — g_campaign_model = 0x%08X", reinterpret_cast<uintptr_t>(ptr));
}

__declspec(naked) static void HookedModelCtor1() {
    __asm {
        pushad
        push ecx
        call LogModelHook
        add esp, 4
        popad
        jmp dword ptr [orig_model_ctor_1]
    }
}

__declspec(naked) static void HookedModelCtor2() {
    __asm {
        pushad
        push ecx
        call LogModelHook
        add esp, 4
        popad
        jmp dword ptr [orig_model_ctor_2]
    }
}

static void InstallOneHook(uintptr_t addr, void* hook, void** orig, uintptr_t* slot, const char* label) {
    if (!addr) {
        Log("[twdll] CAMPAIGN_MODEL [%s] address not resolved", label);
        return;
    }
    MH_STATUS mhs = MH_CreateHook(reinterpret_cast<void*>(addr), hook, orig);
    if (mhs != MH_OK) {
        Log("[twdll] CAMPAIGN_MODEL [%s] MH_CreateHook failed (%d)", label, mhs);
        return;
    }
    mhs = MH_EnableHook(reinterpret_cast<void*>(addr));
    if (mhs != MH_OK) {
        Log("[twdll] CAMPAIGN_MODEL [%s] MH_EnableHook failed (%d)", label, mhs);
        return;
    }
    *slot = addr;
    Log("[twdll] CAMPAIGN_MODEL [%s] hook installed at 0x%08X", label, static_cast<unsigned int>(addr));
}

void install_model_hook(uintptr_t base, size_t size) {
    Log("[twdll] Installing CAMPAIGN_MODEL hooks");

    // C1: CAMPAIGN_MODEL ctor for a new campaign (creates WORLD inside).
    const char* sig1 = "81 EC B4 02 00 00 53";
    uintptr_t addr1 = Scanner::find_signature(base, size, sig1);
    InstallOneHook(addr1, reinterpret_cast<void*>(HookedModelCtor1),
                   &orig_model_ctor_1, &model_ctor_1_addr, "C1");

    // C2: CAMPAIGN_MODEL ctor used when loading a save (EmpireFileInSection).
    const char* sig2 = "83 EC 38 53 55 8B 6C 24 ? 8B D9 56 57 55 8D 8B";
    uintptr_t addr2 = Scanner::find_signature(base, size, sig2);
    InstallOneHook(addr2, reinterpret_cast<void*>(HookedModelCtor2),
                   &orig_model_ctor_2, &model_ctor_2_addr, "C2");

    Log("[twdll] install_model_hook: done");
}

void uninstall_model_hook() {
    if (model_ctor_1_addr) {
        MH_DisableHook(reinterpret_cast<void*>(model_ctor_1_addr));
        MH_RemoveHook(reinterpret_cast<void*>(model_ctor_1_addr));
        model_ctor_1_addr = 0;
    }
    if (model_ctor_2_addr) {
        MH_DisableHook(reinterpret_cast<void*>(model_ctor_2_addr));
        MH_RemoveHook(reinterpret_cast<void*>(model_ctor_2_addr));
        model_ctor_2_addr = 0;
    }
    g_campaign_model = nullptr;
    orig_model_ctor_1 = nullptr;
    orig_model_ctor_2 = nullptr;
}
