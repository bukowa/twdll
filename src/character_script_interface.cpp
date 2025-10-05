/// @module twdll_character
/// Functions for interacting with game characters.
#include "character_script_interface.h"
#include "log.h"
#include <cstdio>

extern "C" {
#include <lua.h>
#include <lauxlib.h>
}

// The character wrapper uses the same offset as the unit script interface
#define REAL_CHARACTER_POINTER_OFFSET 0x8

// Reusable memory access helpers
template<typename T>
static T read_from(void* base_address, const size_t offset) {
    return *reinterpret_cast<T*>(static_cast<char*>(base_address) + offset);
}

template<typename T>
static void write_to(void* base_address, const size_t offset, T value) {
    *reinterpret_cast<T*>(static_cast<char*>(base_address) + offset) = value;
}

// Retrieve the underlying character object from the Lua-provided wrapper (indirect pointer)
static void* get_character_from_indirect_wrapper(lua_State* L) {
    void** p_wrapper = static_cast<void**>(lua_touserdata(L, 1));
    if (!p_wrapper || !*p_wrapper) {
        Log("ERROR (get_character_indirect): Argument is not a valid pointer to a wrapper.");
        return nullptr;
    }
    void* wrapper = *p_wrapper;
    void* character_object = *reinterpret_cast<void**>(static_cast<char*>(wrapper) + REAL_CHARACTER_POINTER_OFFSET);
    if (!character_object) {
        Log("ERROR (get_character_indirect): Real Character pointer is null.");
        return nullptr;
    }
    return character_object;
}

// Generic read/write helpers for integer properties on the character object
static int write_int_property(lua_State* L, size_t property_offset) {
    void* character = get_character_from_indirect_wrapper(L);
    if (!character) {
        return 0;
    }
    int new_value = (int)lua_tointeger(L, 2);
    write_to<int>(character, property_offset, new_value);
    return 0;
}

static int read_int_property(lua_State* L, size_t property_offset) {
    void* character = get_character_from_indirect_wrapper(L);
    if (!character) {
        lua_pushnil(L);
        return 1;
    }
    int current_value = read_from<int>(character, property_offset);
    lua_pushinteger(L, current_value);
    return 1;
}

/***
Writes a 32-bit integer at the given memory offset.
@function SetIntAtOffset
*/
static int script_SetIntAtOffset(lua_State* L) {
    void* character = get_character_from_indirect_wrapper(L);
    if (!character) return 0;
    size_t offset = (size_t)lua_tointeger(L, 2);
    int value = (int)lua_tointeger(L, 3);
    write_to<int>(character, offset, value);
    return 0;
}

/**
 * Reads a 32-bit integer at the given memory offset.
 * @function GetIntAtOffset
 * @tparam integer offset
 * @treturn integer value
 */
static int script_GetIntAtOffset(lua_State* L) {
    void* character = get_character_from_indirect_wrapper(L);
    if (!character) { lua_pushnil(L); return 1; }
    size_t offset = (size_t)lua_tointeger(L, 2);
    int v = read_from<int>(character, offset);
    lua_pushinteger(L, v);
    return 1;
}

/**
 * Returns the memory address of the real character object.
 * @function GetMemoryAddress
 * @treturn string memory address (e.g. "0x12345678")
 */
static int script_GetMemoryAddress(lua_State* L) {
    void* character = get_character_from_indirect_wrapper(L);
    if (!character) { lua_pushnil(L); return 1; }
    char address_buffer[64];
    sprintf_s(address_buffer, sizeof(address_buffer), "0x%p", character);
    lua_pushstring(L, address_buffer);
    return 1;
}

// Public registration table
const struct luaL_Reg character_functions[] = {
    {"GetMemoryAddress", script_GetMemoryAddress},
    {"GetIntAtOffset",   script_GetIntAtOffset},
    {"SetIntAtOffset",   script_SetIntAtOffset},
    {NULL, NULL}
};
