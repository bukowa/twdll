#include "tw.h"
#include "log.h"
#include <cstring>

extern const char* GAME_NAME;

/// @module twdll.core
/// Utility functions available in all games.

#include <string>

/// Log a message or arbitrary values to `twdll.log`.
/// Accepts any number of arguments of any type, converting them via Lua tostring.
/// Logging is disabled by default; enable it with `twdll.core.EnableLogging(true)`.
/// @function Log
/// @tparam any ... values to log
/// @usage
/// twdll.core.Log("Campaign initialized. Turn:", 1, "Faction:", faction:name())
static int script_Log(lua_State* L) {
    if (!is_logging_enabled()) {
        return 0;
    }

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
    Log(full_msg);
    return 0;
}

/// Enables or disables logging output to `twdll.log`.
/// Logging is disabled by default on initialization.
/// @function EnableLogging
/// @tparam[opt] boolean enabled whether logging to twdll.log is enabled
/// @treturn boolean current enabled state
/// @usage
/// -- Enable logging:
/// twdll.core.EnableLogging(true)
///
/// -- Query current state:
/// local is_logging = twdll.core.EnableLogging()
///
/// -- Disable logging:
/// twdll.core.EnableLogging(false)
static int script_EnableLogging(lua_State* L) {
    if (l_type(L, 1) != LUA_TNONE && l_type(L, 1) != LUA_TNIL) {
        set_logging_enabled(l_tobool(L, 1) != 0);
    }
    l_pushboolean(L, is_logging_enabled() ? 1 : 0);
    return 1;
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

extern const luaL_Reg twdll_core[] = {
    {"EnableLogging", script_EnableLogging},
    {"Log",           script_Log},
    {"GameBuild",     script_GameBuild},
    {"GetBuildSha",   script_GetBuildSha},
    {nullptr, nullptr}
};

// -----------------------------------------------------------------------------
// twdll.log module & LoggerInstance
// -----------------------------------------------------------------------------

/// @module twdll.log
/// Modular multi-channel logging system supporting named loggers and level filtering.

/// Returns a named logger instance writing with origin tag `[<name>]`.
/// @function GetLogger
/// @tparam string name Logger channel name (e.g. `"MyMod"`)
/// @treturn LOGGER_SCRIPT_INTERFACE logger instance

/// Sets the minimum log level for the root logger or a specific channel.
/// @function SetLevel
/// @tparam string level Log level name (`"trace"`, `"debug"`, `"info"`, `"warn"`, `"error"`, `"fatal"`, `"none"`, or `"inherit"`)
/// @tparam[opt] string channel Optional channel name (defaults to root)

/// Gets the effective log level as a lowercase string.
/// @function GetLevel
/// @tparam[opt] string channel Optional channel name (defaults to root)
/// @treturn string effective level name

/// Checks whether a given log level is enabled for a channel.
/// @function IsEnabled
/// @tparam string level Log level name
/// @tparam[opt] string channel Optional channel name (defaults to `"MAIN"`)
/// @treturn boolean true if level is enabled

/// Flushes unbuffered log outputs to disk.
/// @function Flush

/// Logs a message under channel `[MAIN]` at FATAL severity.
/// @function Fatal
/// @tparam any ... values to log

/// Logs a message under channel `[MAIN]` at ERROR severity.
/// @function Error
/// @tparam any ... values to log

/// Logs a message under channel `[MAIN]` at WARN severity.
/// @function Warn
/// @tparam any ... values to log

/// Logs a message under channel `[MAIN]` at INFO severity.
/// @function Info
/// @tparam any ... values to log

/// Logs a message under channel `[MAIN]` at DEBUG severity.
/// @function Debug
/// @tparam any ... values to log

/// Logs a message under channel `[MAIN]` at TRACE severity.
/// @function Trace
/// @tparam any ... values to log

static const char* const LOGGER_METATABLE = "LOGGER_SCRIPT_INTERFACE";

struct LoggerUserdata {
    char name[64];
};

static LoggerUserdata* check_logger(lua_State* L, int idx) {
    if (l_type(L, idx) != LUA_TUSERDATA) return nullptr;
    return static_cast<LoggerUserdata*>(l_touserdata(L, idx));
}

static std::string lua_args_to_string(lua_State* L, int start_idx) {
    std::string full_msg;
    for (int i = start_idx; l_type(L, i) != LUA_TNONE; ++i) {
        if (i > start_idx) {
            full_msg += " ";
        }
        int type = l_type(L, i);
        if (type == LUA_TNIL) {
            full_msg += "nil";
        } else if (type == LUA_TBOOLEAN) {
            full_msg += (l_tobool(L, i) ? "true" : "false");
        } else {
            l_getfield(L, LUA_GLOBALSINDEX, "tostring");
            l_pushvalue(L, i);
            if (l_pcall(L, 1, 1, 0) == 0) {
                if (const char* s = l_checklstring(L, -1, nullptr)) {
                    full_msg += s;
                }
                l_pop(L, 1);
            } else {
                l_pop(L, 1);
                full_msg += "[tostring error]";
            }
        }
    }
    return full_msg;
}

template<LogLevel Level>
static int script_log_dispatch(lua_State* L) {
    LoggerUserdata* ud = check_logger(L, 1);
    const char* channel = ud ? ud->name : "MAIN";
    int start_idx = ud ? 2 : 1;

    // Fast path: early exit if channel level is not enabled
    if (!is_channel_level_enabled(channel, Level)) {
        return 0;
    }

    std::string msg = lua_args_to_string(L, start_idx);
    LogChannel(channel, Level, msg.c_str());
    return 0;
}

static int script_GetLogger(lua_State* L) {
    const char* name = "MAIN";
    if (l_type(L, 1) != LUA_TNONE && l_type(L, 1) != LUA_TNIL) {
        if (const char* str = l_checklstring(L, 1, nullptr)) {
            name = str;
        }
    }
    auto* ud = static_cast<LoggerUserdata*>(l_newuserdata(L, sizeof(LoggerUserdata)));
    strncpy_s(ud->name, sizeof(ud->name), name, _TRUNCATE);
    l_newmetatable(L, LOGGER_METATABLE);
    l_setmetatable(L, -2);
    return 1;
}

static int script_SetLevel(lua_State* L) {
    LoggerUserdata* ud = check_logger(L, 1);
    if (ud) {
        // Method call: logger:SetLevel(level)
        if (l_type(L, 2) == LUA_TNIL || l_type(L, 2) == LUA_TNONE) {
            set_channel_log_level(ud->name, std::nullopt);
        } else {
            const char* lvl = l_checklstring(L, 2, nullptr);
            if (lvl && (_stricmp(lvl, "inherit") == 0 || _stricmp(lvl, "default") == 0)) {
                set_channel_log_level(ud->name, std::nullopt);
            } else {
                set_channel_log_level(ud->name, parse_log_level(lvl));
            }
        }
        return 0;
    }

    // Static call: twdll.log.SetLevel(level, [channel])
    const char* lvl_str = nullptr;
    const char* ch_str = nullptr;
    if (l_type(L, 1) != LUA_TNONE && l_type(L, 1) != LUA_TNIL) {
        lvl_str = l_checklstring(L, 1, nullptr);
    }
    if (l_type(L, 2) != LUA_TNONE && l_type(L, 2) != LUA_TNIL) {
        ch_str = l_checklstring(L, 2, nullptr);
    }

    if (ch_str) {
        if (!lvl_str || _stricmp(lvl_str, "inherit") == 0 || _stricmp(lvl_str, "default") == 0) {
            set_channel_log_level(ch_str, std::nullopt);
        } else {
            set_channel_log_level(ch_str, parse_log_level(lvl_str));
        }
    } else {
        if (lvl_str) {
            set_root_log_level(parse_log_level(lvl_str));
        }
    }
    return 0;
}

static int script_GetLevel(lua_State* L) {
    LoggerUserdata* ud = check_logger(L, 1);
    if (ud) {
        LogLevel lvl = get_channel_effective_level(ud->name);
        l_pushstring(L, log_level_to_string(lvl));
        return 1;
    }

    const char* ch_str = nullptr;
    if (l_type(L, 1) != LUA_TNONE && l_type(L, 1) != LUA_TNIL) {
        ch_str = l_checklstring(L, 1, nullptr);
    }
    LogLevel lvl = ch_str ? get_channel_effective_level(ch_str) : get_root_log_level();
    l_pushstring(L, log_level_to_string(lvl));
    return 1;
}

static int script_IsEnabled(lua_State* L) {
    LoggerUserdata* ud = check_logger(L, 1);
    if (ud) {
        const char* lvl_str = l_checklstring(L, 2, nullptr);
        bool enabled = is_channel_level_enabled(ud->name, parse_log_level(lvl_str));
        l_pushboolean(L, enabled ? 1 : 0);
        return 1;
    }

    const char* lvl_str = l_checklstring(L, 1, nullptr);
    const char* ch_str = "MAIN";
    if (l_type(L, 2) != LUA_TNONE && l_type(L, 2) != LUA_TNIL) {
        ch_str = l_checklstring(L, 2, nullptr);
    }
    bool enabled = is_channel_level_enabled(ch_str, parse_log_level(lvl_str));
    l_pushboolean(L, enabled ? 1 : 0);
    return 1;
}

static int script_GetName(lua_State* L) {
    LoggerUserdata* ud = check_logger(L, 1);
    if (ud) {
        l_pushstring(L, ud->name);
        return 1;
    }
    l_pushstring(L, "MAIN");
    return 1;
}

static int script_Flush(lua_State*) {
    FlushLog();
    return 0;
}

static int logger_tostring(lua_State* L) {
    LoggerUserdata* ud = check_logger(L, 1);
    std::string s = "LoggerInstance(";
    s += (ud ? ud->name : "invalid");
    s += ")";
    l_pushstring(L, s.c_str());
    return 1;
}

static const luaL_Reg logger_instance_methods[] = {
    {"SetLevel",   script_SetLevel},
    {"GetLevel",   script_GetLevel},
    {"IsEnabled",  script_IsEnabled},
    {"GetName",    script_GetName},
    {"Fatal",      script_log_dispatch<LogLevel::Fatal>},
    {"Error",      script_log_dispatch<LogLevel::Error>},
    {"Warn",       script_log_dispatch<LogLevel::Warn>},
    {"Info",       script_log_dispatch<LogLevel::Info>},
    {"Debug",      script_log_dispatch<LogLevel::Debug>},
    {"Trace",      script_log_dispatch<LogLevel::Trace>},
    {"Flush",      script_Flush},
    {nullptr, nullptr}
};

static const luaL_Reg log_module_functions[] = {
    {"GetLogger",  script_GetLogger},
    {"SetLevel",   script_SetLevel},
    {"GetLevel",   script_GetLevel},
    {"IsEnabled",  script_IsEnabled},
    {"Fatal",      script_log_dispatch<LogLevel::Fatal>},
    {"Error",      script_log_dispatch<LogLevel::Error>},
    {"Warn",       script_log_dispatch<LogLevel::Warn>},
    {"Info",       script_log_dispatch<LogLevel::Info>},
    {"Debug",      script_log_dispatch<LogLevel::Debug>},
    {"Trace",      script_log_dispatch<LogLevel::Trace>},
    {"Flush",      script_Flush},
    {nullptr, nullptr}
};

void register_log_module(lua_State* L) {
    // 1. Register or retrieve metatable for LoggerInstance
    if (l_newmetatable(L, LOGGER_METATABLE)) {
        l_pushstring(L, "__index");
        l_createtable(L, 0, 16);
        for (const luaL_Reg* f = logger_instance_methods; f->name; ++f) {
            l_pushstring(L, f->name);
            l_pushcclosure(L, f->func, 0);
            l_settable(L, -3);
        }
        l_settable(L, -3);

        l_pushstring(L, "__tostring");
        l_pushcclosure(L, logger_tostring, 0);
        l_settable(L, -3);
    }
    l_pop(L, 1);

    // 2. Create and populate twdll.log table, leaving it on top of stack
    l_createtable(L, 0, 16);
    for (const luaL_Reg* f = log_module_functions; f->name; ++f) {
        l_pushstring(L, f->name);
        l_pushcclosure(L, f->func, 0);
        l_settable(L, -3);
    }
}
