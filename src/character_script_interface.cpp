/// @module twdll_character
/// Functions for interacting with game characters.
#include "script_utils.h"
#include "character_script_interface.h"
#include "game_lua_api.h" // Include game_lua_api.h for function pointers
#include <cstddef> // For NULL

#define REAL_CHARACTER_POINTER_OFFSET 0x8

#define MOVEMENT_POINTS_OFFSET 0x14
#define AMBITION_OFFSET 0x558
#define GRAVITAS_OFFSET 0x55C

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

    size_t offset = (size_t)g_game_lua_tointeger(L, 2);
    int value = (int)g_game_lua_tointeger(L, 3);

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
        g_game_lua_pushnil(L);
        return 1;
    }

    size_t offset = (size_t)g_game_lua_tointeger(L, 2);
    int v = read_from<int>(character, offset);

    g_game_lua_pushinteger(L, v);
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

/**
 * Sets the movement points of a character.
 * @function SetMovementPoints
 * @tparam userdata character The character object.
 * @tparam integer value The new movement points value.
 */
static int script_SetMovementPoints(lua_State* L) {
    void* character = get_object_from_indirect_wrapper(L, "character", REAL_CHARACTER_POINTER_OFFSET);
    if (!character) return 0;

    int value = (int)g_game_lua_tointeger(L, 2);
    write_to<int>(character, MOVEMENT_POINTS_OFFSET, value);
    return 0;
}

/**
 * Gets the movement points of a character.
 * @function GetMovementPoints
 * @tparam userdata character The character object.
 * @treturn integer The movement points of the character.
 */
static int script_GetMovementPoints(lua_State* L) {
    void* character = get_object_from_indirect_wrapper(L, "character", REAL_CHARACTER_POINTER_OFFSET);
    if (!character) {
        g_game_lua_pushnil(L);
        return 1;
    }

    int v = read_from<int>(character, MOVEMENT_POINTS_OFFSET);
    g_game_lua_pushinteger(L, v);
    return 1;
}

/**
 * Sets the ambition of a character.
 * @function SetAmbition
 * @tparam userdata character The character object.
 * @tparam integer value The new ambition value.
 */
static int script_SetAmbition(lua_State* L) {
    void* character = get_object_from_indirect_wrapper(L, "character", REAL_CHARACTER_POINTER_OFFSET);
    if (!character) return 0;

    int value = (int)g_game_lua_tointeger(L, 2);
    write_to<int>(character, AMBITION_OFFSET, value);
    return 0;
}

/**
 * Gets the ambition of a character.
 * @function GetAmbition
 * @tparam userdata character The character object.
 * @treturn integer The ambition of the character.
 */
static int script_GetAmbition(lua_State* L) {
    void* character = get_object_from_indirect_wrapper(L, "character", REAL_CHARACTER_POINTER_OFFSET);
    if (!character) {
        g_game_lua_pushnil(L);
        return 1;
    }

    int v = read_from<int>(character, AMBITION_OFFSET);
    g_game_lua_pushinteger(L, v);
    return 1;
}

/**
 * Sets the gravitas of a character.
 * @function SetGravitas
 * @tparam userdata character The character object.
 * @tparam integer value The new gravitas value.
 */
static int script_SetGravitas(lua_State* L) {
    void* character = get_object_from_indirect_wrapper(L, "character", REAL_CHARACTER_POINTER_OFFSET);
    if (!character) return 0;

    int value = (int)g_game_lua_tointeger(L, 2);
    write_to<int>(character, GRAVITAS_OFFSET, value);
    return 0;
}

/**
 * Gets the gravitas of a character.
 * @function GetGravitas
 * @tparam userdata character The character object.
 * @treturn integer The gravitas of the character.
 */
static int script_GetGravitas(lua_State* L) {
    void* character = get_object_from_indirect_wrapper(L, "character", REAL_CHARACTER_POINTER_OFFSET);
    if (!character) {
        g_game_lua_pushnil(L);
        return 1;
    }

    int v = read_from<int>(character, GRAVITAS_OFFSET);
    g_game_lua_pushinteger(L, v);
    return 1;
}


// Public registration table
const luaL_Reg character_functions[] = {
    {"GetMemoryAddress", script_GetMemoryAddress},
    {"GetIntAtOffset",   script_GetIntAtOffset},
    {"SetIntAtOffset",   script_SetIntAtOffset},
    {"GetMovementPoints", script_GetMovementPoints},
    {"SetMovementPoints", script_SetMovementPoints},
    {"GetAmbition", script_GetAmbition},
    {"SetAmbition", script_SetAmbition},
    {"GetGravitas", script_GetGravitas},
    {"SetGravitas", script_SetGravitas},
    {NULL, NULL}
};
