# `twdll` Reverse-Engineering CLI Specification

## 1. Core Architectural Principles

1. **Conceptual Alignment**: Folder names, module names, CLI commands, and entrypoint function names must mirror the problem domain 1-to-1:
   - Data flows strictly in one direction: **`init` -> `analyze` -> `extract` -> `derive` -> `query` (`search`, `show`, `sql`)**.
   - Every pipeline step owns its matching `.twdll/<step>/` directory and its flat `cli/src/twdll/<step>.py` module:

   | CLI Command | Step Directory | Python Module | Entrypoint Function | Return Type (`domain.py`) |
   | :--- | :--- | :--- | :--- | :--- |
   | `twdll init` | `.twdll/init/` | `twdll/init.py` | `init.init(...)` | `InitResult` |
   | `twdll status` | *(reads `.twdll/*`)* | `twdll/init.py` | `init.status(...)` | `WorkspaceStatus` |
   | `twdll clean` | `.twdll/<target>/` | `twdll/init.py` | `init.clean(...)` | `CleanResult` |
   | `twdll analyze` | `.twdll/analyze/` | `twdll/analyze.py` | `analyze.analyze(...)` | `AnalyzeResult` |
   | `twdll extract` | `.twdll/extract/` | `twdll/extract.py` | `extract.extract(...)` | `ExtractResult` |
   | `twdll derive` | `.twdll/derive/` | `twdll/derive.py` | `derive.derive(...)` | `DeriveResult` |
   | `twdll search` | *(reads `extract`/`derive`)* | `twdll/query.py` | `query.search(...)` | `tuple[SearchHit, ...]` |
   | `twdll show` | *(reads `extract`/`derive`)* | `twdll/query.py` | `query.show(...)` | `FunctionDetail` |
   | `twdll sql` | *(reads `extract`/`derive`)* | `twdll/query.py` | `query.sql(...)` | `SqlQueryResult` |

2. **Explicit Intent & Single-Point Path/Naming Constants (No Primitive Obsession)**:
   - Never use raw `str` or untyped `int` where a domain concept exists.
   - Represent all finite domain states, directory names, and file extensions with `StrEnum` (`Decompiler`, `BinaryName`, `BinaryArch`, `HostOS`, `StepName`, `CleanTarget`, `WorkspaceName`, `FileSuffix`, `ToolName`, `ToolSource`, `BinarySource`, `SymbolKind`, `XrefKind`, `StringEncoding`, `MatchMethod`, `SearchKind`, `DatabaseTarget`, `ProgressKind`).
   - Represent virtual memory addresses and external identifiers with `typing.NewType` (`Address = NewType("Address", int)`, `SteamAppId = NewType("SteamAppId", str)`).
   - Represent all filesystem locations in domain models with `pathlib.Path` (`Path | None` when optional—never `""` sentinel strings). Conversion to/from strings happens exclusively at serialization boundaries (TOML, JSON, SQLite).
   - **Single-Point Path & Naming Rule**: Outside `domain.py`, no step module, CLI command, or test may hardcode workspace directory names (`".twdll"`, `"binaries"`, `"tools"`), target names (`"attila_linux_64"`), or artifact suffixes (`".sqlite"`, `".gpr"`, `".i64"`). Every module and test must reference `domain.py` enums and `WorkspacePaths` so any future naming change is a single-point edit in `domain.py`.

