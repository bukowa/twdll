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
// Hooking Infrastructure
// =================================================================================
struct Hook {
    const char *name;
    uintptr_t ida_address;
    void *replacement_function;
};

extern "C" {
    void* __cdecl A_memmove(void* dest, const void* src, size_t count);
}

void* CustomMemmove(void* dest, const void* src, size_t count)
{
    return A_memmove(dest, src, count);
}

DWORD WINAPI InstallHooks(HMODULE hGameDll) {
    {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "Hooking/patching thread started. empire.retail.dll is loaded at " << hGameDll
                  << std::endl;
    }

    // --- Define all hooks to redirect old functions to our new ones ---
    Hook hooks_to_install[] = {
        {"_memmove", 0x116BD8D0, (void *)&CustomMemmove},
        {"_memmove_0", 0x116BDE50, (void *)&CustomMemmove},
        // {"_memset", 0x116BE510, &memset} // Example for other potential hooks
    };

    constexpr uintptr_t idaImageBase = 0x10000000;
    uintptr_t moduleBase = (uintptr_t)hGameDll;

    for (const auto &hook : hooks_to_install) {
        // THIS LINE IS NOW CORRECTED
        void *pTarget = (void *)(moduleBase + (hook.ida_address - idaImageBase));

        if (MH_CreateHook(pTarget, hook.replacement_function, nullptr) != MH_OK) {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_logFile << "  [!] FATAL: Failed to create hook for " << hook.name << " at " << pTarget
                      << "." << std::endl;
        } else {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_logFile << "  -> Hook created for " << hook.name << " at " << pTarget << "."
                      << std::endl;
        }
    }

    if (MH_EnableHook(MH_ALL_HOOKS) == MH_OK) {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "Successfully enabled all hooks. Game is now using the patched, modern "
                     "memmove implementation."
                  << std::endl;
    } else {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "FATAL: Failed to enable hooks." << std::endl;
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
            MH_DisableHook(
                (LPVOID)GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA"));
            CreateThread(nullptr, 0, (LPTHREAD_START_ROUTINE)InstallHooks, hModule, 0, nullptr);
        }
    }
    return hModule;
}

// =================================================================================
// DLL Entry Point (Minimal and Safe)
// =================================================================================
BOOL APIENTRY DllMain(HMODULE hModule, DWORD ul_reason_for_call, LPVOID lpReserved) {
#if defined(_MSC_VER)
    // This block will only be compiled if you use the MSVC compiler (cl.exe)
    g_logFile << "  -> PROOF: This DLL was compiled with MSVC. It will use the msvcr memmove."
              << std::endl;
#elif defined(__GNUC__)
    // This block will only be compiled if you use a GCC-based compiler (like MinGW's g++.exe)
    g_logFile << "  -> PROOF: This DLL was compiled with MinGW/GCC. It will use the glibc memmove."
              << std::endl;
#else
    // For any other compiler
    g_logFile << "  -> PROOF: This DLL was compiled with an UNKNOWN compiler." << std::endl;
#endif
    if (ul_reason_for_call == DLL_PROCESS_ATTACH) {
        DisableThreadLibraryCalls(hModule);
        g_logFile.open("C:\\Games\\Total War - Rome 2\\memalochook_log.txt",
                       std::ios::out | std::ios::trunc);
        g_logFile << "memalochook.dll injected." << std::endl;

        if (MH_Initialize() != MH_OK) {
            g_logFile << "FATAL: MH_Initialize failed!" << std::endl;
            return TRUE;
        }

        void *pLoadLibraryA =
            (void *)GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA");

        if (MH_CreateHook(pLoadLibraryA,
                          (void *)&HookedLoadLibraryA,
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
        MH_Uninitialize();
        if (g_logFile.is_open())
            g_logFile.close();
    }
    return TRUE;
}

// Dummy export to ensure the DLL can be loaded.
extern "C" __declspec(dllexport) void Initialize() {}