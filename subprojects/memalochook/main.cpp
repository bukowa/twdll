#include <cstdint>
#include <windows.h>
#include <cstddef>
#include <fstream>

#include <mutex>
#include <shlwapi.h> // For PathFindFileNameA
#include <cstring>   // For the standard memmove function

#pragma comment(lib, "shlwapi.lib")

#include "MinHook.h"

// =================================================================================
// Global State & Logging
// =================================================================================
std::ofstream g_logFile;
std::mutex g_logMutex;

// =================================================================================
// Modern memmove Replacement
// =================================================================================
typedef void *(*t_memmove)(void *dest, const void *src, size_t size);
// A trampoline is not needed because we are completely replacing the function
// with the standard library's memmove.

// =================================================================================
// Hooking Infrastructure
// =================================================================================
struct Hook {
    const char *name;
    uintptr_t ida_address;
    void *replacement_function;
};

// =================================================================================
// The Worker Thread (Installs hooks into empire.retail.dll)
// =================================================================================
DWORD WINAPI InstallHooks(HMODULE hGameDll) {
    {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "Hooking thread started. empire.retail.dll is loaded." << std::endl;
        g_logFile << "Targeting internal, statically-linked functions..." << std::endl;
    }

    // --- Define all hooks to be installed ---
    Hook hooks_to_install[] = {
        {"_memmove", 0x116BD8D0, &memmove},
        {"_memmove_0", 0x116BDE50, &memmove},
        // {"_memset", 0x116BE510, &memset}
    };

    constexpr uintptr_t idaImageBase = 0x10000000; // Default for this game
    uintptr_t moduleBase = (uintptr_t)hGameDll;

    // Create all hooks in a loop
    for (const auto &hook : hooks_to_install) {
        void *pTarget = (void *)(moduleBase + (hook.ida_address - idaImageBase));
        if (MH_CreateHook(pTarget, hook.replacement_function, nullptr) == MH_OK) {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_logFile << "  -> Hook created for " << hook.name << " at " << pTarget << std::endl;
        } else {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_logFile << "  [!] FATAL: Failed to create hook for " << hook.name << "." << std::endl;
        }
    }

    // Enable all hooks at once
    if (MH_EnableHook(MH_ALL_HOOKS) == MH_OK) {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "Successfully enabled all hooks." << std::endl;
    } else {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "FATAL: Failed to enable hooks." << std::endl;
    }
}
// =================================================================================
// LoadLibraryA Hook (Our Trigger)
// =================================================================================
typedef HMODULE(WINAPI *t_LoadLibraryA)(LPCSTR lpLibFileName);
t_LoadLibraryA o_LoadLibraryA = nullptr;

HMODULE WINAPI HookedLoadLibraryA(LPCSTR lpLibFileName) {
    HMODULE hModule = o_LoadLibraryA(lpLibFileName);
    if (hModule && lpLibFileName) {
        if (_stricmp(PathFindFileNameA(lpLibFileName), "empire.retail.dll") == 0) {
            // Found it! Disable our own hook and start the main worker thread.
            MH_DisableHook(GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA"));
            CreateThread(nullptr, 0, (LPTHREAD_START_ROUTINE)InstallHooks, hModule, 0, nullptr);
        }
    }
    return hModule;
}

// =================================================================================
// DLL Entry Point (Minimal and Safe)
// =================================================================================
BOOL APIENTRY DllMain(HMODULE hModule, DWORD ul_reason_for_call, LPVOID lpReserved) {
    if (ul_reason_for_call == DLL_PROCESS_ATTACH) {
        DisableThreadLibraryCalls(hModule);
        g_logFile.open("C:\\Games\\Total War - Rome 2\\memalochook_log.txt",
                       std::ios::out | std::ios::trunc);
        g_logFile << "memalochook.dll injected." << std::endl;

        if (MH_Initialize() != MH_OK) {
            g_logFile << "FATAL: MH_Initialize failed!" << std::endl;
            return TRUE;
        }

        void *pLoadLibraryA = GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA");
        if (MH_CreateHook(pLoadLibraryA,
                          &HookedLoadLibraryA,
                          reinterpret_cast<void **>(&o_LoadLibraryA)) != MH_OK ||
            MH_EnableHook(pLoadLibraryA) != MH_OK) {
            g_logFile << "FATAL: Could not install hook for LoadLibraryA." << std::endl;
        } else {
            g_logFile << "Initial hook on LoadLibraryA installed. Waiting for game DLL..."
                      << std::endl;
        }
    } else if (ul_reason_for_call == DLL_PROCESS_DETACH) {
        MH_Uninitialize();
        if (g_logFile.is_open())
            g_logFile.close();
    }
    return TRUE;
}

extern "C" __declspec(dllexport) void Initialize() {}