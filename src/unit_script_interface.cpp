/// @module twdll_unit
/// Functions for interacting with campaign units.
#include "log.h"
#include <cstdio> // For sprintf_s

extern "C" {
#include <lua.h>
#include <lauxlib.h>
}

#define REAL_POINTER_OFFSET 0x8
#define UNIT_CURRENT_STRENGTH_OFFSET 0x44
#define UNIT_MAX_STRENGTH_OFFSET     0x48
#define UNIT_MOVEMENT_POINTS_OFFSET  0x64

template<typename T>
T read_from(void *base_address, const size_t offset) {
    return *reinterpret_cast<T *>(static_cast<char *>(base_address) + offset);
}

template<typename T>
void write_to(void *base_address, const size_t offset, T value) {
    *reinterpret_cast<T *>(static_cast<char *>(base_address) + offset) = value;
}

// --- Pointer Retrieval Helpers ---
static void *get_unit_from_direct_wrapper(lua_State *L) {
    void *wrapper = lua_touserdata(L, 1);
    if (!wrapper) {
        Log("ERROR (get_unit_direct): Argument 1 was not a userdata object.");
        return nullptr;
    }
    void *pUnit = *(void **) ((char *) wrapper + REAL_POINTER_OFFSET);
    if (!pUnit) {
        Log("ERROR (get_unit_direct): Pointer at offset was NULL.");
        return nullptr;
    }
    return pUnit;
}

static void *get_unit_from_indirect_wrapper(lua_State *L) {
    void **p_wrapper = static_cast<void **>(lua_touserdata(L, 1));
    if (!p_wrapper || !*p_wrapper) {
        Log("ERROR (get_unit_indirect): Argument is not a valid pointer to a wrapper.");
        return nullptr;
    }
    void *wrapper = *p_wrapper;
    void *unit_object = *reinterpret_cast<void **>(static_cast<char *>(wrapper) + REAL_POINTER_OFFSET);
    if (!unit_object) {
        Log("ERROR (get_unit_indirect): Real Unit pointer is null.");
        return nullptr;
    }
    return unit_object;
}

static auto write_int_property(lua_State *L, size_t property_offset) -> int {
    void *unit = get_unit_from_indirect_wrapper(L);
    if (!unit) {
        return 0; // Error already logged by helper
    }
    int new_value = (int) lua_tointeger(L, 2);
    write_to<int>(unit, property_offset, new_value);
    return 0;
}

static int read_int_property(lua_State *L, size_t property_offset) {
    void *unit = get_unit_from_indirect_wrapper(L);
    if (!unit) {
        lua_pushnil(L);
        return 1;
    }
    int current_value = read_from<int>(unit, property_offset);
    lua_pushinteger(L, current_value);
    return 1;
}

/***
Sets the current unit strength (soldier count).
@function SetStrength
@tparam integer value new current strength
*/
static int script_SetStrength(lua_State *L) {
    return write_int_property(L, UNIT_CURRENT_STRENGTH_OFFSET);
}

/***
Sets the maximum unit strength (soldier capacity).
@function SetMaxStrength
@tparam integer value new maximum strength
*/
static int script_SetMaxStrength(lua_State *L) {
    return write_int_property(L, UNIT_MAX_STRENGTH_OFFSET);
}

/***
Sets the remaining movement points for the unit.
@function SetMovementPoints
@tparam integer value new movement points
*/
static int script_SetMovementPoints(lua_State *L) {
    return write_int_property(L, UNIT_MOVEMENT_POINTS_OFFSET);
}

/***
Gets the current unit strength (soldier count).
@function GetStrength
@treturn integer current strength
*/
static int script_GetStrength(lua_State *L) {
    return read_int_property(L, UNIT_CURRENT_STRENGTH_OFFSET);
}

/***
Gets the maximum unit strength (soldier capacity).
@function GetMaxStrength
@treturn integer maximum strength
*/
static int script_GetMaxStrength(lua_State *L) {
    return read_int_property(L, UNIT_MAX_STRENGTH_OFFSET);
}

/***
Gets the remaining movement points for the unit.
@function GetMovementPoints
@treturn integer movement points
*/
static int script_GetMovementPoints(lua_State *L) {
    return read_int_property(L, UNIT_MOVEMENT_POINTS_OFFSET);
}

/**
 * Returns the memory address of the real unit object as a hexadecimal string.
 * @function GetMemoryAddress
 * @treturn string memory address (e.g. "0x12345678")
 */
static int script_GetMemoryAddress(lua_State* L) {
    // Step 1: Get the pointer to the unit object.
    void* unit = get_unit_from_indirect_wrapper(L);
    if (!unit) {
        lua_pushnil(L); // If there's an error, return nil.
        return 1;
    }

    // Step 2: Prepare a buffer for the resulting string.
    // A 64-bit address in hex is 16 chars + "0x" + null terminator = 19.
    // 64 characters is more than enough and is safe.
    char address_buffer[64];

    // Step 3: Format the pointer into a hexadecimal string.
    // %p is the standard, portable format specifier for printing pointers.
    sprintf_s(address_buffer, sizeof(address_buffer), "0x%p", unit);

    // Step 4: Push the formatted string to Lua.
    lua_pushstring(L, address_buffer);

    // We are returning 1 value (the string).
    return 1;
}


// =============================================================================
//  Lua Function Registration
// =============================================================================

static const struct luaL_Reg unit_functions[] = {
    // Setters
    {"SetStrength", script_SetStrength},
    {"SetMaxStrength", script_SetMaxStrength},
    {"SetMovementPoints", script_SetMovementPoints},
    // Getters
    {"GetStrength", script_GetStrength},
    {"GetMaxStrength", script_GetMaxStrength},
    {"GetMovementPoints", script_GetMovementPoints},
    // Diagnostic Functions
    {"GetMemoryAddress",   script_GetMemoryAddress}, // <-- DODAJ TĘ LINIJKĘ

    {NULL, NULL}
};
