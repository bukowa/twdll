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
- **Runtime Memory Probing over Trial-and-Error Guessing.** When a pointer chain, struct offset, or entity layout is uncertain or deeply nested, do not blindly guess offsets. Inject a temporary diagnostic memory probe into the hook or accessor that scans memory or dumps raw pointers/strings in-process. One empirical live run provides 100% ground truth and eliminates search loops.
- **Warscape Built-in ABI Types Layout Ground Truth.** Never guess or misinterpret engine container layouts:
  - **`CA::String` / `CA_STD::string`**:
    - **32-bit**: `+0x0: uint32_t length`, `+0x4: uint32_t capacity`, **`+0x8: const char* buffer`** (Total size: 12B / `0xC`).
    - **64-bit**: `+0x0: uint64_t length`, `+0x8: uint64_t capacity`, **`+0x10: const char* buffer`** (Total size: 24B / `0x18`).
    - *Rule:* `CA::String` NEVER stores a pointer at offset `0x0`. Always read `*(const char**)(str + 0x8)` in 32-bit.
  - **`SAFE_PTR<T>`**: `+0x0: vtable`, `+0x4: T* object` (32-bit, 8B) / `+0x0: vtable`, `+0x8: T* object` (64-bit, 16B).
  - **`CA_STD::VECTOR<T>`**: `+0x0: start*`, `+0x4: finish*`, `+0x8: end_of_storage*` (32-bit, 12B).
- **Diagnose Leaf vs Trunk Before Redesigning Access Paths.** When a property resolution or telemetry hook returns `nil` / `unknown`, do not immediately assume the root pointer chain is wrong. First check if the parent pointer is non-null. If the parent exists, verify the leaf field's exact ABI type (`CA::String`, enum, bitflag) and disassembly accessor before speculating on alternative root paths.
- **No Research Scaffolding or Exploratory Leftovers in Production.** Diagnostic memory probes, ad-hoc string validation parsers, diagnostic loops, and speculative fallback branches must be cleanly removed once the true struct offset or ABI layout is identified and verified. Production code must contain only the direct, verified path.
- **Reasoning-First CAI Telemetry Standard.** Never log bare decision outcomes (e.g. `Decision: SACK`) without the full decision chain. All Campaign AI telemetry hooks must follow `docs/cai_telemetry_architecture_standard.md`: log the evaluating Faction Context, the Environmental Drivers/Needs (food, squalor, threat), the Candidate Option Set with mathematical weights/probabilities, and the explicit Rejection Reason for `NONE` outcomes.
- **No Autonomous Search Loops on Test Failure.** When an in-game test fails or an offset proves
  wrong, never start autonomous search chains across codebases or IDA MCP without first presenting
  the diagnosis, stating what needs to be researched, and getting explicit user approval ("ok").
- **Explicit Dual-Path API Standard (Custom In-Memory vs Database Localisation Keys).** Never use ambiguous or implicit string-guessing heuristics in a single setter (e.g. checking if a string starts with "names_"). Always provide explicit, symmetric method pairs: `Set<Property>` for direct in-memory UTF-8 custom values and `Set<Property>Key` for database localisation keys. All interactions, mode switches, and engine impacts between these methods must be comprehensively documented in LDoc with clear `@usage` examples, explanations of when to use which approach, and their behavior across game save/load and localization languages.
- **Flag conflicting or counterproductive instructions.** At the end of a turn, if you found an
  instruction in this file that conflicts with another, or that pushed you to do something
  counterproductive (e.g. a tool you were forced to call that added no value), mention it briefly
  so the user can fix the instruction itself rather than patching around it next time. After a
  session that surfaced repeated mistakes (loops, wrong assumptions, failed tool calls), propose
  a concrete edit to this file that would prevent the same mistake — fix the root cause in the
  instructions, not the symptom.

---

## Feature Development Loop

Every feature (new Lua API, new hook, new struct offset) runs the same loop. Do not skip steps; do not collapse them
into one giant action.

1. **Identify & Outline** — what to add: user request, feature idea, or gap vs. vanilla API.
   - Present a concise **Outline / Proposal** of the proposed Lua API shape, candidate methods, and plan to the user.
   - Wait for user feedback and approval ("ok") before launching into any deep reverse-engineering or IDA queries.
2. **Targeted Research (separate repo)** — only after outline approval, verify any missing offsets and struct
   layouts in the reverse-engineering project (64-bit DWARF reference + 32-bit IDA MCP on `empire.retail.dll`),
   never guessed here. Follow that project's conventions: anchor priority, gap analysis, `disasm` as the single
   source of truth. Keep it targeted.