3. **The Data Quality Chain (`analyze` -> `extract` -> `derive` -> `query`)**:
   - **`analyze` discovers the truth inside the binary (if `analyze` misses it, every later step is blind)**:
     - A game binary on disk is just raw bytes. `extract` cannot find functions, strings, or call links by itself—it only copies what `analyze` already discovered inside the decompiler project.
     - Therefore, **`analyze` must always prioritize complete, accurate discovery over speed shortcuts**: every function boundary (including switch jump-tables), every cross-reference (`xref`: who calls which function, who uses which string, who points to which vtable or global variable), and every stack frame, calling convention, and debug symbol/type.
     - **Rule**: Never disable decompiler analyzers or add "speed workarounds" in `analyze` (such as turning off constant reference, switch table, or stack analyzers) that lose functions, xrefs, strings, types, or decompiled C quality downstream.
   - **`extract` exports 100% verified facts into SQLite (no guessing, no unverified columns)**:
     - `extract` opens the finished decompiler project (and in future iterations, dedicated lossless binary/DWARF parsers like `ddbug` / `gimli`) and writes its findings to SQLite (`.twdll/extract/`).
     - **Rule**: Every table and column in `extract` must be empirically verified 1-by-1 against the real decompiler API (`pyghidra` / IDAPython) before implementation so we only store genuine facts the tool natively provides, never invented or approximated columns.
   - **`derive` connects the dots across games**:
     - Uses the strings, callgraphs, and vtables from `extract` to match unnamed 32-bit Windows functions (`rome2_windows_32`, `attila_windows_32`, `tob_windows_32`) to the named 64-bit Linux reference functions (`attila_linux_64`). The higher the fidelity of `analyze` and `extract`, the more functions `derive` can automatically name and match.
   - **`query` (`search`, `show`, `sql`) gives the modder & AI agent the complete picture**:
     - Lets a modder or AI agent look up any function, struct, vtable, string, disassembly, or decompiled C code to build C++ hooks without opening Ghidra or IDA.

---

## 2. Workspace & Flat 1-to-1 Directory Layout

All local state lives in `twdll.toml` and `.twdll/` at the workspace root. Every pipeline step owns its matching `.twdll/<step>/` directory with flat, uniformly named files (no per-binary or per-decompiler subdirectories in `binaries/`, `analyze/`, or `extract/`):

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
    │   ├── ghidra_attila_linux_64.gpr (+ ghidra_attila_linux_64.rep/)
    │   ├── ghidra_attila_windows_32.gpr (+ ghidra_attila_windows_32.rep/)
    │   ├── ida_attila_linux_64.i64
    │   └── ida_attila_windows_32.i64
    ├── extract/                # Step 3 (`twdll extract` / `twdll clean extract`)
    │   ├── ghidra_attila_linux_64.sqlite
    │   ├── ghidra_attila_windows_32.sqlite
    │   ├── ida_attila_linux_64.sqlite
    │   └── ida_attila_windows_32.sqlite
    └── derive/                 # Step 4 (`twdll derive` / `twdll clean derive`)
        ├── ghidra.sqlite       # Derived callgraphs, vtables, and cross-binary matches
        └── ida.sqlite          # Derived callgraphs, vtables, and cross-binary matches
```

- **Windows safety**: `.twdll/uv/` is separate from `.twdll/init/` so `twdll clean init` or `twdll clean all` never attempts to delete the running `python.exe` inside `.twdll/uv/venv/`.
- **`twdll.toml` format**:
  ```toml
  # Active decompiler: "ghidra" or "ida"
  decompiler = "ghidra"

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

All CLI code lives under `cli/src/twdll/` with a flat 1-to-1 module layout (no nested `steps/` subpackage and no separate `workspace.py`):

```text
cli/
├── pyproject.toml
├── src/
│   └── twdll/
│       ├── __init__.py
│       ├── py.typed
│       ├── domain.py           # Layer 0: All enums, NewTypes, constants, paths, and dataclasses (stdlib-only)
│       ├── database.py         # Layer 1: All sqlite3 schemas, writers, and queries (stdlib-only)
│       ├── init.py             # Layer 2: Config, toolchain, Steam discovery, staging, status, clean
│       ├── analyze.py          # Layer 2: Headless Ghidra / IDA project analysis
│       ├── extract.py          # Layer 2: Base fact extraction into .twdll/extract/<decompiler>.sqlite
│       ├── derive.py           # Layer 2: Pure-Python derivations into .twdll/derive/<decompiler>.sqlite
│       ├── query.py            # Layer 2: Read-only search, show, and sql queries
│       └── cli.py              # Layer 3: Typer CLI commands and output formatting
└── tests/
    ├── test_init.py
    ├── test_analyze.py
    ├── test_extract.py
    ├── test_derive.py
    └── test_query.py
```

