#include "lua_api.h"
#include <cstdint>
#include <windows.h>
#include <Psapi.h>
#include <cstring>
#include "log.h"
#include "signature_scanner.h"

#ifndef BUILD_TESTING_LUA // Start of production-only block

lua_pushstring_t g_game_lua_pushstring = nullptr;
lua_newuserdata_t g_game_lua_newuserdata = nullptr;
luaL_newmetatable_t g_game_luaL_newmetatable = nullptr;
lua_pushcclosure_t g_game_lua_pushcclosure = nullptr;
lua_settable_t g_game_lua_settable = nullptr;
lua_setmetatable_t g_game_lua_setmetatable = nullptr;
lua_setfield_t g_game_lua_setfield = nullptr;
luaL_checklstring_t g_game_luaL_checklstring = nullptr;
luaL_register_t g_game_luaL_register = nullptr;
lua_createtable_t g_game_lua_createtable = nullptr;
lua_touserdata_t g_game_lua_touserdata = nullptr;
lua_pushinteger_t g_game_lua_pushinteger = nullptr;
lua_tointeger_t g_game_lua_tointeger = nullptr;
lua_pushnumber_t g_game_lua_pushnumber = nullptr;
lua_tonumber_t g_game_lua_tonumber = nullptr;
lua_pushnil_t g_game_lua_pushnil = nullptr;
lua_pcall_t g_game_lua_pcall = nullptr;
luaB_loadstring g_game_luaB_loadstring = nullptr;

struct EmpireSignatureInfo {
    const char* function_name;
    void** target_function_ptr;
    const char* signature;
};

