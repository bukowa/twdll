#include <lua.h>
#include <stdio.h>
#include <string.h>

int respond_to_ping(lua_State *L) {
    const char *msg = lua_tostring(L, 1);

    if (msg && strcmp(msg, "ping") == 0) {
        lua_pushstring(L, "pong");
    } else {
        lua_pushstring(L, "Invalid message");
    }

    return 1;
}

void log_hello_world_to_file(lua_State *L) {
    FILE *file = fopen("ping_response_log.txt", "a");
    if (file) {
        fprintf(file, "Hello World!\n");
    }
    fclose(file);
}

__declspec(dllexport) int luaopen_libtwdll(lua_State *L) {
    log_hello_world_to_file(L);
    lua_pushcfunction(L, respond_to_ping);
    return 1;
}
