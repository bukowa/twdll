/// @module twdll.unit
/// Campaign unit properties for Total War: Attila.
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
@tparam userdata unit the unit object
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemAddress    (lua_State* L) { return tw_mem_address(L, "unit", UNIT_PTR); }

/***
Gets the current number of men in the unit.
@function GetNumberOfMan
@tparam userdata unit the unit object
@treturn integer current number of men
*/
static int GetNumberOfMan   (lua_State* L) { return Props::NumberOfMan.get(L); }

/***
Sets the current number of men in the unit.
@function SetNumberOfMan
@tparam userdata unit the unit object
@tparam integer value new number of men
*/
static int SetNumberOfMan   (lua_State* L) { return Props::NumberOfMan.set(L); }

/***
Gets the maximum number of men the unit can have.
@function GetMaxNumberOfMan
@tparam userdata unit the unit object
@treturn integer maximum number of men
*/
static int GetMaxNumberOfMan(lua_State* L) { return Props::MaxNumberOfMan.get(L); }

/***
Sets the maximum number of men the unit can have.
@function SetMaxNumberOfMan
@tparam userdata unit the unit object
@tparam integer value new maximum number of men
*/
static int SetMaxNumberOfMan(lua_State* L) { return Props::MaxNumberOfMan.set(L); }

/***
Gets the movement points remaining for the unit.
@function GetMovementPoints
@tparam userdata unit the unit object
@treturn integer movement points
*/
static int GetMovementPoints(lua_State* L) { return Props::MovementPoints.get(L); }

/***
Sets the movement points for the unit.
@function SetMovementPoints
@tparam userdata unit the unit object
@tparam integer value new movement points
*/
static int SetMovementPoints(lua_State* L) { return Props::MovementPoints.set(L); }

extern const luaL_Reg unit_functions[] = {
    {"GetMemoryAddress",  GetMemAddress},
    {"GetNumberOfMan",    GetNumberOfMan},
    {"SetNumberOfMan",    SetNumberOfMan},
    {"GetMaxNumberOfMan", GetMaxNumberOfMan},
    {"SetMaxNumberOfMan", SetMaxNumberOfMan},
    {"GetMovementPoints", GetMovementPoints},
    {"SetMovementPoints", SetMovementPoints},
    {nullptr, nullptr}
};
