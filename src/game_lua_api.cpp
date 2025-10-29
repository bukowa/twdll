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
uintptr_t g_game_base_address = 0; // Definition for g_game_base_address
uintptr_t LUA_PUSHSTRING_OFFSET =
    0x0; // This is now unused, but kept for consistency if needed elsewhere

// Array of signatures to find
// clang-format off
static SignatureInfo g_signatures_to_find[] = {
    {
        "lua_pushstring",
        (void**)&g_game_lua_pushstring,
        {
            {"Rome2.dll", "55 8B EC 8B 45 ?? 85 C0 75 ?? 8B 4D"},
            {"empire.retail.dll", "55 8b 6c 24 0c 85 ed 75 10 8b 4c 24 08 8b 41 08 89 68 04 83 41 08 08"}
        }
    },
    {
        "lua_newuserdata",
        (void**)&g_game_lua_newuserdata,
        {
            {"Rome2.dll", "55 8B EC 56 8B 75 ? 8B 4E ? 8B 41 ? 3B 41 ? 72 ? 56 E8 ? ? ? ? 83 C4 ? 8B 46"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "luaL_newmetatable",
        (void**)&g_game_luaL_newmetatable,
        {
            {"Rome2.dll", "55 8B EC 56 FF 75 ?? 8B 75 ?? 68"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_pushcclosure",
        (void**)&g_game_lua_pushcclosure,
        {
            {"Rome2.dll", "55 8B EC 56 8B 75 ? 57 8B 4E ? 8B 41"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_settable",
        (void**)&g_game_lua_settable,
        {
            {"Rome2.dll", "55 8B EC 56 FF 75 ? 8B 75 ? 56 E8 ? ? ? ? 8B 56 ? 8D 4A"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_setmetatable",
        (void**)&g_game_lua_setmetatable,
        {
            {"Rome2.dll", "55 8B EC 56 8B 75 ? 57 FF 75 ? 56 E8 ? ? ? ? 8B 4E"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_setfield",
        (void**)&g_game_lua_setfield,
        {
            {"Rome2.dll", "55 8B EC 83 EC ? 53 56 8B 75 ? 57 FF 75 ? 56 E8 ? ? ? ? 8B 55 ? 83 C4 ? 8B CA 8B F8 8D 59 ? ? ? 41 84 C0 75 ? 2B CB 51 52 56 E8 ? ? ? ? 89 45"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "luaL_checklstring", // Changed from checkstring
        (void**)&g_game_luaL_checklstring,
        {
            {"Rome2.dll", "55 8B EC 53 57 FF 75 ? 8B 7D ? FF 75"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "luaL_register",
        (void**)&g_game_luaL_register,
        {
            {"Rome2.dll", "55 8B EC 6A ? FF 75 ? FF 75 ? FF 75 ? E8 ? ? ? ? 83 C4 ? 5D C3 ? ? ? ? ? ? ? ? 55"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_createtable",
        (void**)&g_game_lua_createtable,
        {
            {"Rome2.dll", "55 8B EC 56 57 8B 7D ? 8B 4F ? 8B 41 ? 3B 41 ? 72 ? 57 E8 ? ? ? ? 83 C4 ? FF 75 ? 8B 77 ? FF 75 ? 57 E8 ? ? ? ? 83 C4 ? ? ? C7 46 ? ? ? ? ? 83 47 ? ? 5F 5E 5D C3 ? 55 8B EC 8B 4D ? 8B 41 ? 83 78"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_touserdata",
        (void**)&g_game_lua_touserdata,
        {
            {"Rome2.dll", "55 8B EC FF 75 ? FF 75 ? E8 ? ? ? ? 83 C4 ? 8B 48 ? 83 E9"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_pushinteger",
        (void**)&g_game_lua_pushinteger,
        {
            {"Rome2.dll", "55 8B EC 8B 4D ? 66 0F 6E 45"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_tointeger",
        (void**)&g_game_lua_tointeger,
        {
            {"Rome2.dll", "55 8B EC 83 EC ? FF 75 ? FF 75 ? E8 ? ? ? ? 83 C4 ? 83 78 ? ? 74 ? 8D 4D ? 51 50 E8 ? ? ? ? 83 C4 ? 85 C0 75 ? 8B E5 5D C3 ? ? ? ? 8B E5"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_pushnumber",
        (void**)&g_game_lua_pushnumber,
        {
            {"Rome2.dll", "55 8B EC 8B 4D ? F3 0F 10 45 ? 8B 41"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_tonumber",
        (void**)&g_game_lua_tonumber,
        {
            {"Rome2.dll", "55 8B EC 83 EC ? FF 75 ? FF 75 ? E8 ? ? ? ? 83 C4 ? 83 78 ? ? 74 ? 8D 4D ? 51 50 E8 ? ? ? ? 83 C4 ? 85 C0 75 ? ? ? 8B E5"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_pushnil",
        (void**)&g_game_lua_pushnil,
        {
            {"Rome2.dll", "55 8B EC 8B 4D ? 8B 41 ? C7 40"},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_pcall",
        (void**)&g_game_lua_pcall,
        {
            {"Rome2.dll", "55 8B EC 8B 45 ? 83 EC ? 57 8B 7D ? 85 C0 75"},
            {"empire.retail.dll", ""}
        }
    }
};
//clang-format on

// Function to initialize the game_lua_api function pointers
void initialize_game_lua_api() {
    const char* game_modules[] = {"Rome2.dll", "empire.retail.dll"};
    HMODULE hGameModule = NULL;
    const char* detected_module_name = "Unknown";

    for (const char* module_name : game_modules) {
        hGameModule = GetModuleHandle(module_name);
        if (hGameModule != NULL) {
            detected_module_name = module_name;
            break;
        }
    }

    g_game_base_address = (uintptr_t)hGameModule;

    if (g_game_base_address == 0) {
        Log("ERROR: Neither Rome2.dll nor empire.retail.dll found. Cannot initialize game Lua API.");
        return;
    }
    
    Log("Detected game module.");

    // Get module information to determine its size for signature scanning
    MODULEINFO mi = { 0 };
    if (!GetModuleInformation(GetCurrentProcess(), (HMODULE)g_game_base_address, &mi, sizeof(mi))) {
        Log("ERROR: Could not get module information for detected game module.");
        return;
    }
    size_t module_size = mi.SizeOfImage;

        // Iterate through the function signatures

        for (const auto& func_sig_info : g_signatures_to_find) {

            Log("Scanning for function: %s", func_sig_info.function_name);

            Log("DEBUG: Signature vector size for %s: %zu", func_sig_info.function_name, func_sig_info.signatures.size());

            try {

                // Find the signature for the detected module

                for (const auto& mod_sig : func_sig_info.signatures) {

                    if (strcmp(mod_sig.module_name, detected_module_name) == 0) {

                        if (mod_sig.signature_str == nullptr) {

                            Log("ERROR: Signature for %s in %s is NULL, skipping scan.", func_sig_info.function_name, mod_sig.module_name);

                            break; 

                        }

                        if (strlen(mod_sig.signature_str) == 0) {

                            Log("INFO: Signature for %s in %s is empty, skipping scan.", func_sig_info.function_name, mod_sig.module_name);

                            break; 

                        }

                        uintptr_t found_address = find_signature(g_game_base_address, module_size, mod_sig.signature_str);

                        if (found_address) {

                            *func_sig_info.target_function_ptr = (void*)found_address;

                            Log("Found function %s via signature.", func_sig_info.function_name);

                        } else {

                            Log("ERROR: Signature not found for function %s.", func_sig_info.function_name);

                        }

                        break; // Found the right module, no need to check others for this function

                    }

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