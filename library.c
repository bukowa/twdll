#include <lua.h>
#include <stdio.h>
#include <windows.h>
#include <lauxlib.h>

lua_State *global_luaState;

DWORD WINAPI ConsoleThread(LPVOID param) {
    AllocConsole();
    freopen("CONOUT$", "w", stdout); // Redirect stdout to new console
    freopen("CONIN$", "r", stdin); // Redirect stdin to new console

    char input[256];

    while (1) {
        printf("Lua> ");
        fflush(stdout);

        if (fgets(input, sizeof(input), stdin)) {

            if (strcmp(input, ">pg\n") == 0) {
                luaL_dostring(global_luaState,
                    "return (function(tbl) "
                    "local str = '' "
                    "for k, v in pairs(tbl) do "
                    "    str = str .. tostring(k) .. ': ' .. tostring(v) .. '\\n' "
                    "end "
                    "return str "
                    "end)(_G)");
            }

            else if (luaL_dostring(global_luaState, input)) {
                // Execute in Lua 5.1
                printf("Lua Error: %s\n", lua_tostring(global_luaState, -1));
                lua_pop(global_luaState, 1); // Remove error message from stack
            }

            // Print Lua function's return value (if any)
            int nresults = lua_gettop(global_luaState); // Get the number of return values
            for (int i = 1; i <= nresults; ++i) {
                if (lua_isnil(global_luaState, i)) {
                    printf("twdll>nil\n");
                } else if (lua_isstring(global_luaState, i)) {
                    printf("%s\n", lua_tostring(global_luaState, i));
                } else if (lua_isnumber(global_luaState, i)) {
                    printf("%g\n", lua_tonumber(global_luaState, i));
                } else if (lua_istable(global_luaState, i)) {
                    printf("twdll>table\n");
                } else {
                    printf("Unknown return type\n");
                }
            }
            lua_settop(global_luaState, 0); // Clear the stack after handling the results
        }
    }
}

void start_interactive_console() {
    HANDLE thread = CreateThread(NULL, 0, ConsoleThread, NULL, 0, NULL);
    if (thread) CloseHandle(thread); // Close handle to avoid leaks
}

__declspec(dllexport) int luaopen_libtwdll(lua_State *L) {
    global_luaState = L;
    start_interactive_console();
    return 0;
}
