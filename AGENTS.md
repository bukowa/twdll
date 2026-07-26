# twdll — Agent Conventions

**twdll** is a script extender that injects C++ logic into Total War: Attila's Lua engine.
It is a **32-bit MSVC DLL** loaded in-process by the game.

> **Rome 2 development is FROZEN** (as of 2026-07-26).
> A major game update is expected that will change all addresses and possibly restructure internals.
> Do not modify `rome2/` until the update lands and structures are re-verified.
> All active development targets **Attila only**.

---

## Architecture at a Glance

```
src/
├── main.cpp               # DLL entry: DllMain + luaopen_twdll (game-agnostic, high-level only)
├── common/
│   ├── tw.h               # Single shared header: memory helpers, Property/Getter templates
│   ├── lua_api.{h,cpp}    # Thin wrappers around the Lua C API (signature-resolved at runtime)
│   ├── lua_core.cpp       # Core twdll module functions (Log, GameBuild, set_max_units_*)
│   ├── log.{h,cpp}        # Logging (Log("..."))
│   ├── signature_scanner.{h,cpp}  # Scanner:: namespace — runtime address location
│   └── campaign_hooks.{h,cpp}     # Singleton capture via MinHook (WORLD, CAMPAIGN_UI)
├── attila/                # Attila-specific structs and Lua bindings
│   ├── tw_types.h         # TW_* structs (in namespace twdll), GameScriptInterface<T>,
│   │                      #   tw_unwrap<T>, tw_push_wrapped<T>, TW_ASSERT_OFFSET — Attila only
│   ├── lua_sigs.cpp       # Attila byte signatures for all hooked Lua C API functions
│   ├── faction.cpp        # twdll_faction module + FACTION_SCRIPT_INTERFACE metatable injection
│   ├── character.cpp      # twdll_character module
│   ├── unit.cpp           # twdll_unit module
│   ├── battle_unit.cpp    # twdll_battle_unit module
│   ├── military_force.cpp # twdll_military_force module
│   ├── world.cpp          # twdll_world module (g_world singleton)
│   ├── campaign_ui.cpp    # twdll_campaign_ui module (g_campaign_ui singleton)
│   └── ...
└── rome2/                 # Rome 2-specific structs and Lua bindings (FROZEN)
```

**One rule above all:** game-specific code (structs, offsets, functions) lives in `attila/` or
`rome2/`. Shared machinery lives in `common/`. `main.cpp` is game-agnostic.

---

## The Duplication Rule (Critical)

> **Structs and globals are ALWAYS duplicated per game — never shared via `common/`.**

Even if `attila/world.cpp` and `rome2/world.cpp` look byte-for-byte identical today, they **must**
remain separate files. Offsets, field names, and field counts diverge between game versions, often
without warning. Discovering that `TW_World::faction_count` lives at `0x50` in Attila and `0x54`
in Rome 2 after the fact is far easier to fix when the structs are already independent.

**What belongs in `common/`:**
- Hook installation logic (`campaign_hooks.cpp`)
- Signature scanning (`signature_scanner.cpp`)
- Lua API wrappers (`lua_api.cpp`)
- Core module functions (`lua_core.cpp`)
- Logging (`log.cpp`)
- Memory helpers and Property templates (`tw.h`)

**What is ALWAYS duplicated in `attila/` AND `rome2/`:**
- All `TW_*` struct definitions (even if currently identical or empty), in `namespace twdll`
- All `static_assert` / `TW_ASSERT_OFFSET` field-offset checks
- All per-game constants and offsets
- Game-specific Lua accessor functions and registration tables
- `tw_types.h` — the per-game struct + unwrap header
- `lua_sigs.cpp` — byte signatures for the game's embedded Lua C API functions

Lua functions that happen to share an implementation today (e.g. `GetMemoryAddress`) are still
defined in the game-specific file — they may diverge as the structure is mapped out further.

## Code Style

### Naming

