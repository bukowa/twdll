# twdll CLI & Reverse-Engineering Pipeline — Architecture & Scope Specification

**Status:** Stage 1 Complete — Stage 2 RE Extraction & Search Active  
**Target Games:** Total War: Rome II (`rome2`), Total War: Attila (`attila`), Total War: Thrones of Britannia (`tob`)  
**Reference Binary:** Total War: Attila 64-bit Linux ELF with DWARF debug symbols (`attila_64`)

---

## 1. Vision & Sequencing Strategy

`twdll` is an in-process 32-bit C++ Lua script extender and reverse-engineering framework for Creative Assembly's Warscape engine titles of the Rome II / Attila generation.

### Core Goal
Create a **single, deterministic, self-contained workflow** driven by a clean **Go CLI (`twdll`)** so that both experienced developers and non-expert modders working with an LLM coding agent can:
1. Initialize the entire build and reverse-engineering toolchain locally (`twdll init`).
2. Acquire and verify the 32-bit game binaries (`empire.retail.dll`) and the mandatory 64-bit Linux `Attila` DWARF reference binary (`attila_64`).
3. Analyze those binaries from scratch into clean decompiler databases (`twdll analyze`).
4. Extract and query reverse-engineering data (`twdll extract`, `search`, `show` — designed iteratively after databases are ready).
5. Only **after** the RE pipeline is complete, rewrite the C++ `src/` architecture for 3-game interoperability and wire up the build/test/deploy CLI commands.

### Strict Stage Boundaries (Anti-Rushing Protocol)
To prevent agents from implementing everything at once or making premature assumptions, development is gated into **three strictly sequential stages**:

- **Stage 1 — Toolchain Bootstrap, `status`, `init`, `analyze` & `clean` (COMPLETE):**
  Stage 1 commands (`twdll status`, `twdll init`, `twdll analyze`, and `twdll clean`, plus global `--config`, `--data-dir`, `--root`, `--color`, and `--version` flags) are implemented and verified.
  Stage 1 downloads all pinned portable tools, stages and verifies all binaries, and builds tuned IDA/Ghidra databases in `.twdll/db/`.
- **Stage 1.5 — Pre-Testing Hardening & 2-Tier E2E (COMPLETE):**
  Restricted `KnownOfficialHashes` to pinned `attila_64`, added `TWDLL_TOOLS_DIR` for zero-junction isolation (with `init --clear` guard), and implemented Tier 1 (`go test -tags=e2e`) + Tier 2 (`cli/scripts/run-e2e-bg.ps1`) real-decompiler E2E tests before starting Stage 2 extraction.
- **Stage 2 — RE Extraction & Search (`extract`, `search`, `show`) (ACTIVE DESIGN SCOPE):**
  Stage 1 databases are ready in `.twdll/db/` and DWARF feasibility analysis is complete. Designing and agreeing on extraction format, `.twdll/extracted/` storage schema, and `extract` / `search` / `show` commands before implementation.
- **Stage 3 — Multi-Game C++ Rewrite & Build/Test Commands (FUTURE SCOPE — DO NOT IMPLEMENT YET):**
  Rewriting `src/` for 3-game (`rome2`, `attila`, `tob`) interoperability and implementing `build`, `pack`, `install`, `run`, `test`, and `docs` commands happen **only after** Stages 1 and 2 are complete.

---

## 2. Hard Non-Goals & Guardrails for Agents (Stage 1)

Previous prototype branches failed because agents invented unrequested subsystems, stubbed future commands prematurely, or wrote parallel codepaths. The following rules are **mandatory**:

