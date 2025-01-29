#include <lua.h>

static int log_message(lua_State *L) {
    // Check if at least one argument is passed and is a string
    if (lua_isstring(L, 1)) {
        const char *message = lua_tostring(L, 1);
    } else {
    }

    return 0;  // No return value
}

__declspec(dllexport) int luaopen_libtwdll(lua_State *L) {
    lua_register(L, "log_message", log_message);
    return 1;
}
