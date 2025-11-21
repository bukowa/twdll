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
// Hooking Strategies
// =================================================================================

// Strategy 1: For launching a new process.
// Assumes the game DLL is not loaded yet, so we hook LoadLibraryA to wait for it.
void SetupHooksForLaunch() {
    spdlog::info("Using LAUNCH strategy: hooking LoadLibraryA to wait for game DLL.");

    LPVOID pLoadLibraryA = (LPVOID)GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA");
    if (!pLoadLibraryA) {
        spdlog::error("Could not find LoadLibraryA address.");
        return;
    }

    if (MH_CreateHook(pLoadLibraryA, (LPVOID)&HookedLoadLibraryA, reinterpret_cast<LPVOID*>(&o_LoadLibraryA)) != MH_OK) {
        spdlog::error("Failed to create hook for LoadLibraryA.");
        return;
    }

    if (MH_EnableHook(pLoadLibraryA) != MH_OK) {
        spdlog::error("Failed to enable hook for LoadLibraryA.");
    }
}

// Strategy 2: For attaching to a running process.
// Checks if the game DLL is already loaded. If so, hooks it. If not, falls back to hooking LoadLibraryA.
void SetupHooksForAttach() {
    spdlog::info("Using ATTACH strategy: checking for existing game DLL.");

    const char* gameDllName = "empire.retail.dll";
    HMODULE hGameDll = GetModuleHandleA(gameDllName);

    if (hGameDll) {
        // Game DLL is already in memory, so we can hook it directly.
        spdlog::info("{} is already loaded. Installing hooks immediately.", gameDllName);
        InstallHooksForModule(gameDllName, hGameDll);
        MH_EnableHook(MH_ALL_HOOKS); // It's safe to call this again
    }
    else {
        // Game DLL not loaded yet. Fall back to the launch strategy.
        spdlog::info("{} not yet loaded. Hooking LoadLibraryA to wait.", gameDllName);
        SetupHooksForLaunch();
    }
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

        // Select the hooking strategy based on the build flag.
#ifdef TWDLL_INJECT_ATTACH
        SetupHooksForAttach();
#else
        SetupHooksForLaunch();
#endif

    } else if (ul_reason_for_call == DLL_PROCESS_DETACH) {
        spdlog::info("memalochook.dll detached.");
        // Uninitialize MinHook before shutting down logging, as hooks might try to log.
        MH_Uninitialize();
        ShutdownLogging();
    }
    return TRUE;
}

// Dummy export to ensure the DLL can be loaded.
extern "C" __declspec(dllexport) void Initialize() {}
