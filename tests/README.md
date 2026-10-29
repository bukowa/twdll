# twdll — Testing Standards & Agent Instructions

This document defines the strict engineering standards for writing tests in **twdll**.
All AI agents and contributors **must** follow these rules without exception.

---

## 1. Adversarial & Fault-Seeking Philosophy (Anti-Tautology)

> [!IMPORTANT]
> **Tests must NEVER be written to merely satisfy the implementation.**
> Tests that simply verify that a setter sets a variable in memory or that a function returns without checking side-effects provide false confidence and violate the testing standard.

- **Hunt for Failures**: Tests must actively probe failure modes:
  - `nullptr` / `NULL` pointers passed to formatters, receivers, and pointers.
  - Boundary extremes (e.g. 0-byte inputs, empty strings, 32 KB+ string payloads).
  - Malformed format specifiers (e.g. `100% discount %% %%s %d`).
  - Concurrent execution, thread contention, and race conditions under rapid state toggling.
  - File system boundaries (non-existent directories, locked files, disk size checks).
- **Physical Ground Truth over In-Memory States**:
  - Do not merely assert that `is_logging_enabled()` returns `false`.
  - Assert that **zero bytes** were written to the physical file on disk (`get_file_size() == 0`).
  - Read back the raw file content from disk and verify exact byte sequences, line terminations (`\n`), and expected data payloads.
- **Fail Early and Loudly**:
  - When a bug or crash occurs (e.g. `0xC0000005` Access Violation), the test must catch it and fail with a clear assertion message.

---

## 2. Directory Mirroring Standard

The `tests/unit/` directory structure **must mirror the `src/` directory structure 1:1**:

```
src/                                  tests/unit/
├── common/                           ├── common/
│   ├── log.cpp          ----->       │   └── log_test.cpp
│   └── lua_core.cpp                  │   └── ...
├── attila/                           ├── attila/
│   ├── cai/             ----->       │   ├── cai/
│   │   └── cai_core.cpp              │   │   └── cai_core_test.cpp
│   └── ...                           │   └── ...
└── main.cpp                          └── main.cpp
```

- Each source file `src/<path>/<name>.cpp` that contains testable C++ logic has a corresponding test file `tests/unit/<path>/<name>_test.cpp`.
- The test runner entry point lives at `tests/unit/main.cpp`.

---

## 3. Test Suites Overview

### Tier 1: Standalone Native C++ Unit Tests (`tests/unit/`)
- **Framework**: `doctest` (`vendor/doctest/doctest.h`, v2.5.3).
- **Speed**: Executes in **< 100 milliseconds** outside the game.
- **Target**: `twdll_unit_tests` executable in CMake.
- **Running**:
  *Never execute `.exe` binaries directly.* Always run through CMake / ctest via presets:
  ```powershell
  & "C:\Program Files\Microsoft Visual Studio\18\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\ctest.exe" --preset attila
  ```
  or via the CMake target:
  ```powershell
  & "C:\Program Files\Microsoft Visual Studio\18\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe" --build build\attila --target tw-unit-test
  ```
  Inside CLion, tests run directly through the built-in IDE test runner UI.

### Tier 2: In-Game Lua Integration Tests (`tests/shared/testing.lua`)
- **Execution**: Runs inside *Total War: Attila* on `FirstTickAfterWorldCreated` via `tw-test`.
- **Target**: Tests end-to-end game engine hooks, Lua bindings, and save/load persistence.
- **Reporting**: Uses `report(name, condition)` and `record_skip()` helpers.

