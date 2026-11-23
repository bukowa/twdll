**DO NOT BE OBSEQUIOUS**

# Project Guidance

`twdll` is a C++ modding SDK, core Lua script extender (`twdll.dll`), and plugin ecosystem for Warscape Total War (`rome2`, `attila`, `tob`).

Total War modders clone this repository and work with an AI agent + the local CLI (`./twdll` / `twdll.cmd`) to reverse-engineer game internals and build mods without manual RE expertise. The end goal (Stage 3) is a shared C++ SDK and optional plugin loader where `twdll.dll` and third-party DLL plugins coexist without RE boilerplate or compatibility conflicts.

Because copyrighted game binaries and decompiler databases cannot be redistributed, every user builds a local SQLite Knowledge Base from their own game files:
- **32-bit Windows targets (`rome2_windows_32`, `attila_windows_32`, `tob_windows_32`):** Stripped retail `empire.retail.dll` binaries hooked at runtime.
- **64-bit Linux reference (`attila_linux_64`):** The Linux `Attila` ELF binary with DWARF debug symbols used as the reference anchor to map the stripped 32-bit binaries.

## Core Domain & Data Flow

Data flows strictly in one direction across 1-to-1 step directories under `.twdll/`: **`init` -> `analyze` -> `extract` -> `derive`**.

- **`Workspace`**: User config (`twdll.toml`) and local data directory (`.twdll/`). Every step owns its matching `.twdll/<step>/` directory so `twdll clean <step>` is a direct folder removal:
  - `.twdll/uv/`: Isolated portable `uv`, Python runtime, and virtualenv bootstrapped by `./twdll` / `twdll.cmd`.
  - `.twdll/init/`: Provisioned tools (`.twdll/init/tools/`) and flat staged game binaries (`.twdll/init/binaries/<binary>[.dll]`).
  - `.twdll/analyze/`: Flat native decompiler projects per `(Decompiler, Binary)` (`.twdll/analyze/<decompiler>_<binary>.gpr|.i64`).
  - `.twdll/extract/`: Flat extracted SQLite databases per `(Decompiler, Binary)` (`.twdll/extract/<decompiler>_<binary>.sqlite`).
  - `.twdll/derive/`: Derived SQLite databases/artifacts (`.twdll/derive/<decompiler>.sqlite`).
- **`Toolchain`**: Never relies on user `PATH`. Resolves tools strictly from `twdll.toml` overrides or `.twdll/init/tools/` (for `ghidra` + `java`), and `twdll.toml` or default IDA 9.5 install paths (for `ida`).
- **`Binary`**: Architecture-header-verified (ELF64 / PE32) binaries staged flat under `.twdll/init/binaries/` (auto-discovered from Steam across Linux & Windows or set in `twdll.toml`).
- **`Domain & Database`**: All enums, constants, config/status models, and SQLite table row `@dataclass(frozen=True, slots=True)` definitions live in a single stdlib-only `domain.py` file. All `sqlite3` schemas and queries live in `database.py`, imported directly by the CLI, PyGhidra, IDAPython, and derivation steps.

## Closed CLI Scope

Implemented in Python 3.12+ (`cli/pyproject.toml` with `typer`, `vdf`, `pyghidra`, and stdlib `sqlite3`/`tomllib`/`pathlib`), cross-platform (Linux & Windows hosts analyzing both Windows PE `.dll` and Linux ELF targets), non-interactive, `--json` where possible:
- **`Setup` (`init`, `status`, `clean`)**: Workspace (`twdll.toml`), portable tools, non-blocking binary discovery & staging, and step-based cleanup (`twdll clean init|analyze|extract|derive|all`).
- **`Analyze` (`analyze`)**: Binary + Decompiler -> `.twdll/analyze/<decompiler>_<binary>.gpr|.i64`.
- **`Extract` (`extract`)**: Decompiler Project -> `.twdll/extract/<decompiler>_<binary>.sqlite` (verified facts via PyGhidra / IDAPython + `database`).
- **`Derive` (`derive`)**: Python derivation steps -> `.twdll/derive/<decompiler>.sqlite` (callgraphs, vtables, cross-game matches via `database`).
- **`Query` (`search`, `show`, `sql`)**: Query extracted & derived SQLite databases (core lookups via `search`/`show`; arbitrary read-only queries via `sql`).
- **`SDK` *(Stage 3, frozen—do not stub or implement yet)* (`build`, `test`, `pack`, `install`)**: Core C++ extender, plugins, and mods.

## Repository State & Working Rules

