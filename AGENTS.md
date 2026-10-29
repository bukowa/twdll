# twdll — Agent Conventions

**twdll** is a script extender that injects C++ logic into Total War: Attila's Lua engine.
It is a **32-bit MSVC DLL** loaded in-process by the game.

> **Scope:** All active development targets **Attila only**. Rome 2 (`src/rome2/`, `tests/rome2/`, `docs/rome2/`) is inactive — do not modify it without explicit instructions.

---

## Core Operating Principles

- **The user's instructions and this file are authoritative.** Treat them as high-privilege specifications.
- **Review Gates Before Changes**:
  1. **Feature Outline**: Present API proposal / outline and get user confirmation ("ok") before deep reverse engineering.
  2. **Code Diff Approval (Reverse-Engineered Code Only)**: Show the exact diff and wait for explicit user confirmation ("ok") ONLY when adding or modifying reverse-engineered code: struct layouts/offsets in `tw_types.h`, memory hooks/patches, and signatures in `game_sigs.cpp`. Standard infrastructure, logging, utilities, and tests do not require upfront diff gates.
  3. **Test Failure Gate**: When a test fails or crashes, stop immediately. Report the observed output, diagnosis, and proposed research target. Wait for "ok" before researching.
- **Fail Fast, Iterate Cleanly**: Prefer simple, direct solutions. Avoid speculative tool-call chains or open-ended search loops.
- **No Leftover Scaffolding**: Diagnostic memory probes or temporary logs must be removed once struct offsets or behaviors are verified.
- **Clean Engine State Rollback**: Any mutation of global variables (`OFFSET_*`), opcode patches, or MinHook detours MUST be rolled back in the corresponding `uninstall_*_hook()` on teardown to prevent state bleeding across game reloads.

---

## Reverse Engineering & Engine Rules

- **Warscape ABI Ground Truth (32-bit)**:
  - `CA::String`: `+0x0: len`, `+0x4: cap`, `+0x8: const char* buf` (Total 12B). Pointer is NEVER at `+0x0`.
  - `SAFE_PTR<T>`: `+0x0: vtable`, `+0x4: T* object` (Total 8B).
  - `CA_STD::VECTOR<T>`: `+0x0: start*`, `+0x4: finish*`, `+0x8: end_of_storage*` (Total 12B).
- **Structure Over Raw Offsets**: Model all engine paths in `tw_types.h` guarded by `TW_ASSERT_OFFSET`. No raw byte arithmetic (`ptr + 0x...`) in `.cpp` files.
- **Signatures Over Hardcoded RVAs**: All native engine functions must be resolved via unique byte signatures in `game_sigs.cpp`. Module handles use `g_empire_module`.
- **Diagnostic Probes Over Guessing**: When struct layouts or pointer chains are uncertain, inject a temporary diagnostic probe into a hook to dump memory in-process rather than trial-and-error guessing.

---

## Lua API & Code Conventions

- **Language**: All code comments, docstrings, and log messages must be in English.
- **C++ Internals**: Use `snake_case` for internal helpers, trampolines, and patches. Global singletons use `g_<thing>`.
- **Canonical PascalCase**: Every Lua-exposed function must have exactly ONE canonical `PascalCase` name (e.g. `GetLoyalty`, `SetLoyalty`, `AddTrait`). No aliases or `snake_case` duplicates unless explicitly requested.
- **List Methods**: Methods returning a list interface must follow `Get<Thing>List` (e.g. `GetReligionList`, matching vanilla `character_list()`).
- **Dual-Path API**: When setting custom text vs database keys, use explicit symmetric pairs: `Set<Prop>` (in-memory UTF-8) and `Set<Prop>Key` (localisation database key).

---

## Building

Build via CMake presets directly from PowerShell (do not use `VsDevCmd.bat` or `cmd /c`):
- **Configure**:
  `& "C:\Program Files\Microsoft Visual Studio\18\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe" --preset attila`
- **Build DLL (`twdll.dll`)**:
  `& "C:\Program Files\Microsoft Visual Studio\18\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\cmake.exe" --build --preset attila`

---

## Testing

- **Standalone C++ Unit Tests (doctest)**:
  Mirrors `src/` 1:1 in `tests/unit/`. Run via:
  `& "C:\Program Files\Microsoft Visual Studio\18\Community\Common7\IDE\CommonExtensions\Microsoft\CMake\CMake\bin\ctest.exe" --preset attila`
- **In-Game Lua Integration Tests**:
  Located in `tests/shared/testing.lua` (never edit the auto-generated pack copy). Run via:
  `.\tools\twdll.ps1 test attila`
- **Test Integrity**:
  - Test real behavior, boundaries, and nil safety without sabotaging the test harness.
  - Record outcomes using `report(name, condition)` and `record_skip()`.
  - Do not attempt in-process reading of `twdll.log` while the file appender is actively locked by the engine.

---

## Documentation & Changelog

- **LDoc**: Every Lua function requires `@function`, `@tparam`, `@treturn` comments. Update `docs/CMakeLists.txt` when adding source files. Regenerate docs via `cmake --build build/attila --target docs`.
- **Changelog**: Add user-facing changes under `## [Unreleased]` in `CHANGELOG.md` matching exact Lua method signatures (no internal C++ offsets or implementation details).
- **Git Commits & History**:
  - Use Conventional Commits (`feat(core): ...`, `fix(character): ...`). **DO NOT PUSH** to remote unless explicitly requested by the user.
  - **Non-Destructive History Rewriting**: When reorganizing or rewriting commits, never run `git reset --hard` on the active branch. Always create a backup branch/tag, build the rewritten commits on an isolated staging branch (`clean-history`), verify zero difference (`git diff <backup>` is empty), and only then update the branch pointer (`git branch -f`).
  - **Commit Hygiene & Separation**:
    - Agent/project instructions (`AGENTS.md`) belong in the very first commit of a refactor.
    - Third-party dependencies (`vendor/`) must be isolated in their own `deps: add ...` commit without application code.
    - Never commit intermediate/half-baked states that are overwritten in later commits — commit the final, verified working state directly.
    - Standalone documentation notes (`README.md`) belong in their own dedicated commit.
