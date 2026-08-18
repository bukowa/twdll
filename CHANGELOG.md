# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- Added `twdll.cai.EnableLogging([enabled])` — enables or disables real-time Campaign AI decision logging and telemetry to `twdll.log`, capturing settlement occupation choices (`OCCUPY`, `SACK`, `RAZE`, `LOOT`, etc.) and decision bitmasks.
- Added `REGION_SCRIPT_INTERFACE:GetReligionList()` returning a `RELIGION_LIST_SCRIPT_INTERFACE` list of all religions present in the region, and `REGION_SCRIPT_INTERFACE:GetReligionProportion(religion_key)` returning the raw float proportion (0.0 to 1.0) of a specific religion. Added `RELIGION_SCRIPT_INTERFACE` (`GetKey`, `GetProportion`, `GetIconPath`) representing individual religion entries.
- Added `twdll.campaign_ui.SetEncyclopediaUrl(url)` and `twdll.campaign_ui.GetEncyclopediaUrl()` — dynamically gets or sets the base URL prefix for the in-game encyclopedia (e.g. redirecting in-game encyclopedia buttons to a custom local or web server), with automatic restoration of the game's default URL when passed `nil` or an empty string.
- Extended the game's settlement slot interface (`SLOT_SCRIPT_INTERFACE`) with `GetBuildingRotation`, `SetBuildingRotation(rotation)`, and `Refresh()` — reads or sets the campaign-map building model rotation (`0..5`, representing 60-degree increments), persisted natively across save/load. Visual changes can be applied via `twdll.campaign_ui.RefreshSettlements()` or `slot:Refresh()`.
- Added `twdll.campaign_ui.RefreshSettlements()` to force an immediate campaign-map visual refresh of all settlement building models without reloading.
- Extended the game's character interface (`CHARACTER_SCRIPT_INTERFACE`) with `GetInfluence` / `SetInfluence` — read or write the character's political influence (`m_political_gravitas`).
- Extended the game's character interface (`CHARACTER_SCRIPT_INTERFACE`) with `SetDefaultBodyGuard(unit_key)` — sets a persistent default bodyguard unit for a general, saved natively with savegames and enforced at all recruitment and replacement choke points.
- Extended the game's character interface (`CHARACTER_SCRIPT_INTERFACE`) with `GetPoliticalParty()` and `SetPoliticalParty(party)` — gets or sets the character's political party allegiance, accepting either a `CAMPAIGN_POLITICAL_PARTY` object or a party record key string.
- Extended the game's faction interface (`FACTION_SCRIPT_INTERFACE`) with `GetPoliticalPartyList()`, `GetPoliticalParty(party_key)`, `GetPrimaryParty()` and `HasPoliticalParties()` for campaign politics, plus a new `CAMPAIGN_POLITICAL_PARTY` script interface (`GetKey`, `GetSenators`, `GetPower`, `IsPrimary`, `SetPrimary`) for the party objects they return. `GetPoliticalPartyList()` returns a native-style party list (`POLITICAL_PARTY_LIST_SCRIPT_INTERFACE`) iterated with `num_items()`, `item_at(index)`, and `is_empty()`.
- Extended the game's region interface (`REGION_SCRIPT_INTERFACE`) with:
  - `GetPopulationSurplus` / `SetPopulationSurplus` — read or set the population surplus a region has to spend on expanding settlement slots.
  - `GetGrowthPoints` / `SetGrowthPoints` — read or set the growth points a region has accumulated.
- Extended the game's faction interface (`FACTION_SCRIPT_INTERFACE`) with `SetCapital(region)` — makes the given region the faction's capital.
- Extended the game's faction interface (`FACTION_SCRIPT_INTERFACE`) with `InstantlyResearchTechnology(technology_key)` — instantly completes a technology through the game's own native path (`FACTION_TECHNOLOGY_MANAGER::instant_set_researched` + effect/availability refresh), firing events, achievements and unit upgrades and completing parent prerequisites recursively.
- Extended the game's unit interface (`UNIT_SCRIPT_INTERFACE`) with `ConvertUnit(unit_key)` — replaces the unit with a new unit of the given type in the same army, using the engine's native unit conversion path (same as religion/technology upgrades). Health proportion (scaled men count), experience, and combat statistics are preserved, and general bodyguard snapshots are automatically synchronised. The original unit object is destroyed.

### Changed
- `GetInfluence` / `SetInfluence` replace the stale `GetGravitas` / `SetGravitas` (Rome 2 offset) and the incorrectly-modelled `GetAmbition` / `SetAmbition`. Only influence is exposed for now; loyalty and ambition require further research.

## [0.9.0] - 2026-08-06

