# twdll CLI & Reverse-Engineering Pipeline — Architecture & Scope Specification

**Status:** Approved Scope for Stage 1 (Environment Setup & Decompiler Analysis)  
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

- **Stage 1 — Toolchain Bootstrap, `status`, `init` & `analyze` (ACTIVE SCOPE):**
  Only three CLI commands exist in Stage 1: **`twdll status`**, **`twdll init`**, and **`twdll analyze`**.
  Stage 1 downloads all pinned portable tools (including both build tools and RE tools), copies and hashes all game binaries, and builds clean IDA/Ghidra databases in `.twdll/db/`.
- **Stage 2 — RE Extraction & Search (`extract`, `search`, `show`) (DESIGN GATE — DO NOT IMPLEMENT YET):**
  Once Stage 1 produces working databases in `.twdll/db/`, **STOP**. Do not write extraction or database storage code until the user and agent discuss and reach explicit consensus on the extraction format and storage schema.
- **Stage 3 — Multi-Game C++ Rewrite & Build/Test Commands (FUTURE SCOPE — DO NOT IMPLEMENT YET):**
  Rewriting `src/` for 3-game (`rome2`, `attila`, `tob`) interoperability and implementing `build`, `pack`, `install`, `run`, `test`, and `docs` commands happen **only after** Stages 1 and 2 are complete.

---

## 2. Hard Non-Goals & Guardrails for Agents (Stage 1)

Previous prototype branches failed because agents invented unrequested subsystems, stubbed future commands prematurely, or wrote parallel codepaths. The following rules are **mandatory**:

1. **ONLY implement `status`, `init`, and `analyze` in Stage 1:** Do NOT implement, wire flags for, or stub `extract`, `search`, `show`, `build`, `pack`, `install`, `run`, `test`, or `docs`.
2. **Do NOT touch `src/` or `CMakeLists.txt` in Stage 1:** C++ source changes and build/test wiring belong to Stage 3.
3. **No Python / `uv` CLI runtime:** The CLI is written in **Go**, compiled to a single native binary with minimal dependencies.
4. **No dual interactive-TUI vs headless-heuristic codepaths:** Never implement two separate onboarding flows (`if isatty` TUI wizard vs `--yes` silent path-guessing). Every command follows one unified `Options` -> optional TTY `Prompter` -> pure `Run(ctx, opts)` pipeline.
5. **No custom `--agent` mode or bespoke AI protocol:** Agents and humans use the exact same standard CLI commands, flags, exit codes, and `stderr` diagnostics (`clig.dev` / GitHub CLI `gh` pattern). `--json` is provided *only* on data-inspection commands (`twdll status --json`).
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
Located at repository root (gitignored). Stores **only explicit user choices and external machine paths** — zero internal `.twdll/` paths:

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
All downloaded tools, copied binaries, decompiler databases, logs, and machine state live under **one single gitignored directory** at `.twdll/` using fixed convention-over-configuration paths:

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
├── logs/                       # Raw headless decompiler logs
└── state.json                  # Machine-managed SHA-256 & completion manifest
```

### 3.4 Binary Integrity & SHA-256 Tracking
SHA-256 hashes serve two roles:
1. **Local Consistency (`.twdll/state.json`):** Tracks per binary (`attila_64`, `attila_32`, `rome2_32`, `tob_32`):
   - `source_path`: original game/depot path,
   - `binary_sha256`: hash of the isolated copy inside `.twdll/binaries/`,
   - `analyzed_sha256`: per-decompiler hash of the binary from which the completed database in `.twdll/db/<decompiler>/` was built.
   This detects game updates in Steam folders, interrupted analyses, or decompiler switches automatically.
2. **Known Official Hashes (versioned in `cli/`):** A table of known official Steam SHA-256 digests warns the user during `init` / `status` if a game binary is modified, outdated, or updated by a new patch.

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
- Binary presence and SHA-256 status in `.twdll/binaries/` (including Steam folder drift and known-hash match).
- Per-binary decompiler database status in `.twdll/db/<decompiler>/` (`ANALYZED` vs `NEEDS_ANALYSIS`).

#### 2. `twdll init [flags]`
Idempotent 5-step linear setup process (loads existing `twdll.toml` as defaults on re-runs):
- **Step 1 — Portable Build & Pack Toolchain:** Download and verify pinned portable `cmake`, `ninja`, `llvm` (`clang-cl`, `lld-link`, `llvm-rc`), `xwin` (32-bit `x86` MSVC CRT + Windows SDK), and `rpfm_cli` inside `.twdll/tools/`.
- **Step 2 — 32-bit Target Games (`attila`, `rome2`, `tob`):** Confirm/accept paths for owned games (minimum 1 required). Verify `<game>.exe` and `empire.retail.dll`, compute SHA-256, and copy `empire.retail.dll` into `.twdll/binaries/<game>_32/`.
- **Step 3 — Mandatory Reference Binary (`attila_64`):** Check `.twdll/binaries/attila_64/Attila` for valid 64-bit ELF header + `.debug_info` DWARF section. If missing, check default Steam depot path (`steamapps/content/app_325610/depot_325624/Attila`). If absent, copy `download_depot 325610 325624 <manifest>` to clipboard, open `steam://nav/console`, and either wait for user confirmation/custom path (interactive TTY) or exit cleanly with actionable instructions (`!TTY`). Copy verified ELF into `.twdll/binaries/attila_64/Attila`.
- **Step 4 — Decompiler Toolchain (`ghidra` | `ida`):**
  - `ghidra`: Download and verify pinned portable Ghidra + matching Adoptium OpenJDK into `.twdll/tools/` and verify `analyzeHeadless`.
  - `ida`: Validate `ida_dir` contains headless `idat` / `idat64`.
