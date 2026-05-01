#pragma once

#include "lua/lua_api.h"

// Makes the functions from the .cpp file available to the main library file.
extern const struct luaL_Reg unit_functions[];
