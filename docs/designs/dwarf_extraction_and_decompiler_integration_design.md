# Backend-Neutral DWARF Extraction & Decompiler Integration (`ddbug_parser` -> SQLite -> Ghidra / IDA)

*   **Author:** Bukowa
*   **Status:** Proposed
*   **Date:** 2026-11-23

--------------------------------------------------------------------------------

## Overview

This design decouples DWARF debug extraction from any single decompiler backend so `twdll` can extract complete, low-RAM DWARF data from `attila_linux_64` once into SQLite and feed both **Ghidra 12.2** (now) and **IDA 9.5** (later) through a clean backend-neutral contract.

Instead of relying on Ghidra's built-in DWARF analyzer—which indexes all 242.78 MB of `.debug_info` in JVM heap and stalls or OOMs on 8 GiB hosts—`twdll` uses a small Rust `ddbug_parser` (`v0.6.0`, `gimli`-based) extractor to write structured DWARF facts (types, structs, inheritance, function signatures, local variable locations, inlined call sites, and source paths) directly into SQLite. Decompiler backends (`ghidra` now, `ida` later) then consume those DWARF tables selectively when generating C pseudocode, rather than forcing the decompiler to swallow millions of unused C++ template DIEs upfront.

## Background & Terminology

*   **Reference binary (`attila_linux_64`)**: The 724 MB 64-bit Linux ELF binary for Total War: Attila, containing 469 DWARF v4 compilation units, 313,972 `.symtab` entries, and ~624 MB of `.debug_*` sections (`.debug_info` 242.78 MB, `.debug_str` 123.85 MB, `.debug_pubnames` 86.94 MB, `.debug_loc` 85.92 MB, `.debug_pubtypes` 30.21 MB, `.debug_ranges` 30.14 MB, `.debug_line` 21.30 MB). It serves as the reference anchor for mapping stripped 32-bit Windows PE DLLs (`attila_windows_32.dll`, `rome2_windows_32.dll`, `tob_windows_32.dll`).
*   **`ddbug` / `ddbug_parser` (`v0.6.0`)**: A Rust `gimli`-based DWARF parser benchmarked on `attila_linux_64` at **26.56s** wall time and **2.88 GiB** peak RSS with zero errors, resolving cross-CU types, struct member offsets/sizes, inheritance, function signatures, local variable register/stack locations, and inlined call sites.
*   **`llvm-dwarfdump`**: LLVM's DWARF inspection tool, benchmarked on `attila_linux_64` at **30.32s** and **1.64 GiB** peak RSS (`--statistics`), useful as a low-RAM validation and raw-attribute cross-check tool.
*   **Pointer-cutoff struct closure**: A selective type-injection strategy where value-embedded fields and non-overlapping base classes are expanded into the decompiler's type database, while pointer and reference fields terminate recursion as sized empty struct stubs (preserving type names and pointer stride without loading thousands of transitive STL/engine types).

## The Problem

1. **Ghidra's built-in DWARF analyzer is structurally brittle on low-RAM hosts**: Even with `Import Data Types`, `Create Function Signatures`, and `Import Local Variable Info` disabled, Ghidra 12.2 unconditionally indexes all 242.78 MB of `.debug_info` across all 469 compilation units in JVM heap (and `pyghidra.start()` defaults to ergonomic heap sizing, ~1.9 GiB on an 8 GiB machine).
2. **Disabling DWARF without a replacement degrades C pseudocode**: Without struct layouts and function signatures, Ghidra's decompiler emits untyped pointer arithmetic (`*(int *)(param_1 + 0x18)`) and generic `undefined8` parameters instead of `this->field_name`.
3. **Pure-Python DWARF parsing is too slow and memory-heavy**: Benchmarking `pyelftools` (`0.33`) on `attila_linux_64` took 53.66s and 3.56 GiB peak RSS for just 10 of 469 compilation units (~41.2 minutes projected).
4. **DWARF extraction must not be locked to Ghidra**: `twdll` targets both Ghidra (open-source baseline) and IDA 9.5 (deferred Windows backend). Coupling DWARF tables or extraction strictly to Ghidra internals would force rework when IDA support is added or when cross-referencing DWARF facts directly in `twdll sql`.

