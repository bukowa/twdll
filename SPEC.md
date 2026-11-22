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

2. **Explicit Intent (No Primitive Obsession)**:
   - Never use raw `str` or untyped `int` where a domain concept exists.
   - Represent all finite domain states and classifications with `StrEnum` (`Decompiler`, `BinaryName`, `BinaryArch`, `HostOS`, `StepName`, `CleanTarget`, `ToolName`, `ToolSource`, `BinarySource`, `SymbolKind`, `XrefKind`, `StringEncoding`, `MatchMethod`, `SearchKind`, `DatabaseTarget`).
   - Represent virtual memory addresses and external identifiers with `typing.NewType` (`Address = NewType("Address", int)`, `SteamAppId = NewType("SteamAppId", str)`).
   - Represent all filesystem locations in domain models with `pathlib.Path` (`Path | None` when optional—never `""` sentinel strings). Conversion to/from strings happens exclusively at serialization boundaries (TOML, JSON, SQLite).

---

## 2. Workspace & 1-to-1 Directory Layout

All local state lives in `twdll.toml` and `.twdll/` at the workspace root. Every pipeline step owns its matching `.twdll/<step>/` directory so `twdll clean <step>` is a direct `shutil.rmtree(.twdll/<step>)`:

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
    │   └── binaries/           # Header-verified staged game binaries
    │       ├── attila_64/Attila
    │       ├── attila_32/empire.retail.dll
    │       ├── rome2_32/empire.retail.dll
    │       └── tob_32/empire.retail.dll
    ├── analyze/                # Step 2 (`twdll analyze` / `twdll clean analyze`)
    │   ├── ghidra/<binary>/    # Native Ghidra headless project (.gpr + .rep)
    │   └── ida/<binary>/       # Native IDA 9.5 database (.i64)
    ├── extract/                # Step 3 (`twdll extract` / `twdll clean extract`)
    │   ├── ghidra.sqlite       # Base facts extracted from Ghidra projects
    │   └── ida.sqlite          # Base facts extracted from IDA databases
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
  attila_64 = ""
  attila_32 = ""
  rome2_32 = ""
  tob_32 = ""

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
- `BinaryName(StrEnum)`: `ATTILA_64 = "attila_64"`, `ATTILA_32 = "attila_32"`, `ROME2_32 = "rome2_32"`, `TOB_32 = "tob_32"`
- `BinaryArch(StrEnum)`: `ELF64 = "elf64"`, `PE32 = "pe32"`
- `HostOS(StrEnum)`: `LINUX = "linux"`, `WINDOWS = "windows"` (`HostOS.current()`)
- `StepName(StrEnum)`: `INIT = "init"`, `ANALYZE = "analyze"`, `EXTRACT = "extract"`, `DERIVE = "derive"`
- `CleanTarget(StrEnum)`: `INIT = "init"`, `ANALYZE = "analyze"`, `EXTRACT = "extract"`, `DERIVE = "derive"`, `ALL = "all"` (`CleanTarget.steps() -> tuple[StepName, ...]`)
- `ToolName(StrEnum)`: `GHIDRA = "ghidra"`, `JAVA = "java"`, `IDA = "ida"`
- `ToolSource(StrEnum)`: `TOML = "twdll.toml"`, `PORTABLE = ".twdll/init/tools"`, `DEFAULT_DIR = "default_install_dir"`, `MISSING = "missing"`
- `BinarySource(StrEnum)`: `TOML = "twdll.toml"`, `TOML_INVALID = "twdll.toml (invalid)"`, `STEAM = "steam"`, `STAGED = "staged"`, `MISSING = "missing"`
- `SymbolKind(StrEnum)`: `FUNCTION = "function"`, `DATA = "data"`, `IMPORT = "import"`, `EXPORT = "export"`, `LABEL = "label"`
- `XrefKind(StrEnum)`: `CALL = "call"`, `DATA_READ = "data_read"`, `DATA_WRITE = "data_write"`, `OFFSET = "offset"`
- `StringEncoding(StrEnum)`: `ASCII = "ascii"`, `UTF8 = "utf8"`, `UTF16_LE = "utf16_le"`
- `MatchMethod(StrEnum)`: `EXACT_SYMBOL = "exact_symbol"`, `UNIQUE_STRING = "unique_string"`, `CALLGRAPH_NEIGHBOR = "callgraph_neighbor"`, `VTABLE_SLOT = "vtable_slot"`
- `SearchKind(StrEnum)`: `FUNCTION = "function"`, `SYMBOL = "symbol"`, `STRING = "string"`, `VTABLE = "vtable"`
- `DatabaseTarget(StrEnum)`: `EXTRACT = "extract"`, `DERIVE = "derive"`

