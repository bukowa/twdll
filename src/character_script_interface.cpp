/// @module twdll_character
/// Functions for interacting with game characters.
#include "script_utils.h"
#include "character_script_interface.h"

#define REAL_CHARACTER_POINTER_OFFSET 0x8

/***
Writes a 32-bit integer at the given memory offset.
@function SetIntAtOffset
@tparam userdata character the character object (first argument)
@tparam integer offset
@tparam integer value
*/
static int script_SetIntAtOffset(lua_State* L) {
    void* character = get_object_from_indirect_wrapper(L, "character", REAL_CHARACTER_POINTER_OFFSET);
    if (!character) return 0;

    size_t offset = (size_t)lua_tointeger(L, 2);
    int value = (int)lua_tointeger(L, 3);

    write_to<int>(character, offset, value);
    return 0;
}

/**
 * Reads a 32-bit integer at the given memory offset.
 * @function GetIntAtOffset
 * @tparam userdata character the character object (first argument)
 * @tparam integer offset
 * @treturn integer value
 */
static int script_GetIntAtOffset(lua_State* L) {
    void* character = get_object_from_indirect_wrapper(L, "character", REAL_CHARACTER_POINTER_OFFSET);
    if (!character) {
        lua_pushnil(L);
        return 1;
    }

    size_t offset = (size_t)lua_tointeger(L, 2);
    int v = read_from<int>(character, offset);

    lua_pushinteger(L, v);
    return 1;
}

/**
 * Returns the memory address of the real character object.
 * @function GetMemoryAddress
 * @tparam userdata character the character object (first argument)
 * @treturn string memory address (e.g. "0x12345678")
 */
static int script_GetMemoryAddress(lua_State* L) {
    return get_memory_address_lua(L, "character", REAL_CHARACTER_POINTER_OFFSET);
}

// Public registration table
const luaL_Reg character_functions[] = {
    {"GetMemoryAddress", script_GetMemoryAddress},
    {"GetIntAtOffset",   script_GetIntAtOffset},
    {"SetIntAtOffset",   script_SetIntAtOffset},
    {NULL, NULL}
};