- **Step 5 — Save & Report:** Write `twdll.toml` and `.twdll/state.json`, then display per-binary database status (`ANALYZED` vs `NEEDS_ANALYSIS -> run twdll analyze`).

#### 3. `twdll analyze [--force] [--jobs N]`
- Inspects `.twdll/state.json` and skips binaries whose `.twdll/db/<decompiler>/` database already matches `binary_sha256` (unless `--force`).
- Runs headless IDA (`idat`) or Ghidra (`analyzeHeadless`) from scratch in parallel, governed by available system RAM and CPU cores (e.g., up to 4 concurrent processes for lightweight IDA; RAM-budgeted concurrency for JVM-heavy Ghidra).
- Writes verbose decompiler output to `.twdll/logs/analyze_<target>.log`, emits sparse progress updates, and records `analyzed_sha256` atomically **only** on clean exit code `0` (interrupted runs are wiped and restarted cleanly on next run).

---

## 5. Stage 1 Implementation Plan (Two Focused Milestones)

Execute Stage 1 in two separate, verified milestones before stopping for the Stage 2 design discussion:

- [ ] **Milestone 1.1 — Bootstrap, Core Config, `status` & `init`:**
  - Create `twdll.cmd` + `./twdll` wrappers and Go module under `cli/`.
  - Implement `twdll.toml` + `.twdll/state.json` + SHA-256 verification + `gh`-style `Options`/`Prompter`/TTY output layer.
  - Implement `twdll status [--json]` and all 5 steps of `twdll init` (portable `cmake`, `ninja`, `llvm`, `xwin`, `rpfm_cli`, `ghidra`/`jdk` downloads, 32-bit game binary copy, Steam Console `attila_64` ELF+DWARF acquisition).
  - *Exit Gate:* `.\twdll init` populates `.twdll/tools/` and `.twdll/binaries/` cleanly and `.\twdll status --json` reports all tools and binaries ready.

- [ ] **Milestone 1.2 — Headless Decompiler Database Builder (`twdll analyze`):**
  - Implement headless fresh-database builders for both IDA (`idat`) and Ghidra (`analyzeHeadless`) over `.twdll/binaries/`.
  - Implement RAM/CPU-aware parallel runner, `.twdll/logs/` routing, clean progress reporting, and atomic `analyzed_sha256` checkpointing in `.twdll/state.json`.
  - *Exit Gate:* `.\twdll analyze` builds verified databases in `.twdll/db/` from scratch and skips up-to-date targets on re-run.

> **HARD STOP AFTER MILESTONE 1.2:** Once `twdll init` and `twdll analyze` are verified, **stop and discuss Stage 2 with the user** to design the extraction format, `.twdll/extracted/` storage layout, and `extract` / `search` / `show` commands.

---

## 6. Preserved Design Context for Future Stages (Reference Only — Do Not Implement Now)

The notes below preserve agreed decisions from initial architecture discussions so context is not lost in future conversations:

### Stage 2 Context (RE Extraction, Search & LLM Agent Skills)
- `.twdll/extracted/` will act as a decompiler-agnostic barrier above `.twdll/db/(ghidra|ida)`.
- Downstream commands (`extract`, `search`, `show`) and LLM agent skills will operate exclusively on `.twdll/extracted/` without requiring live IDA/Ghidra MCP servers.
- Exact storage format (files vs SQLite/index) and extraction scripts will be designed interactively with the user at the start of Stage 2.

### Stage 3 Context (Multi-Game C++ Rewrite & Build/Test CLI Commands)
- **C++ Multi-Game Architecture (`rome2`, `attila`, `tob`):**
  - Move shared 32-bit Warscape ABI containers (`CA::String`, `CA::UniString`, `CA_STD::VECTOR<T>`, `UTILITYLIB::CONST_SAFE_PTR<T>`, entity links, `UTILITYDLL::I_TWEAKER`) into a common engine foundation using canonical names from `attila_64` DWARF.
  - Write module logic and Lua bindings once in shared `.cpp` modules using canonical DWARF class/field names, wrapping resolved native signatures as class methods rather than loose globals.
  - Isolate per-game folders (`rome2`, `attila`, `tob`) to verified 32-bit struct layouts and byte signatures, supporting incremental field coverage across games.
- **Future Build/Test CLI Commands (`build`, `pack`, `install`, `run`, `test`, `docs`):**
  - Compile via portable `.twdll/tools/` (`clang-cl` + `xwin` + `cmake` + `ninja`), exporting `compile_commands.json` with explicit `/imsvc` paths for `clangd` IDE autocomplete.
  - Unattended Windows game launch in `run` / `test` must: (1) ensure `steam_appid.txt` next to `<game>.exe`, (2) set `HKCU\SOFTWARE\The Creative Assembly\<Game>\last_game_version = 131076` to suppress post-crash dialogs, (3) exclude `CPU0` from processor affinity on Windows 11, and (4) automatically back up and restore the user's `user.script.txt` around test runs.
  - Legacy `tools/twdll.ps1` (along with private `tdd`, `mp`, and `consulscriptum` logic) will be removed when Stage 3 replaces it; Linux/WSL end-to-end verification will run once the pipeline is complete.
