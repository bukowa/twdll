#pragma once

extern "C" {
#include <lua.h>
    #include <lauxlib.h> // For luaL_Reg
}

// Makes the functions from the .cpp file available to the main library file.
extern const struct luaL_Reg unit_functions[];
