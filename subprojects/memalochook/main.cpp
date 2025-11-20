#include <windows.h>
#include <shlwapi.h> // For PathFindFileNameA

#pragma comment(lib, "shlwapi.lib")

#include "MinHook.h"
#include "hook_manager.h"
#include "log.h"

// =================================================================================
// LoadLibraryA Hook (Our Trigger)
// =================================================================================
typedef HMODULE(WINAPI *t_LoadLibraryA)(LPCSTR lpLibFileName);
t_LoadLibraryA o_LoadLibraryA = nullptr;

HMODULE WINAPI HookedLoadLibraryA(LPCSTR lpLibFileName) {
    HMODULE hModule = o_LoadLibraryA(lpLibFileName);
    if (hModule && lpLibFileName) {
        const char* moduleName = PathFindFileNameA(lpLibFileName);
        if (_stricmp(moduleName, "empire.retail.dll") == 0) {
            spdlog::info("empire.retail.dll loaded, installing hooks...");
            // Now that the game DLL is loaded, install hooks for it.
            InstallHooksForModule(moduleName, hModule);
            MH_EnableHook(MH_ALL_HOOKS);

            // We can disable the LoadLibraryA hook now as we're done.
            MH_DisableHook(
                (LPVOID)GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA"));
        }
    }
    return hModule;
}

// =================================================================================
// DLL Entry Point (Minimal and Safe)
// =================================================================================
BOOL APIENTRY DllMain(HMODULE hModule, DWORD ul_reason_for_call, LPVOID lpReserved) {
    if (ul_reason_for_call == DLL_PROCESS_ATTACH) {
        InitializeLogging();
        spdlog::info("memalochook.dll attached.");

        DisableThreadLibraryCalls(hModule);
        MH_Initialize();

        // Install immediate hooks for the main executable
        InstallHooksForModule(nullptr, GetModuleHandle(NULL));
        MH_EnableHook(MH_ALL_HOOKS);

        // Hook LoadLibraryA to catch when the game DLL is loaded
        void *pLoadLibraryA =
            (void *)GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA");

        MH_CreateHook(pLoadLibraryA,
                      (void *)&HookedLoadLibraryA,
                      reinterpret_cast<void **>(&o_LoadLibraryA));
        MH_EnableHook(pLoadLibraryA);

    } else if (ul_reason_for_call == DLL_PROCESS_DETACH) {
        spdlog::info("memalochook.dll detached.");
        ShutdownLogging();
        MH_Uninitialize();
    }
    return TRUE;
}

// Dummy export to ensure the DLL can be loaded.
extern "C" __declspec(dllexport) void Initialize() {}