### `import-linter` Architectural Contracts (`cli/pyproject.toml`)
1. `include_external_packages = true` at `[tool.importlinter]` root.
2. **Strict one-way layers**:
   - `twdll.cli`
   - `twdll.init`, `twdll.analyze`, `twdll.extract`, `twdll.derive`, `twdll.query` (independent sibling step modules)
   - `twdll.database`
   - `twdll.domain`
3. **Centralized SQLite**: Only `twdll.database` may import `sqlite3`.
4. **CLI separation**: Only `twdll.cli` may import `typer`.
5. **Stdlib-only core**: `twdll.domain` and `twdll.database` may only import Python standard library modules so PyGhidra and IDAPython scripts can import them directly.

---

## 4. LLM-Friendly Code Rules & Quality Guardrails

1. **No Deep `if / else` Chains — Use `match / case` & Declarative Tables**:
   - Store per-enum configuration in declarative lookup tables in `domain.py` (`BINARY_SPECS`, `GHIDRA_PORTABLE_SPEC`, `JDK_PORTABLE_SPECS`).
   - Use Python 3.12 `match / case` when dispatching on `StrEnum` (`Decompiler`, `BinaryName`, `BinaryArch`, `HostOS`, `StepName`, `CleanTarget`, `ToolName`, `DatabaseTarget`, `SearchKind`). Omit wildcard `case _:` on enum matches so `basedpyright` (strict) enforces compile-time exhaustiveness.
   - Use flat early-return guard clauses (`if not path.is_file(): return False`) for simple precondition checks.
2. **Small, Single-Purpose Functions (Enforced by Ruff)**:
   - Separate pure data parsing/resolution from disk/network I/O side effects.
   - Never use boolean mode flags that mix read-only inspection with disk mutation.
   - Never drill 6–9 mock parameters through top-level functions; keep helper functions small so unit tests call them directly on `tmp_path`.
   - Ruff lint rules in `cli/pyproject.toml` enforce function size mechanically:
     - `C901`: `max-complexity = 8`
     - `PLR0912`: `max-branches = 8`
     - `PLR0913`: `max-args = 5`
     - `PLR0915`: `max-statements = 30`
3. **Strict Typing & Zero Suppression Comments**:
   - `basedpyright` runs in `strict` mode with `venvPath = "../.twdll/uv"` and `venv = "venv"`.
   - Zero `# noqa` and zero `# type: ignore` anywhere in the codebase. For Windows-only `winreg`, use `importlib.import_module("winreg")` inside a helper returning `ModuleType | None`.
