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
// Modern memmove from the C runtime library our DLL is linked against.
// We are declaring it here to make it clear what we are using.
// =================================================================================
extern "C" void* __cdecl memmove(void* dest, const void* src, size_t size);

// =================================================================================
// Hooking Infrastructure
// =================================================================================
struct Hook {
    const char* name;
    uintptr_t ida_address;
    void* replacement_function;
};

// =================================================================================
// In-Memory Patching Logic
// =================================================================================

// This is the modified function for self-verification.
bool PatchNewMemmoveBranch() {
    uintptr_t pNewMemmove = (uintptr_t)&memmove;
    constexpr uintptr_t offset_to_jnb = 0x4C;
    const uintptr_t pTargetJnb = pNewMemmove + offset_to_jnb;
    const unsigned char patch = 0xEB; // JMP short opcode

    std::lock_guard<std::mutex> lock(g_logMutex);
    g_logFile << "  -> Attempting to patch NEW memmove function..." << std::endl;
    g_logFile << "  -> New memmove is at: " << (void*)pNewMemmove << std::endl;
    g_logFile << "  -> Target JNB is at:  " << (void*)pTargetJnb << " (Offset: 0x" << std::hex << offset_to_jnb << ")" << std::dec << std::endl;

    // --- VERIFICATION STEP 1: READ THE ORIGINAL BYTE ---
    unsigned char original_byte = *(unsigned char*)pTargetJnb;
    g_logFile << "  -> VERIFY: Original byte at target is 0x" << std::hex << (int)original_byte << std::dec << "." << std::endl;

    // Check if it's what we expect (jnb short has opcode 0x73)
    if (original_byte != 0x73) {
        g_logFile << "  [!] WARNING: Original byte was not 0x73 (jnb). Patch may be incorrect for this runtime version." << std::endl;
    }

    DWORD oldProtect;
    if (!VirtualProtect((LPVOID)pTargetJnb, 1, PAGE_EXECUTE_READWRITE, &oldProtect)) {
        g_logFile << "  [!] ERROR: VirtualProtect failed." << std::endl;
        return false;
    }

    // Apply the patch
    *(unsigned char*)pTargetJnb = patch;

    DWORD temp;
    VirtualProtect((LPVOID)pTargetJnb, 1, oldProtect, &temp);
    FlushInstructionCache(GetCurrentProcess(), (LPCVOID)pTargetJnb, 1);
    
    // --- VERIFICATION STEP 2: READ THE NEW BYTE ---
    unsigned char new_byte = *(unsigned char*)pTargetJnb;
    g_logFile << "  -> VERIFY: New byte after patch is 0x" << std::hex << (int)new_byte << std::dec << "." << std::endl;

    if (new_byte == patch) {
        g_logFile << "  -> SUCCESS: Patch confirmed. The 'rep movsb' instruction will now be bypassed." << std::endl;
        return true;
    } else {
        g_logFile << "  [!] FATAL: Patch verification FAILED. Wrote 0xEB but read back 0x" << std::hex << (int)new_byte << "." << std::endl;
        return false;
    }
}
// =================================================================================
// The Worker Thread (Installs hooks and patches into empire.retail.dll)
// =================================================================================
DWORD WINAPI InstallHooks(HMODULE hGameDll) {
    {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "Hooking/patching thread started. empire.retail.dll is loaded at " << hGameDll << std::endl;
    }

    // --- Define all hooks to redirect old functions to our new ones ---
    Hook hooks_to_install[] = {
        {"_memmove",   0x116BD8D0, &memmove},
        {"_memmove_0", 0x116BDE50, &memmove},
        // {"_memset", 0x116BE510, &memset} // Example for other potential hooks
    };

    constexpr uintptr_t idaImageBase = 0x10000000;
    uintptr_t moduleBase = (uintptr_t)hGameDll;

    // --- Step 1: Create the hooks in a disabled state ---
    for (const auto& hook : hooks_to_install) {
        // THIS LINE IS NOW CORRECTED
        void* pTarget = (void*)(moduleBase + (hook.ida_address - idaImageBase));
        
        if (MH_CreateHook(pTarget, hook.replacement_function, nullptr) != MH_OK) {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_logFile << "  [!] FATAL: Failed to create hook for " << hook.name << " at " << pTarget << "." << std::endl;
        } else {
            std::lock_guard<std::mutex> lock(g_logMutex);
            g_logFile << "  -> Hook created for " << hook.name << " at " << pTarget << "." << std::endl;
        }
    }
    
    // --- Step 2: Apply our in-memory patch to the new memmove function ---
    PatchNewMemmoveBranch();
    
    // --- Step 3: Enable all hooks at once ---
    if (MH_EnableHook(MH_ALL_HOOKS) == MH_OK) {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "Successfully enabled all hooks. Game is now using the patched, modern memmove implementation." << std::endl;
    } else {
        std::lock_guard<std::mutex> lock(g_logMutex);
        g_logFile << "FATAL: Failed to enable hooks." << std::endl;
    }

    return 0;
}

// =================================================================================
// LoadLibraryA Hook (Our Trigger)
// =================================================================================
typedef HMODULE(WINAPI* t_LoadLibraryA)(LPCSTR lpLibFileName);
t_LoadLibraryA o_LoadLibraryA = nullptr;

HMODULE WINAPI HookedLoadLibraryA(LPCSTR lpLibFileName) {
    HMODULE hModule = o_LoadLibraryA(lpLibFileName);
    if (hModule && lpLibFileName) {
        if (_stricmp(PathFindFileNameA(lpLibFileName), "empire.retail.dll") == 0) {
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

        void* pLoadLibraryA = GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA");
        if (MH_CreateHook(pLoadLibraryA,
                          &HookedLoadLibraryA,
                          reinterpret_cast<void**>(&o_LoadLibraryA)) != MH_OK ||
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

// Dummy export to ensure the DLL can be loaded.
extern "C" __declspec(dllexport) void Initialize() {}