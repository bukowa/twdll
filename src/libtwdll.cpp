#include <windows.h>
#include "MinHook.h"
#include "log.h"
#include "unit_script_interface.cpp"
#include "battle_unit_script_interface.h"
#include "character_script_interface.h"

// We still need the extern "C" block for the Lua headers
extern "C" {
    #include <lua.h>
    #include <lauxlib.h>
}

// First, define a type for the original function we are hooking.
// From Ghidra, we know its signature is: float __fastcall GetUnitStrength(void* pUnit);
// The 'this' pointer (pUnit) is passed in the ECX register.
typedef float (__fastcall *tGetUnitStrength)(void* pUnit);

// This global variable will store a pointer to the original, un-hooked game function.
tGetUnitStrength oGetUnitStrength = NULL;

float __fastcall hkGetUnitStrength(void* pUnit_Cpp) { // pUnit_Cpp is the REAL C++ pointer

    // --- LOGGING ---
    // Now we log both pointers at the same time!
    char buffer[256];
    sprintf_s(buffer, sizeof(buffer),
              "C++ Ptr: 0x%p",
              pUnit_Cpp);
    Log(buffer);

    // Call the original function to keep the game running
    return oGetUnitStrength(pUnit_Cpp);
}


// The main entry point for the DLL, called by Lua
extern "C" __declspec(dllexport) int luaopen_libtwdll(lua_State *L) {
    static bool hooks_are_initialized = false;

    // Register our new, powerful functions
    // lua_register(L, "heal_unit", HealUnit);
    // lua_register(L, "set_unit_strength", SetUnitStrength);
    // lua_register(L, "set_movement_points", SetMovementPoints);
    // lua_register(L, "get_movement_points", GetMovementPoints);
    luaL_register(L, "unit", unit_functions);
    luaL_register(L, "character", character_functions);
    luaL_register(L, "battle_unit", battle_unit_functions);
    luaL_register(L, "battle_unit_stats", battle_unit_stats_functions);

    // --- The Safety Check ---
    if (hooks_are_initialized) {
        // If we've already run the initialization, do NOT try to hook again.
        // We just need to re-register our functions with the NEW Lua state.
        Log("--- libtwdll already initialized.---");
    }
    else {
        // --- First-Time Initialization ---
        // This block will only ever execute ONCE during the entire game session.
        Log("--- libtwdll first-time initialization. Placing hooks... ---");

        if (MH_Initialize() != MH_OK) {
            Log("FATAL: MinHook failed to initialize!");
            return 0; // Stop if we can't initialize.
        }

        if (MH_Initialize() != MH_OK) {
            Log("FATAL: MinHook failed to initialize!");
            return 0;
        }

        // --- Define the static offset for our target function ---
        // This is the value we calculated: 0x94CA60
        DWORD offset_GetUnitStrength = 0x94CA60;

        // Get the base address of the game's DLL at runtime
        HMODULE hGameDll = GetModuleHandleA("Rome2.dll");

        if (!hGameDll) {
            Log("FATAL: Could not find Rome2.dll in memory!");
            // As a fallback for the Steam version, you could add:
            // hGameDll = GetModuleHandleA("empire.retail.dll");
            // if (!hGameDll) { /* return error */ }
            return 0;
        }

        // Calculate the real, dynamic address of the function to hook
        LPVOID pTarget = (LPVOID)((DWORD)hGameDll + offset_GetUnitStrength);

        // --- Create and enable the hook ---
        // MH_CreateHook takes:
        // 1. The address of the function to hook (pTarget).
        // 2. The address of our custom function (hkGetUnitStrength).
        // 3. A pointer to a variable where it can store the original function's address (oGetUnitStrength).

        if (MH_CreateHook(pTarget, &hkGetUnitStrength, reinterpret_cast<LPVOID*>(&oGetUnitStrength)) != MH_OK) {
            Log("FATAL: Hook failed to create!");
            return 0;
        }
        if (MH_EnableHook(pTarget) != MH_OK) {
            Log("ERROR: MinHook failed to enable hook!");
            return 0;
        }

        // Set the flag to 'true' so this block never runs again.
        hooks_are_initialized = true;
    }

    // Let the user know it worked
    Log("--- libtwdll first-time initialization. ---");
    return 0;
}