4. **Cross-Platform Path & Archive Handling**:
   - Use `pathlib.Path` everywhere (no hardcoded `/` or `\`, no `shell=True`).
   - Pass `filter="data"` when extracting tar archives so Python 3.13/3.14 emits zero deprecation warnings.
   - Parametrize unit tests across `[HostOS.LINUX, HostOS.WINDOWS]`.

---

## 5. Domain Model (`domain.py`)

`domain.py` is the single source of truth for all `NewType`s, `StrEnum`s, declarative tables, path builders, configuration/status models, and SQLite row dataclasses. All dataclasses use `@dataclass(frozen=True, slots=True)`.

### 5.1 Domain `NewType`s & `StrEnum`s
- `Address = NewType("Address", int)`
- `SteamAppId = NewType("SteamAppId", str)`
- `Decompiler(StrEnum)`: `GHIDRA = "ghidra"`, `IDA = "ida"`
- `BinaryName(StrEnum)`: `ATTILA_LINUX_64 = "attila_linux_64"`, `ATTILA_WINDOWS_32 = "attila_windows_32"`, `ROME2_WINDOWS_32 = "rome2_windows_32"`, `TOB_WINDOWS_32 = "tob_windows_32"`
- `BinaryArch(StrEnum)`: `ELF64 = "elf64"`, `PE32 = "pe32"`
- `HostOS(StrEnum)`: `LINUX = "linux"`, `WINDOWS = "windows"` (`HostOS.current()`)
- `StepName(StrEnum)`: `INIT = "init"`, `ANALYZE = "analyze"`, `EXTRACT = "extract"`, `DERIVE = "derive"`
- `CleanTarget(StrEnum)`: `INIT = "init"`, `ANALYZE = "analyze"`, `EXTRACT = "extract"`, `DERIVE = "derive"`, `ALL = "all"` (`CleanTarget.steps() -> tuple[StepName, ...]`)
- `WorkspaceName(StrEnum)`: `CONFIG_FILE = "twdll.toml"`, `DATA_DIR = ".twdll"`, `UV_DIR = "uv"`, `TOOLS_DIR = "tools"`, `BINARIES_DIR = "binaries"`
- `FileSuffix(StrEnum)`: `DLL = ".dll"`, `SQLITE = ".sqlite"`, `GHIDRA_GPR = ".gpr"`, `GHIDRA_REP = ".rep"`, `IDA_I64 = ".i64"`
- `ToolName(StrEnum)`: `GHIDRA = "ghidra"`, `JAVA = "java"`, `IDA = "ida"`
- `ToolSource(StrEnum)`: `TOML = "twdll.toml"`, `PORTABLE = ".twdll/init/tools"`, `DEFAULT_DIR = "default_install_dir"`, `MISSING = "missing"`
- `BinarySource(StrEnum)`: `TOML = "twdll.toml"`, `TOML_INVALID = "twdll.toml (invalid)"`, `STEAM = "steam"`, `STAGED = "staged"`, `MISSING = "missing"`
- `SymbolKind(StrEnum)`: `FUNCTION = "function"`, `DATA = "data"`, `IMPORT = "import"`, `EXPORT = "export"`, `LABEL = "label"`
- `XrefKind(StrEnum)`: `CALL = "call"`, `DATA_READ = "data_read"`, `DATA_WRITE = "data_write"`, `OFFSET = "offset"`
- `StringEncoding(StrEnum)`: `ASCII = "ascii"`, `UTF8 = "utf8"`, `UTF16_LE = "utf16_le"`
- `MatchMethod(StrEnum)`: `EXACT_SYMBOL = "exact_symbol"`, `UNIQUE_STRING = "unique_string"`, `CALLGRAPH_NEIGHBOR = "callgraph_neighbor"`, `VTABLE_SLOT = "vtable_slot"`
- `SearchKind(StrEnum)`: `FUNCTION = "function"`, `SYMBOL = "symbol"`, `STRING = "string"`, `VTABLE = "vtable"`
- `DatabaseTarget(StrEnum)`: `EXTRACT = "extract"`, `DERIVE = "derive"`
- `ProgressKind(StrEnum)`: `DOWNLOAD = "download"`, `EXTRACT_ARCHIVE = "extract_archive"`, `STAGE_BINARY = "stage_binary"`, `STEP = "step"`

### 5.2 Declarative Specs & Workspace Paths
- `BinarySpec(name: BinaryName, steam_app_id: SteamAppId, default_install_dir: str, file_name: str, staged_file_name: str, arch: BinaryArch)`
- `BINARY_SPECS: dict[BinaryName, BinarySpec]`
- `PortableArchiveSpec(tool: ToolName, url: str, archive_name: str, extracted_prefix: str)`
- `GHIDRA_PORTABLE_SPEC: PortableArchiveSpec`
- `JDK_PORTABLE_SPECS: dict[HostOS, PortableArchiveSpec]`
- `default_ida_dirs(host_os: HostOS, home_dir: Path) -> tuple[Path, ...]`
- `default_steam_roots(host_os: HostOS, home_dir: Path) -> tuple[Path, ...]`
- `WorkspacePaths(root: Path)`: deterministic properties/methods built strictly from `WorkspaceName`, `StepName`, and `FileSuffix` for `config_file`, `data_dir`, `uv_dir`, `step_dir(step)`, `init_dir`, `tools_dir`, `binaries_dir`, `staged_binary_path(binary)`, `analyze_dir`, `analyze_stem(decompiler, binary)`, `ghidra_gpr_path(binary)`, `ghidra_rep_dir(binary)`, `ida_db_path(binary)`, `extract_dir`, `extract_db_path(decompiler, binary)`, `derive_dir`, `derive_db_path(decompiler)`.

### 5.3 Typed Domain Models & SQLite Row Dataclasses
- **Setup (`init`, `status`, `clean`)**:
  - `ToolchainConfig(ghidra_dir: Path | None = None, java_home: Path | None = None, ida_dir: Path | None = None)`
  - `WorkspaceConfig(decompiler: Decompiler = Decompiler.GHIDRA, binaries: dict[BinaryName, Path | None], toolchain: ToolchainConfig)`
  - `ToolLocation(name: ToolName, ready: bool, path: Path | None, source: ToolSource)`
  - `ToolchainStatus(decompiler: Decompiler, ready: bool, tools: tuple[ToolLocation, ...])`
  - `BinaryStatus(name: BinaryName, arch: BinaryArch, staged: bool, staged_path: Path | None, discovered_path: Path | None, source: BinarySource, valid_header: bool)`
  - `StepSummary(step: StepName, directory: Path, exists: bool, entries: tuple[str, ...])`
  - `WorkspaceStatus(root: Path, config_file: Path, config_exists: bool, host_os: HostOS, decompiler: Decompiler, toolchain: ToolchainStatus, binaries: tuple[BinaryStatus, ...], steps: tuple[StepSummary, ...])`
  - `InitResult(config_file: Path, config_created: bool, decompiler: Decompiler, toolchain: ToolchainStatus, binaries: tuple[BinaryStatus, ...])`
  - `CleanResult(target: CleanTarget, removed_directories: tuple[Path, ...])`
- **Analyze (`analyze`)**:
  - `AnalyzeTargetResult(binary: BinaryName, decompiler: Decompiler, project_dir: Path, analyzed: bool, skipped: bool)`
  - `AnalyzeResult(decompiler: Decompiler, targets: tuple[AnalyzeTargetResult, ...])`
- **Extract (`extract`) — SQLite Row Dataclasses (`.twdll/extract/<decompiler>.sqlite`)**:
  - `ExtractedBinaryRow(binary: BinaryName, arch: BinaryArch, decompiler: Decompiler, image_base: Address, analyzed_at: str)`
  - `FunctionRow(binary: BinaryName, address: Address, name: str, demangled_name: str | None, size: int, signature: str | None, is_thunk: bool)`
  - `SymbolRow(binary: BinaryName, address: Address, name: str, demangled_name: str | None, kind: SymbolKind)`
  - `StringRow(binary: BinaryName, address: Address, value: str, encoding: StringEncoding)`
  - `XrefRow(binary: BinaryName, from_address: Address, to_address: Address, kind: XrefKind)`
  - `ExtractTargetResult(binary: BinaryName, functions_count: int, symbols_count: int, strings_count: int, xrefs_count: int)`
  - `ExtractResult(decompiler: Decompiler, db_path: Path, targets: tuple[ExtractTargetResult, ...])`
- **Derive (`derive`) — SQLite Row Dataclasses (`.twdll/derive/<decompiler>.sqlite`)**:
  - `CallEdgeRow(binary: BinaryName, caller_address: Address, callee_address: Address)`
  - `VtableRow(binary: BinaryName, address: Address, class_name: str, entry_count: int)`
  - `VtableEntryRow(binary: BinaryName, vtable_address: Address, slot_index: int, target_address: Address)`
  - `FunctionMatchRow(source_binary: BinaryName, source_address: Address, target_binary: BinaryName, target_address: Address, confidence: float, method: MatchMethod)`
  - `DeriveResult(decompiler: Decompiler, db_path: Path, call_edges_count: int, vtables_count: int, matches_count: int)`
- **Query (`search`, `show`, `sql`)**:
  - `SearchHit(binary: BinaryName, kind: SearchKind, address: Address, name: str, detail: str | None)`
  - `FunctionDetail(function: FunctionRow, callers: tuple[Address, ...], callees: tuple[Address, ...], strings: tuple[StringRow, ...], matches: tuple[FunctionMatchRow, ...])`
  - `SqlQueryResult(columns: tuple[str, ...], rows: tuple[tuple[int | float | str | bytes | None, ...], ...])`
- **Boundary JSON Serialization (`to_dict(obj) -> dict[str, object]`)**:
  - A single pure serialization helper in `domain.py` converts dataclasses containing `Path`, `StrEnum`, and `NewType` values into JSON-serializable primitives (`Path -> str`, `StrEnum -> str`) at the CLI output boundary.

---

## 6. Decompiler Analysis (`analyze.py`) & Extraction (`extract.py` / `database.py`)

### 6.1 Step 2 — Decompiler Analysis (`twdll analyze`)
- **Single-Process Ghidra Execution via `pyghidra`**:
  - Drives Ghidra 12.2 directly in-process from Python via `pyghidra` (setting `JAVA_HOME` to the resolved JDK 25 directory) without spawning `analyzeHeadless` (`.sh` / `.bat`), external `.java` pre-scripts, or multi-worker subprocess schedulers.
  - Runs Ghidra's standard auto-analysis sequentially (one binary at a time) with full default code, xref, switch-table, stack, and DWARF analyzers enabled—no analyzer-disabling speed workarounds that drop xrefs or control flow.
  - Saves projects flat in `.twdll/analyze/` (`ghidra_<binary>.gpr` and `ghidra_<binary>.rep/`).
- **Skip & Clean Behavior**:
  - Skips a target if its completed Ghidra project (`ghidra_<binary>.gpr` and `ghidra_<binary>.rep/`) already exists; re-analysis from scratch uses `twdll clean analyze`.
- **IDA 9.5 Backend (Deferred — Research Findings Recorded)**:
  - Deferred until tested on a host with IDA 9.5 installed.
  - When implemented, evaluate IDA 9.x `idalib` (`import idapro` from `<ida_dir>/idalib/python/`) first to determine whether in-process Python execution is cleaner than `idat` batch subprocesses.
  - Preserve empirical IDA 9.5 DWARF/PE32 rules from commit `9049ef9`:
    1. On `attila_linux_64`, clear `AF_STKARG | AF_REGARG` in `INF_AF` (`(get_inf_attr(INF_AF) & ~(AF_STKARG | AF_REGARG)) | AF_DODATA | AF_FINAL`) to avoid the IDA 9.5 type-propagation hang across 168,382 DWARF functions (keep `AF_STKARG | AF_REGARG` enabled on 32-bit stripped PE targets).
    2. Pass `-Opdb:off` on stripped 32-bit PE targets (`rome2_windows_32`, `attila_windows_32`, `tob_windows_32`) to avoid symbol-server stalls.
    3. Never pass `-Odwarf:lazy_types=1` (degrades forward-declared C++ structs into `gap0` byte arrays) and never clear `AF2_DOEH` (fragments `.eh_frame` functions).

### 6.2 Step 3 — Fact Extraction (`twdll extract` & `database.py`)
`database.py` is the only module allowed to import `sqlite3`. It translates between SQLite storage primitives (`TEXT`, `INTEGER`, `REAL`) and the typed `domain.py` row dataclasses.

- **Flat Per-Binary Extracted Databases (`.twdll/extract/<decompiler>_<binary>.sqlite`)**:
  - Mirrors `.twdll/analyze/<decompiler>_<binary>.gpr|.i64` 1-to-1 in a flat directory.
- **Completion Proof in SQLite (`meta` Table)**:
  - Mere file existence does not prove extraction completed. Each `.twdll/extract/<decompiler>_<binary>.sqlite` includes a `meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)` table where the completion marker and the staged binary's `binary_sha256` are written **only in the final commit** after all extraction tables finish writing.
  - `twdll extract` skips a binary only if its SQLite `meta` table confirms completion for the current binary's SHA-256 (re-extraction from scratch uses `twdll clean extract`).
- **Pre-Implementation Column Verification Gate**:
  - Before locking or implementing the final SQLite schema for `.twdll/extract/<decompiler>_<binary>.sqlite`, run an empirical `pyghidra` verification task against Ghidra 12.2 to verify every candidate table and column 1-by-1 against the real decompiler API (confirming exact API method, return type, semantics, and column name without workarounds).
  - Baseline tables (`functions`, `symbols`, `strings`, `xrefs`, plus verified disassembly, decompiled C, and variables) are expanded strictly from verified decompiler outputs.
- **Future Dedicated Lossless DWARF Extraction (`ddbug` / `gimli`)**:
  - Decompiler DWARF importers (`Ghidra` and `IDA`) force C++ DWARF DIE trees into C-centric internal type systems and can drop or flatten templates, virtual/multiple inheritance, inlined functions (`DW_TAG_inlined_subroutine`), and complex variable locations.
  - Keep `.twdll/extract/<decompiler>_<binary>.sqlite` focused strictly on decompiler outputs, reserving an orthogonal decompiler-agnostic slot under `.twdll/extract/` for a dedicated lossless DWARF parser (such as `ddbug` / `gimli`) in a future iteration.

### 6.3 Step 4 — Derived Database Schema (`.twdll/derive/<decompiler>.sqlite`, Deferred Until `analyze` & `extract` Complete)
- `call_edges (binary TEXT NOT NULL, caller_address INTEGER NOT NULL, callee_address INTEGER NOT NULL, PRIMARY KEY (binary, caller_address, callee_address))`
- `vtables (binary TEXT NOT NULL, address INTEGER NOT NULL, class_name TEXT NOT NULL, entry_count INTEGER NOT NULL, PRIMARY KEY (binary, address))`
- `vtable_entries (binary TEXT NOT NULL, vtable_address INTEGER NOT NULL, slot_index INTEGER NOT NULL, target_address INTEGER NOT NULL, PRIMARY KEY (binary, vtable_address, slot_index))`
- `function_matches (source_binary TEXT NOT NULL, source_address INTEGER NOT NULL, target_binary TEXT NOT NULL, target_address INTEGER NOT NULL, confidence REAL NOT NULL, method TEXT NOT NULL, PRIMARY KEY (source_binary, source_address, target_binary))`

### 6.4 Read-Only Enforcement for `twdll sql`
- Arbitrary user queries executed via `twdll sql` open the target SQLite file using read-only URI mode (`file:<path>?mode=ro` with `uri=True`) so `twdll sql` can never mutate extracted or derived databases.

---

## 7. Closed CLI Command & Flag Surface

Every command runs against the current working directory (`Path.cwd()`) and supports `--json` for deterministic machine-readable output (re-running any step from scratch uses `twdll clean <step>`):

| Group | Command | Arguments & Options | Entrypoint | Output / Side Effect |
| :--- | :--- | :--- | :--- | :--- |
| **Setup** | `twdll init` | `[--decompiler ghidra\|ida] [--no-toolchain] [--no-binaries] [--json]` | `init.init` | Creates/updates `twdll.toml`, provisions `.twdll/init/tools/`, stages discovered binaries flat into `.twdll/init/binaries/` |
| **Setup** | `twdll status` | `[--json]` | `init.status` | Read-only inspection of `twdll.toml`, toolchain, staged/discovered binaries, and `.twdll/<step>/` directories |
| **Setup** | `twdll clean` | `<init\|analyze\|extract\|derive\|all> [--json]` | `init.clean` | Removes the corresponding `.twdll/<step>/` directory (`shutil.rmtree`); preserves `.twdll/uv/` and `twdll.toml` |
| **Analyze** | `twdll analyze` | `[binary] [--decompiler ghidra\|ida] [--json]` | `analyze.analyze` | Runs single-process `pyghidra` decompiler analysis on staged binary(ies) into `.twdll/analyze/<decompiler>_<binary>.gpr` |
| **Extract** | `twdll extract` | `[binary] [--decompiler ghidra\|ida] [--json]` | `extract.extract` | Extracts verified facts from `.twdll/analyze/<decompiler>_<binary>.gpr` into `.twdll/extract/<decompiler>_<binary>.sqlite` with `meta` completion + SHA-256 proof |
| **Derive** | `twdll derive` | `[--decompiler ghidra\|ida] [--json]` | `derive.derive` | Computes callgraphs, vtables, and cross-binary function matches into `.twdll/derive/<decompiler>.sqlite` |
| **Query** | `twdll search` | `<pattern> [--binary <name>] [--kind function\|symbol\|string\|vtable] [--decompiler ghidra\|ida] [--limit N] [--json]` | `query.search` | Pattern search across functions, symbols, strings, and vtables |
| **Query** | `twdll show` | `<binary> <identifier> [--decompiler ghidra\|ida] [--json]` | `query.show` | Detailed view of a function/symbol by hex address or name (signature, callers, callees, strings, cross-binary matches) |
| **Query** | `twdll sql` | `<query> [--db extract\|derive] [--decompiler ghidra\|ida] [--json]` | `query.sql` | Executes a read-only SQL query (`?mode=ro`) against extracted or derived SQLite databases |

---

## 8. Incremental Implementation Plan & Quality Gate

Each slice is implemented and reviewed step-by-step (announcing the exact file(s) before writing and waiting for user approval) and must pass all 5 verification commands with zero errors and zero warnings:

```bash
ruff format --check cli
ruff check cli
basedpyright -p cli
lint-imports cli/pyproject.toml
pytest cli/tests
```

1. **Slice 1 — Foundation & Setup (`init`, `status`, `clean`)** *(Completed)*:
   - `cli/pyproject.toml`, `cli/src/twdll/domain.py`, `cli/src/twdll/database.py`, `cli/src/twdll/init.py`, `cli/src/twdll/cli.py`, `cli/tests/test_init.py`.
2. **Slice 2 — Decompiler Analysis (`analyze`, Ghidra via `pyghidra`)**:
   - `cli/src/twdll/analyze.py`, `cli/src/twdll/cli.py` (`analyze` command), `cli/tests/test_analyze.py`.
3. **Slice 3 — Base Fact Extraction (`extract`, Ghidra via `pyghidra`)**:
   - Step 3A (Verification Task): Empirically verify candidate `extract` tables and columns 1-by-1 against Ghidra 12.2 `pyghidra` APIs.
   - Step 3B (Implementation): `cli/src/twdll/domain.py`, `cli/src/twdll/database.py`, `cli/src/twdll/extract.py`, `cli/src/twdll/cli.py` (`extract` command), `cli/tests/test_extract.py`.
4. **Slice 4 — Derivations (`derive`)** *(Deferred until Slices 2 & 3 complete)*:
   - `cli/src/twdll/derive.py`, `cli/src/twdll/cli.py` (`derive` command), `cli/tests/test_derive.py`.
5. **Slice 5 — Query Commands (`search`, `show`, `sql`)** *(Deferred until Slices 2 & 3 complete)*:
   - `cli/src/twdll/query.py`, `cli/src/twdll/cli.py` (`search`, `show`, `sql` commands), `cli/tests/test_query.py`.