## Goals

*   **Backend-neutral DWARF SQLite schema**: Extract `attila_linux_64` DWARF facts once via `ddbug_parser` into decompiler-independent SQLite tables (types, structs/members/inheritance, functions/parameters, local variables/locations, inlined calls, and source locations) queryable via `twdll sql`.
*   **Low-RAM Ghidra `analyze` on `attila_linux_64`**: Disable Ghidra's built-in `DWARF` analyzer during `twdll analyze` (retaining ELF `.symtab`, demangling, control flow, and cross-references) so analysis fits comfortably on 8 GiB hosts.
*   **Selective type/signature injection for readable C pseudocode**: Inject per-function signatures and pointer-cutoff struct closures from the DWARF SQLite tables into the active decompiler backend (Ghidra now, IDA 9.5 later) when generating C pseudocode, rather than importing the entire STL/engine type graph upfront.
*   **Dual-backend compatibility (`ghidra` and `ida`)**: Keep DWARF extraction and storage backend-neutral so IDA 9.5 can either populate/join the same SQLite tables natively (e.g., via `-Odwarf:lazy_types=1`) or consume the `ddbug_parser` tables via IDAPython.
*   **Validation & fallback**: Use `llvm-dwarfdump` (`--statistics` / `--verify` / `--lookup`) as an optional low-RAM verification cross-check against `ddbug_parser` output.

## Non-Goals

*   **Rewriting or forking Ghidra's Java `DWARFAnalyzer`**: All DWARF parsing happens out-of-process via `ddbug_parser`.
*   **Implementing IDA 9.5 in this design**: IDA support remains deferred until Windows host testing; this design defines the backend-neutral DWARF contract and adapter boundary that IDA will plug into later.
*   **Full `.debug_loc` injection into Ghidra**: Local variable register/stack locations are stored in SQLite for direct `twdll sql` and agent queries, rather than attempting fragile multi-range local variable injection into Ghidra's decompiler.
*   **Stage 3 C++ SDK (`src/`, `build`/`test`/`pack`/`install`)**: Frozen and out of scope.

--------------------------------------------------------------------------------

## Technical Plan (Architecture)

### 1. Backend-Neutral DWARF Data Flow

DWARF extraction runs as a decompiler-independent producer feeding SQLite, with decompiler backends acting as consumers for C pseudocode:

```text
  attila_linux_64 (ELF64 + DWARF v4)
          |
          +---> [ ddbug_parser (Rust) ] ---> DWARF SQLite Tables (types, structs,
          |                                  members, inheritance, functions,
          |                                  params, locals/locations, inlines)
          |                                         |
          |                                         +---> [ twdll sql ] (direct queries)
          |                                         |
          +---> [ twdll analyze ]                   v
          |     (Ghidra: DWARF=off,          [ Decompiler Type Injector ]
          |      ELF + .symtab + xrefs)       (Ghidra now / IDA 9.5 later)
          |                                         |
          +-----------------------------------------+---> [ Decompiler Pseudocode ]
                                                          (this->field, typed calls)
```

### 2. DWARF SQLite Data Model (Backend-Neutral)

All DWARF tables use plain integer addresses (paired with `image_base`), backend-neutral naming, and zero decompiler-specific IDs:

