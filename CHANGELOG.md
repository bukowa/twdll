# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.4.0] - 2025-10-20

### Added
- Added a new faction script interface (`twdll_faction`) with `GetMemoryAddress`, `GetGold`, and `SetGold` functions.

### Fixed
- Corrected documentation generation issues.

### Changed
- The build process now utilizes `CMakePresets` for a more streamlined configuration.

## [0.3.0] - 2025-10-07

### Fixed
- Fixed building for win32 lol

## [0.2.1] - 2025-10-07

### Added
- Added `GetMovementPoints`, `SetMovementPoints`, `GetAmbition`, `SetAmbition`, `GetGravitas`, and `SetGravitas` to the character interface.

## [0.2.0-alpha.1] - 2025-10-06

### Added

### Fixed
- fix!: rename unit str prop to number of man

## [0.1.0-alpha.2] - 2025-10-05

### Fixed
- Documentation now takes into account first arguments.


## [0.1.0-alpha.1] - 2025-10-05

### Added
- Merged battle_unit_stats_functions into battle_unit_functions; stats are now accessible via twdll_battle_unit, and the separate twdll_battle_unit_stats module has been removed.

### Fixed
- Various bugs