1. **ONLY implement `status`, `init`, `analyze`, and `clean` before Stage 2:** Do NOT implement, wire flags for, or stub `extract`, `search`, `show`, `build`, `pack`, `install`, `run`, `test`, or `docs`.
2. **Do NOT touch `src/` or `CMakeLists.txt` in Stage 1:** C++ source changes and build/test wiring belong to Stage 3.
3. **No Python / `uv` CLI runtime:** The CLI is written in **Go**, compiled to a single native binary with minimal dependencies.
4. **No dual interactive-TUI vs headless-heuristic codepaths:** Never implement two separate onboarding flows (`if isatty` TUI wizard vs `--yes` silent path-guessing). Every command follows one unified `Options` -> optional TTY `Prompter` -> pure `Run(ctx, opts)` pipeline.
5. **No custom `--agent` mode or bespoke AI protocol:** Agents and humans use the exact same standard CLI commands, flags, exit codes, and `stderr` diagnostics (`clig.dev` / GitHub CLI `gh` pattern). `--json` emits structured machine-readable output on `stdout` (`twdll status --json`, `twdll init --json`, and Stage 2 extraction/query commands).
6. **No external pre-built decompiler databases:** Users cannot supply pre-existing `.i64`, `.idb`, or `.gpr` databases. Every database in `.twdll/db/` is created and analyzed from scratch from verified raw binaries.
7. **No `DepotDownloader`:** Acquiring `attila_64` uses the user's own Steam client (`steam://nav/console` + `download_depot` command copied to clipboard).
8. **No live decompiler MCP requirement:** The pipeline is 100% self-contained without requiring IDA/Ghidra GUIs or MCP plugins.

---

## 3. Workspace Topology & Configuration Contracts

### 3.1 Bootstrap Wrapper (`./twdll` & `twdll.cmd`)
To run on clean machines without Go pre-installed:
- Two minimal (~25-line) scripts sit at the repository root: `twdll.cmd` (Windows, using built-in `System32\curl.exe` and `tar.exe`) and `twdll` (POSIX `sh`, using `curl` and `tar`).
- Behavior:
  1. If `.twdll/bin/twdll(.exe)` exists and is up to date with `cli/` sources, execute it directly with all arguments (`0 ms` overhead).
  2. Otherwise, ensure portable Go exists in `.twdll/tools/go/` (downloading the pinned official Go archive if needed without admin privileges), compile `./cli` into `.twdll/bin/twdll(.exe)`, and execute it.

### 3.2 User Configuration (`twdll.toml`)
Located at repository root (gitignored). Default `twdll.toml` stores **only explicit user choices and external machine paths** — zero internal `.twdll/` paths (`data_dir` is `omitempty` and only used for custom isolated `--config` files):

```toml
default_game = "attila" # "attila" | "rome2" | "tob"
decompiler   = "ghidra" # "ghidra" | "ida"
ida_dir      = ""       # Required only when decompiler = "ida"

[games]
attila = "C:/Program Files (x86)/Steam/steamapps/common/Total War Attila"
rome2  = ""
tob    = ""
```

### 3.3 Self-Contained Local Directory (`.twdll/`)
All downloaded tools, copied binaries, decompiler databases, extracted RE artifacts, logs, and machine state live under **one single gitignored directory** at `.twdll/` using fixed convention-over-configuration paths:

```text
.twdll/
├── bin/                        # Compiled twdll CLI executable
├── tools/                      # Pinned portable zero-admin toolchains installed by `init`:
│   ├── go/                     # Portable Go toolchain (bootstrapped by wrapper)
│   ├── cmake/                  # Portable CMake
│   ├── ninja/                  # Portable Ninja
│   ├── llvm/                   # Portable LLVM (clang-cl, lld-link, llvm-rc)
│   ├── xwin/                   # 32-bit (x86) MSVC CRT & Windows SDK headers/libs
│   ├── rpfm/                   # Portable rpfm_cli packager
│   ├── jdk/                    # Portable OpenJDK (installed when decompiler = "ghidra")
│   └── ghidra/                 # Portable Ghidra (installed when decompiler = "ghidra")
├── binaries/                   # Isolated copies of verified raw binaries:
│   ├── attila_64/Attila        # 64-bit Linux ELF with DWARF (.debug_info)
│   ├── attila_32/empire.retail.dll
│   ├── rome2_32/empire.retail.dll
│   └── tob_32/empire.retail.dll
├── db/                         # Clean databases built from scratch by `twdll analyze`:
│   ├── ghidra/                 # Headless Ghidra projects
│   └── ida/                    # Headless IDA .i64 / .idb databases
├── extracted/                  # Stage 2 decompiler-agnostic RE index & cache
├── logs/                       # Raw headless decompiler logs & analyze_resources.jsonl
└── state.json                  # Machine-managed SHA-256 & completion manifest
```

