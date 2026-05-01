# Changelog

All notable changes to this project will be documented in this file.

## [0.5.0] - 2026-05-01

### Added
- **Automated Integration Testing**: A zero-click integration testing pipeline using `game_startup_mode campaign_load`. Includes an automatic log tailer that parses test results and instantly terminates the game when tests pass or fail.
- **Dynamic Build Flags**: CMake now auto-detects `Rome2.dll` to determine if it should build using Retail or Steam flags.
- **CMake Presets**: Added `run_alone_tail`, `run_steam_tail`, and `run_alone_test_tail` presets for incredibly rapid, automated deployment and debugging.

### Changed
- **Massive Architecture Cleanup**: Transitioned entirely to an ultra-minimalist, zero-dependency C++ framework.
- **Static Linking**: The DLL is now compiled with `/MT` (Static MSVC Runtime), ensuring flawless injection across all player machines without requiring VC++ Redistributables.
- **API File Renaming**: Refactored the `src/` directory to use short, professional naming conventions (e.g., `battle_unit.cpp` instead of `battle_unit_script_interface.cpp`).
- **Log Streaming**: Converted `log.cpp` to use `_fsopen` with `_SH_DENYNO`, fixing sharing violations and allowing real-time PowerShell tailing in the IDE.

### Removed
- Removed dead `money.cpp` legacy logic.
- Purged the raw Lua 5.1 C source code from the project root (`include/lua`).
- Stripped out all unused Python, ImGui, and MinHook dependencies and documentation.
- Removed IDE configuration clutter (`.clang-format`, `.clangd`, `.emmyrc.json`).
