/// @module twdll_character
/// Campaign character properties for Rome 2.
#include "../tw.h"
#include <cstddef>

// ── Memory layout ─────────────────────────────────────────────────────────────
#pragma pack(push, 1)
struct TW_Character {
    char pad_00[0x14];
    int  movement_points;  // 0x14
    char pad_18[0x540];
    int  ambition;         // 0x558
    int  gravitas;         // 0x55C
};
#pragma pack(pop)

static_assert(offsetof(TW_Character, movement_points) == 0x14,  "TW_Character Rome2: movement_points");
static_assert(offsetof(TW_Character, ambition)        == 0x558, "TW_Character Rome2: ambition");
static_assert(offsetof(TW_Character, gravitas)        == 0x55C, "TW_Character Rome2: gravitas");

constexpr size_t CHAR_PTR = 0x8;

// ── Accessors ─────────────────────────────────────────────────────────────────
/***
Gets the remaining movement points for the character.
@function GetMovementPoints
@tparam userdata character the character object (first argument)
@treturn integer movement points
*/
/***
Sets the remaining movement points for the character.
@function SetMovementPoints
@tparam userdata character the character object (first argument)
@tparam integer value new movement points
*/
TW_PROP(MovementPoints, TW_Character, movement_points, CHAR_PTR, "character")

/***
Gets the ambition level of the character.
@function GetAmbition
@tparam userdata character the character object (first argument)
@treturn integer ambition level
*/
/***
Sets the ambition level of the character.
@function SetAmbition
@tparam userdata character the character object (first argument)
@tparam integer value new ambition level
*/
TW_PROP(Ambition,       TW_Character, ambition,        CHAR_PTR, "character")

/***
Gets the gravitas of the character.
@function GetGravitas
@tparam userdata character the character object (first argument)
@treturn integer gravitas
*/
/***
Sets the gravitas of the character.
@function SetGravitas
@tparam userdata character the character object (first argument)
@tparam integer value new gravitas
*/
TW_PROP(Gravitas,       TW_Character, gravitas,        CHAR_PTR, "character")

/***
Returns the memory address of the real character object as a hexadecimal string.
@function GetMemoryAddress
@tparam userdata character the character object (first argument)
@treturn string memory address (e.g. "0x12345678")
*/
static int GetMemAddress(lua_State* L) { return tw_mem_address(L,  "character", CHAR_PTR); }
static int GetIntAtOffset(lua_State* L) { return tw_get_int_at(L, "character", CHAR_PTR); }
static int SetIntAtOffset(lua_State* L) { return tw_set_int_at(L, "character", CHAR_PTR); }

// ── Lua registration table ────────────────────────────────────────────────────
extern const luaL_Reg character_functions[] = {
    {"GetMemoryAddress",  GetMemAddress},
    {"GetIntAtOffset",    GetIntAtOffset},
    {"SetIntAtOffset",    SetIntAtOffset},
    {"GetMovementPoints", GetMovementPoints},
    {"SetMovementPoints", SetMovementPoints},
    {"GetAmbition",       GetAmbition},
    {"SetAmbition",       SetAmbition},
    {"GetGravitas",       GetGravitas},
    {"SetGravitas",       SetGravitas},
    {nullptr, nullptr}
};
