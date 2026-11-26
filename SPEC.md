# `twdll` Reverse-Engineering CLI Specification

`twdll` is a cross-platform (Linux and Windows) Python 3.12+ reverse-engineering CLI and stdio MCP (Model Context Protocol) server for Warscape Total War binaries. It analyzes the 64-bit Linux reference binary (`attila_linux_64`, Total War: Attila, which includes DWARF debug symbols) and maps functions, strings, and types to stripped 32-bit Windows retail binaries (`attila_windows_32` — Total War: Attila, `rome2_windows_32` — Total War: Rome II, and `tob_windows_32` — Total War Saga: Thrones of Britannia).

## 1. Core Architectural Principles

1. **Conceptual Alignment**: Folder names, module names, CLI commands, and entrypoint function names mirror the problem domain 1-to-1:
   - Data flows strictly in one direction: **`init` -> `analyze` -> `extract` -> `sql`**.
   - Every pipeline step owns its matching `.twdll/<step>/` directory and its flat `cli/src/twdll/<step>.py` module:

   | CLI Command | Step Directory | Python Module | Entrypoint Function | Return Type (`domain.py`) |
   | :--- | :--- | :--- | :--- | :--- |
   | `twdll init` | `.twdll/init/` | `twdll/init.py` | `init.init(...)` | `InitResult` |
   | `twdll status` | *(reads `.twdll/*`)* | `twdll/init.py` | `init.status(...)` | `WorkspaceStatus` |
   | `twdll clean` | `.twdll/<target>/` | `twdll/init.py` | `init.clean(...)` | `CleanResult` |
   | `twdll mcp` | *(stdio server)* | `twdll/mcp.py` | `mcp.serve_mcp(...)` | `None` |
   | `twdll analyze` | `.twdll/analyze/` | `twdll/analyze.py` | `analyze.analyze(...)` | `AnalyzeResult` |
   | `twdll extract` | `.twdll/extract/` | `twdll/extract.py` | `extract.extract(...)` | `ExtractResult` |
   | `twdll sql` | *(reads `extract`)* | `twdll/query.py` | `query.sql(...)` | `SqlQueryResult` |

2. **Explicit Intent & Single-Point Path/Naming Constants**:
   - Never use raw `str` or untyped `int` where a domain concept exists.
   - Represent finite domain states, directory names, and file extensions with `StrEnum`, virtual addresses and external IDs with `typing.NewType`, and filesystem locations with `pathlib.Path` (`Path | None` when optional, never `""`).
   - **Single-Point Path & Naming Rule**: Outside `domain.py`, no step module, CLI command, or test may hardcode workspace directory names (`".twdll"`, `"binaries"`, `"tools"`), target names (`"attila_linux_64"`), or artifact suffixes (`".sqlite"`, `".gpr"`, `".i64"`, `".meta.json"`). Every module and test must reference `domain.py` enums and `WorkspacePaths`.
   - **Pre-Release Zero-Versioning Rule**: `twdll` is pre-release (unreleased). Never add schema version numbers (`schema_version`, `_META_SCHEMA_VERSION`), version-bump invalidation, or backward-compatibility migration shims to `.meta.json`, SQLite `meta` tables, or config files.

3. **Pipeline Data Quality Chain (`analyze` -> `extract` -> `sql`)**:
   - **`analyze`**: Produces decompiler projects containing discovered function boundaries, switch jump-tables, cross-references (`xrefs`), strings, stack frames, and calling conventions, paired with an atomic `.meta.json` completion marker. On DWARF-bearing ELF64 targets (`attila_linux_64`), Ghidra's built-in `DWARF` analyzer is disabled to prevent heap exhaustion on 242.78 MB `.debug_info` while retaining `.symtab`, demangling, control flow, and xrefs. Do not disable other default decompiler analyzers if doing so drops xrefs or control flow.
   - **`extract`**: Runs in two ordered sub-steps for DWARF-bearing targets:
     1. **DWARF extraction first (backend-neutral)**: Extracts C++ DWARF v4 facts and `.symtab` `_ZTV*` vtable slots from `attila_linux_64` out-of-process via `twdll-dwarf` (`gimli` + `object` + `libsqlite3` FFI, cross-checked against `llvm-dwarfdump` where needed) into SQLite (`dwarf_*` tables) independently of Ghidra or IDA.
     2. **Decompiler fact & pseudocode extraction**: Copies empirically verified fields from the decompiler API (`pyghidra` / IDAPython) into per-binary SQLite databases (`.twdll/extract/`), using the `dwarf_*` tables for coarse batch type/vtable/signature population (pre-flattened structs, synthesized `<Class>_vtbl` vtables, `CUSTOM_STORAGE` SysV `this`/`sret` parameter registers, stack/register locals, and inline call comments) before generating C pseudocode. Every table, column, and join key must be empirically verified (Step 3A) before schema lock.
   - **`sql`**: Exposes completed per-binary extracted SQLite databases for read-only cross-binary SQL inspection via an `:memory:` connection.

