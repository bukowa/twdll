# twdll — Agent Conventions

**twdll** is a script extender that injects C++ logic into Total War: Rome 2 and Attila's Lua engine.
It is a **32-bit MSVC DLL** loaded in-process by the game.

---

## Architecture at a Glance

```
src/
├── main.cpp               # DLL entry: DllMain + luaopen_twdll
├── common/
│   ├── tw.h               # Single shared header: memory helpers, Property/Getter templates
│   ├── lua_api.{h,cpp}    # Thin wrappers around the Lua C API
│   ├── log.{h,cpp}        # Logging (Log("..."))
│   ├── signature_scanner.{h,cpp}  # Scanner:: namespace — runtime address location
│   └── campaign_hooks.{h,cpp}     # Singleton capture via MinHook (WORLD, CAMPAIGN_UI)
├── attila/                # Attila-specific structs and Lua bindings
└── rome2/                 # Rome 2-specific structs and Lua bindings
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
- Logging (`log.cpp`)
- Memory helpers and Property templates (`tw.h`)

**What is ALWAYS duplicated in `attila/` AND `rome2/`:**
- All `TW_*` struct definitions (even if currently identical or empty)
- All `static_assert` field-offset checks
- All per-game constants and offsets
- Game-specific Lua accessor functions and registration tables

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
| Structs | `TW_PascalCase` | `TW_Faction`, `TW_Unit` |
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

Every game-specific file that exposes object fields follows this exact pattern:

```cpp
// ── Memory layout ─────────────────────────────────────────────────────────────
#pragma pack(push, 1)
struct TW_Foo {
    char pad_00[0xABC];
    int  some_field;  // 0xABC
};
#pragma pack(pop)

static_assert(offsetof(TW_Foo, some_field) == 0xABC, "TW_Foo Attila: some_field");

constexpr size_t FOO_PTR = 0x8;  // offset to real object inside Lua userdata wrapper
```

Rules:
- `#pragma pack(push, 1)` always wraps the struct.
- Every non-padding field has an inline comment with its hex offset.
- A `static_assert` per field validates the layout at compile time.
- `FOO_PTR` is the indirection offset inside the Lua userdata wrapper (usually `0x8`).

---

## Property / Accessor Pattern

**Why we use it:** We strictly tie all Lua-exposed fields to `TW_*` structs via pointer-to-member (`&TW_Foo::bar`). This guarantees that the `static_assert` check in the file is the **single source of truth** for memory layout. If you use raw hex offsets in accessor functions, the `static_assert` cannot protect you from layout changes.

Use the templates from `tw.h` based on the object type:

1. **`twdll::Property<F, S>`** — read-write field on a Lua userdata object.
2. **`twdll::Getter<F, S>`** — read-only field on a Lua userdata object.
3. **`twdll::GlobalGetter<F, S>`** — read-only field on a global singleton (like `g_world`).
4. **`twdll::NestedProperty<F, S, N>`** — read-write field inside a nested pointer.

Instantiate one static object per field, and expose it with a one-liner:

```cpp
// 1. Regular userdata property (read/write):
static twdll::Property<int, TW_Foo> Bar{&TW_Foo::bar, FOO_PTR, "foo"};
static int GetBar(lua_State* L) { return Bar.get(L); }
static int SetBar(lua_State* L) { return Bar.set(L); }

// 2. Global singleton (read-only):
static twdll::GlobalGetter<int, TW_World> FactionCount{&TW_World::faction_count, &g_world};
static int GetFactionCount(lua_State* L) { return FactionCount.get(L); }
```

Never write raw `tw_read` / `tw_write` calls in accessor functions — that is what these templates are for. Raw helpers (`tw_get_int_at`, `tw_mem_address`) are strictly reserved for diagnostic/exploratory Lua functions.

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

Module-level doc goes at the top of the file:
```cpp
/// @module twdll_faction
/// Faction properties and campaign world access for Total War: Attila.
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

| Module | Functions |
|--------|-----------|
| `twdll` | `Log`, `GameBuild`, `set_max_units_in_army`, `set_max_units_in_navy` |
| `twdll_world` | `GetMemoryAddress`, `GetFactionCount` (Attila only for now) |
| `twdll_campaign_ui` | `GetMemoryAddress` |
| `twdll_unit` | `GetNumberOfMan`, `SetNumberOfMan`, `GetMaxNumberOfMan`, `SetMaxNumberOfMan`, `GetMovementPoints`, `SetMovementPoints`, `GetMemoryAddress` |
| `twdll_character` | ... |
| `twdll_faction` | `GetGold`, `SetGold`, `GetMemoryAddress` |
| `twdll_battle_unit` | ... |
| `twdll_military_force` | ... |

### When You Change the Lua API

If you add, remove, or rename a Lua function:
1. Update `tests/shared/testing.lua` to use the new name.
2. **Do not touch** `tests/attila/pack/shared/testing.lua` — it is regenerated automatically.

---

## What NOT to Do

- **Do not hardcode virtual addresses.** They change between game patches. Use `Scanner::` instead.
- **Do not put game-specific structs in `common/`.** They belong in `attila/` or `rome2/`.
- **Do not use macros** for property accessors — the `twdll::Property` template exists for this.
- **Do not skip `static_assert`** on struct fields — it is the only compile-time safety net for offsets.
- **Do not expose a Lua function without a doc comment** — the doc generator will silently omit it.
- **Do not add a new singleton hook** without a corresponding Lua accessor in `world.cpp` or similar.
- **Do not use raw offsets for struct fields** (like `WORLD_FACTIONS_OFFSET = 0x50`). Define it in the `TW_*` struct and use `offsetof` / pointer-to-member.
- **Do not edit `tests/attila/pack/shared/testing.lua`** — edit `tests/shared/testing.lua` instead.
