#include "money.h"

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