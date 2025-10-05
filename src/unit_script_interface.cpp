/// @module twdll_unit
/// Functions for interacting with campaign units.
#include "script_utils.h"
#include "unit_script_interface.h"

// The offset to get the real pointer from the wrapper object.
#define REAL_UNIT_POINTER_OFFSET 0x8

// Define property offsets for units
#define UNIT_CURRENT_STRENGTH_OFFSET 0x44
#define UNIT_MAX_STRENGTH_OFFSET     0x48
#define UNIT_MOVEMENT_POINTS_OFFSET  0x64

/***
Sets the current unit strength (soldier count).
@function SetStrength
@tparam userdata unit the unit object (first argument)
@tparam integer value new current strength
*/
static int script_SetStrength(lua_State *L) {
    return write_int_property(L, REAL_UNIT_POINTER_OFFSET, UNIT_CURRENT_STRENGTH_OFFSET, "unit");
}

/***
Sets the maximum unit strength (soldier capacity).
@function SetMaxStrength
@tparam userdata unit the unit object (first argument)
@tparam integer value new maximum strength
*/
static int script_SetMaxStrength(lua_State *L) {
    return write_int_property(L, REAL_UNIT_POINTER_OFFSET, UNIT_MAX_STRENGTH_OFFSET, "unit");
}

/***
Sets the remaining movement points for the unit.
@function SetMovementPoints
@tparam userdata unit the unit object (first argument)
@tparam integer value new movement points
*/
static int script_SetMovementPoints(lua_State *L) {
    return write_int_property(L, REAL_UNIT_POINTER_OFFSET, UNIT_MOVEMENT_POINTS_OFFSET, "unit");
}

/***
Gets the current unit strength (soldier count).
@function GetStrength
@tparam userdata unit the unit object (first argument)
@treturn integer current strength
*/
static int script_GetStrength(lua_State *L) {
    return read_int_property(L, REAL_UNIT_POINTER_OFFSET, UNIT_CURRENT_STRENGTH_OFFSET, "unit");
}

/***
Gets the maximum unit strength (soldier capacity).
@function GetMaxStrength
@tparam userdata unit the unit object (first argument)
@treturn integer maximum strength
*/
static int script_GetMaxStrength(lua_State *L) {
    return read_int_property(L, REAL_UNIT_POINTER_OFFSET, UNIT_MAX_STRENGTH_OFFSET, "unit");
}

/***
Gets the remaining movement points for the unit.
@function GetMovementPoints
@tparam userdata unit the unit object (first argument)
@treturn integer movement points
*/
static int script_GetMovementPoints(lua_State *L) {
    return read_int_property(L, REAL_UNIT_POINTER_OFFSET, UNIT_MOVEMENT_POINTS_OFFSET, "unit");
}

/**
 * Returns the memory address of the real unit object as a hexadecimal string.
 * @function GetMemoryAddress
 * @tparam userdata unit the unit object (first argument)
 * @treturn string memory address (e.g. "0x12345678")
 */
static int script_GetMemoryAddress(lua_State* L) {
    return get_memory_address_lua(L, "unit", REAL_UNIT_POINTER_OFFSET);
}

// =============================================================================
//  Lua Function Registration
// =============================================================================

const struct luaL_Reg unit_functions[] = {
    // Setters
    {"SetStrength", script_SetStrength},
    {"SetMaxStrength", script_SetMaxStrength},
    {"SetMovementPoints", script_SetMovementPoints},
    // Getters
    {"GetStrength", script_GetStrength},
    {"GetMaxStrength", script_GetMaxStrength},
    {"GetMovementPoints", script_GetMovementPoints},
    // Diagnostic Functions
    {"GetMemoryAddress",   script_GetMemoryAddress},

    {NULL, NULL}
};
