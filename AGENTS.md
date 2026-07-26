# twdll — Agent Conventions

**twdll** is a script extender that injects C++ logic into Total War: Attila's Lua engine.
It is a **32-bit MSVC DLL** loaded in-process by the game.

> **Rome 2 support is inactive.** Files in `src/rome2/`, `tests/rome2/`, and `docs/rome2/` are
> preserved but not maintained. All active development targets **Attila only**. Do not modify
> `rome2/` without explicit intent to resume that target.

---

## Architecture at a Glance

```
src/
├── main.cpp               # DLL entry: DllMain + luaopen_twdll (game-agnostic, high-level only)
├── common/
│   ├── tw.h               # Single shared header: memory helpers, Property/Getter templates
│   ├── lua_api.{h,cpp}    # Thin wrappers around the Lua C API (signature-resolved at runtime)
│   ├── lua_core.cpp       # Core twdll module functions (Log, GameBuild)
│   ├── log.{h,cpp}        # Logging (Log("..."))
│   ├── signature_scanner.{h,cpp}  # Scanner:: namespace — runtime address location
│   └── campaign_hooks.{h,cpp}     # Singleton capture via MinHook (WORLD, CAMPAIGN_UI)
├── attila/                # Attila-specific structs and Lua bindings
│   ├── tw_types.h         # ALL TW_* structs, TW_ASSERT_OFFSET, TW_PTR_OFFSET,
│   │                      #   GameScriptInterface<T>, tw_unwrap<T>, tw_push_wrapped<T>
│   ├── lua_sigs.cpp       # Attila byte signatures for all hooked Lua C API functions
│   ├── faction.cpp        # FACTION_SCRIPT_INTERFACE metatable injection (GetGold, SetGold, ...)
│   ├── character.cpp      # twdll_character module (untested — not registered in main.cpp)
│   ├── unit.cpp           # twdll_unit module
│   ├── battle_unit.cpp    # twdll_battle_unit module (untested — not registered in main.cpp)
│   ├── military_force.cpp # twdll_military_force module (untested — not registered in main.cpp)
│   ├── world.cpp          # twdll_world module (g_world singleton)
│   └── campaign_ui.cpp    # twdll_campaign_ui module (g_campaign_ui singleton)
└── rome2/                 # Rome 2-specific files (INACTIVE — see note above)
```

**One rule above all:** game-specific code (structs, offsets, functions) lives in `attila/` or
`rome2/`. Shared machinery lives in `common/`. `main.cpp` is game-agnostic.

> **Only register a module in `main.cpp` once it is tested and confirmed working.**
> Files like `character.cpp`, `battle_unit.cpp`, `military_force.cpp` exist but are not yet
> registered — do not add them to `luaopen_twdll` without a passing test.

---

## Code Style

### Naming

| Thing | Convention | Example |
|---|---|---|
| Files | `snake_case` | `campaign_hooks.cpp` |
| C++ functions (internal) | `PascalCase` | `FindPrologue`, `LogWorldHook` |
| Lua script functions | `PascalCase` | `GetGold`, `SetGold` |
| Structs | `TW_PascalCase` in `namespace twdll` | `twdll::TW_Faction`, `twdll::TW_Unit` |
| Global singletons | `g_snake_case` | `g_world`, `g_campaign_ui` |
| Static originals (MinHook) | `orig_snake_case` | `orig_world_ctor` |
| Compile-time ptr offsets | `UPPER_SNAKE_CASE` constexpr | `FACTION_PTR`, `UNIT_PTR` |
| `namespace Scanner` functions | `PascalCase` | `Scanner::FindString` |
| `namespace twdll` classes | `PascalCase` | `twdll::Property` |
| Props namespace block | `namespace Props` | `Props::Gold`, `Props::NumberOfMan` |

---

## Memory Layout Pattern

All `TW_*` structs live in `namespace twdll` inside `attila/tw_types.h`.

### tw_types.h — struct definition

