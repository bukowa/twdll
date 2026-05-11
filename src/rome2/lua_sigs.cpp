// rome2/lua_sigs.cpp — Rome 2 Lua function signatures
// Contains: game module name, byte signatures for all hooked Lua functions.
// Consumed by src/lua/lua_api.cpp via extern g_signatures[].
// No preprocessor conditionals — this file IS the Rome 2 variant.

#include "../common/lua_api.h"
#include "../common/signature_scanner.h"

const char* GAME_NAME = "Rome2";
const char* GAME_MODULE_NAME = "empire.retail.dll";

// clang-format off
#define LUA_PUSHSTRING_SIG      "55 8B 6C 24 ? 85 ED 75 ? 8B 4C 24"
#define LUA_NEWUSERDATA_SIG     "56 8B 74 24 ? 8B 4E ? 8B 41 ? 3B 41 ? 72 ? 56 E8 ? ? ? ? 83 C4 ? 8B 46"
#define LUAL_NEWMETATABLE_SIG   "56 FF 74 24 ? 8B 74 24 ? 68"
#define LUA_PUSHCCLOSURE_SIG    "55 56 8B 74 24 ? 57 8B 4E ? 8B 41"
#define LUA_SETTABLE_SIG        "56 FF 74 24 ? 8B 74 24 ? 56 E8 ? ? ? ? 8B 56 ? 8D 4A"
#define LUA_SETMETATABLE_SIG    "56 8B 74 24 ? 57 FF 74 24 ? 56 E8 ? ? ? ? 8B 4E ? 83 C4"
#define LUA_SETFIELD_SIG        "83 EC ? 53 56 8B 74 24 ? 57 FF 74 24 ? 56 E8 ? ? ? ? 8B 54 24 ? 83 C4 ? 8B CA 8B F8 8D 59 ? ? ? 41 84 C0 75 ? 2B CB 51 52 56 E8 ? ? ? ? 89 44 24"
#define LUAL_CHECKLSTRING_SIG   "53 55 8B 6C 24 ? 57 FF 74 24"
#define LUAL_REGISTER_SIG       "55 8B 6C 24 ? 56 8B 74 24 ? 57 8B 7C 24 ? 85 ED"
#define LUA_CREATETABLE_SIG     "56 57 8B 7C 24 ? 8B 4F ? 8B 41 ? 3B 41 ? 72 ? 57 E8 ? ? ? ? 83 C4 ? FF 74 24 ? 8B 77 ? FF 74 24 ? 57 E8 ? ? ? ? 83 C4 ? ? ? C7 46 ? ? ? ? ? 83 47 ? ? 5F 5E C3 ? ? 8B 4C 24 ? 8B 41 ? 83 78"
#define LUA_TOUSERDATA_SIG      "FF 74 24 ? FF 74 24 ? E8 ? ? ? ? 83 C4 ? 8B 48 ? 83 E9 ? 74 ? 83 E9 ? 74"
#define LUA_PUSHINTEGER_SIG     "8B 4C 24 ? 66 0F 6E 44 24"
#define LUA_TOINTEGER_SIG       "83 EC ? FF 74 24 ? FF 74 24 ? E8 ? ? ? ? 83 C4 ? 83 78 ? ? 74 ? ? ? ? 51 50 E8 ? ? ? ? 83 C4 ? 85 C0 75 ? 83 C4 ? C3 ? ? ? ? 83 C4"
#define LUA_PUSHNUMBER_SIG      "8B 4C 24 ? F3 0F 10 44 24 ? 8B 41"
#define LUA_TONUMBER_SIG        "83 EC ? FF 74 24 ? FF 74 24 ? E8 ? ? ? ? 83 C4 ? 83 78 ? ? 74 ? ? ? ? 51 50 E8 ? ? ? ? 83 C4 ? 85 C0 75 ? ? ? 83 C4"
#define LUA_PUSHNIL_SIG         "8B 4C 24 ? 8B 41 ? C7 40"
#define LUA_PCALL_SIG           "8B 44 24 ? 83 EC ? 53 56 57 8B 7C 24"
#define LUAB_LOADSTRING_SIG     "51 56 57 8B 7C 24 ? 8D 44 24 ? 50 6A"
// clang-format on

// Table consumed by initialize_lua_api() in lua_api.cpp.
extern const TW_SignatureInfo g_signatures[] = {
    {"lua_pushstring",   (void**)&g_game_lua_pushstring,    LUA_PUSHSTRING_SIG},
    {"lua_newuserdata",  (void**)&g_game_lua_newuserdata,   LUA_NEWUSERDATA_SIG},
    {"luaL_newmetatable",(void**)&g_game_luaL_newmetatable, LUAL_NEWMETATABLE_SIG},
    {"lua_pushcclosure", (void**)&g_game_lua_pushcclosure,  LUA_PUSHCCLOSURE_SIG},
    {"lua_settable",     (void**)&g_game_lua_settable,       LUA_SETTABLE_SIG},
    {"lua_setmetatable", (void**)&g_game_lua_setmetatable,  LUA_SETMETATABLE_SIG},
    {"lua_setfield",     (void**)&g_game_lua_setfield,       LUA_SETFIELD_SIG},
    {"luaL_checklstring",(void**)&g_game_luaL_checklstring, LUAL_CHECKLSTRING_SIG},
    {"luaL_register",    (void**)&g_game_luaL_register,     LUAL_REGISTER_SIG},
    {"lua_createtable",  (void**)&g_game_lua_createtable,   LUA_CREATETABLE_SIG},
    {"lua_touserdata",   (void**)&g_game_lua_touserdata,    LUA_TOUSERDATA_SIG},
    {"lua_pushinteger",  (void**)&g_game_lua_pushinteger,   LUA_PUSHINTEGER_SIG},
    {"lua_tointeger",    (void**)&g_game_lua_tointeger,     LUA_TOINTEGER_SIG},
    {"lua_pushnumber",   (void**)&g_game_lua_pushnumber,    LUA_PUSHNUMBER_SIG},
    {"lua_tonumber",     (void**)&g_game_lua_tonumber,      LUA_TONUMBER_SIG},
    {"lua_pushnil",      (void**)&g_game_lua_pushnil,        LUA_PUSHNIL_SIG},
    {"lua_pcall",        (void**)&g_game_lua_pcall,          LUA_PCALL_SIG},
    {"luaB_loadstring",  (void**)&g_game_luaB_loadstring,   LUAB_LOADSTRING_SIG},
    {nullptr, nullptr, nullptr}
};

extern const uintptr_t OFFSET_MAX_UNITS_ARMY = 0x193FC68;
extern const uintptr_t OFFSET_MAX_UNITS_NAVY = 0x193FC6C;