### 3.4 Binary Integrity & SHA-256 Tracking
SHA-256 hashes serve two roles:
1. **Local Consistency (`.twdll/state.json`):** Tracks per binary (`attila_64`, `attila_32`, `rome2_32`, `tob_32`):
   - `source_path`: original game/depot path,
   - `binary_sha256`: hash of the isolated copy inside `.twdll/binaries/`,
   - `analyzed_sha256`: per-decompiler hash of the binary from which the completed database in `.twdll/db/<decompiler>/` was built.
   This detects local binary changes, interrupted analyses, or decompiler switches automatically.
2. **Known Reference Digest (`cli/internal/binaries/verify.go`):** Keeps `KnownOfficialHashes` only for the pinned `attila_64` depot binary (`3804137149572368482`) for the informational `[official]` badge in `twdll status`. 32-bit `empire.retail.dll` binaries update via Steam at any time and are validated by PE32 x86 header (`IMAGE_FILE_MACHINE_I386`) + local `state.json` hashes only.

---

## 4. Stage 1 Command Specifications (`status`, `init`, `analyze`)

### 4.1 Human & Agent Ergonomics (`gh` / `clig.dev` Standard)
1. **`Options` + `Prompter` Separation:**
   - Command flags + existing `twdll.toml` values populate a plain Go `Options` struct.
   - Only when `isatty(stdin) == true` and `--no-input` is not set, a thin `Prompter` asks the user for missing fields (showing current `twdll.toml` values as defaults).
   - Core execution (`RunInit`, `RunAnalyze`) never reads from `stdin`. When invoked by an LLM agent (`!isatty` or `--no-input`), missing required inputs immediately return a non-zero exit code with an actionable diagnostic on `stderr`:
     ```text
     error: missing reference binary (Attila 64-bit Linux DWARF)
       expected: C:/Program Files (x86)/Steam/steamapps/content/app_325610/depot_325624/Attila
       help: run `download_depot 325610 325624 3804137149572368482` in Steam Console (steam://nav/console)
       help: once finished, re-run `twdll init` (or pass `--attila-64 <path>`)
     ```
2. **Automatic Stream Formatting (`isatty(stdout/stderr)`):**
   - Interactive terminal (`TTY`): single-line `\r` progress bars and clean colors.
   - Agent / pipe (`!TTY`): zero ANSI escape codes, zero spinners — sparse plain-text milestone lines so LLM context windows are never polluted.

### 4.2 Stage 1 Commands

#### 1. `twdll status [--json]`
Reports complete workspace readiness:
- Installed portable tools in `.twdll/tools/` (`cmake`, `ninja`, `llvm`, `xwin`, `rpfm_cli`, `ghidra`/`jdk` or `ida`).
- Configured games from `twdll.toml`.
- Binary presence and SHA-256 status in `.twdll/binaries/` (plus `[official]` match for pinned `attila_64`).
- Per-binary decompiler database status in `.twdll/db/<decompiler>/` (`ANALYZED` vs `NEEDS_ANALYSIS`).