---

## 2. Workspace & Flat 1-to-1 Directory Layout

All local state lives in `twdll.toml` and `.twdll/` at the workspace root. Every pipeline step owns its matching `.twdll/<step>/` directory with flat, uniformly named files:

```text
<workspace>/
├── twdll                       # POSIX bootstrap wrapper (provisions .twdll/uv/)
├── twdll.cmd                   # Windows bootstrap wrapper (provisions .twdll/uv/)
├── twdll.toml                  # User configuration file (created by `twdll init`)
└── .twdll/
    ├── uv/                     # Isolated portable uv, Python 3.12+, and virtualenv
    │   ├── bin/
    │   ├── cache/
    │   ├── python/
    │   └── venv/
    ├── init/                   # Step 1 (`twdll init` / `twdll clean init`)
    │   ├── tools/              # Portable Ghidra 12.2 & Temurin JDK 25
    │   └── binaries/           # Flat header-verified staged game binaries
    │       ├── attila_linux_64
    │       ├── attila_windows_32.dll
    │       ├── rome2_windows_32.dll
    │       └── tob_windows_32.dll
    ├── analyze/                # Step 2 (`twdll analyze` / `twdll clean analyze`)
    │   ├── ghidra_attila_linux_64.gpr (+ .rep/ + ghidra_attila_linux_64.meta.json)
    │   ├── ghidra_attila_windows_32.gpr (+ .rep/ + ghidra_attila_windows_32.meta.json)
    │   ├── ida_attila_linux_64.i64
    │   └── ida_attila_windows_32.i64
    └── extract/                # Step 3 (`twdll extract` / `twdll clean extract`)
        ├── ghidra_attila_linux_64.sqlite
        ├── ghidra_attila_windows_32.sqlite
        ├── ida_attila_linux_64.sqlite
        └── ida_attila_windows_32.sqlite
```

- **Windows safety**: `.twdll/uv/` is separate from `.twdll/init/` so `twdll clean init` or `twdll clean all` never attempts to delete the running `python.exe` inside `.twdll/uv/venv/`.
- **`twdll.toml` format**:
  ```toml
  # Active decompiler: "ghidra" or "ida"
  decompiler = "ghidra"
  # Selected 32-bit target binaries (attila_linux_64 reference is always included)
  targets = ["attila_windows_32", "rome2_windows_32", "tob_windows_32"]

  [binaries]
  # Optional explicit file paths (auto-discovered from Steam if empty)
  attila_linux_64 = ""
  attila_windows_32 = ""
  rome2_windows_32 = ""
  tob_windows_32 = ""

  [toolchain]
  # Optional explicit tool directory overrides (never resolved from user PATH)
  ghidra_dir = ""
  java_home = ""
  ida_dir = ""
  ```

---

## 3. Flat 1-to-1 Python Module Layout & Layer Contracts

All CLI code lives under `cli/src/twdll/` with a flat 1-to-1 module layout:

```text
cli/
├── pyproject.toml
├── src/
│   └── twdll/
│       ├── __init__.py
│       ├── py.typed
│       ├── domain.py           # Layer 0: Enums, NewTypes, constants, paths, dataclasses (stdlib-only)
│       ├── database.py         # Layer 1: SQLite schemas, writers, and read-only SQL executor (stdlib-only)
│       ├── _ghidra.py          # Layer 1: Shared internal PyGhidra/JVM lifecycle & project helpers
│       ├── init.py             # Layer 2: Config, toolchain, Steam discovery, staging, status, clean
│       ├── analyze.py          # Layer 2: Headless Ghidra / IDA project analysis
│       ├── extract.py          # Layer 2: Per-binary fact extraction into .twdll/extract/<decompiler>_<binary>.sqlite
│       ├── query.py            # Layer 2: Read-only sql query entrypoint
│       ├── mcp.py              # Layer 3: MCP stdio server adapter (only module importing `mcp`)
│       └── cli.py              # Layer 3: Typer CLI commands and output formatting
└── tests/
    ├── test_architecture.py
    ├── test_init.py
    ├── test_mcp.py
    ├── test_analyze.py
    ├── test_extract.py
    └── test_query.py
```

