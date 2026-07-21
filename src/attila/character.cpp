/// @module twdll_character
/// Campaign character properties for Total War: Attila.
#include "../common/tw.h"
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

static_assert(offsetof(TW_Character, movement_points) == 0x14,  "TW_Character Attila: movement_points");
static_assert(offsetof(TW_Character, ambition)        == 0x558, "TW_Character Attila: ambition");
static_assert(offsetof(TW_Character, gravitas)        == 0x55C, "TW_Character Attila: gravitas");

constexpr size_t CHAR_PTR = 0x8;

// ── Accessors ─────────────────────────────────────────────────────────────────
/***
Gets the remaining movement points for the character.
@function GetMovementPoints
@tparam userdata character the character object (first argument)
@treturn integer movement points
*/
static twdll::Property<int, TW_Character> MovementPoints{&TW_Character::movement_points, CHAR_PTR, "character"};
static int GetMovementPoints(lua_State* L) { return MovementPoints.get(L); }

/***
Sets the remaining movement points for the character.
@function SetMovementPoints
@tparam userdata character the character object (first argument)
@tparam integer value new movement points
*/
static int SetMovementPoints(lua_State* L) { return MovementPoints.set(L); }

/***
Gets the ambition level of the character.
@function GetAmbition
@tparam userdata character the character object (first argument)
@treturn integer ambition level
*/
static twdll::Property<int, TW_Character> Ambition{&TW_Character::ambition, CHAR_PTR, "character"};
static int GetAmbition(lua_State* L) { return Ambition.get(L); }

/***
Sets the ambition level of the character.
@function SetAmbition
@tparam userdata character the character object (first argument)
@tparam integer value new ambition level
*/
static int SetAmbition(lua_State* L) { return Ambition.set(L); }

/***
Gets the gravitas of the character.
@function GetGravitas
@tparam userdata character the character object (first argument)
@treturn integer gravitas
*/
static twdll::Property<int, TW_Character> Gravitas{&TW_Character::gravitas, CHAR_PTR, "character"};
static int GetGravitas(lua_State* L) { return Gravitas.get(L); }

/***
Sets the gravitas of the character.
@function SetGravitas
@tparam userdata character the character object (first argument)
@tparam integer value new gravitas
*/
static int SetGravitas(lua_State* L) { return Gravitas.set(L); }

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
