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
lua_getfield_t g_game_lua_getfield = nullptr;
lua_settop_t g_game_lua_settop = nullptr;
lua_createtable_t g_game_lua_createtable = nullptr;
lua_objlen_t g_game_lua_objlen = nullptr;
lua_tolstring_t g_game_lua_tolstring = nullptr;
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
            {"Rome2.dll", ""},
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
            {"Rome2.dll", ""},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_setmetatable",
        (void**)&g_game_lua_setmetatable,
        {
            {"Rome2.dll", ""},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_setfield",
        (void**)&g_game_lua_setfield,
        {
            {"Rome2.dll", ""},
            {"empire.retail.dll", ""}
        }
    },
    {
        "luaL_checklstring", // Changed from checkstring
        (void**)&g_game_luaL_checklstring,
        {
            {"Rome2.dll", ""},
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
        "lua_getfield",
        (void**)&g_game_lua_getfield,
        {
            {"Rome2.dll", ""},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_settop",
        (void**)&g_game_lua_settop,
        {
            {"Rome2.dll", ""},
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
        "lua_objlen",
        (void**)&g_game_lua_objlen,
        {
            {"Rome2.dll", ""},
            {"empire.retail.dll", ""}
        }
    },
    {
        "lua_tolstring",
        (void**)&g_game_lua_tolstring,
        {
            {"Rome2.dll", ""},
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
        // Find the signature for the detected module
        for (const auto& mod_sig : func_sig_info.signatures) {
            if (strcmp(mod_sig.module_name, detected_module_name) == 0) {
                if (strlen(mod_sig.signature_str) == 0) {
                    Log("INFO: Signature is empty, skipping.");
                    continue;
                }
                uintptr_t found_address = find_signature(g_game_base_address, module_size, mod_sig.signature_str);
                if (found_address) {
                    *func_sig_info.target_function_ptr = (void*)found_address;
                    Log("Found function via signature.");
                } else {
                    Log("ERROR: Signature not found for function.");
                }
                break; // Found the right module, no need to check others for this function
            }
        }
    }

    if (g_game_lua_pushstring) {
        Log("lua_pushstring successfully initialized.");
    } else {
        Log("ERROR: lua_pushstring could not be initialized.");
    }
}