```cpp
// attila/tw_types.h — all structs go here, no exceptions
namespace twdll {

#pragma pack(push, 1)
struct TW_Foo {
    char pad_00[0xABC];
    int  some_field;  // 0xABC
};
#pragma pack(pop)

TW_ASSERT_OFFSET(TW_Foo, some_field, 0xABC);

// Register the Lua userdata pointer offset for this type:
TW_PTR_OFFSET(TW_Foo, 0x8);

} // namespace twdll
```

Rules:
- `#pragma pack(push, 1)` always wraps all struct definitions.
- Every non-padding field has an inline comment with its hex offset.
- `TW_ASSERT_OFFSET` per field — compile-time safety net.
- `TW_PTR_OFFSET(T, offset)` registers the offset of the wrapped pointer inside the Lua userdata.

### .cpp — using the struct

```cpp
// attila/foo.cpp
#include "../common/tw.h"
#include "tw_types.h"

using twdll::TW_Foo;

// Ptr offset comes from the TW_PtrOffset specialization — never hardcoded:
constexpr size_t FOO_PTR = twdll::TW_PtrOffset<TW_Foo>::value;
```

For types registered via `GameScriptInterface<T>` (faction, character) you can also use:
```cpp
constexpr size_t FACTION_PTR = offsetof(twdll::GameScriptInterface<twdll::TW_Faction>, m_wrapped_object);
```
Both forms are equivalent — use whichever is clearer.

### TW_ASSERT_OFFSET and TW_PTR_OFFSET macros

```cpp
// Defined in tw_types.h:
#define TW_ASSERT_OFFSET(S, F, O) \
    static_assert(offsetof(S, F) == O, #S " Attila: " #F " expected at " #O)

#define TW_PTR_OFFSET(T, O) \
    template<> struct TW_PtrOffset<T> { static constexpr size_t value = O; }
```

---

## Property / Accessor Pattern

**Why we use it:** All Lua-exposed fields are tied to `TW_*` structs via pointer-to-member or
`offsetof`. This makes `TW_ASSERT_OFFSET` the **single source of truth** for memory layout —
a field offset change in the struct automatically propagates to the accessor.

Group all property objects in a `namespace Props` block at the top of the file.

Use the templates from `tw.h` based on the field:

1. **`twdll::Property<T, S>`** — read-write field on a Lua userdata object.
2. **`twdll::Getter<T, S>`** — read-only field. Two constructor forms (see below).
3. **`twdll::GlobalGetter<T, S>`** — read-only field on a global singleton (like `g_world`).

```cpp
namespace Props {
    // 1. Read-write property — member pointer form:
    static twdll::Property Gold     {&TW_Faction::gold,           FACTION_PTR, "faction"};
    static twdll::Property NumberOfMan{&TW_Unit::current_number_of_men, UNIT_PTR, "unit"};

    // 2a. Read-only — member pointer form:
    static twdll::Getter RecruitmentQueueSize{&TW_MilitaryForce::recruitment_queue_size, MIL_FORCE_PTR, "military_force"};

    // 2b. Read-only — offset_tag form for embedded (non-pointer) nested structs:
    //     Use when the field is inside an embedded sub-struct (e.g. TW_BattleUnit::stats.charge_bonus)
    static twdll::Getter ChargeBonus{twdll::offset_tag, offsetof(TW_BattleUnit, stats.charge_bonus), BATTLE_UNIT_PTR, "battle_unit"};
}

// Accessor one-liners:
static int GetGold        (lua_State* L) { return Props::Gold.get(L); }
static int SetGold        (lua_State* L) { return Props::Gold.set(L); }
static int GetChargeBonus (lua_State* L) { return Props::ChargeBonus.get(L); }

// 3. Global singleton:
static twdll::GlobalGetter<int, TW_World> FactionCount{&TW_World::faction_count, &g_world};
static int GetFactionCount(lua_State* L) { return FactionCount.get(L); }
```

