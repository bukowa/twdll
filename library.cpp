#include <windows.h>
#include <stdint.h> // For uintptr_t

// We still need the extern "C" block for the Lua headers
extern "C" {
    #include <lua.h>
    #include <lauxlib.h>
}

// This function follows the pointer chain to find the real address
uintptr_t GetMoneyAddress() {
    // Step 1: Get the base address of Rome2.dll.
    uintptr_t moduleBase = (uintptr_t)GetModuleHandleA("Rome2.dll");
    if (!moduleBase) {
        return 0; // Return 0 if the DLL isn't loaded, for safety
    }

    // Step 2: Calculate the address of our static starting pointer.
    uintptr_t staticAddress = moduleBase + 0x01EAF268;

    // Step 3: Read the pointer at that location to get the dynamic base address.
    // The asterisk (*) "dereferences" the pointer, reading the value inside it.
    uintptr_t dynamicBase = *(uintptr_t*)staticAddress;
    if (!dynamicBase) {
        return 0; // Safety check in case the pointer is null
    }

    // Step 4: Add the final offset to get the money address.
    uintptr_t moneyAddress = dynamicBase + 0xF6C;

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


// The main entry point for the DLL, called by Lua
extern "C" __declspec(dllexport) int luaopen_libtwdll(lua_State *L) {
    // Register our new, powerful functions
    lua_register(L, "set_money", SetMoney);
    lua_register(L, "get_money", GetMoney);

    luaL_dostring(L, "pwrite('libtwdll with PERMANENT money cheat loaded!')");
    return 0;
}