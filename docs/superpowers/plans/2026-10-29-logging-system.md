# Modular Logging System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a modular, multi-channel logging system for `twdll` featuring named logger instances (`GetLogger`), root-level inheritance, fast-path zero-overhead checks, and crash-resilient variadic argument logging.

**Architecture:** Extend `src/common/log.*` with a thread-safe channel registry, severity hierarchy, and custom plog formatter. Expose the `twdll.log` table and `LoggerInstance` metatable to Lua in `src/attila/main.cpp` using safe `print(...)`-style variadic stringification with zero format-string crashes. Verify with both doctest unit tests and in-game Lua integration tests.

**Tech Stack:** C++17, MSVC (32-bit), plog, Lua 5.1 (Total War engine API), doctest.

**Spec:** `docs/superpowers/specs/2026-10-29-logging-system-design.md`

## Global Constraints

- Target Architecture: 32-bit MSVC DLL (`empire.retail.dll` host process for Total War: Attila).
- DO NOT PUSH: No git pushes to remote repository. All commits and changes remain local.
- Lua Naming: Canonical **PascalCase** only (e.g. `GetLogger`, `SetLevel`, `GetLevel`, `IsEnabled`, `Info`, `Warn`, `Error`, `Debug`, `Trace`, `Fatal`, `Flush`). No `snake_case` aliases or duplicates.
- Default State: Logging is disabled by default (`plog::none`) upon DLL load, ensuring zero disk writes until enabled.
- Backwards Compatibility: `twdll.core.Log(...)` and `twdll.core.EnableLogging(bool)` must remain fully functional.
- Safety: Logging must never throw a Lua runtime error or crash the host game, even on invalid format specifiers or nil arguments.

## Review Focus

1. Nil and non-string argument handling: `logger:Info(nil, 123, true)` must concatenate into `"nil 123 true"` without throwing a Lua error.
2. Percentage signs in strings: `logger:Info("Success rate: 100%")` must not be treated as a `printf` format string and must log verbatim.
3. Fast-path bailout: When severity is below threshold, `logger:Debug(...)` must return immediately without reading arguments or allocating heap memory.
4. Channel inheritance reset: Calling `logger:SetLevel("inherit")` or `logger:SetLevel(nil)` must clear the override and resume following root level changes.
5. Isolation of `[CORE]`: Changing the Lua root level via `twdll.log.SetLevel("warn")` must not mute `[CORE]` engine messages unless `"core"` is explicitly targeted.

---

### Task 1: C++ Logging Core & Channel Registry in `src/common/log.*`

**Files:**
- Modify: `src/common/log.h`
- Modify: `src/common/log.cpp`
- Test: `tests/unit/common/log_test.cpp`

**Interfaces:**
- Produces:
  - `enum class LogLevel { None = 0, Fatal, Error, Warn, Info, Debug, Trace };`
  - `LogLevel parse_log_level(const char* str);`
  - `const char* log_level_to_string(LogLevel level);`
  - `void set_logging_enabled(bool enabled);`
  - `bool is_logging_enabled();`
  - `void set_root_log_level(LogLevel level);`
  - `LogLevel get_root_log_level();`
  - `void set_channel_log_level(const char* channel, std::optional<LogLevel> level);`
  - `LogLevel get_channel_effective_level(const char* channel);`
  - `bool is_channel_level_enabled(const char* channel, LogLevel level);`
  - `void LogChannel(const char* channel, LogLevel level, const char* message);`
  - `void LogInfo(const char* format, ...);`, `LogWarn(...)`, `LogError(...)`, `LogDebug(...)`, `LogTrace(...)`, `LogFatal(...)`
  - `void FlushLog();`

