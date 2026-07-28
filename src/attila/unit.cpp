/// @module UNIT_SCRIPT_INTERFACE
/// Extensions to the game's unit object.
#include "../common/tw.h"
#include "tw_types.h"

using twdll::TW_Unit;

constexpr size_t UNIT_PTR = twdll::TW_PtrOffset<TW_Unit>::value;

namespace Props {
    static twdll::Property NumberOfMan   {&TW_Unit::current_number_of_men, UNIT_PTR, "unit"};
    static twdll::Property MaxNumberOfMan{&TW_Unit::max_number_of_men,     UNIT_PTR, "unit"};
    static twdll::Property MovementPoints{&TW_Unit::movement_points,       UNIT_PTR, "unit"};
}

/***
Returns the memory address of the unit object as a hexadecimal string.
@function GetMemoryAddress
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemoryAddress    (lua_State* L) { return tw_mem_address(L, "unit", UNIT_PTR); }

/***
Gets the current number of men in the unit.
@function GetNumberOfMan
@treturn integer current number of men
*/
static int GetNumberOfMan   (lua_State* L) { return Props::NumberOfMan.get(L); }

/***
Sets the current number of men in the unit.
@function SetNumberOfMan
@tparam integer value new number of men
*/
static int SetNumberOfMan   (lua_State* L) { return Props::NumberOfMan.set(L); }

/***
Gets the maximum number of men the unit can have.
@function GetMaxNumberOfMan
@treturn integer maximum number of men
*/
static int GetMaxNumberOfMan(lua_State* L) { return Props::MaxNumberOfMan.get(L); }

/***
Sets the maximum number of men the unit can have.
@function SetMaxNumberOfMan
@tparam integer value new maximum number of men
*/
static int SetMaxNumberOfMan(lua_State* L) { return Props::MaxNumberOfMan.set(L); }

/***
Gets the movement points remaining for the unit.
@function GetMovementPoints
@treturn integer movement points
*/
static int GetMovementPoints(lua_State* L) { return Props::MovementPoints.get(L); }

/***
Sets the movement points for the unit.
@function SetMovementPoints
@tparam integer value new movement points
*/
static int SetMovementPoints(lua_State* L) { return Props::MovementPoints.set(L); }

extern const luaL_Reg unit_functions[] = {
    {nullptr, nullptr}
};

static const luaL_Reg unit_methods[] = {
    {"GetMemoryAddress",  GetMemoryAddress},
    {"GetNumberOfMan",    GetNumberOfMan},
    {"SetNumberOfMan",    SetNumberOfMan},
    {"GetMaxNumberOfMan", GetMaxNumberOfMan},
    {"SetMaxNumberOfMan", SetMaxNumberOfMan},
    {"GetMovementPoints", GetMovementPoints},
    {"SetMovementPoints", SetMovementPoints},
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