### 5.2 Declarative Specs & Workspace Paths
- `BinarySpec(name: BinaryName, steam_app_id: SteamAppId, default_install_dir: str, file_name: str, arch: BinaryArch)`
- `BINARY_SPECS: dict[BinaryName, BinarySpec]`
- `PortableArchiveSpec(tool: ToolName, url: str, archive_name: str, extracted_prefix: str)`
- `GHIDRA_PORTABLE_SPEC: PortableArchiveSpec`
- `JDK_PORTABLE_SPECS: dict[HostOS, PortableArchiveSpec]`
- `default_ida_dirs(host_os: HostOS, home_dir: Path) -> tuple[Path, ...]`
- `default_steam_roots(host_os: HostOS, home_dir: Path) -> tuple[Path, ...]`
- `WorkspacePaths(root: Path)`: deterministic properties/methods for `config_file`, `data_dir`, `uv_dir`, `step_dir(step)`, `init_dir`, `tools_dir`, `binaries_dir`, `staged_binary_dir(binary)`, `staged_binary_path(binary)`, `analyze_dir`, `decompiler_analyze_dir(decompiler, binary)`, `extract_dir`, `extract_db_path(decompiler)`, `derive_dir`, `derive_db_path(decompiler)`, `database_path(target, decompiler)`.

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

## 6. Centralized SQLite Layer (`database.py`)

`database.py` is the only module allowed to import `sqlite3`. It translates between SQLite storage primitives (`TEXT`, `INTEGER`, `REAL`) and the typed `domain.py` row dataclasses (`BinaryName`, `BinaryArch`, `Decompiler`, `Address`, `SymbolKind`, `XrefKind`, `StringEncoding`, `MatchMethod`).

### 6.1 Extracted Database Schema (`.twdll/extract/<decompiler>.sqlite`)
- `binaries (binary TEXT PRIMARY KEY, arch TEXT NOT NULL, decompiler TEXT NOT NULL, image_base INTEGER NOT NULL, analyzed_at TEXT NOT NULL)`
- `functions (binary TEXT NOT NULL, address INTEGER NOT NULL, name TEXT NOT NULL, demangled_name TEXT, size INTEGER NOT NULL, signature TEXT, is_thunk INTEGER NOT NULL, PRIMARY KEY (binary, address))`
- `symbols (binary TEXT NOT NULL, address INTEGER NOT NULL, name TEXT NOT NULL, demangled_name TEXT, kind TEXT NOT NULL, PRIMARY KEY (binary, address, name))`
- `strings (binary TEXT NOT NULL, address INTEGER NOT NULL, value TEXT NOT NULL, encoding TEXT NOT NULL, PRIMARY KEY (binary, address))`
- `xrefs (binary TEXT NOT NULL, from_address INTEGER NOT NULL, to_address INTEGER NOT NULL, kind TEXT NOT NULL, PRIMARY KEY (binary, from_address, to_address, kind))`
- Indexes on `functions(binary, name)`, `functions(binary, demangled_name)`, `symbols(binary, name)`, `strings(binary, value)`, `xrefs(binary, to_address)`.

### 6.2 Derived Database Schema (`.twdll/derive/<decompiler>.sqlite`)
- `call_edges (binary TEXT NOT NULL, caller_address INTEGER NOT NULL, callee_address INTEGER NOT NULL, PRIMARY KEY (binary, caller_address, callee_address))`
- `vtables (binary TEXT NOT NULL, address INTEGER NOT NULL, class_name TEXT NOT NULL, entry_count INTEGER NOT NULL, PRIMARY KEY (binary, address))`
- `vtable_entries (binary TEXT NOT NULL, vtable_address INTEGER NOT NULL, slot_index INTEGER NOT NULL, target_address INTEGER NOT NULL, PRIMARY KEY (binary, vtable_address, slot_index))`
- `function_matches (source_binary TEXT NOT NULL, source_address INTEGER NOT NULL, target_binary TEXT NOT NULL, target_address INTEGER NOT NULL, confidence REAL NOT NULL, method TEXT NOT NULL, PRIMARY KEY (source_binary, source_address, target_binary))`

### 6.3 Read-Only Enforcement for `twdll sql`
- Arbitrary user queries executed via `twdll sql` open the target SQLite file using read-only URI mode (`file:<path>?mode=ro` with `uri=True`) so `twdll sql` can never mutate extracted or derived databases.

---

## 7. Closed CLI Command & Flag Surface