- [ ] **Step 1: Write failing unit tests in `tests/unit/common/log_test.cpp`**
  Add test cases covering:
  - `parse_log_level` case-insensitivity and string mappings (`"debug"`, `"warn"`, `"warning"`, `"trace"`, `"none"`, invalid -> fallback).
  - Registry inheritance: `get_channel_effective_level("MyMod")` inherits root level.
  - Channel override: `set_channel_log_level("MyMod", LogLevel::Debug)` overrides root level.
  - Channel reset: `set_channel_log_level("MyMod", std::nullopt)` restores root level inheritance.
  - Isolation of `CORE`: changing root level does not alter `CORE` level.
  - Fast-path predicate: `is_channel_level_enabled` returns false when disabled.

- [ ] **Step 2: Run unit tests to verify failure**
  Run: `& "C:\Program Files\Microsoft Visual Studio\18\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\ctest.exe" --preset attila`
  Expected: FAIL (compilation error due to missing symbols in `log.h`).

- [ ] **Step 3: Implement core logging types and registry in `src/common/log.h` and `src/common/log.cpp`**
  - Define `LogLevel` enum and string conversion helpers.
  - Implement custom plog formatter `TwdllLogFormatter` formatting `YYYY-MM-DD HH:MM:SS.mmm [LEVEL]  [CHANNEL] Message`.
  - Implement thread-safe registry with `std::mutex`:
    - `s_root_level` (default `LogLevel::Info`)
    - `s_core_level` (default `LogLevel::Info`)
    - `std::unordered_map<std::string, std::optional<LogLevel>> s_channel_overrides`
  - Implement `LogChannel`, `FlushLog` (using `_commit` on the file descriptor).
  - Preserve `Log(const char* format, ...)` mapping to `LogChannel("CORE", LogLevel::Info, buf)`.

- [ ] **Step 4: Run unit tests to verify they pass**
  Run: `& "C:\Program Files\Microsoft Visual Studio\18\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe" --build build\attila --target tw-unit-test`
  Followed by: `& "C:\Program Files\Microsoft Visual Studio\18\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\ctest.exe" --preset attila`
  Expected: All unit tests PASS (100%).

- [ ] **Step 5: Commit changes**
  ```bash
  git add src/common/log.h src/common/log.cpp tests/unit/common/log_test.cpp
  git commit -m "feat(core): implement multi-channel log registry and severity hierarchy"
  ```

---

### Task 2: Lua Module `twdll.log` & `LoggerInstance` Metatable in `src/attila/main.cpp`

**Files:**
- Modify: `src/attila/main.cpp`
- Test: `tests/unit/common/log_test.cpp`

**Interfaces:**
- Consumes: Task 1 logging API (`is_channel_level_enabled`, `LogChannel`, `set_root_log_level`, etc.)
- Produces Lua API:
  - `twdll.log.GetLogger(name)` -> `LoggerInstance`
  - `twdll.log.SetLevel(level, [channel])`
  - `twdll.log.GetLevel([channel])`
  - `twdll.log.IsEnabled(level, [channel])`
  - `twdll.log.Fatal(...)`, `Error(...)`, `Warn(...)`, `Info(...)`, `Debug(...)`, `Trace(...)`
  - `twdll.log.Flush()`
  - Metatable for `LoggerInstance`:
    - `:SetLevel(level)`
    - `:GetLevel()`
    - `:IsEnabled(level)`
    - `:GetName()`
    - `:Fatal(...)`, `:Error(...)`, `:Warn(...)`, `:Info(...)`, `:Debug(...)`, `:Trace(...)`

- [ ] **Step 1: Write helper function for variadic Lua argument concatenation**
  Implement `std::string lua_args_to_string(lua_State* L, int start_idx)` in `src/attila/main.cpp` (or shared helper):
  - Iterates from `start_idx` to `lua_gettop(L)`.
  - For strings/numbers: appends `lua_tostring(L, i)`.
  - For booleans: appends `lua_toboolean(L, i) ? "true" : "false"`.
  - For nil: appends `"nil"`.
  - For tables/userdata: appends standard `tostring` representation (`type: pointer`).
  - Separates arguments with space `" "`.

