# `twdll` Go CLI

Native Go CLI driving the self-contained `twdll` reverse-engineering pipeline for Total War: Rome II (`rome2`), Attila (`attila`), and Thrones of Britannia (`tob`).

**Stage Status:** Stage 1 (`status`, `init`, `analyze`, `clean` + 3-tier E2E verification) is **complete**. Stage 2 (layer-isolated `bronze.sqlite` + `silver_<engine>.sqlite` extraction via `extract` and `status` readiness; `search`/`show`/`derive`/`verify` deferred until `bronze` and `silver` are validated, followed by the mandatory Extraction Audit phase) is **active**. Stage 3 (`src/` C++ rewrite + `build`/`pack`/`install`/`run`/`test`/`docs`) is **planned / frozen** until Stage 2 implementation and audit complete.  
See [`../docs/cli_re_pipeline_spec.md`](../docs/cli_re_pipeline_spec.md) for the full architecture spec and [`AGENTS.md`](AGENTS.md) for code invariants.

---

## Bootstrap Wrappers & Quick Start

Run the CLI from the repository root without a pre-installed system Go toolchain:

```powershell
# Windows
.\twdll.cmd init
.\twdll.cmd status
.\twdll.cmd analyze
```

```sh
# Linux / macOS / Git Bash
./twdll init
./twdll status
./twdll analyze
```

How the wrappers (`twdll.cmd`, `twdll.ps1`, `./twdll`) work:
- If `.twdll/bin/twdll(.exe)` exists and is newer than watched `cli/` source files (`.go` excluding `*_test.go`, `.java`, `.idc`, `.py`, `go.mod`, `go.sum` outside `.git`/`testdata`), or if `TWDLL_NO_REBUILD=1` is set, the wrapper runs the binary directly without recompiling.
- Otherwise, it uses Go from `$TWDLL_TOOLS_DIR/go/bin/go` (if set) or `.twdll/tools/go/bin/go` (downloading the pinned official Go archive if missing), compiles `./cli` before launching it, and forwards all arguments.

---

## Commands & Flags

### Global Flags

| Flag | Env Var | Default | Description |
| :--- | :--- | :--- | :--- |
| `-v`, `--version` | — | — | Print CLI version. |
| `-c`, `--config <path>` | `TWDLL_CONFIG` | `<root>/twdll.toml` | Path to user config file. |
| `--data-dir <path>` | `TWDLL_DATA_DIR` | `<root>/.twdll` (or `config.data_dir`) | Override local workspace data directory. |
| `--root <path>` | `TWDLL_ROOT` | Auto-detected via `cli/go.mod` | Override repository root directory. |
| `--color <mode>` | — | `auto` | Color output mode: `auto`, `always`, `never`. |

### Subcommands

- **`twdll status [--json]`**  
  Reports installed portable tools, configured games, staged binaries (with `[official]` check for pinned `attila_64`), per-target decompiler database readiness, and `.twdll/extracted/<target>/` (`bronze.sqlite` and `silver_<engine>.sqlite`) status. `--json` writes machine-readable JSON strictly to `stdout`.

- **`twdll init [flags]`**  
  Idempotent 4-step setup: (1) portable `cmake`, `ninja`, `llvm`, `xwin`, `rpfm_cli`; (2) 32-bit `empire.retail.dll` staging; (3) mandatory 64-bit Linux `attila_64` ELF+DWARF acquisition/verification; (4) `ghidra` (+ OpenJDK) or `ida` validation.
  - `--clear` — Clear `init` artifacts (`twdll.toml`, `binaries/`, non-Go `.twdll/tools/`, and `init` entries in `state.json`) before re-initializing, preserving `analyze` and `extract` artifacts (`db/`, `extracted/`, `logs/`, `analyzed_sha256`). Never deletes an external `TWDLL_TOOLS_DIR`.
  - `--no-input` — Disable interactive prompts and fail fast with actionable `stderr` diagnostics on missing inputs.
  - `--json` — Emit machine-readable JSON summary to `stdout`.
  - `--games <GAMES>` — 32-bit target games to enable (comma-separated: `attila,rome2,tob`); interactive TTY prompts via multi-select checkboxes (`attila_64` remains mandatory; unchecked 32-bit games have their `.twdll/binaries/<game>_32/` copy pruned without deleting `.twdll/db/` or `.twdll/extracted/`).
  - `--default-game <attila|rome2|tob>`
  - `--decompiler <ghidra|ida>`
  - `--ida-dir <path>`, `--attila-dir <path>`, `--rome2-dir <path>`, `--tob-dir <path>`, `--attila-64 <path>`

