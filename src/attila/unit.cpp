/// @module twdll_unit
/// Campaign unit properties for Total War: Attila.
#include "../common/tw.h"
#include <cstddef>

// ── Memory layout ─────────────────────────────────────────────────────────────
// NOTE: Layout currently matches Rome 2. Verify before changing offsets.
#pragma pack(push, 1)
struct TW_Unit {
    char pad_00[0x44];
    int  current_number_of_men;  // 0x44
    int  max_number_of_men;      // 0x48
    char pad_4C[0x18];
    int  movement_points;        // 0x64
};
#pragma pack(pop)

static_assert(offsetof(TW_Unit, current_number_of_men) == 0x44, "TW_Unit Attila: current_number_of_men");
static_assert(offsetof(TW_Unit, max_number_of_men)     == 0x48, "TW_Unit Attila: max_number_of_men");
static_assert(offsetof(TW_Unit, movement_points)       == 0x64, "TW_Unit Attila: movement_points");

constexpr size_t UNIT_PTR = 0x8;

// ── Accessors ─────────────────────────────────────────────────────────────────
/***
Gets the current number of man in the unit.
@function GetNumberOfMan
@tparam userdata unit the unit object (first argument)
@treturn integer current number of man
*/
static twdll::Property<int, TW_Unit> NumberOfMan{&TW_Unit::current_number_of_men, UNIT_PTR, "unit"};
static int GetNumberOfMan(lua_State* L) { return NumberOfMan.get(L); }

/***
Sets the current number of man in the unit.
@function SetNumberOfMan
@tparam userdata unit the unit object (first argument)
@tparam integer value new number of man
*/
static int SetNumberOfMan(lua_State* L) { return NumberOfMan.set(L); }

/***
Gets the maximum number of man in the unit.
@function GetMaxNumberOfMan
@tparam userdata unit the unit object (first argument)
@treturn integer maximum number of man
*/
static twdll::Property<int, TW_Unit> MaxNumberOfMan{&TW_Unit::max_number_of_men, UNIT_PTR, "unit"};
static int GetMaxNumberOfMan(lua_State* L) { return MaxNumberOfMan.get(L); }

/***
Sets the maximum number of man in the unit.
@function SetMaxNumberOfMan
@tparam userdata unit the unit object (first argument)
@tparam integer value new maximum number of man
*/
static int SetMaxNumberOfMan(lua_State* L) { return MaxNumberOfMan.set(L); }

/***
Gets the remaining movement points for the unit.
@function GetMovementPoints
@tparam userdata unit the unit object (first argument)
@treturn integer movement points
*/
static twdll::Property<int, TW_Unit> MovementPoints{&TW_Unit::movement_points, UNIT_PTR, "unit"};
static int GetMovementPoints(lua_State* L) { return MovementPoints.get(L); }

/***
Sets the remaining movement points for the unit.
@function SetMovementPoints
@tparam userdata unit the unit object (first argument)
@tparam integer value new movement points
*/
static int SetMovementPoints(lua_State* L) { return MovementPoints.set(L); }

/***
Returns the memory address of the real unit object as a hexadecimal string.
@function GetMemoryAddress
@tparam userdata unit the unit object (first argument)
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemAddress(lua_State* L) { return tw_mem_address(L, "unit", UNIT_PTR); }

// ── Lua registration table ────────────────────────────────────────────────────
extern const luaL_Reg unit_functions[] = {
    {"GetNumberOfMan",    GetNumberOfMan},
    {"SetNumberOfMan",    SetNumberOfMan},
    {"GetMaxNumberOfMan", GetMaxNumberOfMan},
    {"SetMaxNumberOfMan", SetMaxNumberOfMan},
    {"GetMovementPoints", GetMovementPoints},
    {"SetMovementPoints", SetMovementPoints},
    {"GetMemoryAddress",  GetMemAddress},
    {nullptr, nullptr}
};
