/// @module twdll_battle_unit
/// Functions for interacting with battle units.
#include "script_utils.h"
#include "battle_unit_script_interface.h"

#define REAL_BATTLE_UNIT_POINTER_OFFSET 0x4

// --- Offsets within the main Unit object to find POINTERS to other objects ---
#define UNIT_STATS_POINTER_OFFSET 0x20

// --- Offsets within the nested Unit Stats object ---
#define STATS_CHARGE_BONUS_OFFSET 0x010C
#define STATS_MELEE_ATTACK_OFFSET 0x011C
#define STATS_BASE_MORALE_OFFSET  0x0118
#define STATS_FLOAT_EXAMPLE_OFFSET 0x0110

#define UNIT_FATIGUE_OFFSET 0x948

/**
 * Returns the memory address of the real battle unit object.
 * @function GetMemoryAddress
 * @tparam userdata unit the battle unit object (first argument)
 * @treturn string memory address (e.g. "0x12345678")
 */
static int script_GetBattleMemoryAddress(lua_State* L) {
    return get_memory_address_lua(L, "battle_unit", REAL_BATTLE_UNIT_POINTER_OFFSET);
}

// --- Lua API Bridge Functions for Unit Stats ---

/***
Gets the unit's charge bonus from the nested stats object.
@function GetChargeBonus
@tparam userdata unit the battle unit object (first argument)
@treturn integer charge bonus
*/
static int script_stats_GetChargeBonus(lua_State* L) {
    return read_nested_int_property(L, REAL_BATTLE_UNIT_POINTER_OFFSET, UNIT_STATS_POINTER_OFFSET, STATS_CHARGE_BONUS_OFFSET, "battle_unit");
}

/***
Gets the unit's melee attack from the nested stats object.
@function GetMeleeAttack
@tparam userdata unit the battle unit object (first argument)
@treturn integer melee attack
*/
static int script_stats_GetMeleeAttack(lua_State* L) {
    return read_nested_int_property(L, REAL_BATTLE_UNIT_POINTER_OFFSET, UNIT_STATS_POINTER_OFFSET, STATS_MELEE_ATTACK_OFFSET, "battle_unit");
}

/***
Gets the unit's base morale from the nested stats object.
@function GetBaseMorale
@tparam userdata unit the battle unit object (first argument)
@treturn integer base morale
*/
static int script_stats_GetBaseMorale(lua_State* L) {
    return read_nested_int_property(L, REAL_BATTLE_UNIT_POINTER_OFFSET, UNIT_STATS_POINTER_OFFSET, STATS_BASE_MORALE_OFFSET, "battle_unit");
}

/***
Reads an example float stat from the nested stats object at offset 0x110.
@function GetSomeFloatValue
@tparam userdata unit the battle unit object (first argument)
@treturn number value
*/
static int script_stats_GetSomeFloatValue(lua_State* L) {
    return read_nested_float_property(L, REAL_BATTLE_UNIT_POINTER_OFFSET, UNIT_STATS_POINTER_OFFSET, STATS_FLOAT_EXAMPLE_OFFSET, "battle_unit");
}

/***
Gets the unit's fatigue level.
@function GetFatigue
@tparam userdata unit the battle unit object (first argument)
@treturn integer fatigue
*/
static int script_unit_GetFatigue(lua_State* L) {
    return read_int_property(L, REAL_BATTLE_UNIT_POINTER_OFFSET, UNIT_FATIGUE_OFFSET, "battle_unit");
}

// Tablica z funkcjami, BEZ "static", aby była widoczna na zewnątrz.
const struct luaL_Reg battle_unit_functions[] = {
    {"GetMemoryAddress",  script_GetBattleMemoryAddress},
    {"GetFatigue",        script_unit_GetFatigue},
    // Merged stats functions
    {"GetChargeBonus",    script_stats_GetChargeBonus},
    {"GetMeleeAttack",    script_stats_GetMeleeAttack},
    {"GetBaseMorale",     script_stats_GetBaseMorale},
    {"GetSomeFloatValue", script_stats_GetSomeFloatValue},
    {NULL, NULL}
};
