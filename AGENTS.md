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

## Building

One command builds the DLL. `cmake` is NOT on PATH — when working in CLion
(this repo has `.idea`), invoke the CMake bundled with CLion by full path:

```powershell
& "C:\Program Files\JetBrains\CLion 2026.2\bin\cmake\win\x64\bin\cmake.exe" --build build\attila
```

Deployment/tests go through `tools/twdll.ps1` (`tw-pack`/`tw-install`/
`tw-run`/`tw-test` CMake targets). `tw-test` launches the real game and takes
minutes — leave it to the user unless explicitly asked.

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

### Test Conventions

Every test case in `tests/shared/testing.lua` must record its outcome so the suite
prints a clear summary. Use the suite-level helpers defined at the top of
`run_twdll_tests()`:

- `report(name, condition)` — call once per assertion, in **both** the OK and
  FAILED branches of the existing `if ... then OK else FAILED end` blocks.
- `record_skip()` — call in the SKIPPED branch when a precondition isn't met
  (e.g. `< 2 generals`, empty unit list).

Do not introduce a new test case without wiring it into these helpers. The final
block emits `PASSED: N  FAILED: M  SKIPPED: K` and, when `failed > 0`, a list of
the failed test names — so you can see at a glance whether something broke and
which case. Keep all `[TEST]` log lines via `twdll.core.Log` (not raw
`io.write` to `twdll.log`), so they go through the same logging path as the rest
of the suite.

### When You Change the Lua API

If you add, remove, or rename a Lua function:
1. Update `tests/shared/testing.lua`.
2. **Do not touch** `tests/attila/pack/shared/testing.lua` — regenerated automatically.