**Never** write raw `tw_read`/`tw_write` calls in accessor functions. Raw helpers
(`tw_get_int_at`, `tw_set_int_at`, `tw_mem_address`) are reserved for diagnostic Lua functions only.

---

## Lua Registration Table

Only register functions that are **tested and confirmed working**.

```cpp
// ── Lua registration table ────────────────────────────────────────────────────
extern const luaL_Reg foo_functions[] = {
    {"GetMemoryAddress", GetMemAddress},
    {"GetBar",           GetBar},
    {"SetBar",           SetBar},
    {nullptr, nullptr}
};
```

Align the string and function columns. `GetMemoryAddress` via `tw_mem_address` is present in
every registered object module (useful for debugging from Lua).

---

## Registered Lua API

Only the modules below are registered in `luaopen_twdll` and available from Lua scripts.

| Lua global | Functions |
|---|---|
| `twdll.core` | `Log`, `GameBuild` |
| `twdll.world` | `GetMemoryAddress`, `GetFactionCount`, `SetMaxUnitsInArmy`, `SetMaxUnitsInNavy` |
| `twdll.campaign_ui` | `GetMemoryAddress` |
| `twdll.unit` | `GetMemoryAddress`, `GetNumberOfMan`, `SetNumberOfMan`, `GetMaxNumberOfMan`, `SetMaxNumberOfMan`, `GetMovementPoints`, `SetMovementPoints` |
| `twdll.faction` | (empty — all methods injected into game metatable below) |

Additionally, all `faction` objects expose these as native methods via `FACTION_SCRIPT_INTERFACE`:

| Metatable | Methods |
|---|---|
| `FACTION_SCRIPT_INTERFACE` | `GetMemoryAddress`, `GetGold`, `SetGold`, `SetFactionLeader` |

> **Untested / not registered:** `character`, `battle_unit`, `military_force`. Their `.cpp` files
> exist in `attila/` but are not yet wired into `main.cpp`. Do not document or call them from Lua.

---

## Documentation Comments

The project uses **LDoc** (`/***` blocks for object modules, `///` for core functions).

### Object modules (`attila/`)

Module-level doc at the very top of the file (before `#include`s):
```cpp
/// @module twdll.unit
/// Campaign unit properties for Total War: Attila.
#include "../common/tw.h"
```

> **Note:** Module name uses dot notation (`twdll.unit`), not underscore. Matches the Lua access
> path (`twdll.unit.GetNumberOfMan`), not the global name (`twdll_unit`).

> **Exception:** `faction.cpp` uses `@module FACTION_SCRIPT_INTERFACE` because its functions are
> injected into the game's metatable and called as methods on faction objects, not via a global.

Use `/*** ... */` before each registered function:
```cpp
/***
Gets the amount of gold for the faction.
@function GetGold
@treturn integer amount of gold
*/
static int GetGold(lua_State* L) { return Props::Gold.get(L); }
```

### Core module (`common/lua_core.cpp`)
```cpp
/// Log a message to twdll.log.
/// @function Log
/// @tparam string msg message to log
static int script_Log(lua_State* L) { ... }
```

**Every registered Lua function must have a doc comment.**
Only document functions that are actually registered in a `*_functions[]` table or a metatable.

---

## Campaign Hooks (`campaign_hooks.cpp`)

Singletons (`g_world`, `g_campaign_ui`) are captured at runtime by hooking their constructors.
No hardcoded addresses — located dynamically:

```
FindString(image, size, anchor)    ->  str_addr
FindPushRef(image, size, str_addr) ->  push_addr   (opcode 0x68)
FindPrologue(push_addr)            ->  ctor_addr
```

Hook stubs are `__declspec(naked)` with `pushad`/`popad` to save/restore all registers.
They pass `ecx` (thiscall `this`) to a C helper that stores the pointer.