#ifdef STEAM_BUILD
const char* GAME_MODULE_NAME = "empire.retail.dll";
#define LUA_PUSHSTRING_SIG "55 8B 6C 24 ? 85 ED 75 ? 8B 4C 24"
#define LUA_NEWUSERDATA_SIG "56 8B 74 24 ? 8B 4E ? 8B 41 ? 3B 41 ? 72 ? 56 E8 ? ? ? ? 83 C4 ? 8B 46"
#define LUAL_NEWMETATABLE_SIG "56 FF 74 24 ? 8B 74 24 ? 68"
#define LUA_PUSHCCLOSURE_SIG "55 56 8B 74 24 ? 57 8B 4E ? 8B 41"
#define LUA_SETTABLE_SIG "56 FF 74 24 ? 8B 74 24 ? 56 E8 ? ? ? ? 8B 56 ? 8D 4A"
#define LUA_SETMETATABLE_SIG "56 8B 74 24 ? 57 FF 74 24 ? 56 E8 ? ? ? ? 8B 4E ? 83 C4"
#define LUA_SETFIELD_SIG "83 EC ? 53 56 8B 74 24 ? 57 FF 74 24 ? 56 E8 ? ? ? ? 8B 54 24 ? 83 C4 ? 8B CA 8B F8 8D 59 ? ? ? 41 84 C0 75 ? 2B CB 51 52 56 E8 ? ? ? ? 89 44 24"
#define LUAL_CHECKLSTRING_SIG "53 55 8B 6C 24 ? 57 FF 74 24"
#define LUAL_REGISTER_SIG "55 8B 6C 24 ? 56 8B 74 24 ? 57 8B 7C 24 ? 85 ED"
#define LUA_CREATETABLE_SIG "56 57 8B 7C 24 ? 8B 4F ? 8B 41 ? 3B 41 ? 72 ? 57 E8 ? ? ? ? 83 C4 ? FF 74 24 ? 8B 77 ? FF 74 24 ? 57 E8 ? ? ? ? 83 C4 ? ? ? C7 46 ? ? ? ? ? 83 47 ? ? 5F 5E C3 ? ? 8B 4C 24 ? 8B 41 ? 83 78"
#define LUA_TOUSERDATA_SIG "FF 74 24 ? FF 74 24 ? E8 ? ? ? ? 83 C4 ? 8B 48 ? 83 E9 ? 74 ? 83 E9 ? 74"
#define LUA_PUSHINTEGER_SIG "8B 4C 24 ? 66 0F 6E 44 24"
#define LUA_TOINTEGER_SIG "83 EC ? FF 74 24 ? FF 74 24 ? E8 ? ? ? ? 83 C4 ? 83 78 ? ? 74 ? ? ? ? 51 50 E8 ? ? ? ? 83 C4 ? 85 C0 75 ? 83 C4 ? C3 ? ? ? ? 83 C4"
#define LUA_PUSHNUMBER_SIG "8B 4C 24 ? F3 0F 10 44 24 ? 8B 41"
#define LUA_TONUMBER_SIG "83 EC ? FF 74 24 ? FF 74 24 ? E8 ? ? ? ? 83 C4 ? 83 78 ? ? 74 ? ? ? ? 51 50 E8 ? ? ? ? 83 C4 ? 85 C0 75 ? ? ? 83 C4"
#define LUA_PUSHNIL_SIG "8B 4C 24 ? 8B 41 ? C7 40"
#define LUA_PCALL_SIG "8B 44 24 ? 83 EC ? 53 56 57 8B 7C 24"
#define LUAB_LOADSTRING_SIG "51 56 57 8B 7C 24 ? 8D 44 24 ? 50 6A"
#else
const char* GAME_MODULE_NAME = "Rome2.dll";
#define LUA_PUSHSTRING_SIG "55 8B EC 8B 45 ?? 85 C0 75 ?? 8B 4D"
#define LUA_NEWUSERDATA_SIG "55 8B EC 56 8B 75 ? 8B 4E ? 8B 41 ? 3B 41 ? 72 ? 56 E8 ? ? ? ? 83 C4 ? 8B 46"
#define LUAL_NEWMETATABLE_SIG "55 8B EC 56 FF 75 ?? 8B 75 ?? 68"
#define LUA_PUSHCCLOSURE_SIG "55 8B EC 56 8B 75 ? 57 8B 4E ? 8B 41"
#define LUA_SETTABLE_SIG "55 8B EC 56 FF 75 ? 8B 75 ? 56 E8 ? ? ? ? 8B 56 ? 8D 4A"
#define LUA_SETMETATABLE_SIG "55 8B EC 56 8B 75 ? 57 FF 75 ? 56 E8 ? ? ? ? 8B 4E"
#define LUA_SETFIELD_SIG "55 8B EC 83 EC ? 53 56 8B 75 ? 57 FF 75 ? 56 E8 ? ? ? ? 8B 55 ? 83 C4 ? 8B CA 8B F8 8D 59 ? ? ? 41 84 C0 75 ? 2B CB 51 52 56 E8 ? ? ? ? 89 45"
#define LUAL_CHECKLSTRING_SIG "55 8B EC 53 57 FF 75 ? 8B 7D ? FF 75"
#define LUAL_REGISTER_SIG "55 8B EC 6A ? FF 75 ? FF 75 ? FF 75 ? E8 ? ? ? ? 83 C4 ? 5D C3 ? ? ? ? ? ? ? ? 55"
#define LUA_CREATETABLE_SIG "55 8B EC 56 57 8B 7D ? 8B 4F ? 8B 41 ? 3B 41 ? 72 ? 57 E8 ? ? ? ? 83 C4 ? FF 75 ? 8B 77 ? FF 75 ? 57 E8 ? ? ? ? 83 C4 ? ? ? C7 46 ? ? ? ? ? 83 47 ? ? 5F 5E 5D C3 ? 55 8B EC 8B 4D ? 8B 41 ? 83 78"
#define LUA_TOUSERDATA_SIG "55 8B EC FF 75 ? FF 75 ? E8 ? ? ? ? 83 C4 ? 8B 48 ? 83 E9"
#define LUA_PUSHINTEGER_SIG "55 8B EC 8B 4D ? 66 0F 6E 45"
#define LUA_TOINTEGER_SIG "55 8B EC 83 EC ? FF 75 ? FF 75 ? E8 ? ? ? ? 83 C4 ? 83 78 ? ? 74 ? 8D 4D ? 51 50 E8 ? ? ? ? 83 C4 ? 85 C0 75 ? 8B E5 5D C3 ? ? ? ? 8B E5"
#define LUA_PUSHNUMBER_SIG "55 8B EC 8B 4D ? F3 0F 10 45 ? 8B 41"
#define LUA_TONUMBER_SIG "55 8B EC 83 EC ? FF 75 ? FF 75 ? E8 ? ? ? ? 83 C4 ? 83 78 ? ? 74 ? 8D 4D ? 51 50 E8 ? ? ? ? 83 C4 ? 85 C0 75 ? ? ? 8B E5"
#define LUA_PUSHNIL_SIG "55 8B EC 8B 4D ? 8B 41 ? C7 40"
#define LUA_PCALL_SIG "55 8B EC 8B 45 ? 83 EC ? 57 8B 7D ? 85 C0 75"
#define LUAB_LOADSTRING_SIG "55 8B EC 51 56 57 8B 7D ? 8D 45 ? 50 6A ? 57"
#endif