| Thing | Convention | Example |
|---|---|---|
| Files | `snake_case` | `campaign_hooks.cpp` |
| C++ functions (internal) | `PascalCase` | `FindPrologue`, `LogWorldHook` |
| Lua script functions | `PascalCase` | `GetGold`, `SetGold` |
| Lua module functions (core, `snake_case` legacy) | `snake_case` | `set_max_units_in_army` |
| Structs | `TW_PascalCase` in `namespace twdll` | `twdll::TW_Faction`, `twdll::TW_Unit` |
| Global singletons | `g_snake_case` | `g_world`, `g_campaign_ui` |
| Static originals (MinHook) | `orig_snake_case` | `orig_world_ctor` |
| Compile-time offsets | `UPPER_SNAKE_CASE` constexpr | `FACTION_PTR`, `UNIT_PTR` |
| `namespace Scanner` functions | `PascalCase` | `Scanner::FindString` |
| `namespace twdll` classes | `PascalCase` | `twdll::Property` |

### File Headers

Every `.cpp/.h` starts with a one-liner comment:
```cpp
// campaign_hooks.cpp — String-anchored dynamic hooking for campaign singletons.
```
For `.h`, add `#pragma once` before the comment.

### Section Dividers

Use the `──` ruler style for major logical sections:
```cpp
// ── Memory layout ─────────────────────────────────────────────────────────────
// ── Accessors ─────────────────────────────────────────────────────────────────
// ── Lua registration table ────────────────────────────────────────────────────
```
Width is 80 characters. Do **not** use `===`, `---`, or `***`.

### Includes

Order:
1. Own header (`"campaign_hooks.h"`)
2. Other project headers (`"log.h"`, `"signature_scanner.h"`)
3. System / Windows (`<windows.h>`, `<psapi.h>`)
4. Third-party (`<MinHook.h>`)
5. Standard library (`<cstdint>`, `<cstring>`)

Blank line between each group.

---

## Memory Layout Pattern

All `TW_*` structs live in `namespace twdll` inside `attila/tw_types.h`. Structs for types that
come from Lua userdata (faction, character, etc.) use `GameScriptInterface<T>` for the pointer
offset; structs in standalone files (unit, battle_unit, military_force) define the struct locally.

### tw_types.h pattern (Lua userdata types — faction, character)

```cpp
// attila/tw_types.h
namespace twdll {

#pragma pack(push, 1)
struct TW_Foo {
    char pad_00[0xABC];
    int  some_field;  // 0xABC
};
#pragma pack(pop)

TW_ASSERT_OFFSET(TW_Foo, some_field, 0xABC);

} // namespace twdll
```

And in the corresponding `.cpp`, the pointer offset is derived — **never hardcoded**:
```cpp
// The real object ptr lives at offsetof(GameScriptInterface<TW_Foo>, m_wrapped_object) == 0x8
constexpr size_t FOO_PTR = offsetof(twdll::GameScriptInterface<twdll::TW_Foo>, m_wrapped_object);
```

### Standalone struct pattern (unit, battle_unit, military_force)

For modules whose struct is not in `tw_types.h`, define the struct locally in the `.cpp`:
```cpp
// attila/unit.cpp — struct is NOT in tw_types.h, no namespace twdll wrapping needed here
#pragma pack(push, 1)
struct TW_Unit {
    char pad_00[0x44];
    int  current_number_of_men;  // 0x44
};
#pragma pack(pop)

static_assert(offsetof(TW_Unit, current_number_of_men) == 0x44, "TW_Unit Attila: current_number_of_men");

constexpr size_t UNIT_PTR = 0x8;  // GameScriptInterface<T>::m_wrapped_object offset
```

Rules:
- `#pragma pack(push, 1)` always wraps the struct.
- Every non-padding field has an inline comment with its hex offset.
- A `static_assert` (or `TW_ASSERT_OFFSET` for types in `tw_types.h`) per field validates the layout.
- `FOO_PTR` is always `offsetof(GameScriptInterface<T>, m_wrapped_object)` conceptually — use
  the `offsetof` form for types in `tw_types.h`, `0x8` literal only for standalone structs where
  the `GameScriptInterface` template isn't available.

### TW_ASSERT_OFFSET macro (tw_types.h types only)

```cpp
// Defined in tw_types.h:
#define TW_ASSERT_OFFSET(S, F, O) \
    static_assert(offsetof(S, F) == O, #S " Attila: " #F " expected at " #O)

// Usage:
TW_ASSERT_OFFSET(TW_Faction, gold, 0x7DC);
```

---

## Property / Accessor Pattern

