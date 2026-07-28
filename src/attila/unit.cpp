/// @module UNIT_SCRIPT_INTERFACE
/// Extensions to the game's unit object.
#include "../common/tw.h"
#include "tw_types.h"

using twdll::TW_Unit;

constexpr size_t UNIT_PTR = twdll::TW_PtrOffset<TW_Unit>::value;

namespace Props {
    static twdll::Property NumMen      {&TW_Unit::num_men,        UNIT_PTR, "unit"};
    static twdll::Property MaxNumMen   {&TW_Unit::max_num_men,    UNIT_PTR, "unit"};
    static twdll::Property ActionPoints{&TW_Unit::action_points,  UNIT_PTR, "unit"};
}

/***
Returns the memory address of the unit object as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemoryAddress    (lua_State* L) { return tw_mem_address(L, "unit", UNIT_PTR); }

/***
Gets the current number of men in the unit.
@function GetNumMen
@treturn integer current number of men
*/
static int GetNumMen        (lua_State* L) { return Props::NumMen.get(L); }

/***
Sets the current number of men in the unit.
@function SetNumMen
@tparam integer value new number of men
*/
static int SetNumMen        (lua_State* L) { return Props::NumMen.set(L); }

/***
Gets the maximum number of men the unit can have.
@function GetMaxNumMen
@treturn integer maximum number of men
*/
static int GetMaxNumMen     (lua_State* L) { return Props::MaxNumMen.get(L); }

/***
Sets the maximum number of men the unit can have.
@function SetMaxNumMen
@tparam integer value new maximum number of men
*/
static int SetMaxNumMen     (lua_State* L) { return Props::MaxNumMen.set(L); }

/***
Gets the action points remaining for the unit.
@function GetActionPoints
@treturn integer action points
*/
static int GetActionPoints  (lua_State* L) { return Props::ActionPoints.get(L); }

/***
Sets the action points for the unit.
@function SetActionPoints
@tparam integer value new action points
*/
static int SetActionPoints  (lua_State* L) { return Props::ActionPoints.set(L); }

extern const luaL_Reg unit_functions[] = {
    {nullptr, nullptr}
};

static const luaL_Reg unit_methods[] = {
    {"GetMemoryAddress",  GetMemoryAddress},
    {"GetNumMen",         GetNumMen},
    {"SetNumMen",         SetNumMen},
    {"GetMaxNumMen",      GetMaxNumMen},
    {"SetMaxNumMen",      SetMaxNumMen},
    {"GetActionPoints",   GetActionPoints},
    {"SetActionPoints",   SetActionPoints},
    {nullptr, nullptr}
};

void register_unit_methods(lua_State* L) {
    l_newmetatable(L, "UNIT_SCRIPT_INTERFACE");
    l_getfield(L, -1, "__index");
    if (l_type(L, -1) == LUA_TTABLE) {
        for (const luaL_Reg* f = unit_methods; f->name; ++f) {
            l_pushstring(L, f->name);
            l_pushcclosure(L, f->func, 0);
            l_settable(L, -3);
        }
        Log("[twdll] UNIT_SCRIPT_INTERFACE extended");
    } else {
        Log("[twdll] WARNING: UNIT_SCRIPT_INTERFACE __index not found");
    }
    l_pop(L, 2);
}