Every command supports `--workspace <PATH>` (defaulting to current working directory) and `--json` for deterministic machine-readable output:

| Group | Command | Arguments & Options | Entrypoint | Output / Side Effect |
| :--- | :--- | :--- | :--- | :--- |
| **Setup** | `twdll init` | `[--decompiler ghidra\|ida] [--no-toolchain] [--no-binaries] [--workspace PATH] [--json]` | `init.init` | Creates/updates `twdll.toml`, provisions `.twdll/init/tools/`, stages discovered binaries into `.twdll/init/binaries/<name>/` |
| **Setup** | `twdll status` | `[--workspace PATH] [--json]` | `init.status` | Read-only inspection of `twdll.toml`, toolchain, staged/discovered binaries, and `.twdll/<step>/` directories |
| **Setup** | `twdll clean` | `<init\|analyze\|extract\|derive\|all> [--workspace PATH] [--json]` | `init.clean` | Removes the corresponding `.twdll/<step>/` directory (`shutil.rmtree`); preserves `.twdll/uv/` and `twdll.toml` |
| **Analyze** | `twdll analyze` | `[--binary <name>] [--decompiler ghidra\|ida] [--force] [--workspace PATH] [--json]` | `analyze.analyze` | Runs headless decompiler analysis on staged binary(ies) into `.twdll/analyze/<decompiler>/<binary>/` |
| **Extract** | `twdll extract` | `[--binary <name>] [--decompiler ghidra\|ida] [--workspace PATH] [--json]` | `extract.extract` | Extracts base facts from `.twdll/analyze/<decompiler>/<binary>/` into `.twdll/extract/<decompiler>.sqlite` |
| **Derive** | `twdll derive` | `[--decompiler ghidra\|ida] [--workspace PATH] [--json]` | `derive.derive` | Computes callgraphs, vtables, and cross-binary function matches into `.twdll/derive/<decompiler>.sqlite` |
| **Query** | `twdll search` | `<pattern> [--binary <name>] [--kind function\|symbol\|string\|vtable] [--decompiler ghidra\|ida] [--limit N] [--workspace PATH] [--json]` | `query.search` | Pattern search across functions, symbols, strings, and vtables |
| **Query** | `twdll show` | `<binary> <identifier> [--decompiler ghidra\|ida] [--workspace PATH] [--json]` | `query.show` | Detailed view of a function/symbol by hex address or name (signature, callers, callees, strings, cross-binary matches) |
| **Query** | `twdll sql` | `<query> [--db extract\|derive] [--decompiler ghidra\|ida] [--workspace PATH] [--json]` | `query.sql` | Executes a read-only SQL query (`?mode=ro`) against `.twdll/extract/<decompiler>.sqlite` or `.twdll/derive/<decompiler>.sqlite` |

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

1. **Slice 1 — Foundation & Setup (`init`, `status`, `clean`)**:
   - Step 1A: `cli/pyproject.toml` (configure `basedpyright` venv path, `import-linter` `include_external_packages = true` and flat module contracts, and Ruff complexity rules `C901`, `PLR0912`, `PLR0913`, `PLR0915`).
   - Step 1B: `cli/src/twdll/domain.py` & `cli/src/twdll/database.py` (complete domain `NewType`s, `StrEnum`s, declarative specs, typed `Path`/enum models, row dataclasses, and SQLite schema/query primitives).
   - Step 1C: `cli/src/twdll/init.py` (replace `workspace.py` + `steps/init.py` with a flat, small-function `init.py` exposing `init()`, `status()`, `clean()` using `match / case` and zero `# type: ignore`).
   - Step 1D: `cli/src/twdll/cli.py` & `cli/tests/test_init.py` (replace `tests/test_setup.py` with `tests/test_init.py`, wire `twdll.init`, verify 100% green across all 5 checks).
2. **Slice 2 — Headless Decompiler Analysis (`analyze`)**:
   - `cli/src/twdll/analyze.py`, `cli.py` (`analyze` command), `cli/tests/test_analyze.py`.
3. **Slice 3 — Base Fact Extraction (`extract`)**:
   - `cli/src/twdll/extract.py`, `cli.py` (`extract` command), `cli/tests/test_extract.py`.
4. **Slice 4 — Derivations (`derive`)**:
   - `cli/src/twdll/derive.py`, `cli.py` (`derive` command), `cli/tests/test_derive.py`.
5. **Slice 5 — Query Commands (`search`, `show`, `sql`)**:
   - `cli/src/twdll/query.py`, `cli.py` (`search`, `show`, `sql` commands), `cli/tests/test_query.py`.