**Why we use it:** We strictly tie all Lua-exposed fields to `TW_*` structs via pointer-to-member
(`&TW_Foo::bar`). This guarantees that the `static_assert` / `TW_ASSERT_OFFSET` check is the
**single source of truth** for memory layout. If you use raw hex offsets in accessor functions,
the `static_assert` cannot protect you from layout changes.

Use the templates from `tw.h` based on the object type:

1. **`twdll::Property<F, S>`** — read-write field on a Lua userdata object.
2. **`twdll::Getter<F, S>`** — read-only field on a Lua userdata object.
3. **`twdll::GlobalGetter<F, S>`** — read-only field on a global singleton (like `g_world`).
4. **`twdll::NestedProperty<F, S, N>`** — read-write field inside a nested pointer within `S`.

Instantiate one static object per field, and expose it with a one-liner:

```cpp
// 1. Regular userdata property (read/write) — types from tw_types.h use twdll:: prefix:
static twdll::Property<int, twdll::TW_Faction> Gold{&twdll::TW_Faction::gold, FACTION_PTR, "faction"};
static int GetGold(lua_State* L) { return Gold.get(L); }
static int SetGold(lua_State* L) { return Gold.set(L); }

// 2. Standalone struct property (no twdll:: prefix needed, struct is local):
static twdll::Property<int, TW_Unit> NumberOfMan{&TW_Unit::current_number_of_men, UNIT_PTR, "unit"};
static int GetNumberOfMan(lua_State* L) { return NumberOfMan.get(L); }
static int SetNumberOfMan(lua_State* L) { return NumberOfMan.set(L); }

// 3. Global singleton (read-only):
static twdll::GlobalGetter<int, TW_World> FactionCount{&TW_World::faction_count, &g_world};
static int GetFactionCount(lua_State* L) { return FactionCount.get(L); }

// 4. Nested pointer (e.g. TW_BattleUnit::unit_stats->charge_bonus):
static twdll::NestedProperty<float, TW_BattleUnit, TW_UnitStats> ChargeBonus{
    &TW_UnitStats::charge_bonus, &TW_BattleUnit::unit_stats, BATTLE_UNIT_PTR, "battle_unit"
};
static int GetChargeBonus(lua_State* L) { return ChargeBonus.get(L); }
```

Never write raw `tw_read` / `tw_write` calls in accessor functions — that is what these templates
are for. Raw helpers (`tw_get_int_at`, `tw_set_int_at`, `tw_mem_address`) are strictly reserved
for diagnostic/exploratory Lua functions.

---

## Lua Registration Table

Always `extern const luaL_Reg`, always terminated with `{nullptr, nullptr}`:

```cpp
// ── Lua registration table ────────────────────────────────────────────────────
extern const luaL_Reg foo_functions[] = {
    {"GetBar",           GetBar},
    {"SetBar",           SetBar},
    {"GetMemoryAddress", GetMemAddress},
    {nullptr, nullptr}
};
```

Align the string and function columns. `GetMemoryAddress` via `tw_mem_address` is present in
every object module (useful for debugging from Lua).

---

## Documentation Comments

The project uses **LDoc** (`/***` blocks for object modules, `///` for core functions).

### Object modules (`attila/`, `rome2/`)

Module-level doc goes at the **very top of the file** (before `#include`s), using dot notation:
```cpp
/// @module twdll.faction
/// Faction properties and campaign world access for Total War: Attila.
#include "../common/tw.h"
```

> **Note:** The module name uses a **dot** (`twdll.faction`), not an underscore. The Lua global
> name is `twdll_faction` (registered via `luaL_register`), but the LDoc module identifier uses
> dots as path separators.

> **Exception:** Modules that inject into a game metatable use the metatable name as the module
> identifier instead, e.g. `@module FACTION_SCRIPT_INTERFACE`. This makes it clear in the docs
> that the functions are called as methods on game objects, not via a `twdll_*` global.

Use `/*** ... */` immediately before each function:
```cpp
/***
Gets the amount of gold for the faction.
@function GetGold
@tparam userdata faction the faction object (first argument)
@treturn integer amount of gold
*/
static int GetGold(lua_State* L) { return Gold.get(L); }
```

### Core module (`common/lua_core.cpp`)
Use `///` doc comments:
```cpp
/// Get the game build name.
/// @function GameBuild
/// @return The game name string
/// @usage local game = twdll.GameBuild()
static int script_GameBuild(lua_State* L) { ... }
```

