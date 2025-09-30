#include <windows.h>
#include <stdint.h> // For uintptr_t
#include "MinHook.h"

// We still need the extern "C" block for the Lua headers
extern "C" {
    #include <lua.h>
    #include <lauxlib.h>
}

void Log(const char* message) {
    FILE* file = nullptr;
    fopen_s(&file, "libtwdll.log", "a"); // append mode
    if (!file) return;

    fprintf(file, "[libtwdll] %s\n", message);
    fclose(file);
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

// Heals a unit by setting its current strength to its max strength.
static int HealUnit(lua_State* L) {
    Log("HealUnit function called from Lua.");

    // Step 1: Get the userdata "wrapper" object from the first Lua argument.
    void* wrapper = lua_touserdata(L, 1);
    if (!wrapper) {
        Log("ERROR: Argument 1 was not a userdata object. Aborting.");
        return 0;
    }

    // Log the address of the wrapper object itself
    char buffer[128];
    sprintf_s(buffer, sizeof(buffer), "Step 1 SUCCESS: Got userdata wrapper at address: 0x%p", wrapper);
    Log(buffer);

    // Step 2: Get the REAL CppUnit* pointer from inside the wrapper at offset +8.
    void* pUnit = *(void**)((char*)wrapper + 8);
    if (!pUnit) {
        Log("ERROR: Pointer at offset +8 was NULL. Aborting.");
        return 0;
    }

    // Log the address of the real C++ unit object
    sprintf_s(buffer, sizeof(buffer), "Step 2 SUCCESS: Found real C++ Unit pointer at address: 0x%p", pUnit);
    Log(buffer);

    // Step 3: Read the max strength from offset +0x48 of the REAL unit object.
    int max_strength = *(int*)((char*)pUnit + 0x48);
    sprintf_s(buffer, sizeof(buffer), "Step 3 SUCCESS: Read MaxStrength. Value is: %d", max_strength);
    Log(buffer);

    // Check if the value is reasonable. If it's a huge or negative number, something is still wrong.
    if (max_strength <= 0 || max_strength > 500) {
        Log("WARNING: MaxStrength value seems incorrect. The unit structure may be wrong.");
    }

    // Step 4: Write the max strength value to the current strength offset (+0x44).
    *(int*)((char*)pUnit + 0x44) = max_strength;
    Log("Step 4 SUCCESS: Wrote new value to CurrentStrength offset.");

    return 0; // Return nothing to Lua
}
//
// // Our NEW Lua-callable function. It will heal the LAST unit we hooked.
// static int HealLastUnit(lua_State* L) {
//     g_luaState = L;
//     if (!g_lastHookedUnit) {
//         Log("ERROR: No unit has been hooked yet. Query a unit's strength first.");
//         return 0;
//     }
//
//     void* pUnit = g_lastHookedUnit;
//
//     char buffer[128];
//     sprintf_s(buffer, sizeof(buffer), "HealLastUnit called for unit at 0x%p", pUnit);
//     Log(buffer);
//
//     // Read max strength from offset +0x48
//     int max_strength = *(int*)((char*)pUnit + 0x48);
//
//     // Write it to current strength at offset +0x44
//     *(int*)((char*)pUnit + 0x44) = max_strength;
//
//     Log("Unit healed successfully.");
//
//     return 0;
// }

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

// The offset for the number of men within the C++ Unit object.
// We discovered this from Ghidra (param_1 + 0x44).
#define UNIT_STRENGTH_OFFSET 0x44

// The offset of the REAL C++ pointer inside the Lua userdata block.
// We discovered this from Ghidra (*(int *)((int)this + 8)).
#define POINTER_IN_USERDATA_OFFSET 0x8

static int SetUnitStrength(lua_State* L) {
    char log_buffer[256];

    // --- Step 1: Get the pointer to the SCRIPT_INTERFACE object ---
    // The userdata block from Lua contains a pointer to our interface object.
    void** p_interface_ptr = (void**)lua_touserdata(L, 1);
    if (!p_interface_ptr) {
        Log("SetUnitStrength ERROR: Argument is not a userdata object.");
        return 0;
    }
    void* interface_object = *p_interface_ptr;
    if (!interface_object) {
        Log("SetUnitStrength ERROR: Pointer to interface object is null.");
        return 0;
    }
    // --- LOGGING ---
    sprintf_s(log_buffer, sizeof(log_buffer), "SetUnitStrength: 1. Got SCRIPT_INTERFACE pointer [0x%p]", interface_object);
    Log(log_buffer);

    // --- Step 2: Get the pointer to the REAL Unit object ---
    // The real pointer is at an offset inside the interface object.
    void* unit_object = *(void**)((char*)interface_object + POINTER_IN_USERDATA_OFFSET);
    if (!unit_object) {
        Log("SetUnitStrength ERROR: Real Unit pointer is null.");
        return 0;
    }
    // --- LOGGING ---
    sprintf_s(log_buffer, sizeof(log_buffer), "SetUnitStrength: 2. Extracted REAL Unit pointer [0x%p]", unit_object);
    Log(log_buffer);

    // --- Step 3: Get the value from Lua and perform the write ---
    int new_strength = (int)lua_tointeger(L, 2);
    int* strength_variable_address = (int*)((char*)unit_object + UNIT_STRENGTH_OFFSET);

    // --- LOGGING ---
    sprintf_s(log_buffer, sizeof(log_buffer), "SetUnitStrength: 3. Writing value [%d] to final address [0x%p]...", new_strength, strength_variable_address);
    Log(log_buffer);

    *strength_variable_address = new_strength;

    Log("SetUnitStrength: Write complete.");
    return 0;
}

// The main entry point for the DLL, called by Lua
extern "C" __declspec(dllexport) int luaopen_libtwdll(lua_State *L) {
    // Register our new, powerful functions
    lua_register(L, "set_money", SetMoney);
    lua_register(L, "get_money", GetMoney);
    lua_register(L, "set_settl", PatchSettlementSlots);
    lua_register(L, "heal_unit", HealUnit);
    lua_register(L, "set_unit_strength", SetUnitStrength);


    luaL_dostring(L, "pwrite('libtwdll with PERMANENT money cheat loaded!')");

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