3. **Propose Code Diff** — show the exact struct/offset additions and the C++ implementation diff. Wait for "ok"
   (see "Before Modifying Existing Files").
4. **Implement** — structs + `TW_ASSERT_OFFSET` in `tw_types.h`, methods in the `.cpp`, registration in `main.cpp`.
5. **Build** — `ninja twdll` (see Building).
6. **Test** — add cases to `tests/shared/testing.lua`; run `tw-test` (real game, takes minutes — the user usually
   runs it). Assert against verified values, not "not nil".
7. **Docs** — LDoc comments, add the file to `TWDLL_DOC_SOURCES`, regenerate docs, verify the HTML (see Documentation).
8. **Changelog** — one `[Unreleased]` bullet, user-facing only (see Changelog).
9. **Finish** — report status; commit/push only on explicit user request.

Steps 1 (outline approval), 3 (code diff proposal), and 6 (test) are the gates that catch wrong assumptions — never
skip them. One feature = one tight pass, not an open-ended tool-call chain.

### Test Failure & Investigation Gate

When a test fails, crashes, or returns unexpected values (`nil`, wrong values, assertion failures):
1. **DO NOT launch into an autonomous research or search loop** across local binaries, decompiled sources, or IDA MCP.
2. **Stop immediately and report to the user**:
   - **Observed Result**: Exact test log output and symptoms (e.g. `Olbia Slot initial rotation = nil`, `sem->m_slots count = 136214320`).
   - **Diagnosis / Hypothesis**: What failed and why (e.g. wrong struct offset, embedded sub-object vs pointer dereference).
   - **Proposed Research Target**: Exactly what struct/function/offset needs to be checked and where.
3. **Wait for explicit user permission ("ok") before starting the research pass.**

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
- **Do not name list-returning methods inconsistently** — any method returning a `*_LIST_SCRIPT_INTERFACE`
  userdata MUST follow the `Get<Thing>List` naming convention (e.g. `GetReligionList`, `GetPoliticalPartyList`,
  `GetCandidateList`, `GetFamilyMemberList`), matching the native Total War list convention (`character_list()`,
  `region_list()`, `faction_list()`). Never name it as a bare plural (e.g. `GetReligions` or `GetPoliticalParties`) without the `List` suffix.
- **Do not create aliases** — never create `snake_case`, alternative names, or duplicate method registrations
  for Lua-exposed functions unless explicitly requested by the user. Every function must have exactly ONE canonical
  `PascalCase` name (e.g. `InstantDefect`, `GetLoyalty`, `SetLoyalty`, `GetReligionList`, `AddTrait`, `RemoveTrait`).

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

### Lua API Naming Conventions

- **Canonical PascalCase Only (No Aliases)**: Every function registered in Lua MUST have exactly ONE canonical `PascalCase`
  name (e.g. `InstantDefect`, `GetLoyalty`, `SetLoyalty`, `AddTrait`, `RemoveTrait`, `GetTraitList`). Never register
  unrequested `snake_case` aliases or duplicate wrappers.
- **List Interface Return Methods**: Functions that return a list interface (`*_LIST_SCRIPT_INTERFACE`)
  must always be named `Get<Thing>List` in `PascalCase` (e.g. `region:GetReligionList()`, `faction:GetPoliticalPartyList()`).

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
- **Strict Parameter Fidelity & Verification**: Every changelog bullet MUST strictly match the exact method signatures, argument types, and real behavior implemented in C++ and documented in LDoc. Never speculate, generalize, or list unsupported parameters or targets (e.g. claiming a function spawns in a "region" or "military force" when the code only accepts settlements or coordinates). Always cross-check the C++ binding code before writing or editing changelog entries.
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

### Manual / Visual Test Scripts

When verifying visual, UI, or interactive features (e.g. building model rotation, clan/family tree panel updates, political allegiance reassignments, agent creation), the agent should provide a clear, standalone Lua snippet based on `tests/shared/manual_test_template.lua` that the user can run directly in-game (e.g. via Scriptum) to visually inspect the outcome.
**Scriptum / In-Game Environment Rule**: Never use `package.loadlib` in Scriptum scripts since the DLL is already loaded into the engine's global environment; always use `twdll = _G.script_env.twdll` (or `_G.script_env and _G.script_env.twdll or package.loadlib('twdll', "luaopen_twdll")()`).

### When You Change the Lua API

If you add, remove, or rename a Lua function:
1. Update `tests/shared/testing.lua`.
2. **Do not touch** `tests/attila/pack/shared/testing.lua` — regenerated automatically.
3. Regenerate and commit the docs (see Documentation).
