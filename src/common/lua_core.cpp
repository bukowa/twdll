#include "tw.h"
#include "runtime.h"

extern const char* GAME_NAME;

/// @module twdll.core
/// Utility functions available in all games.

#include <string>
#include <cwctype>

/// Log a message or arbitrary values to `twdll.log` (`<namespace>.log` for namespaced builds).
/// Accepts any number of arguments of any type, converting them via Lua tostring.
/// @function Log
/// @tparam any ... values to log
/// @usage
/// twdll.core.Log("Campaign initialized. Turn:", 1, "Faction:", faction:name())
static int script_Log(lua_State* L) {
    std::string full_msg;
    for (int i = 1; l_type(L, i) != LUA_TNONE; ++i) {
        if (i > 1) full_msg += "\t";
        l_getfield(L, LUA_GLOBALSINDEX, "tostring");
        l_pushvalue(L, i);
        if (l_pcall(L, 1, 1, 0) == 0) {
            if (const char* s = l_checklstring(L, -1, nullptr))
                full_msg += s;
            l_pop(L, 1);
        } else {
            l_pop(L, 1);
        }
    }
    Log("%s", full_msg.c_str());
    return 0;
}

/// Returns the target game build name (e.g. `"Attila"`).
/// @function GameBuild
/// @treturn string game name
/// @usage
/// local game_name = twdll.core.GameBuild()
static int script_GameBuild(lua_State* L) {
    l_pushstring(L, GAME_NAME);
    return 1;
}

#ifndef TWDLL_BUILD_SHA
#define TWDLL_BUILD_SHA "unknown"
#endif

/// Returns the git commit SHA hash from which the DLL was compiled.
/// @function GetBuildSha
/// @treturn string 40-character hexadecimal git commit SHA
/// @usage
/// local sha = twdll.core.GetBuildSha()
static int script_GetBuildSha(lua_State* L) {
    l_pushstring(L, TWDLL_BUILD_SHA);
    return 1;
}


/// Returns the Lua API version of this DLL. It increases whenever functions are
/// added or changed in a way a caller may need to check for.
/// @function GetApiVersion
/// @treturn integer API version
/// @usage
/// if twdll.core.GetApiVersion() < 2 then ... end
static int script_GetApiVersion(lua_State* L) {
    l_pushinteger(L, TWDLL_API_VERSION);
    return 1;
}

/// Returns the namespace this DLL was built with: the name of its global tables,
/// its Lua registry key and its log file (`"twdll"` for the public build).
/// @function GetNamespace
/// @treturn string namespace
/// @usage
/// local ns = twdll.core.GetNamespace() -- "twdll"
static int script_GetNamespace(lua_State* L) {
    l_pushstring(L, TWDLL_NAMESPACE);
    return 1;
}

/// Returns the full path of the loaded DLL file.
/// @function GetModulePath
/// @treturn string absolute path (UTF-8)
/// @usage
/// twdll.core.Log("loaded from", twdll.core.GetModulePath())
static int script_GetModulePath(lua_State* L) {
    l_pushstring(L, runtime_module_path().c_str());
    return 1;
}

static std::wstring lower_ws(std::wstring s) {
    for (auto& c : s) c = static_cast<wchar_t>(towlower(c));
    return s;
}

/// Deletes older builds of this DLL that a mod left next to it: every
/// `<namespace>_*.dll` in the DLL's folder except the running file. Files still in
/// use (for example by another running game) are skipped. Only available in
/// namespaced builds, so the public build can never delete another mod's DLL.
/// @function CleanupOldFiles
/// @treturn table|nil array of deleted file names, or nil plus a reason
/// @usage
/// local deleted = tdd_twdll.core.CleanupOldFiles()
static int script_CleanupOldFiles(lua_State* L) {
    if (strcmp(TWDLL_NAMESPACE, "twdll") == 0) {
        l_pushnil(L);
        l_pushstring(L, "CleanupOldFiles is disabled for the default 'twdll' namespace");
        return 2;
    }

    wchar_t self_path[MAX_PATH] = {};
    DWORD n = GetModuleFileNameW(runtime_module(), self_path, MAX_PATH);
    if (n == 0 || n >= MAX_PATH) {
        l_pushnil(L);
        l_pushstring(L, "could not resolve the DLL path");
        return 2;
    }
    std::wstring full(self_path);
    size_t slash = full.find_last_of(L"\\/");
    std::wstring dir = slash == std::wstring::npos ? L"" : full.substr(0, slash + 1);
    std::wstring self_name = lower_ws(slash == std::wstring::npos ? full : full.substr(slash + 1));

    std::wstring ns = tw_utf8_to_wide(TWDLL_NAMESPACE, strlen(TWDLL_NAMESPACE));
    std::wstring pattern = dir + ns + L"_*.dll";

    l_createtable(L, 0, 0);
    int index = 0;
    WIN32_FIND_DATAW fd;
    HANDLE h = FindFirstFileW(pattern.c_str(), &fd);
    if (h != INVALID_HANDLE_VALUE) {
        do {
            if (fd.dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY) continue;
            std::wstring name(fd.cFileName);
            if (lower_ws(name) == self_name) continue;
            std::wstring target = dir + name;
            std::string name_utf8 = tw_wide_to_utf8(name.c_str(), name.size());
            if (DeleteFileW(target.c_str())) {
                Log("[twdll] CleanupOldFiles: deleted '%s'", name_utf8.c_str());
                l_pushinteger(L, ++index);
                l_pushstring(L, name_utf8.c_str());
                l_settable(L, -3);
            } else {
                Log("[twdll] CleanupOldFiles: could not delete '%s' (%lu)", name_utf8.c_str(), GetLastError());
            }
        } while (FindNextFileW(h, &fd));
        FindClose(h);
    }
    return 1;
}

extern const luaL_Reg twdll_core[] = {
    {"Log",         script_Log},
    {"GameBuild",   script_GameBuild},
    {"GetBuildSha", script_GetBuildSha},
    {"GetApiVersion",   script_GetApiVersion},
    {"GetNamespace",    script_GetNamespace},
    {"GetModulePath",   script_GetModulePath},
    {"CleanupOldFiles", script_CleanupOldFiles},
    {nullptr, nullptr}
};