*   **`dwarf_units`**: Compilation unit ID, source directory/name, language, and producer.
*   **`dwarf_types`**: Type ID, kind (`base`, `struct`, `union`, `enum`, `typedef`, `pointer`, `reference`, `array`, `subroutine`, `const`/`volatile`), qualified name, `byte_size`, and target type ID.
*   **`dwarf_members`**: Parent struct/union ID, member name, member type ID, `byte_offset`, `byte_size`, and optional bitfield offset/size.
*   **`dwarf_inheritance`**: Derived struct ID, base struct ID, and `byte_offset` (preserving empty-base-optimization metadata so adapters can skip zero-size overlapping bases).
*   **`dwarf_enumerators`**: Enum type ID, enumerator name, and integer value.
*   **`dwarf_functions`**: Function address, linkage name, qualified name, return type ID, `byte_size`, source file/line, and `is_inline` / `sret` flags.
*   **`dwarf_parameters`**: Function ID, ordinal, parameter name, type ID, and DWARF location summary (enabling adapters to detect hidden `sret` shifts).
*   **`dwarf_variables` & `dwarf_inlines`**: Local variable names/types/locations and inlined subroutine call-site address ranges, source file/line, and callee names (for SQLite querying and optional decompiler comments).

### 3. Decompiler Adapter Contract (`Ghidra` Now, `IDA 9.5` Later)

Decompiler backends implement a narrow type-and-signature injection boundary when decompiling DWARF-backed targets:

*   **Pointer-cutoff transitive closure (shared Python logic)**: Given a target function, Python resolves its return type, parameter types, and value-embedded structs/base classes from the DWARF SQLite tables, stopping recursion at pointer and reference fields (emitting sized empty struct stubs with `byte_size > 0` for pointer targets).
*   **Ghidra 12.2 adapter rules**:
    *   During `twdll analyze` on `attila_linux_64`, disable Ghidra's built-in `DWARF` analyzer while keeping ELF, `.symtab`, demangler, and control-flow/xref analyzers enabled.
    *   On Linux x86-64 (`attila_linux_64`), pass `Class *this` as explicit parameter 0 under the default System V ABI calling convention (never `__thiscall`, which is reserved for 32-bit Windows PE targets and causes parameter shifts on x86-64).
    *   Skip empty-base-optimization bases that overlap offset 0 with real data members, check parameter 0 locations for hidden `sret` return-pointer shifts on non-trivial C++ return types `<= 16` bytes, and flush the decompiler cache after injecting types on an open program.
*   **IDA 9.5 adapter path (deferred)**:
    *   IDA 9.5 can either use its native lazy DWARF importer (`-Odwarf:lazy_types=1`) to produce Hex-Rays pseudocode while exporting facts into the shared `extract` schema, or consume the same pointer-cutoff closure via IDAPython `tinfo_t` if uniform cross-backend filtering is desired.

### 4. Packaging & Toolchain Integration

*   **Rust DWARF exporter (`twdll-dwarf`)**: A small standalone Rust CLI binary built around `ddbug_parser v0.6.0` + `rusqlite` (or JSON-lines piped to Python `twdll.database`) that reads an ELF64 binary and writes the DWARF tables atomically.
*   **Toolchain resolution**: Managed alongside `twdll`'s existing isolated toolchain (built/fetched via `twdll init` or configured via `twdll.toml`, never relying on ambient `PATH`), with graceful fallback or clear error reporting if the binary is absent.
*   **`import-linter` isolation**: Python orchestration lives in the step/adapter layer, while SQLite reads/writes remain strictly inside `twdll.database` and Ghidra calls remain strictly inside `twdll._ghidra`.

## Success Metrics & Risks

*   **Acceptance Criteria**:
    *   `twdll` extracts full DWARF facts from `attila_linux_64` (469 CUs) in under 60s and under 3.2 GiB peak RSS on an 8 GiB host.
    *   Ghidra `twdll analyze` on `attila_linux_64` completes without `OutOfMemoryError` when `DWARF` is disabled.
    *   Selective injection + `DecompInterface` on sampled `attila_linux_64` C++ member functions produces typed `this->field_name` pseudocode (including classes with `std::vector` / `std::string` members) without importing full STL type bloat.
    *   `twdll sql` can query struct member offsets, function parameters, local variables, and inlined call sites directly from the DWARF SQLite tables.