**Every exposed Lua function must have a doc comment.** Missing docs break the generated
reference pages at `docs/attila/` and `docs/rome2/`.

---

## Campaign Hooks (`campaign_hooks.cpp`)

Singletons (`g_world`, `g_campaign_ui`) are captured at runtime by hooking their constructors.
No hardcoded addresses — the hook is located dynamically via three Scanner steps:

```
FindString(image, size, anchor)    ->  str_addr
FindPushRef(image, size, str_addr) ->  push_addr   (opcode 0x68)
FindPrologue(push_addr)            ->  ctor_addr
```

Hook stubs are `__declspec(naked)` to avoid compiler-generated prologue interference.
They save/restore all registers with `pushad`/`popad` and pass `ecx` (thiscall `this`) to the
C helper that stores the pointer.

To add a new singleton:
1. Find a unique string literal inside its constructor (use IDA / `kody/`).
2. Add a `g_foo = nullptr` global and `orig_foo_ctor = nullptr` static.
3. Write a `LogFooHook(void*)` helper and a `__declspec(naked) HookedFooCtor()`.
4. Add one entry to the `anchors[]` array in `install_campaign_hooks()`.
5. Expose `g_foo` in `campaign_hooks.h`.

---

## Logging

Always prefix with `[twdll]` and include the function name:
```cpp
Log("[twdll] install_campaign_hooks: GetModuleInformation failed (%lu)", GetLastError());
Log("[twdll] [%s] hook installed OK", label);
```

Use `%lu` for `DWORD`/`GetLastError()`, `%d` for `int`, `0x%08X` for addresses (32-bit).

---

## Building

The project compiles to a 32-bit DLL for each game via CMake presets:

```powershell
cmake --preset attila
cmake --build --preset attila
```

From an external PowerShell (not Developer shell), set MSVC env vars manually before building —
see the `AGENTS.md` in the `reverse_engineering/` repo for the full environment setup command.

Target: `empire.retail.dll` (loaded in-process by the game for both Rome 2 and Attila).

---

## Testing

### Layout

```
tests/
├── shared/
│   └── testing.lua        # SOURCE OF TRUTH — edit only this file
├── attila/
│   └── pack/
│       └── shared/
│           └── testing.lua  # AUTO-GENERATED — do not edit directly
└── rome2/
    └── pack/
        └── (same structure)
```

`tests/shared/testing.lua` is the **only file you should edit**. During `tw-pack` / `tw-test`,
`tools/twdll.ps1` copies it into `tests/<game>/pack/shared/` automatically. Any manual changes
to the generated copy will be overwritten on the next pack build.

### Running Tests

```powershell
cmake --build --preset attila --target tw-test
```

This builds the DLL, packages it, launches the game with the save `tests.save`, and the Lua test
script runs at `FirstTickAfterWorldCreated`. Results appear in `twdll.log`.

### Test Structure (`testing.lua`)

Tests are triggered by the `FirstTickAfterWorldCreated` event. Each test:
1. Calls twdll Lua API functions
2. Logs `OK` or `FAILED` with context via `twdll.Log`
3. Cross-checks results against the game's own engine API where possible

### API Module Names

| Lua global | Functions |
|------------|-----------|
| `twdll` | `Log`, `GameBuild`, `set_max_units_in_army`, `set_max_units_in_navy` |
| `twdll_world` | `GetMemoryAddress`, `GetFactionCount` |
| `twdll_campaign_ui` | `GetMemoryAddress` |
| `twdll_unit` | `GetNumberOfMan`, `SetNumberOfMan`, `GetMaxNumberOfMan`, `SetMaxNumberOfMan`, `GetMovementPoints`, `SetMovementPoints`, `GetMemoryAddress` |
| `twdll_character` | `GetMovementPoints`, `SetMovementPoints`, `GetAmbition`, `SetAmbition`, `GetGravitas`, `SetGravitas`, `GetMemoryAddress`, `GetIntAtOffset`, `SetIntAtOffset` |
| `twdll_faction` | (empty — all methods are injected into `FACTION_SCRIPT_INTERFACE` metatable) |
| `twdll_battle_unit` | `GetMemoryAddress`, `GetFatigue`, `GetChargeBonus`, `GetMeleeAttack`, `GetBaseMorale`, `GetSomeFloatValue` |
| `twdll_military_force` | `GetMemoryAddress`, `GetRecruitmentQueueSize` |