#### 2. `twdll init [--games <list>] [--json] [flags]`
Idempotent 4-step linear setup process (loads existing `twdll.toml` as defaults on re-runs; `--clear` clears `init`-owned `twdll.toml`, `binaries/`, non-Go local `tools/`, and `init` state while preserving `db/`, `extracted/`, `logs/`, and `analyzed_sha256`):
- **Step 1/4 — Portable Build & Pack Toolchain:** Download and verify pinned portable `cmake`, `ninja`, `llvm` (`clang-cl`, `lld-link`, `llvm-rc`), `xwin` (32-bit `x86` MSVC CRT + Windows SDK), and `rpfm_cli` inside `.twdll/tools/`.
- **Step 2/4 — Selective 32-bit Target Games (`attila`, `rome2`, `tob`):** Interactively select enabled 32-bit games via `Prompter.MultiSelect` checkboxes (or `--games attila,rome2` non-interactively; minimum 1 required, while unchecking a game prunes its staged copy in `.twdll/binaries/` without deleting `.twdll/db/` or `.twdll/extracted/`). Verify 32-bit x86 PE `empire.retail.dll`, compute SHA-256, and copy `empire.retail.dll` into `.twdll/binaries/<game>_32/`.
- **Step 3/4 — Mandatory Reference Binary (`attila_64`):** Check `.twdll/binaries/attila_64/Attila` for valid 64-bit ELF header + `.debug_info` DWARF section. If missing, check default Steam depot path (`steamapps/content/app_325610/depot_325624/Attila`). If absent, copy `download_depot 325610 325624 <manifest>` to clipboard, open `steam://nav/console`, and either wait for user confirmation/custom path (interactive TTY) or exit cleanly with actionable instructions (`!TTY`). Copy verified ELF into `.twdll/binaries/attila_64/Attila`.
- **Step 4/4 — Decompiler Toolchain (`ghidra` | `ida`):**
  - `ghidra`: Download and verify pinned portable Ghidra + matching Adoptium OpenJDK into `.twdll/tools/` and verify `analyzeHeadless`.
  - `ida`: Validate `ida_dir` contains headless `idat` / `idat64`.
- **Save & Report:** Write `twdll.toml` and `.twdll/state.json`, then display readiness summary (`next: run twdll analyze`).

