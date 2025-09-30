#include <windows.h>
#include <stdint.h> // For uintptr_t
#include "MinHook.h"

// We still need the extern "C" block for the Lua headers
extern "C" {
    #include <lua.h>
    #include <lauxlib.h>
}

lua_State *g_luaState = NULL;

// First, define a type for the original function we are hooking.
// From Ghidra, we know its signature is: float __fastcall GetUnitStrength(void* pUnit);
// The 'this' pointer (pUnit) is passed in the ECX register.
typedef float (__fastcall *tGetUnitStrength)(void* pUnit);

// This global variable will store a pointer to the original, un-hooked game function.
tGetUnitStrength oGetUnitStrength = NULL;

// This is our custom hook function. It will run INSTEAD of the game's function.
float __fastcall hkGetUnitStrength(void* pUnit) {

    // --- CRITICAL STEP ---
    // First, call the original game function to get the real strength value.
    // If we don't do this, we'll have nothing to print, and the game will get a bad value.
    float original_strength = oGetUnitStrength(pUnit);

    // --- Our Proof-of-Concept Logic ---
    // Create a message to print. We'll include the unit's address AND the strength.
    // We use %f to format a floating-point number.
    char message_buffer[256];
    sprintf_s(message_buffer, sizeof(message_buffer),
              "pwrite('HOOKED! Unit: 0x%p | Original Strength: %f')",
              pUnit, original_strength);

    // Execute this Lua string in the game's Lua state to print to the console.
    if (g_luaState) {
        luaL_dostring(g_luaState, message_buffer);
    }

    // --- Final Step ---
    // Now, return the original value that we got from the game's function.
    // This ensures the Lua script that called it gets the correct result.
    return original_strength;
}
// This function follows the pointer chain to find the real address
uintptr_t GetMoneyAddress() {
    uintptr_t moduleBase = 0;
    uintptr_t staticOffset = 0;
    uintptr_t finalOffset = 0;

    // First, check if we're running the Steam version by looking for its DLL.
    moduleBase = (uintptr_t)GetModuleHandleA("Empire.Retail.dll");
    if (moduleBase) {
        // --- STEAM VERSION DETECTED ---
        staticOffset = 0x01EF2A00;
        finalOffset = 0x80C;
    } else {
        // If the Steam DLL is not found, check for the Standalone version's DLL.
        moduleBase = (uintptr_t)GetModuleHandleA("Rome2.dll");
        if (moduleBase) {
            // --- STANDALONE VERSION DETECTED ---
            staticOffset = 0x01EAF268;
            finalOffset = 0xF6C;
        } else {
            // If neither DLL is found, we can't continue.
            return 0;
        }
    }

    // The rest of the logic is the same for both versions, just with different variables.

    // 1. Calculate the address of our static starting pointer.
    uintptr_t staticAddress = moduleBase + staticOffset;

    // 2. Read the pointer at that location to get the dynamic base address.
    uintptr_t dynamicBase = *(uintptr_t*)staticAddress;
    if (!dynamicBase) {
        return 0; // Safety check
    }

    // 3. Add the final offset to get the money address.
    uintptr_t moneyAddress = dynamicBase + finalOffset;

    return moneyAddress;
}


// Writes a 4-byte integer to the money address.
// Lua usage: set_money(99999)
static int SetMoney(lua_State* L) {
    // 1. Get the new value to write from the first Lua argument.
    int value = (int)lua_tointeger(L, 1);

    // 2. Automatically find the current money address by following the pointer path.
    uintptr_t address = GetMoneyAddress();
    if (!address) {
        return 0; // Failed to find address, do nothing.
    }

    // 3. Cast the address to a pointer and write the new value.
    int* pointer = (int*)address;
    *pointer = value;

    return 0; // Return 0 values to Lua.
}

// Reads the integer from the money address.
// Lua usage: local current_money = get_money()
static int GetMoney(lua_State* L) {
    uintptr_t address = GetMoneyAddress();
    if (!address) {
        lua_pushnil(L); // Push nil if we couldn't find it
        return 1;
    }

    int* pointer = (int*)address;
    int value = *pointer;

    lua_pushinteger(L, value); // Push the found value to Lua
    return 1; // Return 1 value to Lua.
}

static int PatchSettlementSlots(lua_State* L) {
    uintptr_t moduleBase = (uintptr_t)GetModuleHandleA("Rome2.dll");
    if (!moduleBase) {
        lua_pushstring(L, "PatchNotSet");
        return 1;
    }
    // --- Patch #1: Building Slots ---
    uintptr_t buildSlotInstruction = moduleBase + 0xFC94E0;
    DWORD* buildSlotValue = (DWORD*)(buildSlotInstruction + 3); // The value is 3 bytes into the instruction

    // --- Patch #2: Population Slots ---
    uintptr_t popSlotInstruction = moduleBase + 0xFB1AAF;

    DWORD* popSlotValue = (DWORD*)(popSlotInstruction + 3); // The value is 3 bytes into this instruction too

    // --- Apply Patches (with memory protection changes) ---
    DWORD oldProtect;

    // Patch building slots to 10
    VirtualProtect(buildSlotValue, sizeof(DWORD), PAGE_EXECUTE_READWRITE, &oldProtect);
    *buildSlotValue = 10;
    VirtualProtect(buildSlotValue, sizeof(DWORD), oldProtect, &oldProtect);

    // Patch population slots to 6 (or higher if you want)
    VirtualProtect(popSlotValue, sizeof(DWORD), PAGE_EXECUTE_READWRITE, &oldProtect);
    *popSlotValue = 6;
    VirtualProtect(popSlotValue, sizeof(DWORD), oldProtect, &oldProtect);
    lua_pushstring(L, "PatchSet");
    return 1;
}

// The main entry point for the DLL, called by Lua
extern "C" __declspec(dllexport) int luaopen_libtwdll(lua_State *L) {
    // Register our new, powerful functions
    lua_register(L, "set_money", SetMoney);
    lua_register(L, "get_money", GetMoney);
    lua_register(L, "set_settl", PatchSettlementSlots);

    luaL_dostring(L, "pwrite('libtwdll with PERMANENT money cheat loaded!')");
    g_luaState = L; // Save the Lua state globally

    if (MH_Initialize() != MH_OK) {
        luaL_dostring(L, "pwrite('ERROR: MinHook failed to initialize!')");
        return 0;
    }

    // --- Define the static offset for our target function ---
    // This is the value we calculated: 0x94CA60
    DWORD offset_GetUnitStrength = 0x94CA60;

    // Get the base address of the game's DLL at runtime
    HMODULE hGameDll = GetModuleHandleA("Rome2.dll");

    if (!hGameDll) {
        luaL_dostring(L, "pwrite('ERROR: Could not find Rome2.dll in memory!')");
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
        luaL_dostring(L, "pwrite('ERROR: MinHook failed to create hook!')");
        return 0;
    }

    if (MH_EnableHook(pTarget) != MH_OK) {
        luaL_dostring(L, "pwrite('ERROR: MinHook failed to enable hook!')");
        return 0;
    }

    // Let the user know it worked
    luaL_dostring(L, "pwrite('libtwdll.dll loaded and unit strength hook placed successfully!')");
    return 0;
}