To add a new singleton:
1. Find a unique string literal inside its constructor (use IDA).
2. Add `g_foo = nullptr` global and `orig_foo_ctor = nullptr` static.
3. Write `LogFooHook(void*)` helper and `__declspec(naked) HookedFooCtor()`.
4. Add one entry to `anchors[]` in `install_campaign_hooks()`.
5. Expose `g_foo` in `campaign_hooks.h`.

---

## Logging

Always prefix with `[twdll]` and include the function name:
```cpp
Log("[twdll] install_campaign_hooks: GetModuleInformation failed (%lu)", GetLastError());
Log("[twdll] SetFactionLeader: faction=0x%08X new=0x%08X", faction, new_char);
```

Use `%lu` for `DWORD`/`GetLastError()`, `%d` for `int`, `0x%08X` for addresses (32-bit).

---

## Building

```powershell
cmake --preset attila
cmake --build --preset attila
```

Target output: `empire.retail.dll` (loaded in-process by the game).

---

## Testing

### Layout

```
tests/
├── shared/
│   └── testing.lua          # SOURCE OF TRUTH — edit only this file
└── attila/
    └── pack/
        └── shared/
            └── testing.lua  # AUTO-GENERATED — do not edit directly
```

`tests/shared/testing.lua` is the only file to edit. `tools/twdll.ps1` copies it into
`tests/attila/pack/shared/` on each `tw-pack`/`tw-test` run.

### Running Tests

```powershell
cmake --build --preset attila --target tw-test
```

Builds the DLL, packages it, launches the game with `tests.save`, runs the Lua test suite at
`FirstTickAfterWorldCreated`. Results appear in `twdll.log`.

### When You Change the Lua API

If you add, remove, or rename a Lua function:
1. Update `tests/shared/testing.lua`.
2. **Do not touch** `tests/attila/pack/shared/testing.lua` — regenerated automatically.

---

## Metatable Paradigm

The game uses `Lunar<T>` to register C++ types in Lua. Each type gets a metatable named after
the class (e.g. `"FACTION_SCRIPT_INTERFACE"`). Our DLL loads via `require()` after the game has
already called `Lunar::Register`, so all metatables exist when `luaopen_twdll` runs.

### Metatable injection (faction.cpp)

```cpp
// faction.cpp pattern:
extern const luaL_Reg faction_functions[];  // empty — registered as twdll_faction
static const luaL_Reg faction_methods[];    // injected into FACTION_SCRIPT_INTERFACE
void register_faction_methods(lua_State* L); // called once from main.cpp
```

`main.cpp` only calls `register_faction_methods(L)` — all metatable details stay in `faction.cpp`.

### tw_push_wrapped

`tw_push_wrapped<T>(lua_State* L, T* raw_ptr)` creates Lua userdata with `GameScriptInterface<T>`
layout so that `tw_unwrap<T>` works on the result. Use when pushing a game object to Lua from C++.

---

## What NOT to Do

- **Do not hardcode virtual addresses** — use `Scanner::` for runtime location.
- **Do not put game-specific structs in `common/`** — they belong in `attila/tw_types.h`.
- **Do not define structs locally in `.cpp` files** — all `TW_*` structs go in `tw_types.h`.
- **Do not hardcode `FOO_PTR = 0x8`** — use `TW_PtrOffset<T>::value` (set via `TW_PTR_OFFSET`).
- **Do not use macros** for property accessors — use `twdll::Property` / `twdll::Getter`.
- **Do not skip `TW_ASSERT_OFFSET`** on struct fields — it is the only compile-time layout check.
- **Do not register an untested module in `main.cpp`** — test first, then register.
- **Do not document functions that are not registered** — LDoc only for active Lua API.
- **Do not edit `tests/attila/pack/shared/testing.lua`** — edit `tests/shared/testing.lua`.
- **Do not use `twdll_foo` (underscore) in `@module` tags** — use `twdll.foo` (dot notation).
- **Do not use raw offsets in accessor functions** — define in `TW_*` struct, access via member pointer or `offsetof`.
