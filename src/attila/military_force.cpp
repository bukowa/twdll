/// @module MILITARY_FORCE_SCRIPT_INTERFACE
/// Extensions to the game's military force object.
#include "../common/tw.h"
#include "game_api.h"
#include "tw_types.h"

using twdll::TW_MilitaryForce;

constexpr size_t MIL_FORCE_PTR = twdll::TW_PtrOffset<TW_MilitaryForce>::value;

namespace Props {
    static twdll::Getter RecruitmentQueueSize{&TW_MilitaryForce::recruitment_queue_size, MIL_FORCE_PTR, "military_force"};
}

/***
Returns the memory address of the military force object as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemoryAddress           (lua_State* L) { return tw_mem_address(L, "military_force", MIL_FORCE_PTR); }

/***
Returns the number of units in the recruitment queue.
@function GetRecruitmentQueueSize
@treturn integer number of units
*/
static int GetRecruitmentQueueSize (lua_State* L) { return Props::RecruitmentQueueSize.get(L); }

extern const luaL_Reg military_force_functions[] = {
    {nullptr, nullptr}
};

static const luaL_Reg military_force_methods[] = {
    {"GetMemoryAddress",        GetMemoryAddress},
    {"GetRecruitmentQueueSize", GetRecruitmentQueueSize},
    {nullptr, nullptr}
};

void register_military_force_methods(lua_State* L) {
    l_newmetatable(L, "MILITARY_FORCE_SCRIPT_INTERFACE");
    l_getfield(L, -1, "__index");
    if (l_type(L, -1) == LUA_TTABLE) {
        for (const luaL_Reg* f = military_force_methods; f->name; ++f) {
            l_pushstring(L, f->name);
            l_pushcclosure(L, f->func, 0);
            l_settable(L, -3);
        }
        Log("[twdll] MILITARY_FORCE_SCRIPT_INTERFACE extended");
    } else {
        Log("[twdll] WARNING: MILITARY_FORCE_SCRIPT_INTERFACE __index not found");
    }
    l_pop(L, 2);
}