// clang-format off
static EmpireSignatureInfo g_signatures_to_find[] = {
    {
        "lua_pushstring",
        (void**)&g_game_lua_pushstring,
        LUA_PUSHSTRING_SIG
    },
    {
        "lua_newuserdata",
        (void**)&g_game_lua_newuserdata,
        LUA_NEWUSERDATA_SIG
    },
    {
        "luaL_newmetatable",
        (void**)&g_game_luaL_newmetatable,
        LUAL_NEWMETATABLE_SIG
    },
    {
        "lua_pushcclosure",
        (void**)&g_game_lua_pushcclosure,
        LUA_PUSHCCLOSURE_SIG
    },
    {
        "lua_settable",
        (void**)&g_game_lua_settable,
        LUA_SETTABLE_SIG
    },
    {
        "lua_setmetatable",
        (void**)&g_game_lua_setmetatable,
        LUA_SETMETATABLE_SIG
    },
    {
        "lua_setfield",
        (void**)&g_game_lua_setfield,
        LUA_SETFIELD_SIG
    },
    {
        "luaL_checklstring", // Changed from checkstring
        (void**)&g_game_luaL_checklstring,
        LUAL_CHECKLSTRING_SIG
    },
    {
        "luaL_register",
        (void**)&g_game_luaL_register,
        LUAL_REGISTER_SIG
    },
    {
        "lua_createtable",
        (void**)&g_game_lua_createtable,
        LUA_CREATETABLE_SIG
    },
    {
        "lua_touserdata",
        (void**)&g_game_lua_touserdata,
        LUA_TOUSERDATA_SIG
    },
    {
        "lua_pushinteger",
        (void**)&g_game_lua_pushinteger,
        LUA_PUSHINTEGER_SIG
    },
    {
        "lua_tointeger",
        (void**)&g_game_lua_tointeger,
        LUA_TOINTEGER_SIG
    },
    {
        "lua_pushnumber",
        (void**)&g_game_lua_pushnumber,
        LUA_PUSHNUMBER_SIG
    },
    {
        "lua_tonumber",
        (void**)&g_game_lua_tonumber,
        LUA_TONUMBER_SIG
    },
    {
        "lua_pushnil",
        (void**)&g_game_lua_pushnil,
        LUA_PUSHNIL_SIG
    },
    {
        "lua_pcall",
        (void**)&g_game_lua_pcall,
        LUA_PCALL_SIG
    },
    {
        "luaB_loadstring",
        (void**)&g_game_luaB_loadstring,
        LUAB_LOADSTRING_SIG
    },
};
//clang-format on

// Function to initialize the game_lua_api function pointers
void initialize_lua_api() {
    HMODULE hGameModule = GetModuleHandle(GAME_MODULE_NAME);

    if (hGameModule == NULL) {
        spdlog::error("Failed to get module handle for {}", GAME_MODULE_NAME);
        return;
    }
    
    uintptr_t g_game_base_address = (uintptr_t)hGameModule;
    
    // Get module information to determine its size for signature scanning
    MODULEINFO mi = { 0 };
    if (!GetModuleInformation(GetCurrentProcess(), (HMODULE)g_game_base_address, &mi, sizeof(mi))) {
        spdlog::error("Failed to get module information for {}", GAME_MODULE_NAME);
        return;
    }
    size_t module_size = mi.SizeOfImage;

    // Iterate through the function signatures
    for (const auto& func_sig_info : g_signatures_to_find) {
        spdlog::info("Scanning for function: {}", func_sig_info.function_name);

        try {
            if (func_sig_info.signature == nullptr) {
                spdlog::error("Signature for {} is NULL, skipping scan.", func_sig_info.function_name);
                continue;
            }

            if (strlen(func_sig_info.signature) == 0) {
                spdlog::warn("Signature for {} is empty, skipping scan.", func_sig_info.function_name);
                continue;
            }

            uintptr_t found_address = find_signature(g_game_base_address, module_size, func_sig_info.signature);

            if (found_address) {
                *func_sig_info.target_function_ptr = (void*)found_address;
                spdlog::info("Found function {} via signature.", func_sig_info.function_name);
            } else {
                spdlog::error("Signature not found for function {}.", func_sig_info.function_name);
            }
        } catch (const std::exception& e) {
            spdlog::critical("Exception caught during signature scan for {}: {}", func_sig_info.function_name, e.what());
        } catch (...) {
            spdlog::critical("Unknown exception caught during signature scan for {}.", func_sig_info.function_name);
        }
    }

    // Verify that all functions were found
    for (const auto& func_sig_info : g_signatures_to_find) {
        if (*func_sig_info.target_function_ptr == nullptr) {
            spdlog::error("Function '{}' was not found.", func_sig_info.function_name);
        } else {
            spdlog::info("Function '{}' successfully initialized.", func_sig_info.function_name);
        }
    }
}

#endif // BUILD_TESTING_LUA // End of production-only block
