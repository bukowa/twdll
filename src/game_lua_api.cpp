#include "game_lua_api.h"
#include <windows.h>
#include "log.h"
#include <Psapi.h> // For GetModuleInformation and MODULEINFO
#include <cstring> // For strcmp

// --- Global variables for game_lua_api.h ---
lua_pushstring_t g_game_lua_pushstring = nullptr;
lua_newuserdata_t g_game_lua_newuserdata = nullptr;
luaL_newmetatable_t g_game_luaL_newmetatable = nullptr;
lua_pushcclosure_t g_game_lua_pushcclosure = nullptr;
lua_settable_t g_game_lua_settable = nullptr;
lua_setmetatable_t g_game_lua_setmetatable = nullptr;
lua_setfield_t g_game_lua_setfield = nullptr;
luaL_checklstring_t g_game_luaL_checklstring = nullptr; // Changed from checkstring
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
uintptr_t g_game_base_address = 0; // Definition for g_game_base_address
uintptr_t LUA_PUSHSTRING_OFFSET =
    0x0; // This is now unused, but kept for consistency if needed elsewhere

lua_State *g_game_LuaState;

// Since we only target one module, we can use a simpler structure for signatures.
struct EmpireSignatureInfo {
    const char* function_name;
    void** target_function_ptr;
    const char* signature;
};

// Array of signatures to find
// clang-format off
static EmpireSignatureInfo g_signatures_to_find[] = {
    {
        "lua_pushstring",
        (void**)&g_game_lua_pushstring,
        "55 8B 6C 24 ? 85 ED 75 ? 8B 4C 24"
    },
    {
        "lua_newuserdata",
        (void**)&g_game_lua_newuserdata,
        "56 8B 74 24 ? 8B 4E ? 8B 41 ? 3B 41 ? 72 ? 56 E8 ? ? ? ? 83 C4 ? 8B 46"
    },
    {
        "luaL_newmetatable",
        (void**)&g_game_luaL_newmetatable,
        "56 FF 74 24 ? 8B 74 24 ? 68"
    },
    {
        "lua_pushcclosure",
        (void**)&g_game_lua_pushcclosure,
        "55 56 8B 74 24 ? 57 8B 4E ? 8B 41"
    },
    {
        "lua_settable",
        (void**)&g_game_lua_settable,
        "56 FF 74 24 ? 8B 74 24 ? 56 E8 ? ? ? ? 8B 56 ? 8D 4A"
    },
    {
        "lua_setmetatable",
        (void**)&g_game_lua_setmetatable,
        "56 8B 74 24 ? 57 FF 74 24 ? 56 E8 ? ? ? ? 8B 4E ? 83 C4"
    },
    {
        "lua_setfield",
        (void**)&g_game_lua_setfield,
        "83 EC ? 53 56 8B 74 24 ? 57 FF 74 24 ? 56 E8 ? ? ? ? 8B 54 24 ? 83 C4 ? 8B CA 8B F8 8D 59 ? ? ? 41 84 C0 75 ? 2B CB 51 52 56 E8 ? ? ? ? 89 44 24"
    },
    {
        "luaL_checklstring", // Changed from checkstring
        (void**)&g_game_luaL_checklstring,
        "53 55 8B 6C 24 ? 57 FF 74 24"
    },
    {
        "luaL_register",
        (void**)&g_game_luaL_register,
        "55 8B 6C 24 ? 56 8B 74 24 ? 57 8B 7C 24 ? 85 ED"
    },
    {
        "lua_createtable",
        (void**)&g_game_lua_createtable,
        "56 57 8B 7C 24 ? 8B 4F ? 8B 41 ? 3B 41 ? 72 ? 57 E8 ? ? ? ? 83 C4 ? FF 74 24 ? 8B 77 ? FF 74 24 ? 57 E8 ? ? ? ? 83 C4 ? ? ? C7 46 ? ? ? ? ? 83 47 ? ? 5F 5E C3 ? ? 8B 4C 24 ? 8B 41 ? 83 78"
    },
    {
        "lua_touserdata",
        (void**)&g_game_lua_touserdata,
        "FF 74 24 ? FF 74 24 ? E8 ? ? ? ? 83 C4 ? 8B 48 ? 83 E9 ? 74 ? 83 E9 ? 74"
    },
    {
        "lua_pushinteger",
        (void**)&g_game_lua_pushinteger,
        "8B 4C 24 ? 66 0F 6E 44 24"
    },
    {
        "lua_tointeger",
        (void**)&g_game_lua_tointeger,
        "83 EC ? FF 74 24 ? FF 74 24 ? E8 ? ? ? ? 83 C4 ? 83 78 ? ? 74 ? ? ? ? 51 50 E8 ? ? ? ? 83 C4 ? 85 C0 75 ? 83 C4 ? C3 ? ? ? ? 83 C4"
    },
    {
        "lua_pushnumber",
        (void**)&g_game_lua_pushnumber,
        "8B 4C 24 ? F3 0F 10 44 24 ? 8B 41"
    },
    {
        "lua_tonumber",
        (void**)&g_game_lua_tonumber,
        "83 EC ? FF 74 24 ? FF 74 24 ? E8 ? ? ? ? 83 C4 ? 83 78 ? ? 74 ? ? ? ? 51 50 E8 ? ? ? ? 83 C4 ? 85 C0 75 ? ? ? 83 C4"
    },
    {
        "lua_pushnil",
        (void**)&g_game_lua_pushnil,
        "8B 4C 24 ? 8B 41 ? C7 40"
    },
    {
        "lua_pcall",
        (void**)&g_game_lua_pcall,
        "8B 44 24 ? 83 EC ? 53 56 57 8B 7C 24"
    },
    {
        "luaB_loadstring",
        (void**)&g_game_luaB_loadstring,
        "51 56 57 8B 7C 24 ? 8D 44 24 ? 50 6A"
    },
};
//clang-format on

