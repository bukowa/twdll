#include "lua_api.h"
#include <cstdint>
#include <cstring>
#include <windows.h>
#include <Psapi.h>
#include "log.h"
#include "signature_scanner.h"

// Game function pointer definitions (declared extern in lua_api.h)
lua_pushstring_t    g_game_lua_pushstring    = nullptr;
lua_newuserdata_t   g_game_lua_newuserdata   = nullptr;
luaL_newmetatable_t g_game_luaL_newmetatable = nullptr;
lua_pushcclosure_t  g_game_lua_pushcclosure  = nullptr;
lua_settable_t      g_game_lua_settable      = nullptr;
lua_setmetatable_t  g_game_lua_setmetatable  = nullptr;
lua_setfield_t      g_game_lua_setfield      = nullptr;
luaL_checklstring_t g_game_luaL_checklstring = nullptr;
luaL_register_t     g_game_luaL_register     = nullptr;
lua_createtable_t   g_game_lua_createtable   = nullptr;
lua_touserdata_t    g_game_lua_touserdata    = nullptr;
lua_pushinteger_t   g_game_lua_pushinteger   = nullptr;
lua_tointeger_t     g_game_lua_tointeger     = nullptr;
lua_pushnumber_t    g_game_lua_pushnumber    = nullptr;
lua_tonumber_t      g_game_lua_tonumber      = nullptr;
lua_pushnil_t       g_game_lua_pushnil       = nullptr;
lua_pcall_t         g_game_lua_pcall         = nullptr;
luaB_loadstring     g_game_luaB_loadstring   = nullptr;

// Provided by the game-specific translation unit (rome2/lua_sigs.cpp or attila/lua_sigs.cpp)
extern const char*            GAME_MODULE_NAME;
extern const TW_SignatureInfo g_signatures[];

void initialize_lua_api() {
    HMODULE hMod = GetModuleHandleA(GAME_MODULE_NAME);
    if (!hMod) {
        Log("[twdll] initialize_lua_api: failed to get handle for '%s'", GAME_MODULE_NAME);
        return;
    }

    MODULEINFO mi = {};
    if (!GetModuleInformation(GetCurrentProcess(), hMod, &mi, sizeof(mi))) {
        Log("[twdll] initialize_lua_api: GetModuleInformation failed for '%s'", GAME_MODULE_NAME);
        return;
    }

    uintptr_t base = reinterpret_cast<uintptr_t>(hMod);
    size_t    size = mi.SizeOfImage;

    for (const TW_SignatureInfo* s = g_signatures; s->function_name != nullptr; ++s) {
        if (!s->signature || s->signature[0] == '\0') {
            Log("[twdll] Skipping '%s': empty signature", s->function_name);
            continue;
        }
        try {
            uintptr_t addr = find_signature(base, size, s->signature);
            if (addr) {
                *s->target_function_ptr = reinterpret_cast<void*>(addr);
                Log("[twdll] Found: %s", s->function_name);
            } else {
                Log("[twdll] NOT FOUND: %s", s->function_name);
            }
        } catch (const std::exception& e) {
            Log("[twdll] Exception scanning '%s': %s", s->function_name, e.what());
        } catch (...) {
            Log("[twdll] Unknown exception scanning '%s'", s->function_name);
        }
    }
}
