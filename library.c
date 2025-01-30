#include <lua.h>
#include <stdio.h>

int ping(lua_State *L) {
    lua_pushstring(L, "pong");
    return 1;
}

void log_lua_state(lua_State *L) {
    FILE *file = fopen("logging.txt", "a");
    if (file) {
        fprintf(file, "Hello World!\n");
    }
    fclose(file);
}

__declspec(dllexport) int luaopen_libtwdll(lua_State *L) {
    log_lua_state(L);
    lua_pushcfunction(L, ping);
    return 1;
}