#### 3. `twdll analyze [-d ghidra|ida] [--clear] [--force] [-j N] [-m SIZE] [--full-analysis] [targets...]`
- Inspects `.twdll/state.json` + binary traits (`InspectBinary` ELF DWARF / PE PDB inspection) and skips targets whose `.twdll/db/<decompiler>/` database already matches `binary_sha256` (unless `--force` or `--clear`; `--clear` removes the target's existing database, analyze log, and `analyzed_sha256` before analyzing).
- Schedules 32-bit targets in parallel before running `attila_64` solo, prevents OS sleep during batch runs, and writes raw decompiler logs to `.twdll/logs/analyze_<engine>_<target>.log` plus resource telemetry to `.twdll/logs/analyze_resources.jsonl`.
- **Ghidra tuning (`!FullAnalysis`):** Keeps `"DWARF.Import Data Types"`, `"DWARF.Import Functions"`, and `"DWARF.Create Function Signatures"` enabled while disabling 19 cascade/bloat analyzers (including `"Create Address Tables"` / `"Create Address Tables - One Time"`), configures `G1GC + UseStringDeduplication`, sizes heap (`8G–16G` for `attila_64`, `5G–6G` for 32-bit), and uses non-blocking swap/pagefile warnings on 8–12 GB hosts (`--full-analysis` restores heavy passes at `22G–36G`).
- **IDA tuning (`!FullAnalysis`):** Suppresses futile PDB HTTP lookups via `-Opdb:off` when no local `.pdb` exists and passes `-Odwarf:import_lnnums=0` on `attila_64`, while preserving `AF2_DOEH` and eager DWARF types (`0` struct/pseudocode degradation).
- Records `analyzed_sha256` atomically **only** on clean exit code `0`.

#### 4. `twdll clean [-d ghidra|ida|all] [--all] [targets...]`
- Removes generated decompiler databases under `.twdll/db/<decompiler>/<target>` and matching `.twdll/logs/analyze_<engine>_<target>.log` files, and clears `analyzed_sha256` in `.twdll/state.json` (plus `analyze_resources.jsonl` when unscoped) without deleting `.twdll/tools/` or `.twdll/binaries/`. Passing `--all` alone wipes all `init` and `analyze` artifacts (`twdll.toml`, `state.json`, `binaries/`, `db/`, `logs/`, non-Go local `tools/`) while preserving `.twdll/bin/`, `.twdll/tools/go/`, and any external `TWDLL_TOOLS_DIR`.

---

## 5. Stage 1 & Pre-Stage-2 Implementation Tracker

- [x] **Milestone 1.1 — Bootstrap, Core Config, `status` & `init`:**
  - Create `twdll.cmd` + `./twdll` wrappers and Go module under `cli/`.
  - Implement `twdll.toml` + `.twdll/state.json` + SHA-256 verification + `gh`-style `Options`/`Prompter`/TTY output layer.
  - Implement `twdll status [--json]` and all 4 setup steps of `twdll init` (portable `cmake`, `ninja`, `llvm`, `xwin`, `rpfm_cli`, `ghidra`/`jdk` downloads, 32-bit game binary copy, Steam Console `attila_64` ELF+DWARF acquisition).
  - *Exit Gate:* `.\twdll init` populates `.twdll/tools/` and `.twdll/binaries/` cleanly and `.\twdll status --json` reports all tools and binaries ready.

- [x] **Milestone 1.2 — Headless Decompiler Database Builder (`twdll analyze`):**
  - Implement headless fresh-database builders for both IDA (`idat`) and Ghidra (`analyzeHeadless`) over `.twdll/binaries/`.
  - Implement RAM/CPU-aware parallel runner, `.twdll/logs/` routing, clean progress reporting, and atomic `analyzed_sha256` checkpointing in `.twdll/state.json`.
  - *Exit Gate:* `.\twdll analyze` builds verified databases in `.twdll/db/` from scratch and skips up-to-date targets on re-run.

- [x] **Milestone 1.3 — `twdll analyze` Memory, DWARF & IDA Tuning:**
  - Verified single-pass `attila_64` Ghidra at `-Xmx10G` (`10.64 GiB` peak, 0 pseudocode diff), `attila_32` at `-Xmx5G` (`5.57 GiB`), warn-only 8–12+ GB RAM policy, ELF/PE inspection (`inspect.go`), and IDA `-Opdb:off` / `-Odwarf:import_lnnums=0`.

- [x] **Milestone 1.4 — Pre-Testing CLI Simplification & 2-Tier Real-Decompiler E2E Suite:**
  - Restricted `KnownOfficialHashes` to pinned `attila_64`, and added `TWDLL_TOOLS_DIR` override with `init --clear` guard.
  - Implemented Tier 1 quick real-decompiler E2E (`go test -tags=e2e ./internal/e2e` with LLVM-compiled `mini_warscape` PE32/ELF64 fixtures) and Tier 2 background soak runner (`cli/scripts/run-e2e-bg.ps1`, defaulting to `attila_32` with `-Full` for `+ attila_64`).

> **STAGE 1 COMPLETE:** `twdll init`, `twdll analyze`, `twdll clean`, and the 2-tier E2E suite are verified (`attila_64`, `attila_32`, `rome2_32`, `tob_32`). Stage 2 extraction design is now open.

---

## 6. Preserved Design Context for Future Stages (Reference Only — Do Not Implement Now)

The notes below preserve agreed decisions from architecture & research discussions so context is not lost in future conversations:

### Stage 2 Context (RE Extraction, Search, Portable Anchors & Audit Gate — Active)
- `.twdll/extracted/` acts as the decompiler-agnostic barrier above `.twdll/db/(ghidra|ida)` using pure-Go SQLite (`modernc.org/sqlite` + `FTS5`, zero CGO):
  - `.twdll/extracted/<target>/binary.sqlite`: Static binary structure, DWARF4 debug info (`attila_64`), and x86 instruction graph (`functions`, `strings`, `xrefs`, `call_edges`, `vtables`, `rtti_*`, `lua_regs`), plus optional decompiler topology enrichment.
  - `.twdll/extracted/<target>/decomp_ida.sqlite` & `decomp_ghidra.sqlite`: Resumable per-engine SQLite WAL stores for decompiled pseudo-C (`decomp_functions` + `decomp_fts`), `line_ea_json` (`Line <-> EA` mapping), and local/stack variables (`decomp_vars`).
  - `.twdll/extracted/<target>/manifest.json`: Fast readiness/count summary for `status`.
  - `.twdll/extracted/derived.sqlite`: Disposable Go-derived cross-binary mappings (`symbol_matches`, `struct_recoveries`, `vtable_alignments`) rebuilt by `twdll derive` over read-only `binary.sqlite` + `decomp_<engine>.sqlite`.
- `re/anchors/<target>.toml` (Git-tracked) holds `.reloc`-safe byte signatures and `x86asm`-verified 32-bit field offsets validated by `twdll verify`.
- **Zero Optimism & Pre-Stage-3 Audit Gate:** Extracted rows in `binary.sqlite`, `decomp_<engine>.sqlite`, and `derived.sqlite` are never treated as infallible facts. Compiler optimizations, decompiler hallucinations, and bugs in our own Go extractors/heuristics are expected. Stage 2 commands (`extract`, `derive`, `search`, `show`, `verify`) are built incrementally and followed by a mandatory **Extraction Audit & Hardening Phase** (cross-checking IDA, Ghidra, DWARF, and on-demand `x86asm` disassembly, and fixing extractor bugs) before unlocking Stage 3 (`src/` C++ rewrite). `twdll clean --all` wipes `.twdll/extracted/`, whereas scoped `twdll clean` and `twdll init --clear` preserve it.

### Stage 3 Context (Multi-Game C++ Rewrite & Build/Test CLI Commands)
- **C++ Multi-Game Architecture (`rome2`, `attila`, `tob`):**
  - Move shared 32-bit Warscape ABI containers (`CA::String`, `CA::UniString`, `CA_STD::VECTOR<T>`, `UTILITYLIB::CONST_SAFE_PTR<T>`, entity links, `UTILITYDLL::I_TWEAKER`) into a common engine foundation using canonical names from `attila_64` DWARF.
  - Write module logic and Lua bindings once in shared `.cpp` modules using canonical DWARF class/field names, wrapping resolved native signatures as class methods rather than loose globals.
  - Isolate per-game folders (`rome2`, `attila`, `tob`) to verified 32-bit struct layouts and byte signatures, supporting incremental field coverage across games.
- **Future Build/Test CLI Commands (`build`, `pack`, `install`, `run`, `test`, `docs`):**
  - Compile via portable `.twdll/tools/` (`clang-cl` + `xwin` + `cmake` + `ninja`), exporting `compile_commands.json` with explicit `/imsvc` paths for `clangd` IDE autocomplete.
  - Unattended Windows game launch in `run` / `test` must: (1) ensure `steam_appid.txt` next to `<game>.exe`, (2) set `HKCU\SOFTWARE\The Creative Assembly\<Game>\last_game_version = 131076` to suppress post-crash dialogs, (3) exclude `CPU0` from processor affinity on Windows 11, and (4) automatically back up and restore the user's `user.script.txt` around test runs.
  - Legacy `tools/twdll.ps1` (along with private `tdd`, `mp`, and `consulscriptum` logic) will be removed when Stage 3 replaces it; Linux/WSL end-to-end verification will run once the pipeline is complete.
- **Preserved Warscape & Lua API Conventions (from `git show c9a35e9:AGENTS.md` / tag `archive/legacy-agents-v1`):**
  - *32-bit vs 64-bit Warscape ABI Ground Truth:* `CA::String` (`32b`: `+0x0 len`, `+0x4 cap`, `+0x8 char*`, `0xC` bytes; `64b`: `+0x0 len`, `+0x8 cap`, `+0x10 char*`, `0x18` bytes — never dereference `+0x0` as `char*`), `SAFE_PTR<T>` (`+0x0 vtable`, `+0x4`/`+0x8 T*`), `CA_STD::VECTOR<T>` (`start*`, `finish*`, `end_of_storage*`).
  - *Typed Structs & Signatures Only:* Model all engine paths and entity links as typed C++ structs/templates guarded by compile-time offset assertions; resolve all native functions via byte signatures rather than hardcoded RVAs; strip temporary memory probes before finalizing.
  - *Lua API Surface & Lifecycle:* One canonical `PascalCase` name per method (no aliases); `Get<Thing>List` suffix for list-interface returns; explicit symmetric `Set<Prop>` (UTF-8 in-memory) vs `Set<Prop>Key` (DB localisation key) pairs; mandatory snapshot-and-restore rollback of all global engine patches/hooks on Lua environment teardown; reasoning-first CAI telemetry (`docs/cai_telemetry_architecture_standard.md`).