### 3.1 `import-linter` Architectural Contracts (`cli/pyproject.toml`)
1. `include_external_packages = true` at `[tool.importlinter]` root.
2. **Strict one-way layers**:
   - `twdll.cli`
   - `twdll.mcp`
   - `twdll.init | twdll.analyze | twdll.extract | twdll.query` (independent sibling step modules)
   - `twdll._ghidra | twdll.database` (independent shared infrastructure modules)
   - `twdll.domain`
3. **Centralized Ghidra/JVM**: Only `twdll._ghidra` may import `pyghidra` or `jpype`.
4. **Centralized SQLite**: Only `twdll.database` may import `sqlite3`.
5. **CLI separation**: Only `twdll.cli` may import `typer` or `rich`.
6. **MCP separation**: Only `twdll.mcp` may import `mcp` (`allow_indirect_imports = true` so `twdll.cli` may invoke `twdll.mcp.serve_mcp`).
7. **Stdlib-only core**: `twdll.domain` and `twdll.database` may only import Python standard library modules so PyGhidra and IDAPython scripts can import them directly.

---

## 4. Architectural & Code Quality Rules

1. **Declarative Tables & Exhaustive `match / case`**:
   - Store per-enum configuration in declarative lookup tables in `domain.py` (`BINARY_SPECS`, `GHIDRA_PORTABLE_SPEC`, `JDK_PORTABLE_SPECS`).
   - Use Python 3.12 `match / case` when dispatching on `StrEnum`; omit wildcard `case _:` on enum matches so `basedpyright` enforces compile-time exhaustiveness.
   - Use flat early-return guard clauses for simple precondition checks.
2. **Small, Single-Purpose Functions**:
   - Separate pure data parsing/resolution from disk/network I/O side effects.
   - Never use boolean mode flags that mix read-only inspection with disk mutation.
   - Complexity limits are enforced mechanically via Ruff in `cli/pyproject.toml` (`C901 <= 8`, `PLR0912 <= 8`, `PLR0913 <= 6`, `PLR0915 <= 35`).
3. **Strict Typing & Zero Suppression Comments**:
   - `basedpyright` runs in `strict` mode; `ruff` and `test_architecture.py` enforce zero `# noqa`, zero `# type: ignore`, and zero `# pyright: ignore` comments across `cli/src/` and `cli/tests/`.
   - Execution modules (`analyze.py`, `extract.py`, `_ghidra.py`) must always resolve tools via `resolve_toolchain(ctx)` rather than reading `ctx.config.toolchain` directly.