Additionally, `faction` objects support **metatable methods** (callable as `faction:GetGold()`):
`GetGold`, `SetGold`, `GetMemoryAddress`, `SetFactionLeader`. These are injected into `FACTION_SCRIPT_INTERFACE`
by `register_faction_methods()`.

### When You Change the Lua API

If you add, remove, or rename a Lua function:
1. Update `tests/shared/testing.lua` to use the new name.
2. **Do not touch** `tests/attila/pack/shared/testing.lua` — it is regenerated automatically.

---

## Metatable Paradigm (Attila)

The game uses `Lunar<T>` to register C++ types in Lua. Each type gets a metatable named after
the class (e.g. `"FACTION_SCRIPT_INTERFACE"`). Our DLL is loaded via `require()` **after** the
game has already called `Lunar::Register` for all types, so all metatables exist when
`luaopen_twdll` runs.

### Metatable injection (currently: faction only)

Currently only `faction.cpp` injects methods into a game metatable. The pattern for any module
that does this:

```cpp
// Standalone functions — registered as twdll_faction.SetFactionLeader(faction, ...)
extern const luaL_Reg faction_functions[];

// Metatable extensions — injected into FACTION_SCRIPT_INTERFACE
// Accessible as faction:GetGold() on any game-provided faction object
extern const luaL_Reg faction_methods[];

// Called once from luaopen_twdll
void register_faction_methods(lua_State* L);
```

Modules that do NOT currently inject into a metatable (character, unit, etc.) only export
`foo_functions[]` and are called as `twdll_foo.FunctionName(obj, ...)`.

### main.cpp stays high-level

`main.cpp` only calls `register_faction_methods(L)` — no Lua stack manipulation, no metatable
names. All metatable injection details live in the per-object file.

### Unified object identity

Because we inject into the game's existing metatable, **objects created by the game and objects
we create ourselves are indistinguishable to the Lua user**. Both support game methods and our
methods identically. When we implement push functions (see `research/lunar_push_system.md`),
we must maintain this invariant.

### tw_push_wrapped (tw_types.h)

`tw_push_wrapped<T>(lua_State* L, T* raw_ptr)` creates a Lua userdata with
`GameScriptInterface<T>` layout so that `tw_unwrap<T>` works on the resulting object. Use this
when you need to push a game object to Lua from C++.

### tw_types.h location

`attila/tw_types.h` (in `namespace twdll`) contains all Attila-specific engine structs
(`TW_Faction`, `TW_Character`, etc.), the `GameScriptInterface<T>` layout template,
`TW_PtrOffset<T>` trait, `tw_unwrap<T>`, `tw_push_wrapped<T>`, and the `TW_ASSERT_OFFSET` macro.
When adding Rome 2 support in future, copy to `rome2/tw_types.h` and adjust offsets — do NOT
share this file via `common/`.

---

## What NOT to Do

- **Do not hardcode virtual addresses.** They change between game patches. Use `Scanner::` instead.
- **Do not put game-specific structs in `common/`.** They belong in `attila/` or `rome2/`.
- **Do not use macros** for property accessors — the `twdll::Property` template exists for this.
- **Do not skip `static_assert` / `TW_ASSERT_OFFSET`** on struct fields — it is the only compile-time safety net for offsets.
- **Do not expose a Lua function without a doc comment** — the doc generator will silently omit it.
- **Do not add a new singleton hook** without a corresponding Lua accessor in `world.cpp` or similar.
- **Do not use raw offsets for struct fields** (like `WORLD_FACTIONS_OFFSET = 0x50`). Define it in the `TW_*` struct and use `offsetof` / pointer-to-member.
- **Do not hardcode `0x8` as `FOO_PTR`** for types defined in `tw_types.h` — use
  `offsetof(twdll::GameScriptInterface<twdll::TW_Foo>, m_wrapped_object)` instead.
- **Do not edit `tests/attila/pack/shared/testing.lua`** — edit `tests/shared/testing.lua` instead.
- **Do not use `twdll_foo` (underscore) in `@module` LDoc tags** — use `twdll.foo` (dot notation).
