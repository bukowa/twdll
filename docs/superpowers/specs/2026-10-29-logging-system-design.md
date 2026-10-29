# Design Document: Modular Logging System for twdll

**Date:** 2026-10-29  
**Status:** Approved by Builder Review / Ready for Implementation Plan  
**Target Subsystem:** `src/common/log.*`, `src/attila/main.cpp`, `tests/`

---

## 1. Intent and The Why

The `twdll` script extender provides hooks and Lua API extensions for Total War: Attila. Currently, logging consists of a basic `twdll.core.Log(...)` function backed by the `plog` library, with a binary on/off switch (`EnableLogging(bool)`). 

As mods grow in complexity, modders require:
1. **Granular Log Levels**: The ability to distinguish between informational messages (`Info`), non-breaking warnings (`Warn`), critical failures (`Error`/`Fatal`), and high-volume debugging diagnostics (`Debug`/`Trace`).
2. **Attribution & Origin Tracking**: When multiple mods run concurrently alongside the DLL engine, a single flat log stream makes diagnosing failures difficult. Modders need named logger channels (similar to Python's `logging.getLogger(__name__)`) so every line clearly identifies its source (e.g. `[MAIN]`, `[CORE]`, `[SupplySystem]`, `[BattleAI]`).
3. **Hierarchical Level Control**: A global root log level that controls the entire application by default, while allowing individual named loggers to be selectively silenced or elevated to debug level without affecting other scripts.
4. **Resilient, Crash-Free Lua API**: Modders should not worry about runtime errors when logging strings containing format specifiers like `%` or passing non-string values (`numbers`, `booleans`, `tables`, `nil`).

---

## 2. Goals, Non-Goals, and Anti-Goals

### Goals
- **Unified Log File with Strict Chronology**: All log lines (engine, root Lua, and mod-specific loggers) are written sequentially to a single `twdll.log` file with timestamps and origin tags:
  `YYYY-MM-DD HH:MM:SS.mmm [LEVEL] [CHANNEL] Message`
- **Root Logger (`[MAIN]`) & Module-Level Loggers**:
  - Global static methods on `twdll.log` (e.g. `twdll.log.Info(...)`) output under the `[MAIN]` channel.
  - Modders can instantiate named loggers via `local log = twdll.log.GetLogger("ModName")` which output under `[ModName]`.
- **Level Inheritance & Reset**:
  - Setting `twdll.log.SetLevel("warn")` sets the root level and affects all named loggers that have not specified an explicit level override.
  - Setting `log:SetLevel("debug")` on a named logger overrides only that specific logger.
  - Setting `log:SetLevel("inherit")` or `log:SetLevel(nil)` clears the override, resuming root level inheritance.
- **Full Severity Scale**:
  - Supported levels (case-insensitive string input): `"trace"`, `"debug"`, `"info"`, `"warn"` (or `"warning"`), `"error"`, `"fatal"`, `"none"`.
- **Safe Variadic Multi-Argument Logging (`print(...)` style)**:
  - Methods accept 1..N arguments of any basic Lua type, converting them safely and concatenating with spaces.
  - Literal `%` characters in strings never cause formatting exceptions.
- **Fast-Path Performance (<5ns when disabled)**:
  - When a logger's level is disabled for a given severity, the C++ binding exits immediately before inspecting arguments, allocating memory, or stringifying values.
- **Strict Backward Compatibility**:
  - `twdll.core.Log(...)` continues to function (routed to `[INFO] [CORE]`).
  - `twdll.core.EnableLogging(bool)` continues to toggle master logging state: when disabled, file handles are closed and 0 bytes are written to disk.

### Non-Goals
- **Structured JSON / logfmt Serialization**: Complex key-value serialization and table traversal are deferred; arguments are formatted via standard string representation.
- **Multi-File Splitting**: Writing to separate physical files on disk is not in scope; a single file avoids desynchronized timestamps and multi-file locking overhead.
- **External Network Sinks**: No UDP/TCP sockets, syslog, or external log collectors.

### Anti-Goals
- **No Format Crash Exceptions**: Logging must never throw a Lua runtime error or crash the host game under any circumstance.
- **No Blocking I/O in Hot Paths**: Log statements with levels below the active threshold must never perform memory allocations or file writes.

---

## 3. Constraints & Conventions

- **Target Architecture**: 32-bit MSVC DLL (`empire.retail.dll` host process for Total War: Attila).
- **Default State**: Logging is disabled by default (`plog::none`) upon DLL load, ensuring zero performance impact and zero disk writes for normal players.
- **Naming Conventions (`AGENTS.md`)**:
  - All Lua-exposed functions must follow canonical **PascalCase** only (e.g., `GetLogger`, `SetLevel`, `GetLevel`, `Info`, `Warn`, `Error`, `Debug`, `Trace`, `Fatal`, `Flush`).
  - No `snake_case` aliases or alternative duplicates unless explicitly instructed.
- **Engine Safety & Thread Safety**:
  - Logging must be thread-safe (backed by `plog` appender mutexes).
  - Plog's direct file descriptor writes (`_write`) ensure log persistence even upon an abnormal game crash (Access Violation).
- **Git Restrictions**:
  - **DO NOT PUSH** to remote repository. All work remains local.

---

## 4. Architecture & Technical Specifications

### 4.1 Channels and Tagging
1. `[CORE]`: Dedicated channel for internal C++ DLL messages. Isolated from Lua root level inheritance to prevent user scripts from silencing engine diagnostics. Configured via `twdll.core.EnableLogging` or `twdll.log.SetLevel(level, "core")`.
2. `[MAIN]`: Default channel for direct calls to `twdll.log.*(...)` from Lua.
3. `[<CustomName>]`: Channels created via `twdll.log.GetLogger("<CustomName>")`.

### 4.2 Severity Levels
| Level Name | plog Severity | Numeric Value | Description |
|:---|:---|:---|:---|
| `"none"` | `plog::none` | 0 | Logging completely disabled |
| `"fatal"` | `plog::fatal` | 1 | Unrecoverable engine/mod error before crash |
| `"error"` | `plog::error` | 2 | Operation failure, missing entity, invalid state |
| `"warn"` / `"warning"` | `plog::warning` | 3 | Degraded behavior, fallback applied |
| `"info"` | `plog::info` | 4 | High-level lifecycle events, mod initialization |
| `"debug"` | `plog::debug` | 5 | Detailed diagnostics, parameter inspection |
| `"trace"` / `"verbose"` | `plog::verbose` | 6 | Per-frame or tight-loop telemetry |

### 4.3 C++ Logging Interface (`src/common/log.h`)
For internal engine C++ code:
```cpp
void Log(const char* format, ...);                      // Legacy compatibility: logs [INFO] [CORE]
void LogInfo(const char* format, ...);                  // Logs [INFO] [CORE]
void LogWarn(const char* format, ...);                  // Logs [WARN] [CORE]
void LogError(const char* format, ...);                 // Logs [ERROR] [CORE]
void LogDebug(const char* format, ...);                 // Logs [DEBUG] [CORE]
void LogTrace(const char* format, ...);                 // Logs [TRACE] [CORE]
void LogFatal(const char* format, ...);                 // Logs [FATAL] [CORE]
void LogChannel(const char* channel, LogLevel level, const char* message);
```

### 4.4 Lua API Specification

#### Module `twdll.log`
```lua
-- Factory
twdll.log.GetLogger(name: string) -> LoggerInstance

-- Root Logger Level Management
twdll.log.SetLevel(level: string, [channel: string]) -> void
twdll.log.GetLevel([channel: string]) -> string
twdll.log.IsEnabled(level: string, [channel: string]) -> boolean

-- Root Logger Logging Methods (Channel: [MAIN])
twdll.log.Fatal(...)
twdll.log.Error(...)
twdll.log.Warn(...)
twdll.log.Info(...)
twdll.log.Debug(...)
twdll.log.Trace(...)

-- I/O Control
twdll.log.Flush() -> void
```

#### Object `LoggerInstance` (returned by `GetLogger`)
```lua
-- Level Management (Specific to this instance)
logger:SetLevel(level: string) -> void   -- Pass "inherit" or nil to resume root inheritance
logger:GetLevel() -> string             -- Returns effective severity ("info", "debug", etc.)
logger:IsEnabled(level: string) -> boolean
logger:GetName() -> string

-- Logging Methods (Channel: [<Name>])
logger:Fatal(...)
logger:Error(...)
logger:Warn(...)
logger:Info(...)
logger:Debug(...)
logger:Trace(...)
```

### 4.5 Arguments Handling
Any invocation `log.Info(arg1, arg2, ...)` or `logger:Info(arg1, arg2, ...)`:
1. Fast path: checks if the logger's effective level is enabled for the requested severity. If not, returns `0` immediately without reading arguments.
2. If enabled: iterates from argument 1 (or 2 for colon syntax) to `lua_gettop(L)`.
3. For each argument: converts to string via `lua_tostring` (handling booleans, numbers, nil, userdata).
4. Joins arguments with single space delimiters (`" "`).
5. Dispatches line to plog with timestamp, severity, and channel tag.

---

## 5. What is Left to the Builder

The builder (implementer) has the discretion to decide:
1. **Internal C++ Registry Layout**:
   - `std::unordered_map<std::string, LoggerConfig>` with `std::mutex` synchronization.
   - Per-channel struct storing `std::string name` and `std::optional<LogLevel> override_level`.
2. **Lua Userdata vs Table for `LoggerInstance`**:
   - Userdata with a metatable pointing to C functions in `__index`, storing a pointer or index into the C++ logger registry.
3. **Plog Formatter Implementation**:
   - Custom formatter formatting: `YYYY-MM-DD HH:MM:SS.mmm [LEVEL]  [CHANNEL] Message`.
4. **Test Suite Implementation**:
   - Doctest test cases in `tests/unit/common/log_test.cpp`.
   - In-game integration test cases in `tests/shared/testing.lua`.