4. **Cross-Platform Path & Archive Handling**:
   - Use `pathlib.Path` everywhere (no hardcoded `/` or `\`, no `shell=True`).
   - Parametrize unit tests across `[HostOS.LINUX, HostOS.WINDOWS]`.
5. **Explicit `WorkspaceContext` & Command Boundaries**:
   - **Never extract context inside step functions**: Step functions (`init.init`, `init.status`, `init.clean`, `analyze.analyze`, `extract.extract`, `query.sql`) never take raw `root: Path`, `decompiler: Decompiler | None`, or `host_os: HostOS | None`, and never parse `twdll.toml` themselves.
   - **`WorkspaceContext` in `domain.py`**: Encapsulates `paths: WorkspacePaths`, `config: WorkspaceConfig`, and `host_os: HostOS` (`WorkspaceContext.from_root(root)`, `ctx.root`, `ctx.decompiler`), using `dataclasses.replace` for CLI/test overrides.
   - **Root Context vs. Action Arguments**: Workspace overrides (`--decompiler`, `--ghidra-dir`, `--java-home`, `--ida-dir`) live **exclusively** on the root `twdll` callback. Step functions take `ctx: WorkspaceContext` + typed `domain.py` parameters and return frozen `@dataclass` results. Both `cli.py` (Typer) and `mcp.py` (`MCPServer` stdio via `twdll mcp`, configured in `.agents/mcp.json`) wrap those exact entrypoints directly.

---

## 5. Domain Model (`domain.py`)

`domain.py` is the single source of truth for all `NewType`s, `StrEnum`s, declarative tables, path builders, and configuration/status/result dataclasses. High-volume SQLite row and core config dataclasses use `@dataclass(frozen=True, slots=True)`; command result/status dataclasses returned by `mcp.py` omit `slots=True` so `mcp` 2.x/Pydantic generates full `outputSchema` definitions automatically.

### 5.1 Domain `NewType`s & `StrEnum`s
- `Address = NewType("Address", int)` *(decompiler-assigned virtual address; paired with per-binary `image_base`)*
- `SteamAppId = NewType("SteamAppId", str)`
- `Decompiler(StrEnum)`: `GHIDRA = "ghidra"`, `IDA = "ida"`
- `BinaryName(StrEnum)`: `ATTILA_LINUX_64 = "attila_linux_64"`, `ATTILA_WINDOWS_32 = "attila_windows_32"`, `ROME2_WINDOWS_32 = "rome2_windows_32"`, `TOB_WINDOWS_32 = "tob_windows_32"`
- `BinaryArch(StrEnum)`: `ELF64 = "elf64"`, `PE32 = "pe32"`
- `HostOS(StrEnum)`: `LINUX = "linux"`, `WINDOWS = "windows"` (`HostOS.current()`)
- `StepName(StrEnum)`: `INIT = "init"`, `ANALYZE = "analyze"`, `EXTRACT = "extract"`
- `CleanTarget(StrEnum)`: `INIT = "init"`, `ANALYZE = "analyze"`, `EXTRACT = "extract"`, `ALL = "all"` (`CleanTarget.steps() -> tuple[StepName, ...]`)
- `WorkspaceName(StrEnum)`: `CONFIG_FILE = "twdll.toml"`, `DATA_DIR = ".twdll"`, `UV_DIR = "uv"`, `TOOLS_DIR = "tools"`, `BINARIES_DIR = "binaries"`
- `FileSuffix(StrEnum)`: `DLL = ".dll"`, `SQLITE = ".sqlite"`, `GHIDRA_GPR = ".gpr"`, `GHIDRA_REP = ".rep"`, `IDA_I64 = ".i64"`, `META_JSON = ".meta.json"`
- `ToolName(StrEnum)`: `GHIDRA = "ghidra"`, `JAVA = "java"`, `IDA = "ida"`
- `ToolSource(StrEnum)`: `TOML = "twdll.toml"`, `PORTABLE = ".twdll/init/tools"`, `DEFAULT_DIR = "default_install_dir"`, `MISSING = "missing"`
- `BinarySource(StrEnum)`: `TOML = "twdll.toml"`, `TOML_INVALID = "twdll.toml (invalid)"`, `STEAM = "steam"`, `STAGED = "staged"`, `MISSING = "missing"`
- `ProgressKind(StrEnum)`: `DOWNLOAD = "download"`, `EXTRACT_ARCHIVE = "extract_archive"`, `STAGE_BINARY = "stage_binary"`, `STEP = "step"`

### 5.2 Declarative Specs, Progress & Workspace Paths
- `STEAM_CONSOLE_URL = "steam://open/console"`
- `BinarySpec(...)`, `BINARY_SPECS: dict[BinaryName, BinarySpec]`, `REFERENCE_BINARIES`, `SELECTABLE_TARGETS`
- `PortableArchiveSpec(...)`, `GHIDRA_PORTABLE_SPEC`, `JDK_PORTABLE_SPECS`
- `ProgressEvent(kind: ProgressKind, task_key: str, label: str, completed: int, total: int | None)`, `ProgressCallback = Callable[[ProgressEvent], None]`, `noop_progress`
- `default_ida_dirs(...)`, `default_steam_roots(...)`, `default_steamapps_subdirs(...)`
- `WorkspacePaths(root: Path)`: deterministic properties/methods built from `WorkspaceName`, `StepName`, and `FileSuffix` (`config_file`, `data_dir`, `uv_dir`, `step_dir`, `init_dir`, `tools_dir`, `binaries_dir`, `staged_binary_path`, `analyze_dir`, `analyze_stem`, `analyze_artifact_path`, `analyze_meta_path`, `ghidra_gpr_path`, `ghidra_rep_dir`, `ida_db_path`, `extract_dir`, `extract_db_path`).

### 5.3 Typed Domain Models
- **Setup & Context (`init`, `status`, `clean`, `WorkspaceContext`)**:
  - `ToolchainConfig`, `WorkspaceConfig` (`normalize_targets`), `active_binaries(config)`, `WorkspaceContext` (`from_root`, `root`, `decompiler`)
  - `ToolLocation`, `ToolchainStatus`, `BinaryStatus`, `StepSummary`, `WorkspaceStatus`, `InitResult`, `CleanResult`
- **Analyze (`analyze`)**:
  - `AnalyzeTargetResult(binary: BinaryName, decompiler: Decompiler, project_path: Path, analyzed: bool, skipped: bool, duration_seconds: float = 0.0)`
  - `AnalyzeResult(decompiler: Decompiler, targets: tuple[AnalyzeTargetResult, ...])`
- **Extract (`extract`) & Query (`sql`)** *(Result models must omit `slots=True` for MCP `outputSchema`; row schemas finalized after Step 3A)*:
  - `ExtractTargetResult(binary: BinaryName, db_path: Path, skipped: bool, duration_seconds: float = 0.0)`
  - `ExtractResult(decompiler: Decompiler, targets: tuple[ExtractTargetResult, ...])`
  - `SqlScalar = int | float | str | None`, `SqlRow = tuple[SqlScalar, ...]`
  - `SqlQueryResult(columns: tuple[str, ...], rows: tuple[SqlRow, ...], truncated: bool)`

---

## 6. Pipeline Steps: Analysis, Extraction, MCP & Query

### 6.1 Step 2 — Decompiler Analysis (`twdll analyze` & `twdll._ghidra`)
- **Shared Ghidra Lifecycle (`twdll._ghidra`)**:
  - Centralizes `_GhidraProgressMonitor`, `_require_ghidra_toolchain`, `pyghidra.start(install_dir=...)` (setting `JAVA_HOME` and `JAVA_HOME_OVERRIDE` so `PATH` is never consulted), `tempfile.TemporaryDirectory()` staging, and Ghidra project open/save helpers.
  - Runs standard auto-analysis sequentially (32-bit `PE32` binaries first, then 64-bit `ELF64`) with all default analyzers enabled, filtering targets via `active_binaries(ctx.config)`.
  - Saves projects flat in `.twdll/analyze/` (`ghidra_<binary>.gpr` and `ghidra_<binary>.rep/`).
- **Direct Skip Checks via `.meta.json`**:
  - `analyze` writes `<decompiler>_<binary>.meta.json` atomically (`os.replace`) after saving the Ghidra project (`decompiler`, `binary`, `binary_sha256`, `analyzed_at`).
  - Subsequent runs verify both `.meta.json` (valid JSON with matching `binary_sha256`) and existence of `.gpr`/`.rep/` before skipping, without starting a JVM or copying `.rep/` directories. Missing, malformed, or stale `.meta.json` triggers clean re-analysis. Full re-analysis uses `twdll clean analyze`.
- **Verified `pyghidra` 2.x / Ghidra 12.2 API Notes**:
  - Use `pyghidra.open_project(dir_path, stem, create=...)` + `pyghidra.program_context(project, f"/{staged_path.name}")` (`open_program()` is deprecated).
  - Ghidra's `ProjectLocator` forbids path elements starting with `.` (such as `.twdll`), so `open_project` operates in a dot-free `tempfile.TemporaryDirectory()` and syncs `.gpr`/`.rep/` to/from `.twdll/analyze/`.
  - `ProgramLoader.Builder.source(...)` requires `str(staged_path)` to avoid ambiguous Java overloads; run analysis via `pyghidra.analyze(program, monitor=...)` and save via `program.save(...)`.
- **IDA 9.5 Backend (Deferred)**:
  - Raises `NotImplementedError` until tested on a host with IDA 9.5 installed.

### 6.2 MCP Subprocess Isolation (`twdll.mcp`)
- `twdll_init`, `twdll_status`, and `twdll_clean` execute in-process against `_refresh_context(ctx)`.
- JVM-bound tools (`twdll_analyze`, `twdll_extract`) spawn a short-lived worker subprocess (`sys.executable -m twdll.cli ... --json`, `cwd=ctx.root`, `stdin=subprocess.DEVNULL`, `stdout=subprocess.PIPE`, `stderr=subprocess.PIPE`) in an isolated process group (`start_new_session=True` on POSIX; process-tree termination on Windows) so timeouts/cancellations terminate child JVMs.
- Only explicit startup CLI overrides (`--decompiler`, `--ghidra-dir`, `--java-home`, `--ida-dir`) are forwarded to workers so `twdll.toml` edits take effect on every MCP call; target filters are passed as repeated `--binary` options, and non-zero exits, timeouts, or non-JSON `stderr` map to clean MCP tool errors.

### 6.3 Step 3 — Fact & DWARF Extraction (`twdll extract` & `database.py`)
`database.py` is the only module allowed to import `sqlite3`.

- **Pre-Implementation Empirical Gate (Step 3A)**:
  - Before locking the `extract` and `dwarf_*` SQLite schemas and join keys, run two empirical checks:
    1. Inspect `attila_linux_64` DWARF v4 DIE attributes and `.symtab` `_ZTV*` vtables via `gimli` + `object` (verified via `twdll-dwarf` across all 469 CUs: `722,704` types, `166,669` vtable entries, `166,593` concrete functions, `2.13M` inlines) and cross-check raw DIE counts with `llvm-dwarfdump` (1.64 GiB peak RSS) to finalize the exact `dwarf_*` tables and cross-binary mapping keys.
    2. Inspect Ghidra 12.2's analyzed project (`ghidra_attila_linux_64.gpr`: `29` sections, `168,849` functions, `613,011` symbols, `55,185` strings, `2.38M` xrefs) and run an **automated DWARF-to-Ghidra decompilation quality audit loop** (scanning SQLite struct layout completeness and `DecompInterface` C++ output across stratified game namespaces at `~108 funcs/s` / `~3.5–4.1 GiB` peak RSS to detect and fix unmapped struct offsets, missing EBO/base fields, raw virtual call pointer arithmetic, and return/parameter storage mismatches case by case) before locking `twdll.extract` in Step 3B.
- **Ordered Extraction Pipeline**:
  1. **DWARF-first step (`attila_linux_64`)**: Extract globally deduplicated DWARF types, pre-flattened struct layouts, synthesized vtable slot tables, functions, parameters, local variable live ranges, and inlined call sites out-of-process via `twdll-dwarf` (`gimli` + `object` + `libsqlite3` FFI) into backend-neutral SQLite tables—usable by both Ghidra (now) and IDA 9.5 (later).
  2. **Decompiler step**: Extract verified structural facts (and C pseudocode if approved by Step 3A, batch-populating DWARF types, `<Class>_vtbl` vtables, and `CUSTOM_STORAGE` function signatures on Ghidra) into `.twdll/extract/<decompiler>_<binary>.sqlite`.
- **Flat Per-Binary Extracted Databases (`.twdll/extract/<decompiler>_<binary>.sqlite`)**:
  - Mirrors `.twdll/analyze/<decompiler>_<binary>.gpr|.i64` 1-to-1, using integer addresses paired with `image_base` and backend-neutral table names.
  - Written to `.sqlite.staging` in `.twdll/extract/`, recording `"completed"` and `binary_sha256` in `meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)` in the final transaction, closing all SQLite connections, and atomically renaming via `os.replace()`.
  - `twdll extract` skips a binary only if its `meta` table confirms `"completed"` and `binary_sha256` matches the current staged binary (unless `--force` is passed), and fails clearly if a target binary has not been analyzed.

### 6.4 Step 3C — Safe Read-Only SQL (`twdll sql`)
- `twdll.query.sql(ctx, query, limit=200, timeout_ms=5000)` validates `limit > 0` and `timeout_ms > 0` and delegates to `twdll.database`.
- `twdll.database` opens `sqlite3.connect(":memory:", uri=True)` with `PRAGMA query_only = ON` and attaches all completed `.twdll/extract/<decompiler>_<binary>.sqlite` files for `ctx.decompiler` via `f"{db_path.resolve().as_uri()}?mode=ro"` under `<binary>` aliases (failing fast with an actionable error if zero completed databases exist).
- After attaching databases, `twdll.database` installs an allowlist `set_authorizer` (permitting only `SQLITE_SELECT`, `SQLITE_READ`, `SQLITE_FUNCTION`, `SQLITE_RECURSIVE`, blocking `PRAGMA`, `ATTACH`, `DETACH`, and all writes/DDL), installs `set_progress_handler(..., 1000)` for `timeout_ms` (mapping `OperationalError: interrupted` to a timeout error), caps returned rows with `truncated: bool`, and hex-serializes `bytes` values.

---

## 7. Closed CLI Command & Flag Surface

Root `twdll` callback builds `WorkspaceContext` from `Path.cwd()` (`twdll.toml`) with optional root-level context overrides:
`twdll [--decompiler ghidra|ida] [--ghidra-dir PATH] [--java-home PATH] [--ida-dir PATH] <command> ...`

Data and action subcommands receive `ctx: WorkspaceContext` and support `--json` for structured JSON output (`twdll mcp` runs stdio JSON-RPC and has no `--json` flag).

| Group | Command | Arguments & Options | Entrypoint | Output / Side Effect |
| :--- | :--- | :--- | :--- | :--- |
| **Setup** | `twdll init` | `[--no-toolchain] [--no-binaries] [--json]` | `init.init` | Creates/updates `twdll.toml` (preserving existing overrides unless run interactively), provisions `.twdll/init/tools/`, stages discovered binaries flat into `.twdll/init/binaries/` |
| **Setup** | `twdll status` | `[--json]` | `init.status` | Read-only inspection of `twdll.toml`, toolchain, staged/discovered binaries, and `.twdll/<step>/` directories |
| **Setup** | `twdll clean` | `<init\|analyze\|extract\|all> [--json]` | `init.clean` | Removes the corresponding `.twdll/<step>/` directory (`shutil.rmtree`); preserves `.twdll/uv/` and `twdll.toml` |
| **MCP** | `twdll mcp` | *(none; uses root `WorkspaceContext`)* | `mcp.serve_mcp` | Runs the stdio MCP server (`.agents/mcp.json`); isolates JVM tools (`twdll_analyze`, `twdll_extract`) in worker subprocesses |
| **Analyze** | `twdll analyze` | `[binaries...] [--json]` | `analyze.analyze` | Runs decompiler analysis on selected (or active) staged binary(ies) into `.twdll/analyze/<decompiler>_<binary>.gpr|.i64` + `.meta.json` |
| **Extract** | `twdll extract` | `[--binary NAME]... [--force] [--json]` | `extract.extract` | Extracts verified facts from `.twdll/analyze/<decompiler>_<binary>.gpr|.i64` into `.twdll/extract/<decompiler>_<binary>.sqlite` |
| **Query** | `twdll sql` | `<query> [--limit N] [--timeout-ms MS] [--json]` | `query.sql` | Executes a read-only SQL query against attached completed extracted SQLite databases |

---

## 8. Incremental Implementation Plan & Quality Gate

Each step is implemented and reviewed incrementally (announcing the exact file(s) before writing and waiting for user approval) and must pass all 5 verification commands with zero errors and zero warnings:

```bash
uv run ruff format --check
uv run ruff check
uv run basedpyright
uv run lint-imports
uv run pytest
```

1. **Slice 1 — Foundation & Setup (`init`, `status`, `clean`, `mcp`)** *(Completed)*
2. **Slice 2 — Decompiler Analysis (`analyze`, `twdll._ghidra`, MCP Subprocess Isolation)** *(Completed + Rework)*
3. **Slice 3 — Extraction & Read-Only SQL (`extract`, `sql`)**:
   - **Step 3A (Empirical Gate)**: Complete `attila_linux_64` Ghidra analysis (`ghidra_attila_linux_64.gpr`, completed) and run the automated DWARF-to-Ghidra decompilation quality audit loop + `DecompInterface` benchmark against `attila_linux_64` (and `attila_windows_32`) before locking the decompiler-extracted schema.
   - **Step 3B (`extract` + `database.py`)**: Implement verified per-binary SQLite schema, `.sqlite.staging` atomic write, `twdll.extract`, `twdll extract`, and `twdll_extract`.
   - **Step 3C (`sql` + `query.py` + `database.py`)**: Implement `:memory:` `uri=True` read-only attachment, allowlist `set_authorizer`, `set_progress_handler`, `twdll.query.sql`, `twdll sql`, and `twdll_sql`.