// Function to initialize the game_lua_api function pointers
void initialize_game_lua_api() {
    const char* module_name = "empire.retail.dll";
    HMODULE hGameModule = GetModuleHandle(module_name);

    if (hGameModule == NULL) {
        Log("ERROR: %s not found. Cannot initialize game Lua API.", module_name);
        return;
    }

    g_game_base_address = (uintptr_t)hGameModule;
    
    Log("Game module found: %s", module_name);

    // Get module information to determine its size for signature scanning
    MODULEINFO mi = { 0 };
    if (!GetModuleInformation(GetCurrentProcess(), (HMODULE)g_game_base_address, &mi, sizeof(mi))) {
        Log("ERROR: Could not get module information for %s.", module_name);
        return;
    }
    size_t module_size = mi.SizeOfImage;

    // Iterate through the function signatures
    for (const auto& func_sig_info : g_signatures_to_find) {
        Log("Scanning for function: %s", func_sig_info.function_name);

        try {
            if (func_sig_info.signature == nullptr) {
                Log("ERROR: Signature for %s is NULL, skipping scan.", func_sig_info.function_name);
                continue;
            }

            if (strlen(func_sig_info.signature) == 0) {
                Log("INFO: Signature for %s is empty, skipping scan.", func_sig_info.function_name);
                continue;
            }

            uintptr_t found_address = find_signature(g_game_base_address, module_size, func_sig_info.signature);

            if (found_address) {
                *func_sig_info.target_function_ptr = (void*)found_address;
                Log("Found function %s via signature.", func_sig_info.function_name);
            } else {
                Log("ERROR: Signature not found for function %s.", func_sig_info.function_name);
            }
        } catch (const std::exception& e) {
            Log("CRITICAL ERROR: Exception caught during signature scan for %s: %s", func_sig_info.function_name, e.what());
        } catch (...) {
            Log("CRITICAL ERROR: Unknown exception caught during signature scan for %s.", func_sig_info.function_name);
        }
    }

    // Verify that all functions were found
    for (const auto& func_sig_info : g_signatures_to_find) {
        if (*func_sig_info.target_function_ptr == nullptr) {
            Log("ERROR: Function '%s' was not found.", func_sig_info.function_name);
        } else {
            Log("Function '%s' successfully initialized.", func_sig_info.function_name);
        }
    }
}