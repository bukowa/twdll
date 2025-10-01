#include "battle_unit_script_interface.h"
#include "log.h"
#include <cstdio>

extern "C" {
#include <lua.h>
#include <lauxlib.h>
}

#define REAL_BATTLE_UNIT_POINTER_OFFSET 0x4

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

// Tablica z funkcjami, BEZ "static", aby była widoczna na zewnątrz.
const struct luaL_Reg battle_unit_functions[] = {
    {"GetMemoryAddress",  script_GetBattleMemoryAddress},
    {NULL, NULL}
};