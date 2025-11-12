#include <windows.h>
#include <cstddef>
#include <vector>
#include <fstream>
#include <string>
#include <mutex>
#include <shlwapi.h> // For PathFindFileNameA

#pragma comment(lib, "shlwapi.lib")

#include "mimalloc.h"
#include "MinHook.h"

// =================================================================================
// Global Configuration & Logging
// =================================================================================

std::ofstream g_logFile;
std::mutex g_logMutex;

// Forward declaration of the hooking thread
DWORD WINAPI HookGameFunctions(HMODULE hGameDll);

// =================================================================================
// Data-Driven Hooking Configuration
// This is the main structure you will edit.
// =================================================================================

struct HookTarget {
    const char *FunctionName; // For logging purposes
    uintptr_t IdaAddress;     // The absolute address from IDA
    void *ReplacementFunc;    // Pointer to our replacement function
};

const std::vector<HookTarget> g_HookTargets = {
    // {"malloc", 0x116A59A1, (void *)&mi_malloc},
    // {"free", 0x1168D0A8, (void *)&mi_free},
    // {"realloc", 0x116B1986, (void *)&mi_realloc},
    // {"calloc", 0x116AB734, (void *)&mi_calloc},
    // {"_strdup", 0x116B204D, (void *)&mi_strdup},
    {"_msize", 0x116B58D9, (void *)&mi_usable_size},
    {"_recalloc_base", 0x116B56C7, (void *)&mi_recalloc},
    {"_wcsdup", 0x116A2577, (void *)&mi_wcsdup},
    {"operator new", 0x116845AF, (void *)mi_new},
};

// =================================================================================
// The Worker Thread (Now Automated)
// =================================================================================
DWORD WINAPI HookGameFunctions(HMODULE hGameDll) {
    {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "Hooking thread started. Preparing to install " << g_HookTargets.size()
                  << " hooks." << std::endl;
    }

    uintptr_t moduleBase = (uintptr_t)hGameDll;
    constexpr uintptr_t idaImageBase = 0x10000000;
    int successfulHooks = 0;

    // Loop through our table and create a hook for each valid entry.
    for (const auto &target : g_HookTargets) {
        // Skip entries where the address hasn't been found yet.
        if (target.IdaAddress == 0) {
            continue;
        }

        // Calculate the RVA and the real in-memory address.
        uintptr_t rva = target.IdaAddress - idaImageBase;
        void *targetAddress = (void *)(moduleBase + rva);

        // Create the hook for this specific function.
        if (MH_CreateHook(targetAddress, target.ReplacementFunc, nullptr) == MH_OK) {
            {
                std::lock_guard<std::mutex> lock(g_logMutex);
                g_logFile << "  -> Hook created for " << target.FunctionName << " at "
                          << targetAddress << std::endl;
            }
            successfulHooks++;
        } else {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_logFile << "  [!] FATAL: MH_CreateHook failed for " << target.FunctionName
                      << std::endl;
        }
    }

    // After creating all hooks, enable them all at once.
    if (MH_EnableHook(MH_ALL_HOOKS) == MH_OK) {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "Successfully enabled " << successfulHooks << " hooks." << std::endl;
    } else {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "FATAL: MH_EnableHook(MH_ALL_HOOKS) failed!" << std::endl;
    }

    // Clean up the temporary LoadLibraryA hook.
    void *pLoadLibraryA = GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA");
    MH_DisableHook(pLoadLibraryA);
    MH_RemoveHook(pLoadLibraryA);
    {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "LoadLibraryA hook has been removed. Hooking process complete." << std::endl;
    }

    return 0;
}

// =================================================================================
// LoadLibraryA Hook (No Changes Needed Here)
// =================================================================================
typedef HMODULE(WINAPI *t_LoadLibraryA)(LPCSTR lpLibFileName);
t_LoadLibraryA o_LoadLibraryA = nullptr;

HMODULE WINAPI HookedLoadLibraryA(LPCSTR lpLibFileName) {
    HMODULE hModule = o_LoadLibraryA(lpLibFileName);
    if (hModule != NULL && lpLibFileName != NULL) {
        if (_stricmp(PathFindFileNameA(lpLibFileName), "empire.retail.dll") == 0) {
            CreateThread(
                nullptr, 0, (LPTHREAD_START_ROUTINE)HookGameFunctions, hModule, 0, nullptr);
        }
    }
    return hModule;
}

// =================================================================================
// DLL Entry Point (No Changes Needed Here)
// =================================================================================
BOOL APIENTRY DllMain(HMODULE hModule, DWORD ul_reason_for_call, LPVOID lpReserved) {
    if (ul_reason_for_call == DLL_PROCESS_ATTACH) {
        DisableThreadLibraryCalls(hModule);
        g_logFile.open("C:\\Games\\Total War - Rome 2\\memalochook_log.txt",
                       std::ios::out | std::ios::app);

        {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_logFile << "------------------------------------------" << std::endl;
            g_logFile << "memalochook.dll injected." << std::endl;
        }

        if (MH_Initialize() != MH_OK) {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_logFile << "FATAL: MH_Initialize failed!" << std::endl;
            return TRUE;
        }

        void *pLoadLibraryA = GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA");
        if (MH_CreateHook(pLoadLibraryA,
                          &HookedLoadLibraryA,
                          reinterpret_cast<void **>(&o_LoadLibraryA)) != MH_OK ||
            MH_EnableHook(pLoadLibraryA) != MH_OK) {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_logFile << "FATAL: Could not install hook for LoadLibraryA." << std::endl;
        } else {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_logFile << "Hook on LoadLibraryA installed. Waiting for target DLL..." << std::endl;
        }
    } else if (ul_reason_for_call == DLL_PROCESS_DETACH) {
        MH_Uninitialize();
        if (g_logFile.is_open())
            g_logFile.close();
    }
    return TRUE;
}

extern "C" __declspec(dllexport) void Initialize() {}