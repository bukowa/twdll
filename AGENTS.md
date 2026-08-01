# twdll — Agent Conventions

**twdll** is a script extender that injects C++ logic into Total War: Attila's Lua engine.
It is a **32-bit MSVC DLL** loaded in-process by the game.

> **Rome 2 support is inactive.** Files under `src/rome2/`, `tests/rome2/`, and `docs/rome2/`
> are preserved but not maintained. All active development targets **Attila only**. Do not
> modify `rome2/` without explicit intent to resume that target.

This document describes **conventions**, not the current code layout. Read the source for the
up-to-date file/function names; keep the rules here whenever they change.

---

## What NOT to Do

- **Do not put game-specific structs in `common/`** — they belong in the Attila-specific tree.
- **Do not define structs locally in `.cpp` files** — all `TW_*` structs go in the shared types header.
- **Do not hardcode pointer offsets** — use the provided specialization, never a raw `0x8`.
- **Do not use macros** for property accessors — use the provided `Property` / `Getter` templates.
- **Do not skip the compile-time offset assertion** on struct fields — it is the only layout check.
- **Do not register an untested module** — test first, then register it from the DLL entry point.
- **Do not document functions that are not registered** — LDoc only for active Lua API.
- **Do not use raw memory helpers in accessor functions** — they are reserved for diagnostic Lua functions only.
- **Do not write comments or log messages in any language other than English.**

## Before Modifying Existing Files

Never edit the shared types header, any `.cpp` file, or `CMakeLists.txt` directly without first
showing the exact change to the user as a clearly marked code block or diff. Only apply changes
after explicit confirmation ("ok") from the user.

If a build fails after an accepted change, show the compiler error and propose a fix — do not
silently retry or patch in a loop. Report the error, propose one fix, wait for confirmation.

---

## Language

**All comments, doc comments, and log messages must be in English.** No exceptions, including
work-in-progress code, TODO notes, or temporary debugging logs.

Use `snake_case` for internal install-time machinery (hook install/uninstall helpers, original
trampolines, byte patches) — never `PascalCase`. `PascalCase` is reserved for Lua-exposed
functions and the MinHook naked stubs (`Hooked<Ctor>`). Global singletons are `g_<thing>`. This
keeps "install-time plumbing" visually distinct from the Lua API surface.

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
