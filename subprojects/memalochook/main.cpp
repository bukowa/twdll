#include <windows.h>
#include <cstddef>
#include <fstream>
#include <string>
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
// We only need one trampoline, as both original functions have the same signature.
t_memmove o_memmove_trampoline = nullptr;

void *__cdecl HookedMemmove(void *dest, const void *src, size_t size) {
    // This calls the modern, fast, statically-linked memmove from our own DLL.
    return memmove(dest, src, size);
}

// =================================================================================
// The Worker Thread (Installs hooks into empire.retail.dll)
// =================================================================================
DWORD WINAPI InstallHooks(HMODULE hGameDll) {
    {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "Hooking thread started. empire.retail.dll is loaded." << std::endl;
        g_logFile << "Targeting internal, statically-linked memmove functions..." << std::endl;
    }

    // --- YOUR TASK: Find these addresses in IDA ---
    // Look in the "Functions" window of empire.retail.dll for these two.
    const uintptr_t ida_memmove_addr   = 0x116BD8D0; // <-- FIND THIS ADDRESS IN IDA
    const uintptr_t ida_memmove_0_addr = 0x116BDE50; // <-- FIND THIS ADDRESS IN IDA
    // ---

    constexpr uintptr_t idaImageBase = 0x10000000; // Default for this game
    uintptr_t moduleBase = (uintptr_t)hGameDll;

    // Calculate the real, in-memory addresses
    void* pMemmove   = (void*)(moduleBase + (ida_memmove_addr - idaImageBase));
    void* pMemmove_0 = (void*)(moduleBase + (ida_memmove_0_addr - idaImageBase));

    // Hook the first function
    if (MH_CreateHook(pMemmove, &HookedMemmove, reinterpret_cast<void **>(&o_memmove_trampoline)) == MH_OK) {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "  -> Hook created for internal _memmove at " << pMemmove << std::endl;
    } else {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "  [!] FATAL: Failed to create hook for _memmove." << std::endl;
    }
    
    // Hook the second function
    if (MH_CreateHook(pMemmove_0, &HookedMemmove, nullptr) == MH_OK) { // No need for a second trampoline
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "  -> Hook created for internal _memmove_0 at " << pMemmove_0 << std::endl;
    } else {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "  [!] FATAL: Failed to create hook for _memmove_0." << std::endl;
    }

    // Enable all hooks we just created
    if (MH_EnableHook(MH_ALL_HOOKS) == MH_OK) {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "Successfully enabled all hooks." << std::endl;
    } else {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "FATAL: Failed to enable hooks." << std::endl;
    }

    // Clean up the LoadLibraryA hook as it's no longer needed
    void* pLoadLibraryA = GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA");
    MH_DisableHook(pLoadLibraryA);
    MH_RemoveHook(pLoadLibraryA);
    {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "LoadLibraryA hook removed. Setup complete." << std::endl;
    }

    return 0;
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
        g_logFile.open("C:\\Games\\Total War - Rome 2\\memalochook_log.txt", std::ios::out | std::ios::trunc);
        g_logFile << "memalochook.dll injected." << std::endl;

        if (MH_Initialize() != MH_OK) {
            g_logFile << "FATAL: MH_Initialize failed!" << std::endl;
            return TRUE;
        }

        void *pLoadLibraryA = GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA");
        if (MH_CreateHook(pLoadLibraryA, &HookedLoadLibraryA, reinterpret_cast<void **>(&o_LoadLibraryA)) != MH_OK ||
            MH_EnableHook(pLoadLibraryA) != MH_OK) {
            g_logFile << "FATAL: Could not install hook for LoadLibraryA." << std::endl;
        } else {
            g_logFile << "Initial hook on LoadLibraryA installed. Waiting for game DLL..." << std::endl;
        }
    } else if (ul_reason_for_call == DLL_PROCESS_DETACH) {
        MH_Uninitialize();
        if (g_logFile.is_open())
            g_logFile.close();
    }
    return TRUE;
}

extern "C" __declspec(dllexport) void Initialize() {}