*   **Key Risks & Mitigations**:
    *   *`ddbug_parser` omits a raw DWARF detail*: Keep `llvm-dwarfdump --lookup` / `--statistics` as a documented verification cross-check; extend the Rust exporter with `gimli` directly for any missing attribute (e.g., `sret` or `DW_TAG_inheritance` details) since `ddbug_parser` already exposes `gimli`.
    *   *Distribution/build of the Rust exporter*: Provide prebuilt release artifacts or an optional `cargo` build step in `twdll init` for Linux/Windows hosts without requiring Rust at runtime for prebuilt users.
    *   *SysV x64 `this` / `sret` ABI edge cases*: Validate injected signatures against `dwarf_parameters` register locations before applying, and test on sampled `attila_linux_64` methods returning non-trivial `<= 16`-byte structs.

--------------------------------------------------------------------------------

## Alternatives Considered

### Alternative A: Tuning Ghidra 12.2's Built-in `DWARFAnalyzer` Options

*   **Pros**: Zero external Rust binary or custom type-injection code.
*   **Cons**: Verified in Ghidra 12.2 source that `DIEContainer.indexData()` unconditionally indexes all 242.78 MB of `.debug_info` regardless of analyzer options, causing OOMs/stalls on 8 GiB hosts.
*   **Reason for Rejection**: Structurally unviable on low-RAM machines.

### Alternative B: Pure-Python DWARF Extraction via `pyelftools`

*   **Pros**: Pure Python dependency inside `cli/pyproject.toml` with no native binary.
*   **Cons**: Benchmarked at 53.66s and 3.56 GiB RSS for just 10 of 469 CUs (~41.2 minutes projected for `attila_linux_64`).
*   **Reason for Rejection**: Too slow and memory-intensive for full-binary extraction.

### Alternative C: Text-Parsing `llvm-dwarfdump` CLI Output

*   **Pros**: Lower peak RSS (1.64 GiB) and prebuilt `llvm-dwarfdump` packages.
*   **Cons**: DIE dumps are human-readable indented text (no JSON mode for DIEs), requiring a fragile text parser and ~20s startup per filtered query.
*   **Reason for Rejection**: `ddbug_parser` already resolves C++ types, offsets, and signatures into typed Rust structs; `llvm-dwarfdump` is kept only as a secondary validation tool.

### Alternative D: Re-Injecting All DWARF Types into Ghidra Upfront

*   **Pros**: All types present in Ghidra's `DataTypeManager` at all times.
*   **Cons**: Reintroduces `DataTypeManager.resolve()` bloat and `.gpr`/`.rep/` size explosion from hundreds of thousands of STL template instantiations.
*   **Reason for Rejection**: Pointer-cutoff selective injection per target function/module keeps Ghidra's type database small while still producing `this->field_name` pseudocode.

--------------------------------------------------------------------------------

## Detailed Implementation Plan

### Target Files & Components to Create / Modify

#### 1. `tools/twdll-dwarf/` *(or dedicated Rust crate)* (Create)
*   **Type / Scope**: Standalone Rust DWARF-to-SQLite Exporter (`ddbug_parser = "0.6.0"`, `rusqlite`)
*   **Proposed Changes**:
    *   Accept `<elf_path>` and `<output_sqlite_path>` (writing to `.sqlite.staging` and atomically renaming via `completed` + `binary_sha256` in `meta`).
    *   Walk `ddbug_parser::File` units, types, structs/members, inheritance, enums, functions, parameters, local variables/locations, and inlined subroutines, writing normalized rows into the backend-neutral `dwarf_*` SQLite tables.

