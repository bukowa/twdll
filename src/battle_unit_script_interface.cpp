/// @module twdll_battle_unit
/// Functions for interacting with battle units.
#include "battle_unit_script_interface.h"
#include "log.h"
#include <cstdio>

extern "C" {
#include <lua.h>
#include <lauxlib.h>
}

#define REAL_BATTLE_UNIT_POINTER_OFFSET 0x4
// --- Offsets within the main Unit object to find POINTERS to other objects ---
#define UNIT_STATS_POINTER_OFFSET 0x20

// --- Offsets within the nested Unit Stats object ---
#define STATS_CHARGE_BONUS_OFFSET 0x010C
#define STATS_MELEE_ATTACK_OFFSET 0x011C
#define STATS_BASE_MORALE_OFFSET  0x0118
#define STATS_FLOAT_EXAMPLE_OFFSET 0x0110 // To jest ten float ze zdjęcia

#define UNIT_FATIGUE_OFFSET 0x948


template<typename T>
T read_from(void *base_address, const size_t offset) {
    return *reinterpret_cast<T *>(static_cast<char *>(base_address) + offset);
}

template<typename T>
void write_to(void *base_address, const size_t offset, T value) {
    *reinterpret_cast<T *>(static_cast<char *>(base_address) + offset) = value;
}


static void *get_battle_unit_from_indirect_wrapper(lua_State *L) {
    void **p_wrapper = static_cast<void **>(lua_touserdata(L, 1));
    if (!p_wrapper || !*p_wrapper) {
        Log("ERROR (get_battle_unit_indirect): Argument is not a valid pointer to a wrapper.");
        return nullptr;
    }
    void *wrapper = *p_wrapper;
    void *battle_unit_object = *reinterpret_cast<void **>(static_cast<char *>(wrapper) + REAL_BATTLE_UNIT_POINTER_OFFSET);
    if (!battle_unit_object) {
        Log("ERROR (get_battle_unit_indirect): Real BattleUnit pointer is null.");
        return nullptr;
    }
    return battle_unit_object;
}

// --- Generic "Engine" Functions for Nested Objects ---

// Silnik, który czyta int z zagnieżdżonego obiektu.
static int read_nested_int_property(lua_State* L, size_t nested_obj_ptr_offset, size_t final_property_offset) {
    void* base_unit = get_battle_unit_from_indirect_wrapper(L);
    if (!base_unit) { lua_pushnil(L); return 1; }

    void* nested_obj = read_from<void*>(base_unit, nested_obj_ptr_offset);
    if (!nested_obj) { lua_pushnil(L); return 1; }

    int final_value = read_from<int>(nested_obj, final_property_offset);
    lua_pushinteger(L, final_value);
    return 1;
}

// Silnik, który czyta float z zagnieżdżonego obiektu.
static int read_nested_float_property(lua_State* L, size_t nested_obj_ptr_offset, size_t final_property_offset) {
    void* base_unit = get_battle_unit_from_indirect_wrapper(L);
    if (!base_unit) { lua_pushnil(L); return 1; }

    void* nested_obj = read_from<void*>(base_unit, nested_obj_ptr_offset);
    if (!nested_obj) { lua_pushnil(L); return 1; }

    float final_value = read_from<float>(nested_obj, final_property_offset);
    lua_pushnumber(L, final_value);
    return 1;
}

static int read_int_property(lua_State* L, size_t property_offset) {
    void* battle_unit = get_battle_unit_from_indirect_wrapper(L);
    if (!battle_unit) {
        lua_pushnil(L);
        return 1;
    }
    const int current_value = read_from<int>(battle_unit, property_offset);
    lua_pushinteger(L, current_value);
    return 1;
}

/**
 * Returns the memory address of the real battle unit object.
 * @function GetMemoryAddress
 * @tparam userdata unit the battle unit object (first argument)
 * @treturn string memory address (e.g. "0x12345678")
 */
static int script_GetBattleMemoryAddress(lua_State* L) {
    void* battle_unit = get_battle_unit_from_indirect_wrapper(L);
    if (!battle_unit) {
        lua_pushnil(L);
        return 1;
    }
    char address_buffer[64];
    sprintf_s(address_buffer, sizeof(address_buffer), "0x%p", battle_unit);
    lua_pushstring(L, address_buffer);
    return 1;
}

// --- Lua API Bridge Functions for Unit Stats ---

/***
Gets the unit's charge bonus from the nested stats object.
@function GetChargeBonus
@tparam userdata unit the battle unit object (first argument)
@treturn integer charge bonus
*/
static int script_stats_GetChargeBonus(lua_State* L) {
    return read_nested_int_property(L, UNIT_STATS_POINTER_OFFSET, STATS_CHARGE_BONUS_OFFSET);
}

/***
Gets the unit's melee attack from the nested stats object.
@function GetMeleeAttack
@tparam userdata unit the battle unit object (first argument)
@treturn integer melee attack
*/
static int script_stats_GetMeleeAttack(lua_State* L) {
    return read_nested_int_property(L, UNIT_STATS_POINTER_OFFSET, STATS_MELEE_ATTACK_OFFSET);
}

/***
Gets the unit's base morale from the nested stats object.
@function GetBaseMorale
@tparam userdata unit the battle unit object (first argument)
@treturn integer base morale
*/
static int script_stats_GetBaseMorale(lua_State* L) {
    return read_nested_int_property(L, UNIT_STATS_POINTER_OFFSET, STATS_BASE_MORALE_OFFSET);
}

/***
Reads an example float stat from the nested stats object at offset 0x110.
@function GetSomeFloatValue
@tparam userdata unit the battle unit object (first argument)
@treturn number value
*/
// Przykład dla float-a z twojego obrazka (offset 0x110)
static int script_stats_GetSomeFloatValue(lua_State* L) {
    return read_nested_float_property(L, UNIT_STATS_POINTER_OFFSET, STATS_FLOAT_EXAMPLE_OFFSET);
}

/***
Gets the unit's fatigue level.
@function GetFatigue
@tparam userdata unit the battle unit object (first argument)
@treturn integer fatigue
*/
static int script_unit_GetFatigue(lua_State* L) {
    return read_int_property(L, UNIT_FATIGUE_OFFSET);
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
