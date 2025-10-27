/// @module twdll_faction
/// Functions for interacting with factions.
#include "script_utils.h"
#include "faction_script_interface.h"
#include <cstddef> // For NULL

#define REAL_FACTION_POINTER_OFFSET 0x8
#define FACTION_GOLD_OFFSET 0x6C4

/**
 * Returns the memory address of the real faction object.
 * @function GetMemoryAddress
 * @tparam userdata faction the faction object (first argument)
 * @treturn string memory address (e.g. "0x12345678")
 */
static int script_GetFactionMemoryAddress(lua_State* L) {
    return get_memory_address_lua(L, "faction", REAL_FACTION_POINTER_OFFSET);
}

/***
Gets the faction's gold.
@function GetGold
@tparam userdata faction the faction object
@treturn integer current gold
*/
static int script_GetGold(lua_State *L) {
    return read_int_property(L, REAL_FACTION_POINTER_OFFSET, FACTION_GOLD_OFFSET, "faction");
}

/***
Sets the faction's gold.
@function SetGold
@tparam userdata faction the faction object
@tparam integer value new gold amount
*/
static int script_SetGold(lua_State *L) {
    return write_int_property(L, REAL_FACTION_POINTER_OFFSET, FACTION_GOLD_OFFSET, "faction");
}

const struct luaL_Reg faction_functions[] = {
    {"GetMemoryAddress",  script_GetFactionMemoryAddress},
    {"GetGold", script_GetGold},
    {"SetGold", script_SetGold},
    {NULL, NULL}
};