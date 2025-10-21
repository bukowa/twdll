/// @module twdll_military_force
/// Functions for interacting with military forces.
#include "script_utils.h"
#include "military_force_script_interface.h"

#define REAL_MILITARY_FORCE_POINTER_OFFSET 0x8
#define RECRUITMENT_QUEUE_SIZE_OFFSET 0x45C

/**
 * Returns the memory address of the real military force object.
 * @function GetMemoryAddress
 * @tparam userdata military_force the military force object (first argument)
 * @treturn string memory address (e.g. "0x12345678")
 */
static int script_GetMemoryAddress(lua_State* L) {
    return get_memory_address_lua(L, "military_force", REAL_MILITARY_FORCE_POINTER_OFFSET);
}

/**
 * Returns the number of units in the recruitment queue.
 * @function GetRecruitmentQueueSize
 * @tparam userdata military_force the military force object (first argument)
 * @treturn integer number of units
 */
static int script_GetRecruitmentQueueSize(lua_State* L) {
    return read_int_property(L, REAL_MILITARY_FORCE_POINTER_OFFSET, RECRUITMENT_QUEUE_SIZE_OFFSET, "military_force");
}

const struct luaL_Reg military_force_functions[] = {
    {"GetMemoryAddress",  script_GetMemoryAddress},
    {"GetRecruitmentQueueSize", script_GetRecruitmentQueueSize},
    {NULL, NULL}
};