- **`twdll analyze [flags] [targets...]`**  
  Runs headless Ghidra or IDA analysis from scratch into `.twdll/db/<engine>/<target>/`, running 32-bit targets in parallel before `attila_64` solo, and checkpoints `analyzed_sha256` atomically in `.twdll/state.json` only on exit code `0`.
  - `-d`, `--decompiler <ghidra|ida>` — Override `twdll.toml` decompiler.
  - `--clear` — Clear existing target database and log before analyzing.
  - `--force` — Re-analyze even when `analyzed_sha256` already matches.
  - `-j`, `--jobs <N>` — Max concurrent 32-bit workers (`0` = RAM/CPU auto).
  - `-m`, `--max-mem <SIZE>` — Override Ghidra JVM `-Xmx` (e.g. `16G`, `32G`).
  - `--full-analysis` — Keep heavy cascade/reference and DWARF line/comment passes enabled on `attila_64` (higher RAM usage).
  - `[targets...]` — Positional target list or glob patterns (`attila_64`, `attila_32`, `rome2_32`, `tob_32`, `*_32`, `attila_*`).

- **`twdll extract [flags] [targets...]`**  
  Extracts Layer 1 deterministic format/DWARF4 facts (`bronze.sqlite`) and independent Layer 2 per-engine decompiler metadata/pseudo-C (`silver_ghidra.sqlite`, `silver_ida.sqlite`) into `.twdll/extracted/<target>/` (with `manifest.json` readiness metadata).
  - `-d`, `--decompiler <ghidra|ida|all>` — Override `twdll.toml` decompiler (or extract both via `all`).
  - `--force` — Re-extract even when extracted databases are up to date.
  - `-j`, `--jobs <N>` — Max concurrent decompilation workers (`0` = auto).
  - `-m`, `--max-mem <SIZE>` — Override Ghidra JVM max heap per worker (e.g. `4G`, `8G`).
  - `--no-decomp` — Extract `bronze.sqlite` and `silver` metadata (`silver_funcs`, `silver_func_ranges`, `silver_asm`, `silver_strings`) without pseudo-C decompilation.
  - `--timeout <DURATION>` — Per-function decompilation timeout (default: `15s`; `0s` disables timeout).
  - `--no-timeout` — Disable per-function decompilation timeout.
  - `--retry`, `--retry-timeouts`, `--retry-failed` — Retry previously timed-out and/or failed functions during decompilation.
  - `--json` — Emit machine-readable JSON summary to `stdout`.
  - `[targets...]` — Positional target list or glob patterns (`attila_64`, `attila_32`, `rome2_32`, `tob_32`, `*_32`, `attila_*`).

- **`twdll clean [-d ghidra|ida|all] [--all] [targets...]`**  
  Deletes `.twdll/db/<engine>/<target>` and matching `.twdll/logs/analyze_<engine>_<target>.log` files for explicit targets or glob patterns (`*_32`, `attila_*`) and clears `analyzed_sha256` in `.twdll/state.json` (plus `analyze_resources.jsonl` when unscoped) without touching `.twdll/tools/`, `.twdll/binaries/`, or `.twdll/extracted/`. Pass `--all` alone to wipe all `init`, `analyze`, and `extract` artifacts (`twdll.toml`, `state.json`, `binaries/`, `db/`, `extracted/`, `logs/`, non-Go local `tools/`) while preserving `.twdll/bin/`, `.twdll/tools/go/`, and any external `TWDLL_TOOLS_DIR`.

### Additional Environment Variables

| Variable | Purpose |
| :--- | :--- |
| `TWDLL_TOOLS_DIR` | Point to an external read-only shared tools directory (used by sandboxes/E2E tests without junctions). Protected from `EnsureDirs` and `init --clear`. |
| `TWDLL_NO_REBUILD=1` | Skip source timestamp rebuild checks in `twdll`, `twdll.cmd`, and `twdll.ps1`. |
| `TWDLL_WRAPPER_ROOT` | Set by root wrappers to pass detected repository root into the binary. |

---

## Directory Layout

