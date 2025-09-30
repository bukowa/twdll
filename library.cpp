#include <windows.h>
#include <stdio.h> // For sprintf_s

extern "C" {
    #include <lua.h>
    #include <lauxlib.h>
}

#include "MinHook.h"

// The event handler function we want to hook
// Signature: void __thiscall EventHandler(void* pContext, lua_State* L);
typedef void (__thiscall *tEventHandler)(void* pContext, lua_State* L);
tEventHandler oEventHandler = NULL; // This will store the original function

// The global Lua state, which we get when the DLL is loaded
lua_State *g_luaState = NULL;

// --- Our simple hook function ---
// This function will run instead of the game's original event handler
void __fastcall hkEventHandler(void* pContext, void* edx, lua_State* L) {
    // pContext is the C++ 'context' object. We can now inspect it.

    // Let's create a message to print. We'll include the memory address of the context object.
    char message_buffer[256];
    sprintf_s(message_buffer, sizeof(message_buffer), "pwrite('HOOK SUCCESS! Context object is at address: 0x%p')", pContext);

    // Execute this Lua string in the game's Lua state
    // This is the C++ equivalent of typing into the console
    if (g_luaState) {
        luaL_dostring(g_luaState, message_buffer);
    }

    // IMPORTANT: Now call the original game function so the event still fires!
    return oEventHandler(pContext, L);
}


// --- The main entry point for the DLL, called by Lua ---
extern "C" __declspec(dllexport) int luaopen_libtwdll(lua_State *L) {
    // ... all your code ...
    return 0;
}