/// CAMPAIGN_MODEL singleton hook — provides the live CAMPAIGN_MODEL pointer.
#include "../common/tw.h"
#include "../common/signature_scanner.h"
#include <MinHook.h>

#include <windows.h>

static void* orig_model_ctor = nullptr;
static uintptr_t model_ctor_addr = 0;

void* g_campaign_model = nullptr;

static void LogModelHook(void* ptr) {
    g_campaign_model = ptr;
    Log("[twdll] CAMPAIGN_MODEL ctor (load-save) hooked - g_campaign_model = 0x%08X",
        reinterpret_cast<uintptr_t>(ptr));
}

__declspec(naked) static void HookedModelCtor() {
    __asm {
        pushad
        push ecx
        call LogModelHook
        add esp, 4
        popad
        jmp dword ptr [orig_model_ctor]
    }
}

void install_model_hook(uintptr_t base, size_t size) {
    Log("[twdll] Installing CAMPAIGN_MODEL hook");

    // This is the CAMPAIGN_MODEL ctor used whenever a campaign is started or
    // loaded ("CAMPAIGN MODEL LOAD"). A "new campaign" also runs through this
    // path: the game loads the campaign's startpos.esf (a save-format file)
    // with the same loader. The separate "CAMPAIGN MODEL CREATION" ctor is
    // only reachable from CA's dev-only batch startpos preprocess tool, so it
    // is intentionally not hooked.
    const char* sig = "81 EC B4 02 00 00 53";
    uintptr_t addr = Scanner::find_signature(base, size, sig);
    if (!addr) {
        Log("[twdll] CAMPAIGN_MODEL address not resolved");
        return;
    }

    MH_STATUS mhs = MH_CreateHook(reinterpret_cast<void*>(addr), HookedModelCtor, &orig_model_ctor);
    if (mhs != MH_OK) {
        Log("[twdll] CAMPAIGN_MODEL MH_CreateHook failed (%d)", mhs);
        return;
    }
    mhs = MH_EnableHook(reinterpret_cast<void*>(addr));
    if (mhs != MH_OK) {
        Log("[twdll] CAMPAIGN_MODEL MH_EnableHook failed (%d)", mhs);
        return;
    }
    model_ctor_addr = addr;
    Log("[twdll] CAMPAIGN_MODEL hook installed at 0x%08X", static_cast<unsigned int>(addr));
}

void uninstall_model_hook() {
    if (model_ctor_addr) {
        MH_DisableHook(reinterpret_cast<void*>(model_ctor_addr));
        MH_RemoveHook(reinterpret_cast<void*>(model_ctor_addr));
        model_ctor_addr = 0;
    }
    g_campaign_model = nullptr;
    orig_model_ctor = nullptr;
}