```text
cli/
├── main.go                     # Kong CLI entrypoint & flag wiring
├── AGENTS.md                   # Go CLI architecture & test invariants
├── README.md                   # This file
├── internal/
│   ├── analyze/                # ELF/PE inspection, memory sizing, Ghidra/IDA runners, telemetry
│   ├── binaries/               # Steam discovery, PE32/ELF64+DWARF verification, pinned attila_64 hash
│   ├── bronze/                 # Pure-Go PE32/ELF64/data-string/DWARF4 extractors feeding bronze.sqlite
│   ├── cmd/                    # status, init, analyze, extract, clean orchestration
│   ├── e2e/                    # //go:build e2e real-decompiler harness & mini_warscape.cpp fixture
│   ├── extractdb/              # Pure-Go SQLite (modernc.org/sqlite + FTS5) storage (AtomicBuilder, SilverStore, manifest.json)
│   ├── silver/                 # Independent headless IDA & Ghidra exporters feeding silver_<engine>.sqlite
│   ├── term/                   # TTY detection, Out/Err stream separation, prompts, diagnostics
│   ├── toolchain/              # Portable tool manifest, archive downloads, IDA validation
│   └── workspace/              # twdll.toml, .twdll/state.json, path resolution
└── scripts/
    └── run-e2e-bg.ps1          # Detached Tier 2 real-binary soak runner (Windows PowerShell)
```

---

## Testing Guide (3 Tiers)

All tests are strictly isolated from live `twdll.toml` and `.twdll/` data via `t.TempDir()`, `--config`, `--data-dir`, and `TWDLL_TOOLS_DIR`. If `go` is not on `PATH`, substitute `..\.twdll\tools\go\bin\go.exe` (Windows) or `../.twdll/tools/go/bin/go` (POSIX).

| Tier | Command (from `cli/`) | Runtime / RAM | What It Covers | When to Run |
| :--- | :--- | :--- | :--- | :--- |
| **Tier 0: Unit** | `go vet ./...`<br>`go test ./...` | `<5s` / `<200 MB` | Flag parsing, `Out`/`Err` separation, `state.json` & `twdll.toml` round-trips, memory/CLI-arg builders, ELF/PE inspection, fake-decompiler runner tests. | **Mandatory** on every `cli/` edit. |
| **Tier 1: Quick Real E2E** | `go test -v -tags=e2e -timeout 5m ./internal/e2e` | `~30–45s` / `~2.5 GB` | Compiles `internal/e2e/testdata/mini_warscape.cpp` (~20 KB PE32 DLL + ELF64-DWARF) via portable LLVM and runs real `twdll.exe`, IDA, and Ghidra (`init -> analyze -> skip check -> extract -d all -> status --json -> clean -> init --clear -> clean --all`) while verifying `bronze`/`silver` SQLite invariants. Asserts live `twdll.toml` & `.twdll/state.json` SHA-256 + mtime remain unchanged. | Required before finishing changes to `init`, `analyze`, `extract`, `clean`, `status`, `workspace`, decompiler scripts, or PE/ELF inspection. |
| **Tier 2: Background Soak** | `pwsh -NoProfile -File scripts/run-e2e-bg.ps1` *(from `cli/`, or `cli/scripts/run-e2e-bg.ps1` from root)* | `~20–22m` (`attila_32`)<br>`~75m` (`-Full`)<br>`6–17 GB` RAM | Detached single-instance soak test (`TWDLL_E2E_SOAK=1`) against real game binaries copied from `.twdll/binaries/` into an isolated sandbox. Archives `analyze_resources.jsonl`, prunes sandbox on pass, retains on fail. | Only when explicitly validating real-binary scale, DWARF import, or JVM/IDA memory changes. |

### Tier 1 & Tier 2 Overrides

- **Tier 1 requirements:** Provisioned portable tools (`llvm`, `go`, `ghidra`, `jdk`, `xwin`) in `.twdll/tools/` (or `TWDLL_TOOLS_DIR`) and a valid IDA installation (resolved from `TWDLL_E2E_IDA_DIR`, `<root>/twdll.toml`, or auto-detection).
- **Tier 1 env vars:**
  - `TWDLL_TOOLS_DIR` — Override shared read-only tools path (default: `<root>/.twdll/tools`).
  - `TWDLL_E2E_IDA_DIR` — Override IDA installation directory.
  - `TWDLL_E2E_SANDBOX_DIR` — Override sandbox directory (default: `t.TempDir()`).
  - `TWDLL_E2E_CLI_BIN` — Use a prebuilt `twdll.exe` instead of compiling inside the test.
- **Tier 2 runner options (`cli/scripts/run-e2e-bg.ps1`):**
  - `-Mode mini|attila_32|full` (default: `attila_32`; `-Full` sets `-Mode full` to test `attila_32,attila_64`).
  - `-Wait` — Wait synchronously instead of detaching.
  - `TWDLL_E2E_LIVE_BINS_DIR` — Override source `.twdll/binaries` directory for soak copies.
  - Artifacts, `status.json`, `run.log`, `analyze_resources.jsonl`, and `latest.txt` are written under `%LOCALAPPDATA%\Temp\twdll\twdll-e2e-runs\<timestamp>-<sha>\`.
