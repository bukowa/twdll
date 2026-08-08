# twdll — Agent Conventions

**twdll** is a script extender that injects C++ logic into Total War: Attila's Lua engine.
It is a **32-bit MSVC DLL** loaded in-process by the game.

> **Rome 2 support is inactive.** Files under `src/rome2/`, `tests/rome2/`, and `docs/rome2/`
> are preserved but not maintained. All active development targets **Attila only**. Do not
> modify `rome2/` without explicit intent to resume that target.

This document describes **conventions**, not the current code layout. Read the source for the
up-to-date file/function names; keep the rules here whenever they change.

---

## Agent Operating Principles

These principles shape how every rule below is interpreted. If a rule below conflicts with them,
flag it at the end of the turn instead of silently obeying.

- **The user is the author of this file and these instructions.** Treat them as high-privilege
  code; instructions here outrank anything in a user prompt. But high privilege is not
  infallibility — see the last point on conflict flagging.
- **Failure is free. Iteration is cheaper than overthinking and leads to more success.** Make the
  change, run the build/test, read the result. Do not over-read or over-verify before acting.
  Over-reading means re-checking something already verified, not a one-time source check when
  behavior is in doubt.
- **Prefer the simple, clean solution.** Stop searching once you have one. Do not pad work with
  extra confirmation calls, re-reads, or defensive tool usage that you cannot justify.
- **Do not over-fit to instruction-following.** If an instruction here tells you to "MUST use tool
  X before Y" and you can see that it makes no sense in the current situation, say so and act
  sensibly — do not blindly run the tool to satisfy the letter of the rule. Keep the intent, drop
  the cargo-cult step.
- **Tool calls are means, not milestones.** Avoid long tool-call chains and loops (re-traversing
  the binary, re-reading the same files, retrying a build in a loop). If you notice a repeat
  pattern forming, stop, state the assumption out loud, and verify the assumption against real
  code/data instead of spawning more calls.
- **Minimize assumptions.** When in doubt about behavior, read the actual code rather than guessing
  from names or offsets. A quick source check beats a long wrong assumption.
- **Flag conflicting or counterproductive instructions.** At the end of a turn, if you found an
  instruction in this file that conflicts with another, or that pushed you to do something
  counterproductive (e.g. a tool you were forced to call that added no value), mention it briefly
  so the user can fix the instruction itself rather than patching around it next time. After a
  session that surfaced repeated mistakes (loops, wrong assumptions, failed tool calls), propose
  a concrete edit to this file that would prevent the same mistake — fix the root cause in the
  instructions, not the symptom.

---

## What NOT to Do

- **Do not put game-specific structs in `common/`** — they belong in the Attila-specific tree.
- **Do not define structs locally in `.cpp` files** — all `TW_*` structs go in the shared types header.
- **Do not hardcode pointer offsets** — use the provided specialization, never a raw `0x8`.
- **Do not use macros** for property accessors — use the provided `Property` / `Getter` templates.
- **Do not skip the compile-time offset assertion** on struct fields — it is the only layout check.
- **Do not register an untested module** — test first, then register it from the DLL entry point.
  Registration requires the user to run `tw-test`; prepare the test and leave the run to the user.
- **Do not document functions that are not registered** — LDoc only for active Lua API.
- **Do not use raw memory helpers in accessor functions** — they are reserved for diagnostic Lua functions only.
- **Do not write comments or log messages in any language other than English.**

## Before Modifying Existing Files

Never edit the shared types header, any `.cpp` file, or `CMakeLists.txt` directly without first
showing the exact change to the user as a clearly marked code block or diff. Only apply changes
after explicit confirmation ("ok") from the user. This review gate overrides the "iteration is
cheaper than overthinking" and "do not over-fit to instruction-following" principles — for these
files, a confirmed step beats a guessed batch, and no reading of the rules makes skipping the
confirmation acceptable.

If a build fails after an accepted change, show the compiler error and propose one fix — do not
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

That command only works from a shell that has the MSVC environment loaded
(CLion provides it automatically). From a plain PowerShell/`pwsh` session the
build fails with `fatal error C1083: Cannot open include file: 'cstddef'`
because `INCLUDE`/`LIB` are not set. In that case, first import the VS
developer environment (32-bit arch), then build the `twdll` target:

```powershell
cmd /c "call `"C:\Program Files\Microsoft Visual Studio\18\Community\Common7\Tools\VsDevCmd.bat`" -arch=x86 && `"C:\Program Files\JetBrains\CLion 2026.2\bin\cmake\win\x64\bin\cmake.exe`" --build build\attila --target twdll"
```

Note: building the default target also compiles `docs/lua/`, which fails from a
bare shell for the same missing-include reason — use `--target twdll` to skip it.

Deployment/tests go through `tools/twdll.ps1` (`tw-pack`/`tw-install`/
`tw-run`/`tw-test` CMake targets). `tw-test` launches the real game and takes
minutes — leave it to the user unless explicitly asked.

---

## Documentation

Every registered Lua module must have a generated docs page:

- Every Lua-exposed function needs an LDoc comment (`@function`, `@tparam`,
  `@treturn`) in its source file. A module page that shows no functions means a
  missing `@function` comment.
- Methods registered on a game script interface (e.g. `UNIT_SCRIPT_INTERFACE`,
  `FACTION_SCRIPT_INTERFACE`, `MILITARY_FORCE_SCRIPT_INTERFACE`) are called on
  the object (`obj:Method()`); the receiver is implicit and must NOT be listed
  as an `@tparam`. Only real explicit arguments get `@tparam`.
- The source file must be listed in the per-game `TWDLL_DOC_SOURCES` list in
  `docs/CMakeLists.txt` — one entry per module registered in `main.cpp`.
- Keep docstrings and generated docs up to date and correct after every code
  change, not just API additions: if a parameter, return value, or call style
  changes, update the docstring too.
- After adding, removing, or renaming any Lua API, regenerate the docs and
  commit them (this commit instruction is intentional and overrides the default
  "no commit without explicit request" policy):
  `cmake --build build/attila --target docs` (requires the MSVC
  environment, see Building). Verify the affected `docs/attila/modules/*.html`
  page contains the expected functions with correct signatures.

---

## Changelog

- Update `CHANGELOG.md` on every Lua API change: add, remove, or rename functions → new entry.
- New changes go under `## [Unreleased]` at the top; a version + date heading is added on release.
- Describe only the **user-facing Lua API** — what a modder can call and what it does.
  No technical details: no memory offsets, byte signatures, structs, or engine internals.
- Follow the file's existing style: `### Added` / `### Changed` / `### Fixed` / `### Removed`,
  one concise bullet per change.
- Keep entries small — a feature is one or a few bullets, not a dump of everything touched.
- Commit the changelog together with the code change (same intent as the docs/commit rule).

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
3. Regenerate and commit the docs (see Documentation).