- [ ] **Step 2: Implement `LoggerInstance` userdata and metatable**
  - Userdata stores `char channel_name[64]` (or allocated string).
  - Metatable `TWDLL_LOGGER_METATABLE`:
    - `__index` table exposing `:SetLevel`, `:GetLevel`, `:IsEnabled`, `:GetName`, `:Fatal`, `:Error`, `:Warn`, `:Info`, `:Debug`, `:Trace`.
    - Each logging method performs fast-path check `if (!is_channel_level_enabled(self->channel, level)) return 0;` before calling `lua_args_to_string`.

- [ ] **Step 3: Register `twdll.log` module in `luaopen_twdll`**
  - Register global root functions under `twdll.log`:
    - `GetLogger`
    - `SetLevel`, `GetLevel`, `IsEnabled`
    - `Fatal`, `Error`, `Warn`, `Info`, `Debug`, `Trace` (defaulting to channel `"MAIN"`)
    - `Flush`
  - Ensure `twdll.core.Log` and `twdll.core.EnableLogging` continue to be registered.

- [ ] **Step 4: Build DLL and verify zero compilation errors**
  Run: `& "C:\Program Files\Microsoft Visual Studio\18\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe" --build --preset attila`
  Expected: `twdll.dll` compiles cleanly with zero warnings/errors.

- [ ] **Step 5: Commit changes**
  ```bash
  git add src/attila/main.cpp
  git commit -m "feat(lua): expose twdll.log module and LoggerInstance metatable"
  ```

---

### Task 3: In-Game Integration Tests in `tests/shared/testing.lua`, Docs & Changelog

**Files:**
- Modify: `tests/shared/testing.lua`
- Modify: `docs/CMakeLists.txt`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: Task 2 Lua API (`twdll.log.*`, `twdll.log.GetLogger`)

- [ ] **Step 1: Add automated in-game tests in `tests/shared/testing.lua`**
  Add a dedicated test block in `run_twdll_tests()`:
  - Test 1: `twdll.log` table exists and has all canonical PascalCase methods.
  - Test 2: `twdll.log.GetLevel()` returns default level `"info"`.
  - Test 3: `twdll.log.SetLevel("warn")` updates root level; verify `GetLevel()` returns `"warn"`.
  - Test 4: `twdll.log.GetLogger("TestMod")` returns a valid userdata with metatable.
  - Test 5: `logger:GetName()` returns `"TestMod"`.
  - Test 6: `logger:GetLevel()` inherits root level `"warn"`.
  - Test 7: `logger:SetLevel("debug")` overrides level; verify `logger:GetLevel()` returns `"debug"`.
  - Test 8: `logger:SetLevel("inherit")` resets override back to root level `"warn"`.
  - Test 9: Variadic logging with multiple types: `logger:Info("String", 42, true, nil)` executes without error.
  - Test 10: Percentage string safety: `logger:Info("Completed 100% of tasks")` logs without error.
  - Test 11: `twdll.log.Flush()` executes cleanly.

- [ ] **Step 2: Build package and run in-game test suite**
  Run:
  ```powershell
  $env:PATH += ";C:\Users\buk\projects\ConsulScriptum\.deps\rpfm_cli"
  & "C:\Program Files\Microsoft Visual Studio\18\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe" --build build\attila --target tw-pack
  .\tools\twdll.ps1 test attila
  ```
  Expected: In-game integration test suite passes with `[TEST] Final Result: SUCCESS` and 0 failures.

- [ ] **Step 3: Update documentation and Changelog**
  - Add LDoc annotations (`@module twdll.log`, `@type LoggerInstance`) in `src/attila/main.cpp`.
  - Update `CHANGELOG.md` under `## [Unreleased]`:
    ```markdown
    ### Added
    - Modular logging system `twdll.log` with support for named logger instances (`GetLogger`), severity levels (`Fatal`, `Error`, `Warn`, `Info`, `Debug`, `Trace`), level inheritance, and safe variadic `print`-style argument logging.
    ```

- [ ] **Step 4: Commit changes**
  ```bash
  git add tests/shared/testing.lua CHANGELOG.md
  git commit -m "test(log): add in-game integration tests and update changelog for modular logging"
  ```

