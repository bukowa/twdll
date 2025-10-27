/// @module twdll_unit
/// Functions for interacting with campaign units.
#include "script_utils.h"
#include "unit_script_interface.h"
#include <cstddef> // For NULL

// The offset to get the real pointer from the wrapper object.
#define REAL_UNIT_POINTER_OFFSET 0x8

// Define property offsets for units
#define UNIT_CURRENT_NUMBER_OF_MAN_OFFSET 0x44
#define UNIT_MAX_NUMBER_OF_MAN_OFFSET     0x48
#define UNIT_MOVEMENT_POINTS_OFFSET  0x64

/***
Sets the current number of man in the unit.
@function SetNumberOfMan
@tparam userdata unit the unit object (first argument)
@tparam integer value new number of man
*/
static int script_SetNumberOfMan(lua_State *L) {
    return write_int_property(L, REAL_UNIT_POINTER_OFFSET, UNIT_CURRENT_NUMBER_OF_MAN_OFFSET, "unit");
}

/***
Sets the maximum number of man in the unit.
@function SetMaxNumberOfMan
@tparam userdata unit the unit object (first argument)
@tparam integer value new maximum number of man
*/
static int script_SetMaxNumberOfMan(lua_State *L) {
    return write_int_property(L, REAL_UNIT_POINTER_OFFSET, UNIT_MAX_NUMBER_OF_MAN_OFFSET, "unit");
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
Gets the current number of man in the unit.
@function GetNumberOfMan
@tparam userdata unit the unit object (first argument)
@treturn integer current number of man
*/
static int script_GetNumberOfMan(lua_State *L) {
    return read_int_property(L, REAL_UNIT_POINTER_OFFSET, UNIT_CURRENT_NUMBER_OF_MAN_OFFSET, "unit");
}

/***
Gets the maximum number of man in the unit.
@function GetMaxNumberOfMan
@tparam userdata unit the unit object (first argument)
@treturn integer maximum number of man
*/
static int script_GetMaxNumberOfMan(lua_State *L) {
    return read_int_property(L, REAL_UNIT_POINTER_OFFSET, UNIT_MAX_NUMBER_OF_MAN_OFFSET, "unit");
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
    {"SetNumberOfMan", script_SetNumberOfMan},
    {"SetMaxNumberOfMan", script_SetMaxNumberOfMan},
    {"SetMovementPoints", script_SetMovementPoints},
    // Getters
    {"GetNumberOfMan", script_GetNumberOfMan},
    {"GetMaxNumberOfMan", script_GetMaxNumberOfMan},
    {"GetMovementPoints", script_GetMovementPoints},
    // Diagnostic Functions
    {"GetMemoryAddress",   script_GetMemoryAddress},

    {NULL, NULL}
};