- **Frozen Tree**: `src/`, `CMakeLists.txt`, root `tests/` (C++/Lua), and LDoc `docs/` are **FROZEN** until the CLI RE pipeline is complete. CLI unit/integration tests live in `cli/tests/`.
- **Grill Me & Step-by-Step Gate**: Question assumptions and align one question at a time on decisions, CLI flags, dependencies, and `domain`/`database` schemas. Never jump into multi-file code writes unannounced: before writing code, state the exact file(s) and scope you plan to implement next, wait for explicit user approval, and implement incrementally step by step with review between steps.
- **Scope**: Assigned task only; report unrelated bugs, gaps, or refactors at the end without unasked edits.
- **Safety & Git**: Never edit/delete live `.twdll/`, `twdll.toml`, or run `clean` without permission; isolate all tests (`tmp_path`). No hardcoded machine paths. Edit `docs/` or git commit/tag/push on explicit request only. Keep all repository files, code, comments, and CLI output in English.

## CLI Architecture & Quality Rules (`cli/`)

- **Conceptual Alignment & Explicit Intent**: Folder names (`.twdll/<step>/`), flat module names (`cli/src/twdll/<step>.py`), CLI commands (`twdll <cmd>`), and entrypoint functions (`init.init`, `init.status`, `init.clean`, `analyze.analyze`, `extract.extract`, `derive.derive`, `query.search`, `query.show`, `query.sql`) must mirror the problem domain 1-to-1 (see `SPEC.md`). Eliminate primitive obsession in `domain.py`: use explicit `StrEnum`s (`BinaryName`, `WorkspaceName`, `FileSuffix`, etc.), `NewType` (`Address`, `SteamAppId`), and `pathlib.Path` (`Path | None` instead of `""`), declarative lookup tables, and exhaustive Python 3.12 `match / case` instead of nested `if / else` chains. Outside `domain.py`, no module or test may hardcode workspace directory names, target names, or artifact suffixes—all paths must flow through `domain.py` enums and `ctx.paths`.
- **Data-Driven `WorkspaceContext` & Mechanical Lint Enforcement**: Keep `domain.py` strictly data-driven—no speculative helper or mutator methods on dataclasses. Construct `ctx = WorkspaceContext.from_root(root)` once at the CLI/test boundary, apply flag overrides via stdlib `dataclasses.replace`, and pass `ctx: WorkspaceContext` to all step entrypoints and workspace-scoped helpers (never unpack `ctx` back into `(paths, config, host_os)` or `(root, config)`). Enforce architectural invariants mechanically via Ruff (`TID251` banned-api, complexity limits `C901`, `PLR0912`, `PLR0913`, `PLR0915`) and `import-linter` (`cli` -> `mcp` -> step modules -> `database` -> `domain`, banning `sqlite3` outside `database`, `typer`/`rich` outside `cli`, and `mcp` outside `mcp`). Use `pathlib.Path` everywhere (no hardcoded `/` or `\`, no `shell=True`) and test both `"linux"` and `"windows"` host paths via parametrized `pytest` tests.
- **CLI & MCP Dual-Transport Rules (`cli.py` + `mcp.py` + `.agents/`)**:
  - **Root-Only Context Overrides**: Workspace/context overrides (`--decompiler`, `--ghidra-dir`, `--java-home`, `--ida-dir`) live **exclusively** on the root `twdll` callback—never duplicated on subcommands (such as `init` or `extract`).
  - **Zero Duplication**: Every domain entrypoint (`init`, `status`, `clean`, `analyze`, `extract`, `derive`, `search`, `show`, `sql`) takes `ctx: WorkspaceContext` + typed `domain.py` parameters and returns a frozen `@dataclass`. Both `cli.py` (Typer) and `mcp.py` (`MCPServer` stdio via `twdll mcp`) wrap those exact entrypoints directly.
  - **Full Typed MCP Schemas**: `mcp.py` returns frozen `@dataclass(frozen=True)` command result dataclasses directly (omitting `slots=True` on result/status models so `mcp` 2.x/Pydantic generates full `inputSchema` and `outputSchema` automatically; keep `slots=True` on high-volume SQLite row dataclasses).
  - **Agentic Config Location**: All agentic/MCP configuration files live under `.agents/` (e.g., `.agents/mcp.json`).
- **Quality & Guardrails**: Format, lint, typecheck, and test all Python code with `ruff format`, `ruff check`, `basedpyright` (strict), `lint-imports` (`import-linter`), and `pytest`. Never weaken linter/typechecker configs (`cli/pyproject.toml`, `.githooks/`) or add `# noqa` / `# type: ignore`; stop and report if blocked.
