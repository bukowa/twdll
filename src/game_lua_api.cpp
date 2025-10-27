#include "game_lua_api.h"
#include "log.h"
#include <windows.h>
#include <Psapi.h> // For GetModuleInformation and MODULEINFO
#include <cstring> // For strcmp

// --- Global variables for game_lua_api.h ---
lua_pushstring_t g_game_lua_pushstring = nullptr;
uintptr_t g_game_base_address = 0; // Definition for g_game_base_address
uintptr_t LUA_PUSHSTRING_OFFSET = 0x0; // This is now unused, but kept for consistency if needed elsewhere

// Array of signatures to find
static SignatureInfo g_signatures_to_find[] = {
    {
        "Rome2.dll",
        "lua_pushstring",
        "55 8b ec 8b 45 0c 85 c0 75 13 8b 4d 08 8b 41 08 c7 40 04 00 00 00 00 83 41 08 08",
        (void**)&g_game_lua_pushstring
    },
    {
        "empire.retail.dll",
        "lua_pushstring",
        "55 8b 6c 24 0c 85 ed 75 10 8b 4c 24 08 8b 41 08 89 68 04 83 41 08 08",
        (void**)&g_game_lua_pushstring
    }
    // Add more functions and modules here as needed
};

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

    // Iterate through the signatures and find the relevant ones for the detected module
    for (const auto& sig_info : g_signatures_to_find) {
        if (strcmp(sig_info.module_name, detected_module_name) == 0) {
            uintptr_t found_address = find_signature(g_game_base_address, module_size, sig_info.signature_str); 
            if (found_address) {
                *sig_info.target_function_ptr = (void*)found_address;
                Log("Found function via signature.");
            } else {
                Log("ERROR: Signature not found for function.");
            }
        }
    }

    if (g_game_lua_pushstring) {
        Log("lua_pushstring successfully initialized.");
    } else {
        Log("ERROR: lua_pushstring could not be initialized.");
    }
}