#### 2. `cli/src/twdll/domain.py` & `cli/src/twdll/init.py` (Modify)
*   **Type / Scope**: Domain Models, Toolchain Resolution, & Provisioning
*   **Proposed Changes**:
    *   Add `ToolName.DWARF_EXPORTER` (or `twdll-dwarf` binary spec) and `WorkspacePaths` helpers for DWARF SQLite artifacts (`.twdll/extract/dwarf_<binary>.sqlite` or integrated into `.twdll/extract/<decompiler>_<binary>.sqlite`).
    *   Update `init` / `status` / `resolve_toolchain` to locate or provision `twdll-dwarf` without relying on `PATH`.

#### 3. `cli/src/twdll/analyze.py` & `cli/src/twdll/_ghidra.py` (Modify)
*   **Type / Scope**: Low-RAM Ghidra Analysis Configuration
*   **Proposed Changes**:
    *   Before running `pyghidra.analyze(...)` on ELF64 targets (`attila_linux_64`), disable Ghidra's `"DWARF"` analysis option (`Analysis` options) so Ghidra keeps ELF `.symtab`, `GnuDemanglerAnalyzer`, and control-flow/xref analysis without indexing 242.78 MB of `.debug_info`.
    *   Support explicit JVM `-Xmx` configuration via `pyghidra` launch properties/args when configured.

#### 4. `cli/src/twdll/database.py` (Modify)
*   **Type / Scope**: Backend-Neutral DWARF Schema & Closure Queries
*   **Proposed Changes**:
    *   Define DDL and read/query helpers for `dwarf_units`, `dwarf_types`, `dwarf_members`, `dwarf_inheritance`, `dwarf_enumerators`, `dwarf_functions`, `dwarf_parameters`, `dwarf_variables`, and `dwarf_inlines`.
    *   Implement pointer-cutoff transitive closure query (`load_function_type_closure(conn, function_addr)`) returning the function signature plus only value-embedded structs/base classes and sized empty stubs for pointer/reference targets.

#### 5. `cli/src/twdll/_ghidra.py` & `cli/src/twdll/extract.py` (Modify / Create)
*   **Type / Scope**: Selective Ghidra Type Injector & `DecompInterface` Integration
*   **Proposed Changes**:
    *   Implement `inject_function_dwarf_types(program, closure)` using `StructureDataType(CategoryPath, name, byte_size, dtm)`, `replaceAtOffset(...)` (skipping EBO overlaps), `FunctionDefinitionDataType`, and `ApplyFunctionSignatureCmd` (with explicit `this` param 0 under SysV x64, `sret` check, and `decomp_interface.flushCache()`).
    *   Allow `extract` (and on-demand decompilation helpers) to run `twdll-dwarf` on `attila_linux_64` and selectively inject types before invoking `DecompInterface.decompileFunction()`.

#### 6. `cli/tests/` (Create / Modify)
*   **Type / Scope**: Unit & Integration Tests (`pytest`)
*   **Proposed Changes**:
    *   Test pointer-cutoff closure logic in `database.py` (ensuring pointer/reference fields stop recursion at sized stubs, while value-embedded structs and non-overlapping bases expand).
    *   Test Ghidra injector mocking (`StructureDataType`, EBO overlap skip, SysV `this` param 0 vs. `__thiscall` guard, `sret` shift handling) across `HostOS.LINUX` and `HostOS.WINDOWS`.

### Verification Plan

*   **Empirical Benchmark (`twdll-dwarf` on `attila_linux_64`)**: Verify full 469-CU export completes in < 60s and < 3.2 GiB peak RSS, and cross-check function/DIE counts against `llvm-dwarfdump --statistics`.
*   **Pseudocode Quality Check**: Run `twdll analyze` (`DWARF=off`) + selective injection + `DecompInterface` on sampled `attila_linux_64` C++ methods and verify `this->field_name` output.
*   **Static & Unit Checks (`cli/`)**: Run `uv run ruff format --check`, `uv run ruff check`, `uv run basedpyright`, `uv run lint-imports`, and `uv run pytest`.