### Added
- New `twdll.campaign_ui` functions: `SetMaxSlotsMajor` / `SetMaxSlotsMinor` and `GetMaxSlotsMajor` / `GetMaxSlotsMinor` - set/get the building-slot override separately for major (province capital) and minor settlements. The override now distinguishes the two via the settlement's capital flag instead of applying to every settlement.

### Changed
- `ClearMaxSlots` clears both the major and minor overrides.
- Docs build fixed: `custom.css` removed entirely, its rule (`max-width: 75em`) merged into the vendored LDoc stylesheet.
- `AGENTS.md` and `README.md` updated.

### Removed
- `SetMaxSlots` / `GetMaxSlots` removed (0.x, no backward compatibility) - use the `Major` variants instead.

## [0.8.0] - 2026-08-05

### Added
- New `twdll.campaign_ui` functions: `SetMaxSlots`, `GetMaxSlots`, `ClearMaxSlots` — override the maximum number of building slots shown in the settlement panel (`0` restores the game default).
- New `CampaignSettlementCallback::Initialize` hook, installed and uninstalled together with the other campaign hooks; the slot override is re-applied on every settlement panel open/refresh.

### Changed
- `AGENTS.md` and `README.md` updated.

## [0.7.0] - 2026-08-03

### Added
- **Live campaign/battle hooks**: ctor/dtor hooks for the `WORLD`, `CAMPAIGN_UI`, `CAMPAIGN_MODEL` and `BATTLE` (`EMPIREBATTLE::MANAGER`) singletons, installed on first load and uninstalled automatically when the Lua state is collected.
- New `twdll.world` module: `GetFactionCount`, `GetMaxUnitsInArmy` / `SetMaxUnitsInArmy`, `GetMaxUnitsInNavy` / `SetMaxUnitsInNavy`, `GetReinforcementCap` / `SetReinforcementCap` (pass `-1` to restore the game default).
- New `twdll.model` module: `DisbandUnits` — save/load-safe unit removal using the game's own disband path.
- New `twdll.battle` module: `GetBattleInfo` — live battle object, reinforcements manager, cap and queue size.
- New `twdll.campaign_ui` module: `GetMemoryAddress`.
- Extended the game's unit interface (`UNIT_SCRIPT_INTERFACE`) with `GetNumMen` / `SetNumMen`, `GetMaxNumMen` / `SetMaxNumMen`, `GetActionPoints` / `SetActionPoints`.
- Extended the game's faction interface (`FACTION_SCRIPT_INTERFACE`) with `GetTreasury` / `SetTreasury` and `SetFactionLeader` (supports the succession and heir-comes-of-age event variants).
- New Lua runtime signatures: `lua_getfield`, `lua_type`, `lua_settop`.
- Reworked test suite (9 cases: singletons, unit/faction interfaces, `SetFactionLeader`, treasury, `DisbandUnits`, army/navy caps, reinforcement cap, `GetBattleInfo`).

### Changed
- **API restructure**: all modules now live under a single `twdll` table — `twdll.core`, `twdll.unit`, `twdll.faction`, `twdll.military_force`, `twdll.model`, `twdll.world`, `twdll.battle`, `twdll.campaign_ui`. Old flat entry points moved accordingly (`twdll.Log` → `twdll.core.Log`, `twdll.set_max_units_in_army` → `twdll.world.SetMaxUnitsInArmy`).
- Property accessors rewritten from macros to template `Property` / `Getter` / `GlobalGetter` classes, with offset-based getters for embedded fields.
- Hooks install once per Lua state instead of on every DLL load.
- Build/tooling: MSVC toolset bumped to 14.51, GH Actions submodule init fixes, new CodeQL workflow, `tools/twdll.ps1` sets CPU affinity on Win11, docs build fixes.

### Removed
- The `twdll_character` and `twdll_battle_unit` modules are no longer registered in the new single-table layout.

## [0.6.1] - 2026-05-11

### Fixed
- Just another repo cleanup testing the release.

## [0.6.0] - 2026-05-06

### Added
- **Multi-Game Support**: Seamlessly build and run for both Rome 2 and Attila.
- **Pro Architecture**: Struct-based memory mapping with `#pragma pack(1)` and compile-time `static_assert` safety guards.
- **Zero-Boilerplate API**: Automated Lua property mapping via C++ templates/macros.
- **Brand Neutrality**: Renamed all `CA_` prefixes to `TW_` and branding from the API.
- **Clean Build System**: New preset-driven architecture that eliminates IDE target pollution.

## [0.5.0] - 2026-05-01


### Fixed
- Just a repository cleanup that removes the mess I made.

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
