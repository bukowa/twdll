# `twdll` Reverse-Engineering Pipeline Rework (`SPEC.md` & CLI Slices 1–3)

*   **Author:** Bukowa
*   **Status:** Proposed
*   **Date:** 2026-11-23

--------------------------------------------------------------------------------

## Overview

This design reworks [`SPEC.md`](file:///home/buk/repos/twdll/SPEC.md) and aligns the Python CLI codebase around a four-step reverse-engineering pipeline: **`init` -> `analyze` -> `extract` -> `sql`**.

`twdll` produces one read-only SQLite database per binary. AI agents and modders can query these databases directly via `twdll sql` without running a live decompiler or external agent bridge. Before locking the `extract` SQLite schema and deciding whether to include bulk-decompiled C pseudocode and full-text search, an empirical inspection and benchmark pass (**Step 3A**) measures real analyzed `attila_linux_64` and `attila_windows_32` Ghidra 12.2 projects.

## Background & Terminology

*   **Warscape Total War targets (`rome2`, `attila`, `tob`)**: `rome2` (Total War: Rome II), `attila` (Total War: Attila), and `tob` (Total War Saga: Thrones of Britannia). `attila_linux_64` is the 64-bit Linux ELF binary with DWARF debug symbols, used as a reference anchor to map stripped 32-bit Windows PE retail binaries (`attila_windows_32`, `rome2_windows_32`, `tob_windows_32`).
*   **Workspace & config**: `twdll.toml` holds user configuration (including toolchain paths and `active_binaries`, which selects which target binaries to process and defaults to all targets when unset). `.twdll/` is the local data directory partitioned 1-to-1 by pipeline step (`init`, `analyze`, `extract`), while `WorkspaceContext` (`ctx`) is the immutable runtime bundle passed through CLI and Model Context Protocol (`MCP`) entrypoints.
*   **Implemented setup commands (`init`, `status`, `clean`)**: `init` provisions tools and stages binaries into `.twdll/init/`, `status` reports workspace/toolchain/artifact readiness, and `clean` removes `.twdll/<step>/` directories.
*   **Slices vs. Steps vs. Stage 3**: *Slices 1–3* are the Python CLI implementation increments (Slices 1–2: setup, `analyze`, and `mcp`; Slice 3: `extract` and `sql`, broken into Steps 3A/3B/3C). *Stage 3* is the frozen C++ SDK (`src/`, `build`/`test`/`pack`/`install`).

## The Problem

[`SPEC.md`](file:///home/buk/repos/twdll/SPEC.md) and `cli/src/twdll/domain.py` no longer match reality or support reliable end-to-end execution:

1. **Speculative `extract` and post-`extract` stubs**: `domain.py` defines unverified SQLite row models and enums that were written before inspecting Ghidra 12.2 outputs on Warscape binaries. They omit required reverse-engineering fields (DWARF structs/types, memory sections, calling conventions, base-relative offsets, and containing functions on cross-references) and couple `SPEC.md` to unbuilt `derive`, `search`, and `show` commands.
2. **Expensive `analyze` skip checks**: `analyze.py` copies large Ghidra repository directories (`.rep/`) into the system temporary directory on every skip check and every run because Ghidra's `ProjectLocator` rejects path components starting with `.` (such as `.twdll`). Without a lightweight completion marker, checking whether a binary is already analyzed is slow and disk-heavy.
3. **Unsafe in-process JVM usage in `mcp.py`**: `mcp.py` currently invokes `analyze()` in the same Python process as the stdio JSON-RPC server. JPype pins a single JVM per process, holds large amounts of heap memory, prevents reloading `twdll.toml` toolchain overrides, and risks corrupting `stdout`.
4. **No local cross-binary SQLite query interface**: Querying a live decompiler per function does not support cross-binary relational queries, whereas extracting structural facts (and potentially C pseudocode) into per-binary SQLite databases lets agents cross-reference `attila_linux_64` and `attila_windows_32` using local SQLite queries without starting a JVM or decompiler.

## Goals

*   **Align `SPEC.md` with reality**: Document the implemented setup/analysis commands (`init`, `status`, `clean`, `mcp`, `analyze`) and the reworked extraction/query pipeline (`extract`, `sql`), omitting `derive`, `search`, and `show`.
*   **Remove speculative stubs**: Remove all unverified row/enum/result stubs from `domain.py` and omit `slots=True` on all MCP-returned result models so `mcp` 2.x generates complete `inputSchema` and `outputSchema` definitions.
*   **Fix `analyze` and `mcp` runtime issues**:
    *   Introduce `<decompiler>_<binary>.meta.json` markers for direct skip checks without starting a JVM or copying `.rep/` directories.
    *   Filter targets against configured active binaries and isolate shared Ghidra/JVM lifecycle helpers into `twdll._ghidra`.
    *   Execute JVM-bound MCP tools (`analyze`, `extract`) in short-lived worker subprocesses.
*   **Empirical schema and pseudocode decision gate**: Inspect analyzed `attila_linux_64` and `attila_windows_32` Ghidra 12.2 projects and benchmark `DecompInterface` throughput, timeouts, and database size before locking the `extract` SQLite schema and deciding whether bulk C pseudocode and full-text search are viable.
*   **Safe read-only `twdll sql`**: Provide a read-only `:memory:` SQLite query interface that attaches completed per-binary extract databases and enforces allowlist authorizer, row-limit, and timeout guards.

## Non-Goals

*   **`derive`, `search`, and `show`**: Excluded from `SPEC.md` and this design; SQLite queries via `twdll sql` serve all analysis needs until real usage justifies higher-level commands.
*   **IDA 9.5 backend implementation**: Deferred until Windows host testing, though `extract` table and column names remain backend-neutral so IDA can populate the same schema later.
*   **External agent frameworks (e.g., REA / `rea-agents`)**: `twdll` remains self-contained around its isolated `uv`/Python toolchain and `twdll mcp` server with no Node.js or `PATH` dependencies.
*   **Stage 3 C++ SDK / `twdll.dll` (`build`, `test`, `pack`, `install`)**: Frozen and out of scope until the CLI reverse-engineering pipeline is complete.

--------------------------------------------------------------------------------

## Technical Plan (Architecture)

### 1. Pipeline Data Flow

Data flows in one direction across 1-to-1 directories under `.twdll/`:

```text
  twdll.toml + Steam/User Binaries
                 |
                 v
        [ init / status / clean ]
                 |  writes .twdll/init/tools/ + .twdll/init/binaries/<binary>[.dll]
                 v
           [ analyze ]  (via twdll._ghidra + OS temp staging)
                 |  writes .twdll/analyze/<decompiler>_<binary>.gpr|.rep
                 |       + <decompiler>_<binary>.meta.json (atomic marker)
                 v
           [ extract ]  (via twdll._ghidra + twdll.database)
                 |  writes .twdll/extract/<decompiler>_<binary>.sqlite
                 |  (empirically locked schema, atomic rename)
                 v
            [ sql ]     (twdll.query -> twdll.database, read-only :memory:)
                 |  attaches completed <binary>.sqlite files read-only
                 v
      JSON / CLI / MCP Result (SqlQueryResult)
```

### 2. Module Layering & `import-linter` Contracts

`cli/pyproject.toml` enforces strict one-way dependencies across the package:

```text
twdll.cli (Typer/Rich; root overrides --decompiler, --ghidra-dir, --java-home, --ida-dir)
    |
    v
twdll.mcp (MCPServer stdio; subprocess isolation for JVM steps)
    |
    v
twdll.init  |  twdll.analyze  |  twdll.extract  |  twdll.query
                   \                  /  \              /
                    v                v    \            /
                      twdll._ghidra        twdll.database
                            \                  /
                             v                v
                                twdll.domain
```

Mechanical `forbidden` rules in `cli/pyproject.toml`:

*   Only `twdll._ghidra` may import `pyghidra` or `jpype`.
*   Only `twdll.database` may import `sqlite3`.
*   Only `twdll.cli` may import `typer` or `rich`.
*   Only `twdll.mcp` may import `mcp`.

### 3. Domain Model (`twdll.domain`)

*   **Remove unverified stubs**: Remove all speculative row/enum/result definitions and unused `derive` step/path definitions so `domain.py` reflects only implemented and empirically verified models.
*   **Completion marker paths**: Add a metadata suffix and path helper on `WorkspacePaths` for `<decompiler>_<binary>.meta.json`.
*   **MCP schema compatibility**: Omit `slots=True` on all `@dataclass(frozen=True)` result and status models returned by `twdll.mcp` (`mcp` 2.x / Pydantic schema generation omits fields on slotted dataclasses). Keep `slots=True` on high-volume SQLite row dataclasses.

### 4. Shared Ghidra Lifecycle & Analysis (`twdll._ghidra`, `twdll.analyze`)

*   **`twdll._ghidra`**: Centralize JVM startup, toolchain validation, Ghidra progress monitoring, OS temporary staging, and project open/save helpers so `analyze` and `extract` share one `pyghidra`/`jpype` boundary without importing each other.
*   **Direct skip checks via `.meta.json`**: `analyze` writes `<decompiler>_<binary>.meta.json` atomically after saving the Ghidra project. Subsequent runs verify both the marker (matching `binary_sha256` and target identity) and the presence of the `.gpr`/`.rep` artifacts before skipping, without starting a JVM or copying `.rep/` directories. Existing pre-`.meta.json` projects are re-analyzed cleanly (or backfilled) when `.meta.json` is missing or malformed.
*   **OS temp staging**: Because Ghidra's `ProjectLocator` rejects `.twdll`, `analyze` and `extract` stage active Ghidra project work in a per-run OS temporary directory rather than creating transient directories under the workspace root, preserving `.twdll/<step>/` 1-to-1 ownership. `extract` operates read-only on analyzed projects without moving `.twdll/analyze/` artifacts out of place.
*   **Active binary filtering**: `analyze` filters targets using the configured active-binary list so unconfigured targets are skipped.

### 5. MCP Subprocess Isolation (`twdll.mcp`)

*   `twdll_init`, `twdll_status`, and `twdll_clean` continue executing in-process against refreshed workspace context.
*   JVM-bound tools (`twdll_analyze`, `twdll_extract`) spawn a short-lived CLI worker subprocess with the workspace root as `cwd`, isolated stdio (`stdin` closed, `stdout` JSON captured, `stderr` captured), and process-group termination on timeout/cancellation so child JVMs are never orphaned.
*   The server forwards only explicit startup CLI overrides (never config values loaded from `twdll.toml`) so `twdll.toml` edits take effect on every MCP call, and maps non-zero exits or non-JSON `stderr` failures to clean MCP tool errors.

### 6. Empirical Schema Discovery & Extraction (`twdll.extract`, `twdll.database`)

*   **DWARF-first extraction (`attila_linux_64`)**: Because Ghidra 12.2's built-in `DWARF` analyzer unconditionally indexes all 242.78 MB of `.debug_info` in JVM heap (causing OOMs on 8 GiB hosts), `twdll analyze` disables Ghidra's built-in `DWARF` analyzer on `attila_linux_64` (retaining `.symtab`, `GnuDemanglerAnalyzer`, control flow, and xrefs). The first step of `twdll extract` always extracts DWARF debug info out-of-process via `ddbug_parser` (benchmarked at 26.56s / 2.88 GiB peak RSS across all 469 CUs, cross-checked with `llvm-dwarfdump` where needed) into backend-neutral SQLite tables (`dwarf_*`) shared by both Ghidra and future IDA 9.5 backends.
*   **Empirical gate (Step 3A)**: Before locking the `dwarf_*` and `extract` SQLite schemas and join keys, inspect `ddbug_parser` outputs on `attila_linux_64` alongside Ghidra 12.2's Python API (`Function`, `Symbol`, `Data`, `Reference`, `MemoryBlock`, and `DecompInterface` with selective pointer-cutoff DWARF type injection) to finalize table/column contracts and benchmark C pseudocode throughput and `.sqlite` size.
*   **Schema & atomicity invariants**:
    1. **One database per binary**: `.twdll/extract/<decompiler>_<binary>.sqlite`.
    2. **Atomic completion**: Write into a temporary staging database, record completion metadata and `binary_sha256` after all rows are written, close all connections (preventing Windows file locks), and atomically rename to the final `.sqlite` path.
    3. **Integer addresses and `image_base`**: Store memory addresses as plain integers alongside `image_base` so relative offsets (`RVA`) are always computable.
    4. **Backend-neutral naming**: Use backend-neutral table names (`functions`, `symbols`, `strings`, `types`, `xrefs`, `sections`) rather than Ghidra Java class names. Bulk C pseudocode and full-text search (FTS5) are included only if the empirical benchmark confirms acceptable runtime and storage overhead.

### 7. Safe Read-Only `twdll sql` (`twdll.query`, `twdll.database`)

*   `twdll.query.sql` delegates execution to `twdll.database`, which opens an `:memory:` URI connection in query-only mode and attaches all completed `.twdll/extract/<decompiler>_<binary>.sqlite` files for the active decompiler as read-only schemas aliased by binary name (for example `attila_linux_64.functions`).
*   If zero completed extract databases exist, `twdll sql` fails fast with an actionable error.
*   Read-only safety is enforced via an allowlist authorizer (registered after attaching databases, permitting only `SELECT`, `READ`, `FUNCTION`, and recursive CTE operations while blocking `PRAGMA`, `ATTACH`, `DETACH`, and all writes/DDL), a query execution timeout, a row cap (`limit` + `truncated` flag), and hex serialization of `bytes` values for JSON/MCP output.

## Success Metrics & Risks

*   **Acceptance Criteria**:
    *   `analyze` skip check completes without starting a JVM or copying `.rep/` directories.
    *   `twdll mcp` serves `twdll_analyze` and `twdll_extract` without pinning a JVM in the stdio server process, and `twdll.toml` edits take effect on the next call.
    *   `twdll sql` rejects `PRAGMA query_only = OFF`, `ATTACH`, and all write/DDL statements, caps rows with `truncated: bool`, and enforces query timeouts.
    *   Empirical gate records concrete `attila_linux_64` and `attila_windows_32` schema fields and `DecompInterface` throughput before `extract` schema lock.
*   **Key Risks & Mitigations**:
    *   *Pre-`.meta.json` analyzed projects*: Treat missing or malformed `.meta.json` as unverified and re-analyze cleanly.
    *   *Orphaned JVM processes on MCP timeout*: Launch worker subprocesses in their own process group/session and terminate the full process tree on timeout.
    *   *Windows SQLite/Ghidra file locks*: Close all SQLite connections and Ghidra project handles before atomic renames or temporary directory cleanup.

--------------------------------------------------------------------------------

## Alternatives Considered

### Alternative A: Patching `SPEC.md` Incrementally While Keeping `derive`, `search`, and `show`

*   **Pros**: Fewer edits to the existing `SPEC.md` structure and no immediate deletion of Slice 3–5 stubs in `domain.py`.
*   **Cons**: Leaves speculative SQLite schemas and unimplemented commands (`derive`, `search`, `show`) in both the spec and `domain.py`, creating false contracts for implementation and testing.
*   **Reason for Rejection**: The Slice 3–5 schemas and `derive`/`search`/`show` assumptions were speculative; omitting them keeps `SPEC.md` and `domain.py` aligned with what is actually built and verified.

### Alternative B: Locking `extract` SQLite Columns Upfront Before Empirical Inspection

*   **Pros**: Allows writing `database.py` and `extract.py` tables immediately without an inspection script.
*   **Cons**: Guessed schemas already missed DWARF structs/types, memory sections, calling conventions, base-relative offsets, containing functions on xrefs, and C pseudocode.
*   **Reason for Rejection**: Ghidra 12.2 outputs and `DecompInterface` throughput on `attila_linux_64` and `attila_windows_32` must be measured before locking the schema.

### Alternative C: On-Demand-Only C Pseudocode (`twdll decompile`) Without Evaluating Bulk Extraction

*   **Pros**: Faster initial `extract` runtime and smaller `.sqlite` files on disk.
*   **Cons**: Prevents full-text search across decompiled C pseudocode and forces agents to invoke Ghidra/JVM on demand during iterative reverse-engineering queries.
*   **Reason for Rejection**: Game binaries are static, so a one-time bulk decompilation cost may be worthwhile if Step 3A benchmarks show acceptable throughput and database size.

### Alternative D: In-Process `fd 1` / `System.setOut` Redirection in `twdll.mcp`

*   **Pros**: Avoids spawning a subprocess per `twdll_analyze` or `twdll_extract` MCP call.
*   **Cons**: JPype pins a single JVM per process, holds large heap memory in the long-lived MCP server, blocks reloading `twdll.toml` toolchain overrides, and risks `stdout` corruption of stdio JSON-RPC.
*   **Reason for Rejection**: Worker subprocesses isolate JVM memory, allow clean `twdll.toml` reloads, and protect `stdout` JSON-RPC framing.

### Alternative E: Temporary `sql.py` Module Instead of `query.py` (`query.sql`)

*   **Pros**: Narrower filename for a single `sql` command.
*   **Cons**: Breaks the 1-to-1 module layout (`cli.py` -> `query.py`) and causes `import-linter` and test churn if query helpers expand later.
*   **Reason for Rejection**: Keeping `twdll.query` (`query.sql`) preserves the existing architecture and `import-linter` rules.

### Alternative F: Integrating External Agent Frameworks (e.g., REA / `rea-agents`)

*   **Pros**: Prebuilt MCP bridge for interactive Ghidra queries.
*   **Cons**: Requires Node.js/`PATH` dependencies outside `twdll`'s isolated `uv`/Python toolchain, duplicates `twdll mcp`, and queries live Ghidra per request instead of cross-referencing pre-extracted SQLite databases.
*   **Reason for Rejection**: `twdll` stays self-contained with local SQLite and `twdll sql` for local cross-binary queries.

--------------------------------------------------------------------------------

## Detailed Implementation Plan

### Target Files to Create / Modify

#### 1. `SPEC.md` (Modify — Slice 1–3 Alignment)
*   **Type / Scope**: Repository Specification
*   **Proposed Changes**: Rework Sections 1–8 to document the implemented Slice 1–2 foundation (`init`, `status`, `clean`, `mcp`, `analyze`) and the reworked Slice 3 pipeline (`extract`, `sql`). Omit `derive`, `search`, and `show`. Document `twdll._ghidra`, `.meta.json` skip markers, OS temp-dir staging (`tempfile.TemporaryDirectory()`), MCP worker subprocess isolation, Step 3A empirical schema discovery, and read-only `:memory:` `twdll sql`.

#### 2. `cli/pyproject.toml` (Modify — Layer & Forbidden Contracts)
*   **Type / Scope**: Package & Linter Configuration (`import-linter`)
*   **Proposed Changes**: Update `[[tool.importlinter.contracts]]` to place `twdll._ghidra` below `twdll.analyze` and `twdll.extract` (and independent of `twdll.init`, `twdll.query`, and `twdll.database`) in the strict layer contract. Add `twdll._ghidra`, `twdll.extract`, and `twdll.query` to `source_modules` of all three existing `forbidden` contracts (`sqlite3`, `typer`/`rich`, `mcp`). Add a new `forbidden` contract banning `pyghidra` and `jpype` in all `twdll.*` modules except `twdll._ghidra`.

#### 3. `cli/src/twdll/domain.py` (Modify — Model Cleanup & Slice 3 Types)
*   **Type / Scope**: Domain Models & Enums (stdlib-only)
*   **Proposed Changes**:
    *   Remove unverified Slice 3–5 stubs (`ExtractedBinaryRow`, `FunctionRow`, `SymbolRow`, `StringRow`, `XrefRow`, `SymbolKind`, `XrefKind`, `StringEncoding`, `MatchMethod`, `SearchKind`, `DatabaseTarget`, `CallEdgeRow`, `VtableRow`, `VtableEntryRow`, `FunctionMatchRow`, `DeriveResult`, `SearchHit`, `FunctionDetail`), plus `StepName.DERIVE`, `CleanTarget.DERIVE`, `WorkspacePaths.derive_dir`, and `WorkspacePaths.derive_db_path`.
    *   Add `FileSuffix.META_JSON = ".meta.json"` and `WorkspacePaths.analyze_meta_path(self, decompiler: Decompiler, binary: BinaryName) -> Path`.
    *   Remove `slots=True` from `ExtractTargetResult`, `ExtractResult`, and `SqlQueryResult` (all Slice 1–2 result/status models already omit `slots=True`).
    *   Update `ExtractTargetResult` (`binary: BinaryName`, `db_path: Path`, `skipped: bool`, `duration_seconds: float`), `ExtractResult` (`decompiler: Decompiler`, `targets: tuple[ExtractTargetResult, ...]` — removing single `db_path`), `SqlScalar = int | float | str | None` (removing `bytes` since binary values are hex-serialized), `SqlRow = tuple[SqlScalar, ...]`, and `SqlQueryResult` (`columns: tuple[str, ...]`, `rows: tuple[SqlRow, ...]`, `truncated: bool`).

#### 4. `cli/src/twdll/_ghidra.py` (Create — Shared Ghidra/JVM Lifecycle)
*   **Type / Scope**: Shared Internal Ghidra/JVM Lifecycle Module
*   **Proposed Changes**: Extract `_GhidraProgressMonitor`, `_require_ghidra_toolchain`, `pyghidra.start(install_dir=...)`, OS temp-dir staging via `tempfile.TemporaryDirectory()`, and Ghidra project open/save helpers from `analyze.py` so `analyze.py` and `extract.py` share one `pyghidra`/`jpype` boundary without importing each other. Keep helper functions within `cli/pyproject.toml` Ruff complexity limits (`C901 <= 8`, `PLR0912 <= 8`, `PLR0913 <= 6`, `PLR0915 <= 35`).

#### 5. `cli/src/twdll/analyze.py` (Modify — Marker Skip Checks & Active Filtering)
*   **Type / Scope**: Headless Analysis Step
*   **Proposed Changes**:
    *   Delegate JVM startup, progress monitor, `tempfile.TemporaryDirectory()` staging, and project open/save to `twdll._ghidra`.
    *   Write `<decompiler>_<binary>.meta.json` atomically (`os.replace`) after project save with keys `decompiler`, `binary`, `binary_sha256`, `analyzed_at`, and `schema_version`.
    *   On skip checks, verify both `.meta.json` (valid JSON with matching `binary_sha256`) and existence of `.gpr`/`.rep` artifacts; re-analyze cleanly if `.meta.json` is missing, malformed, or stale.
    *   Filter target binaries using `active_binaries(ctx.config)` from `twdll.domain`.

#### 6. `cli/src/twdll/mcp.py` (Modify — Subprocess Isolation & Slice 3 Tools)
*   **Type / Scope**: MCP Stdio Server Adapter
*   **Proposed Changes**:
    *   Keep `twdll_init`, `twdll_status`, and `twdll_clean` in-process with `_refresh_context(ctx)`.
    *   Execute `twdll_analyze` and `twdll_extract` via short-lived worker subprocesses (`sys.executable -m twdll.cli ... --json`, `cwd=ctx.root`, `stdin=subprocess.DEVNULL`, `stdout=subprocess.PIPE`, `stderr=subprocess.PIPE`) in an isolated process group (`start_new_session=True` on POSIX; process-tree termination on Windows) so timeouts/cancellations kill child JVMs.
    *   Track and forward only explicit startup CLI overrides (`--decompiler`, `--ghidra-dir`, `--java-home`, `--ida-dir`) so `twdll.toml` edits take effect on every call; pass target filters via repeated `--binary` options; surface non-zero exits, timeouts, or non-JSON `stderr` as structured MCP tool errors.
    *   Register `twdll_extract` and `twdll_sql` tools returning `ExtractResult` and `SqlQueryResult`.

#### 7. Step 3A Inspection & Benchmark Script (Create in Scratch / Run Ad-Hoc, Non-Shipping)
*   **Type / Scope**: Empirical Discovery & Throughput Benchmark
*   **Proposed Changes**: Run against analyzed `attila_linux_64` and `attila_windows_32` Ghidra 12.2 projects (via `twdll._ghidra`) to inspect available functions, signatures, calling conventions, symbols, strings, cross-references (with containing functions), memory sections, and DWARF/recovered types, and to measure `DecompInterface` bulk C pseudocode throughput, per-function timeouts, and `.sqlite` size. Output gates Step 3B schema and pseudocode decisions.

#### 8. `cli/src/twdll/database.py` (Modify — Step 3B & Step 3C)
*   **Type / Scope**: SQLite Schema & Query Layer (only module importing `sqlite3`)
*   **Proposed Changes**:
    *   (Step 3B) Implement the empirically locked per-binary `extract` SQLite schema (integer addresses with `image_base`, backend-neutral table names, `meta` completion table with `"completed"` and `binary_sha256`, indexed lookup columns, and optional bulk C pseudocode / FTS5 if Step 3A approves). Write to a `.sqlite.staging` file, close all connections, and atomically rename to `.sqlite`.
    *   (Step 3C) Implement `twdll sql` execution: open `sqlite3.connect(":memory:", uri=True)` with `PRAGMA query_only = ON`, attach completed extract databases matching `ctx.decompiler` via `f"{db_path.resolve().as_uri()}?mode=ro"` under `<binary>` aliases (failing fast if zero completed DBs exist), then install an allowlist `set_authorizer` (permitting only `SQLITE_SELECT`, `SQLITE_READ`, `SQLITE_FUNCTION`, `SQLITE_RECURSIVE`), install `set_progress_handler(..., 1000)` for `timeout_ms` (mapping `OperationalError: interrupted` to a timeout error), enforce row `limit` with `truncated: bool`, and hex-serialize `bytes` values.

#### 9. `cli/src/twdll/extract.py` (Create — Step 3B)
*   **Type / Scope**: Headless Fact Extraction Step
*   **Proposed Changes**: Implement `extract(ctx: WorkspaceContext, binaries: list[BinaryName] | None = None, force: bool = False) -> ExtractResult` using `twdll._ghidra` and `twdll.database` to export structural facts (and pseudocode if approved by Step 3A) from analyzed Ghidra projects into `.twdll/extract/<decompiler>_<binary>.sqlite`. Skip when a completed `.sqlite` already matches `binary_sha256` (unless `force=True`), and fail clearly if a target binary has not been analyzed.

#### 10. `cli/src/twdll/query.py` (Create — Step 3C)
*   **Type / Scope**: Read-Only SQL Query Step
*   **Proposed Changes**: Implement `sql(ctx: WorkspaceContext, query: str, limit: int = 200, timeout_ms: int = 5000) -> SqlQueryResult` validating `limit > 0` and `timeout_ms > 0` and delegating execution to `twdll.database`.

#### 11. `cli/src/twdll/cli.py` (Modify — Step 3B & Step 3C)
*   **Type / Scope**: Typer CLI Entrypoint
*   **Proposed Changes**: Expose `twdll extract` (`--binary` repeatable, `--force`, `--json`) and `twdll sql` (`query` argument, `--limit`, `--timeout-ms`, `--json` / Rich table output), preserving root-only context overrides on the `twdll` callback.

#### 12. `cli/tests/` (`test_architecture.py`, `test_analyze.py`, `test_mcp.py`, `test_extract.py`, `test_query.py`) (Modify / Create)
*   **Type / Scope**: Unit & Architecture Test Suite (`pytest`)
*   **Proposed Changes**:
    *   Update `test_architecture.py`: replace `"derive.py"` with `"_ghidra.py"` in `_EXECUTION_STEP_MODULES`, and verify `domain.py` stub removal.
    *   Update `test_analyze.py`: test `.meta.json` skip checks, missing/corrupt `.meta.json` re-analysis, `tempfile.TemporaryDirectory()` staging, and `active_binaries(ctx.config)` filtering across both `"linux"` and `"windows"` host paths.
    *   Update `test_mcp.py`: test subprocess isolation of `twdll_analyze`/`twdll_extract` (`cwd=ctx.root`, explicit-only override forwarding, non-zero/non-JSON error propagation, timeout process-group kill) and MCP `outputSchema` generation for all result dataclasses.
    *   Add `test_extract.py` and `test_query.py`: test atomic `.sqlite` completion, skip/force behavior, `uri=True` attachment, `bytes` hex serialization, allowlist `set_authorizer` rejecting `PRAGMA query_only = OFF` and write/DDL statements, empty `.twdll/extract/` error, row limit truncation, and timeout handling.

### Verification Plan

Run all static checks and tests from `cli/` after each implementation step:

*   **Formatting**: `uv run ruff format --check`
*   **Linting**: `uv run ruff check`
*   **Type Checking (Strict)**: `uv run basedpyright`
*   **Architecture Boundaries**: `uv run lint-imports`
*   **Unit & Integration Tests**: `uv run pytest` (isolated with `tmp_path`, parameterized across `"linux"` and `"windows"` host paths)
*   **Empirical Gate (Step 3A)**: Recorded benchmark and schema inspection run against analyzed `attila_linux_64` and `attila_windows_32` Ghidra 12.2 projects before Step 3B schema implementation.
