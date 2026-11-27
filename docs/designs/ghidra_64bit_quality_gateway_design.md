# 64-Bit Ghidra Quality Analysis Gateway & Iterative Audit Loop (Task A)

* **Author:** Bukowa
* **Status:** Proposed
* **Date:** 2026-11-27

--------------------------------------------------------------------------------

## Overview

`twdll` uses the DWARF-annotated 64-bit Linux binary (`attila_linux_64`) as a semantic reference anchor for Warscape Total War (`rome2`, `attila`, `tob`). Modders and AI agents read decompiled 64-bit pseudocode from a local SQLite Knowledge Base to inspect class layouts, virtual methods, global singletons, and call flows when writing C++ hooks for stripped 32-bit Windows binaries (`attila_windows_32`, `rome2_windows_32`, `tob_windows_32`).

Task A builds a repeatable 64-bit quality gateway around the four DWARF-to-Ghidra injection passes—Pass 1 (type shells), Pass 2 (vtable structs, struct/union layouts, and globals), Pass 3 (`noreturn` flags and ABI function signatures), and Pass 4 (stack/register locals and inline comments)—before building Task B (the production `twdll extract` CLI and MCP pipeline). Based on the 1,000-function `batch_001` baseline, Task A moves Passes 1–4 out of prototype scripts into production `cli/src/twdll/_ghidra/`, fixes all four systemic defect classes across Rust and Python, replaces uncalibrated LLM-only auditing with a deterministic Tier 0 pre-scanner plus `HIGH`-thinking Gemini Flash review loop, and certifies readiness on three deterministic regression layers plus a held-out 500-function exit gate.

## The Problem

Full extraction of `attila_linux_64` covers 166,593 DWARF-matched functions. Building Task B before verifying 64-bit decompilation quality risks writing systemic typing and ABI defects into every extracted SQLite database, requiring full re-extraction across all 166,593 functions and misleading modders or AI agents during hook development.

The 1,000-function baseline audit (`batch_001`, 52 chunks: 869 unflagged, 131 flagged, 134 deduplicated findings across 8 clusters, of which roughly 35–45 were false positives) revealed four problems across the decompilation pipeline and the audit process:

1. **Systemic DWARF-to-Ghidra Decompilation Gaps:** The ~90 non-false-positive findings in `batch_001` traced to four recurring root causes:
   * **Untyped global and static singletons (~21 hits):** Core singletons (`EMPIRE_APP_MODULE::s_instance`, `CAMPAIGN_UI::s_ui`, `FRONT_END_UI::s_ui`, `WS_CONTEXT::m_e_engine`, `WS_PLATFORM::m_pf_device`) have no DWARF global metadata extracted or stamped in Ghidra, forcing raw offset arithmetic and raw vtable pointer casts at call sites.
   * **Broken 9–16-byte integer/member-function returns and false `is_sret` (>= 22 `HIGH` hits):** GCC 4.8 `std::pair<iterator, bool>` methods (such as `std::_Rb_tree::_M_insert_unique` called by `DATABASE_TABLE<*>::clearing_packs` at `0x1de0b20`) are misclassified as hidden-pointer returns (`is_sret = true`) even when concrete DWARF parameter locations place `this` in `RDI`, causing `in_RDX` / `extraout_RDX` corruption. In addition, 16-byte Itanium member-function pointers (returned by `TWUI::cc_find_function` at caller `UIDLL::CampaignWealthTooltip::Function`, `0x2b71350`) are truncated to 8-byte `void *`, 8-byte data-member pointers are unconditionally defaulted to 16 bytes in Rust DWARF parsing, and 2-register `RAX:RDX` return storage uses inverted varnode order and fails on non-power-of-two struct sizes (11, 13, 14, and 15 bytes).
   * **Missing 8–16-byte SSE float-struct returns:** Homogeneous float/double math structs (such as the orientation struct returned by `get_orientation_world` inside caller `WARSCAPE::EMITTER_INSTANCE::update_orbital_effect` at `0x349af30`) receive integer register storage instead of `XMM0` or `XMM0:XMM1`.
   * **External `noreturn` lookup misses:** `cli/src/twdll/_ghidra.py` intentionally disables Ghidra's `"Non-Returning Functions - Discovered"` analyzer during `twdll analyze` to prevent false-positive `noreturn` markings on game functions. Because Pass 3 does not walk Ghidra external library functions and PLT thunks, `libstdc++.so.6` functions such as `std::__throw_bad_alloc` (called by `std::deque<Json::Reader::ErrorInfo, ...>::_M_reallocate_map` at `0x28c8a10`) remain returning, allowing control flow to fall through error paths.
2. **Prototype Drift and Silent Failure Masking:** All ~1,285 lines of Passes 1–4 currently live inside unlinted prototype script `tools/ghidra-audit/extract_batch.py`, which swallows broad `Exception` errors in 12 places without counters and falls back to a hardcoded `/var/tmp` SQLite path. Auditing prototype code and rewriting it in Task B would invalidate exit-gate certification.
3. **Uncalibrated, High-Noise LLM-Only Auditing:** Running `LOW`-thinking Gemini Flash over raw 20-function chunks produced ~35–45 false positives out of 134 findings (primarily `-O2` alignment padding copies, indirect virtual tail-call warnings, empty-lambda registers, and DWARF-declared `void *` containers) while inconsistently missing defects on identical template code (marking `0x696070` unflagged while flagging identical `0x696700`, and repeating the miss on `0x15bbe50` vs. `0x15be350`).
4. **Template Skew and Missing Regression Locks:** Uniform random sampling over-samples repetitive template instantiations and excludes short (20–79 B) and very large (>15 KB) functions. Without deterministic regression gates, later edits can silently reintroduce fixed defects.

## Goals

1. **Promote Passes 1–4 to Production with Repository CI Checks and Telemetry:** Move Passes 1–4 into internal package `cli/src/twdll/_ghidra/`, promote shared DWARF loaders into `cli/src/twdll/database.py`, fix the stale-binary precedence defect in `twdll.domain.resolve_dwarf_exporter`, extend architecture tests to scan package subdirectories recursively, pass all repository Python and Rust checks, and replace all silent `Exception` catches with immutable per-pass telemetry.
2. **Resolve All Four Verified Systemic `batch_001` Defects:**
   * Extract file-scope and static class-member globals into an 11th DWARF SQLite table (`dwarf_globals`) and stamp them safely across initialized and uninitialized (`.bss`) data blocks without colliding with code or Pass 2 `_ZTV` vtables.
   * Fix `ptr_to_member` 8-byte vs. 16-byte sizing, global type-level `is_non_trivial_for_calls` / `is_sret` classification, and System V AMD64 eightbyte ABI classification once inside Rust `tools/twdll-dwarf/`, and apply MSB-first multi-register `Varnode` storage in Pass 3 across all 1–16-byte integer, SSE, and mixed aggregates.
   * Detect and mark external library and PLT `noreturn` targets (`std::__throw_*`, `__clang_call_terminate`, C++ runtime abort/unwind helpers) while preserving the existing false-positive `panic` guard, and verify elimination of caller fallthrough at `0x28c8a10`.
3. **Build a Lean Two-Stage Multi-Round Audit Harness:** Upgrade `tools/ghidra-audit/` with:
   * A single-slot read-only content-hashed Ghidra snapshot cache under gitignored `tools/ghidra-audit/runs/cache/` that leaves `.twdll/analyze/*.gpr` untouched.
   * Consumer-weighted, template-skeleton-deduplicated sampling spanning 20–79 B through >15 KB size strata across two modes (`discovery` and `reverify`).
   * Single-pass chunk materialization: `extract_batch.py` extracts exact `ClangFieldToken` structure-offset references alongside C pseudocode into `extracted.jsonl`, and `prescan.py` is the sole chunk writer, cross-checking exact field offsets against `dwarf_members` and `dwarf_inheritance` to emit factual `/* DWARF-FACTS: ... */` annotations and chunks of 10–15 functions for `HIGH`-thinking Gemini Flash workers.
   * Strict 1-to-1 reconciliation in `merge_findings.py` as the sole mutator of `manifest.json`, tracking explicit `"tier0_clean_skipped"` vs. `"llm_clean"` vs. `"flagged"` dispositions and enforcing a blind-slice recall circuit breaker.
4. **Lock Fixes with Three Deterministic Regression Layers and Pass the Exit Gate:** Certify all fixes via offline CI tests (Layer 1), local DWARF SQLite invariant tests (Layer 2), pinned copyright-safe Golden Set assertions plus Pass 1–4 error budgets (Layer 3), and the Section 4 Exit Gate criteria before starting Task B.

## Non-Goals

1. **Production `twdll extract` / `twdll sql` CLI & Output Schemas (Deferred to Task B):** Designing or implementing `twdll extract` CLI/MCP commands, decompiler output SQLite tables (`functions`, `symbols`, `strings`, `xrefs`, `pseudocode`), `ParallelDecompiler` auto-tuning, or resumable `.sqlite.staging` checkpoints.
2. **Persisting Passes 1–4 into `.twdll/analyze/*.gpr` (Deferred to Task B):** Changing `.twdll/analyze/*.gpr` immutability or deciding whether Passes 1–4 execute during `twdll analyze` vs. `twdll extract`.
3. **32-Bit Windows PE Auditing & IDA 9.5 Adapter (Deferred to Task B):** Auditing `attila_windows_32`, `rome2_windows_32`, or `tob_windows_32`, or implementing IDAPython extraction.
4. **Forking Ghidra 12.2 or Eliminating Irreducible Compiler/Decompiler Limitations:** Modifying Ghidra Java internals or attempting to eliminate non-bugs such as DWARF-declared `void *` containers (`VECTOR_BASE<void *>`, `Lunar::*_T`, `SAFER_REPORTER`), `lua_touserdata`, `-O2` inter-field alignment padding copies, or the seven 60–90-case switch tables where Ghidra cannot recover indirect jump tables.
5. **Function-Local `static` Variables:** Extracting function-scoped Meyer's singletons (`DW_TAG_variable` inside subprogram bodies) is excluded unless active discovery rounds surface systemic defects from them; all ~21 `batch_001` singleton hits are namespace globals or class static members.
6. **Stage 3 Frozen C++ SDK (`src/`, `CMakeLists.txt`, root `tests/`, LDoc `docs/`):** Stage 3 (the C++ extender, plugin loader, and Lua SDK) remains frozen until the CLI reverse-engineering pipeline is complete.

## Critical User Journeys (CUJs)

### CUJ 1: Modder or AI Agent Reads 64-Bit Reference Pseudocode for Hook Development

* **Given:** A local 64-bit reference decompilation produced by `cli/src/twdll/_ghidra/` and `tools/twdll-dwarf/` after Task A passes the Exit Gate.
* **When:** A modder or AI agent inspects a Warscape campaign, battle, or UI method (such as `DATABASE_TABLE<*>::clearing_packs` at `0x1de0b20`, `UIDLL::CampaignWealthTooltip::Function` at `0x2b71350`, `WARSCAPE::EMITTER_INSTANCE::update_orbital_effect` at `0x349af30`, or a method reading `CAMPAIGN_UI::s_ui`).
* **Then:** The pseudocode renders typed global singleton dereferences, named virtual method calls, typed 16-byte integer/member-function/float struct returns without `in_RDX` / `extraout_RDX` or `extraout_XMM0` artifacts, and terminated `std::__throw_*` error branches.

### CUJ 2: Maintainer Executes an Active Multi-Round Audit Batch

* **Given:** An immutable `.twdll/analyze/ghidra_attila_linux_64.gpr`, an up-to-date local DWARF SQLite database built from `tools/twdll-dwarf/`, and prior discovery history in `tools/ghidra-audit/runs/manifest.json` (which records previously audited addresses so subsequent discovery batches sample only unseen functions).
* **When:** The maintainer runs an unseen 250–300-function consumer-weighted round (`extract_batch.py` -> `prescan.py` -> `HIGH`-thinking Gemini Flash workers invoked via `PROMPT_COORDINATOR.md` and `PROMPT_WORKER.md` -> `merge_findings.py`).
* **Then:** The harness reuses or refreshes its single-slot cached Ghidra snapshot without modifying `.twdll/analyze/*.gpr`, records full provenance in `batch_meta.json`, reconciles 100% of routed review chunks and unrouted Tier 0-clean addresses in `merge_findings.py`, enforces the blind-slice recall circuit breaker, and groups findings into `(category, pattern)` clusters linked to exact C pseudocode files and line numbers.

### CUJ 3: Maintainer Fixes a Systemic Defect and Verifies Deterministic Regressions

* **Given:** A `batch_001` or newly discovered systemic defect traced to `tools/twdll-dwarf/` or `cli/src/twdll/_ghidra/`.
* **When:** The maintainer patches the production Rust or Python module, adds offline unit tests and Golden Set assertions in `golden_set.json`, and runs the three regression layers plus an isolated `--mode reverify` slice.
* **Then:** All offline CI checks, local DWARF SQLite invariants, and pinned Golden Set assertions pass with Pass 1–4 error counts within budget, and discovery `manifest.json` remains unmodified by re-verification runs.

### CUJ 4: Maintainer Certifies Readiness for Task B via the 500-Function Exit Gate

* **Given:** Active discovery rounds have resolved all known systemic defect clusters and Layers 1–3 pass.
* **When:** The maintainer executes the held-out 500-function consumer-weighted exit-gate batch.
* **Then:** The merged report confirms all Section 4 Exit Gate criteria pass and the Residual Risk Register in `tools/ghidra-audit/README.md` is updated, authorizing transition to Task B.

## Technical Plan (Architecture)

Task A comprises four layers, summarized below:

### System Architecture & Data Flow

```text
  [1. Rust DWARF Exporter (tools/twdll-dwarf/)]     [2. Production Python Engine (cli/src/twdll/)]
  attila_linux_64 (ELF64 + DWARF)                   domain.py  (backend-neutral immutable models,
          │                                                     fixed resolve_dwarf_exporter)
          ▼                                             ▲
  11 tables: dwarf_globals (UNIQUE addr),               ├──────────────────────────┐
  fixed ptr_to_member 8B/16B, global RDI/RSI            │                          │
  triviality, pre-computed dwarf_types.abi_class  database.py                  _ghidra/
          │                                       (sqlite3 ONLY:               (pyghidra/jpype ONLY:
          ▼                                        11-table DDL,                lifecycle, dwarf_types,
  runs/attila_dwarf.sqlite                         pass-scoped loaders,         dwarf_layouts,
          │                                        WAL-safe finalize)           dwarf_signatures,
          └───────────────┬───────────────────────┘                             dwarf_locals)
                          ▼                                                          │
  [3. Lean Multi-Round Audit Harness (tools/ghidra-audit/)]                          │
  .twdll/analyze/ghidra_attila_linux_64.gpr (IMMUTABLE)                              │
          │ copy-on-miss + pass-scoped load -> pass -> del                           │
          ▼                                                                          ▼
  runs/cache/snapshot.{gpr,rep} + snapshot_meta.json ◄── Pass 1–4 on cache miss ─────┘
          │ open read-only on hit
          ▼
  extract_batch.py (discovery / reverify; 70/20/10 tiers; ClangFieldToken field_refs)
          │ writes extracted.jsonl + initial batch_meta.json (never touches manifest.json)
          ▼
  prescan.py (Tier 0: syntactic signals + exact field_refs vs dwarf_members + dwarf_inheritance)
          │ sole writer of chunks/chunk_NNN.c (10–15 funcs) + prescan_report.json + tier0_summary
          ▼
  HIGH-thinking Gemini Flash workers (PROMPT_COORDINATOR.md / PROMPT_WORKER.md) ──► findings/chunk_NNN.json
          │
          ▼
  merge_findings.py (1-to-1 reconciliation, strict enums, blind-slice circuit breaker,
                     sole atomic mutator of manifest.json)
          │
          ▼
  SUMMARY.md + flagged/<cat>__<pattern>.c ──► Fix in Rust / _ghidra/ ──► [4. 3-Layer Gates]
                                              Layer 1: cargo test/clippy + pytest (offline CI)
                                              Layer 2: pytest -m local_dwarf (local SQLite)
                                              Layer 3: pytest -m local_golden (golden_set.json)
```

### 1. Rust DWARF Exporter: Single Source of Truth for Globals & System V ABI

All DWARF parsing, type-graph triviality, and System V AMD64 ABI classification live inside `tools/twdll-dwarf/` so both Ghidra (Task A) and IDA 9.5 (Task B) consume identical pre-computed facts without duplicating recursive type walkers or holding layout tables in Pass 3 Python RAM:

* **11th Table (`dwarf_globals`):** Extracts file-scope and class static-member `DW_TAG_variable` definitions outside function bodies, resolving `DW_AT_specification` / `DW_AT_abstract_origin` back to both static `DW_TAG_member` declarations and namespace-scope `extern` `DW_TAG_variable` declarations. Addresses resolve from single-operation `DW_OP_addr` (rejecting address `0` and TLS operations) with fallback to an ELF `.symtab` data-symbol index built during the existing vtable scan. Rows deduplicate across compilation units under a `UNIQUE(address)` constraint and seed the `is_referenced` reachability BFS so global singleton types and their pointees are always materialized.
* **Exact `ptr_to_member` Sizing:** When `DW_AT_byte_size` is absent, Rust resolves `ptr_to_member` byte size from the unwrapped target type (`16` bytes for `subroutine` targets, `8` bytes for data-member pointers), eliminating the current default-16 corruption on 8-byte data-member pointers.
* **Global Triviality & `is_sret` Resolution:** Instead of an unreliable per-CU copy-constructor heuristic or per-function-only overrides that leave vtable declarations and by-value parameters corrupted, Rust resolves `is_non_trivial_for_calls` globally after CU merging. Across all concrete functions returning a `<= 16`-byte aggregate whose first integer/pointer parameter has a concrete DWARF register, Rust tallies `RDI` (direct return) vs. `RSI` (hidden `sret` pointer) witnesses per return type, combined with a cross-CU and `.symtab` check for concrete user-defined copy/move constructors, before running fixed-point propagation and finalizing `is_sret`.
* **Pre-Computed `dwarf_types.abi_class`:** During Rust's global type-graph pass—where direct members and non-virtual base-class inheritances are already resident—Rust classifies every type into a compact System V AMD64 ABI class (`int`, `sse`, `int,int`, `sse,sse`, `sse,int`, `int,sse`, or `memory`). Aggregates exceeding 16 bytes, non-trivial-for-calls types, virtual bases, `long double` (x87), packed/unaligned fields, or conflicting layouts classify directly as `memory`.

### 2. Production Python Engine & Architectural Guardrails

The repository `import-linter` contract enforces strict one-way layering with `twdll._ghidra` and `twdll.database` as independent siblings (`sqlite3` allowed only in `database.py`; `pyghidra`/`jpype` allowed only in `_ghidra`).

* **Root Fix for Exporter Precedence (`domain.py`):** Fix `resolve_dwarf_exporter(ctx)` directly in `cli/src/twdll/domain.py` to prefer an in-repo Cargo release build when present and newer than (or instead of a stale binary in) `.twdll/init/tools/twdll-dwarf/`, eliminating hardcoded POSIX binary bypasses.
* **Backend-Neutral Immutable Domain Models (`domain.py`):** Keep `domain.py` free of Ghidra internals and mutable containers. Add `DwarfGlobalRow`, `abi_class` on `DwarfTypeRow`, a `DwarfPassName` enum, and immutable `DwarfPassStats` / `DwarfInjectionReport` telemetry models with `to_dict()` serialization matching existing CLI result models. Pass-input argument bundles stay private inside `cli/src/twdll/_ghidra/`.
* **Centralized SQLite Layer (`database.py`):** Extend the staging schema to 11 tables (adding `dwarf_globals` with `UNIQUE(address)` and `dwarf_types.abi_class`), promote `load_dwarf_vtable_function_signatures` and `load_dwarf_globals`, and reuse `finalize_extract_staging_db` for WAL-safe atomic database publication.
* **Internal Package `cli/src/twdll/_ghidra/` (`SPEC.md §3` Exception):** Split `cli/src/twdll/_ghidra.py` into a six-file internal package (`lifecycle`, `dwarf_types`, `dwarf_layouts`, `dwarf_signatures`, `dwarf_locals`, and `__init__`) so every file remains concise for single-read LLM context and every function satisfies Ruff's complexity caps (`C901`, `PLR0912`, `PLR0913`, `PLR0915`). This is an intentional scoped exception to `SPEC.md §3`'s flat-module layout for the PyGhidra engine, with strict acyclic intra-package layering (`lifecycle` <- `dwarf_types` <- `dwarf_layouts` | `dwarf_signatures` | `dwarf_locals` <- `__init__`). `_ghidra` exports lifecycle helpers and the four standalone pass entrypoints only—never an eager `apply_dwarf_passes` wrapper—so Layer 2 callers (`extract_batch.py` now, `twdll.extract` in Task B) preserve sequential `load -> pass -> del` streaming RAM discipline alongside Ghidra's 8 GiB JVM heap.
* **Pass 1–4 Fixes & Telemetry:**
  * *Pass 1 (`dwarf_types`):* Model 16-byte member-function pointers as a shared two-field struct (`/dwarf/_abi/__ptr_to_member_func`) and 8-byte data-member pointers as signed 64-bit offsets, driven by unwrapped target type rather than raw size fallback.
  * *Pass 2 (`dwarf_layouts`):* Stamp `dwarf_globals` across both initialized and uninitialized (`.bss`) non-executable memory blocks while verifying full end-address block containment, zero instruction overlap across the full byte range, zero overlap with `_ZTV` vtables or prior globals, skipping declaration/stub 1-byte shells, and creating properly namespaced Ghidra symbols. Record all caught errors including the previously silent union fallback catch.
  * *Pass 3 (`dwarf_signatures`):* Perform O(1) lookup on `abi_class` to build MSB-first 1-register and 2-register `Varnode` storage across all 1–16-byte integer, SSE, and mixed returns and parameters (including atomic all-or-nothing register pool reservation on stack spills). Mark external library and PLT `noreturn` targets by walking Ghidra external functions before internal/thunk functions, stripping any ELF `Library` namespace (`libstdc++.so.6::`, `libc.so.6::`, `libgcc_s.so.1::`) and checking original imported names, while preserving the existing non-external `panic` `setNoReturn(False)` guard.
  * *Pass 4 (`dwarf_locals`):* Replace all silent exception catches with structured error telemetry.
* **Recursive Architecture Tests:** Expand `cli/tests/test_architecture.py` to scan all package subdirectories recursively (asserting `_ghidra/` is non-empty) for banned ignore comments and toolchain-resolution violations.

### 3. Lean Multi-Round Audit Harness

The audit workflow in `tools/ghidra-audit/` consolidates into three pipeline scripts plus a small shared helper module and `pytest` gate suites:

* **Single-Slot Read-Only Ghidra Snapshot Cache:** Store one self-validating snapshot under gitignored `tools/ghidra-audit/runs/cache/snapshot.{gpr,rep}` paired with `snapshot_meta.json`. The content hash covers the base `.twdll/analyze/ghidra_attila_linux_64.meta.json`, `runs/attila_dwarf.sqlite` SHA-256, and the SHA-256 of `cli/src/twdll/{domain.py,database.py,_ghidra/*.py}`. On miss, rebuild in a staging directory (pruning any orphaned staging directory first), run Passes 1–4 via sequential `load -> pass -> del`, save `pass_stats`, and atomically swap into place. On hit, open read-only without copying the `.rep` tree or touching `.twdll/analyze/*.gpr`.
* **Two-Mode Extraction (`extract_batch.py`):** Remove all duplicate Pass 1–4 code and the hardcoded `/var/tmp` fallback. Support `--rebuild-dwarf-db` (using `domain.resolve_dwarf_exporter` + WAL-safe `finalize_extract_staging_db`) and two extraction modes:
  * `discovery`: Consumer-weighted (70% Core Modding including un-namespaced Warscape classes, 20% Engine, 10% STL/Middleware diagnostic-only), balanced-bracket template-skeleton-capped (`max_per_skeleton = 2`), stratified across 20–79 B (10%), 80–15,000 B (85%), and >15,000 B (5%).
  * `reverify`: Explicit address targets (`--from-batch`, `--addrs-file`, or `--sql-filter` for targeted ABI probes), writing under `runs/reverify/<id>/` and never mutating discovery `manifest.json`.
  * While decompiling each function, `extract_batch.py` walks Ghidra's live `ClangFieldToken` markup (~20 lines) to capture exact `(struct_name, offset, field_name)` references and emits a single flat `extracted.jsonl` plus initial `batch_meta.json`. It never writes `chunks/` and never mutates `manifest.json`.
* **Deterministic Tier 0 Pre-Scanner & Sole Chunker (`prescan.py`):** Stdlib-only Python (plus `sqlite3` via `twdll.database` or read-only query) that reads `extracted.jsonl` and is the **sole** writer of `chunks/chunk_NNN.c` (10–15 functions per chunk). It scans unambiguous syntactic anomaly tokens (`in_R*`, `in_XMM*`, `extraout_*`, `unaff_*`, `**(code **)(*`, `Could not recover jumptable`, `halt_baddata`, raw global offset arithmetic, `ok=False`), cross-checks exact `ClangFieldToken` `field_refs` against flattened `dwarf_members` + `dwarf_inheritance` in SQLite to label proven alignment gaps (`PROVEN_PADDING_GAP`) without regex C-type guessing, injects single-line `/* DWARF-FACTS: ... */` comments on the line immediately below each unchanged function header, writes `prescan_report.json`, and updates `batch_meta.json` with `tier0_summary`. All Tier 0 anomalies are routed to Tier 1 without a second deduplication pass (preventing real bugs from being hidden behind false-positive template siblings), plus a deterministic 15% blind slice of Tier 0-clean functions in discovery rounds (or 100% of functions on the final 500-function exit gate).
* **Tier 1 LLM Review Execution:** An AI coding-agent coordinator following `PROMPT_COORDINATOR.md` dispatches `chunks/chunk_NNN.c` to parallel `HIGH`-thinking Gemini Flash worker subagents using `PROMPT_WORKER.md`. Workers write `findings/chunk_NNN.json` matching strict allowed enums and exact `reviewed_addrs`.
* **Strict Reconciliation & Sole Manifest Owner (`merge_findings.py`):** Reads `batch_meta.json`, `prescan_report.json`, `chunks/chunk_*.c`, and `findings/chunk_*.json` (supporting `--batch-dir` for both discovery and reverify directories). Enforces exact 1-to-1 `reviewed_addrs` match and rejects invalid enum values rather than coercing them. Evaluates the **blind-slice recall circuit breaker**: any `HIGH` or fixable `MEDIUM` defect found inside the blind Tier 0-clean slice fails the round's Tier 0 recall check, requiring a new Tier 0 detector or full review of that stratum. Only when zero chunks are missing or invalid—and the batch is a discovery or exit-gate run—does `merge_findings.py` atomically update `manifest.json` with explicit `"llm_clean"`, `"flagged"`, or `"tier0_clean_skipped"` statuses.

### 4. Three-Layer Regression Gates & 500-Function Exit Gate

1. **Layer 1 — Offline CI Tests:** Rust `cargo test --offline` + `cargo clippy --offline` across `tools/twdll-dwarf/`; Python `ruff format --check`, `ruff check`, `basedpyright` (strict), `lint-imports`, and `pytest` across `cli/tests/` (including mocked `_ghidra/` tests and `domain.resolve_dwarf_exporter` tests) and `tools/ghidra-audit/test_audit_harness.py`. No brittle Python regex scraping of Rust source files—schema parity is enforced by `cargo test`, `test_database.py`, and Layer 2 against the compiled binary's SQLite output.
2. **Layer 2 — Local DWARF SQLite Invariants (`pytest -m local_dwarf`):** Executed via `tools/ghidra-audit/test_local_gates.py` against `runs/attila_dwarf.sqlite` without launching Ghidra. Asserts 11 populated `dwarf_*` tables, `0` concrete `<= 16`-byte functions with `param[0]` in `RDI` and `is_sret = 1`, presence and `is_referenced = 1` reachability of the five required Warscape singletons (`EMPIRE_APP_MODULE::s_instance`, `CAMPAIGN_UI::s_ui`, `FRONT_END_UI::s_ui`, `WS_CONTEXT::m_e_engine`, `WS_PLATFORM::m_pf_device`) with non-zero unique addresses, `byte_size == 16` for subroutine `ptr_to_member` types and `byte_size == 8` for non-subroutine `ptr_to_member` types, and valid `abi_class` values on aggregate returns.
3. **Layer 3 — Pinned Golden Set & Pass 1–4 Error Budgets (`pytest -m local_golden`):** Executed via `tools/ghidra-audit/test_local_gates.py` against copyright-safe `tools/ghidra-audit/golden_set.json` (storing only hex addresses, qualified names, defect tags, declarative `require_decompile_ok` / `must_contain_regex` / `must_not_contain_regex` rules, and explicit numeric `pass_error_budgets`). Reuses the single-slot cached Ghidra snapshot and asserts all pinned functions and `snapshot_meta.json` Pass 1–4 `error_counts` pass within budget.
4. **Canonical Exit Gate Criteria:** After active 250–300-function discovery rounds resolve all surfacing systemic defect classes, run a held-out 500-function consumer-weighted batch (`--mode discovery --count 500 --exit-gate`) with **100%** LLM review of all 500 functions (~35–42 chunks of 12 functions, run once, eliminating exit-gate blind-slice recall uncertainty). Passing requires all five conditions:
   * `0` unresolved `HIGH` systemic clusters (`>= 2` distinct template skeletons, excluding diagnostic-only STL/Middleware).
   * `0` unresolved `MEDIUM` clusters (`>= 3` distinct template skeletons, excluding diagnostic-only STL/Middleware and patterns explicitly listed in the accepted-irreducible allowlist).
   * All Layer 1, Layer 2, and Layer 3 regression tests pass.
   * Pass 1–4 `error_counts` remain within the numeric budgets in `golden_set.json`.
   * The Residual Risk Register in `tools/ghidra-audit/README.md` documents every accepted irreducible limitation.

## Alternatives Considered

### Alternative A: Lean 3-Fix Pass with a Single Confirmation Batch

* **Pros:** Smaller upfront audit harness footprint (no Tier 0 pre-scanner, snapshot cache, or multi-round orchestration); faster initial time to the first post-fix check.
* **Cons:** Stops after one confirmation batch without targeted ABI probes or iterative pre-scanner refinement across unseen size and namespace strata, raising the risk of carrying undiscovered systemic defects into Task B.
* **Reason for Rejection:** Each 250–300-function discovery round requires only ~8–15 `HIGH`-thinking Gemini Flash chunks (~5–10 minutes wall-clock using the cached snapshot, bounded to 2–4 active rounds before the one-time ~35–42-chunk exit gate), providing substantially broader defect-class coverage before locking Task B's extraction architecture.

### Alternative B: Keep Passes 1–4 in Prototype `extract_batch.py` (or a Single Flat `_ghidra.py`) Until Task B

* **Pros:** Defers Ruff complexity decomposition and `basedpyright --strict` refactoring until Task B.
* **Cons:** Certifying prototype code and rewriting ~1,500 lines later invalidates exit-gate certification and risks introducing rewrite regressions; consolidating all four passes into one ~2,000-line flat `_ghidra.py` degrades LLM context efficiency and edit isolation compared to six ~200–350-line modules.
* **Reason for Rejection:** Promoting Passes 1–4 into internal package `cli/src/twdll/_ghidra/` at the start of Task A certifies the production engine directly and enforces all repository lint, typing, and import-layering checks throughout auditing.

### Alternative C: LLM-Only Review with Expanded Prompt Exclusions (No Tier 0 Pre-Scanner)

* **Pros:** Eliminates `prescan.py` maintenance and unit tests; relies solely on prompt rules in `PROMPT_WORKER.md`.
* **Cons:** Leaves LLM workers without deterministic SQLite padding proofs on `field_0x*` offsets, consumes LLM tokens on repetitive syntactic checks, and retains proven false-negative inconsistency on identical template code (`0x696070` vs. `0x696700` and `0x15bbe50` vs. `0x15be350` in `batch_001`).
* **Reason for Rejection:** Pairing `ClangFieldToken` field extraction with a stdlib-only Tier 0 pre-scanner and flattened `dwarf_members` + `dwarf_inheritance` proofs eliminates padding guesswork deterministically while focusing `HIGH`-thinking review on real anomalies and a circuit-breaker-guarded blind slice.

### Alternative D: 1,000-Function Exit Gate & Raw Aggregate Clean-Percentage Criterion

* **Pros:** Matches `batch_001`'s raw 1,000-function sample size and yields a single headline clean-rate percentage.
* **Cons:** Doubles final review turnaround (~80+ chunks at 12 functions/chunk), while raw clean percentages are skewed by repetitive template instantiations and irreducible `-O2` / `void *` non-bugs.
* **Reason for Rejection:** A held-out 500-function consumer-weighted, template-deduplicated exit gate with 100% LLM review and defect-class convergence (`0` unresolved `HIGH` systemic clusters, `0` unresolved fixable `MEDIUM` clusters, and 100% Layer 1–3 regression pass) achieves stronger semantic coverage at half the review cost.

### Alternative E: Compute System V ABI Classification in Python Pass 3 (or Defer 8–16-Byte SSE Structs)

* **Pros:** Avoids adding an `abi_class` column to `dwarf_types` in Rust `tools/twdll-dwarf/`.
* **Cons:** Running a recursive struct/inheritance ABI classifier in Python Pass 3 forces retaining `dwarf_members` and `dwarf_inheritance` in Python RAM past Pass 2 (breaking streaming RAM release alongside the 8 GiB JVM heap), complicates Ruff complexity caps, requires duplicating ABI classification for IDA 9.5 in Task B, and deferring SSE structs leaves Warscape 3D math structs (`get_orientation_world` at `0x349af30`) mistyped.
* **Reason for Rejection:** Rust `dwarf.rs` already holds global types, direct members, and direct inheritances in memory simultaneously during layout resolution; computing `dwarf_types.abi_class` once in Rust solves integer, SSE, and mixed 1–16-byte aggregates in one place and keeps Pass 3 an O(1) table lookup.

### Alternative F: Mutate `.twdll/analyze/*.gpr` In-Place or Maintain a Multi-Slot LRU-2 Cache

* **Pros:** In-place mutation avoids ~1 GiB of cache disk space; an LRU-2 cache keeps one extra prior snapshot on disk.
* **Cons:** In-place mutation violates `.twdll/analyze/*.gpr` immutability before Task B reconciles `analyze` vs. `extract` ownership. An LRU-2 cache adds eviction machinery, `.last_used` tracking, copy-on-hit `.rep` I/O, and 3–4× peak disk (~3–4 GiB) for a second snapshot that no Task A workflow opens (before/after comparisons use saved `SUMMARY.md` and Golden Set runs).
* **Reason for Rejection:** A single-slot read-only content-hashed snapshot (`runs/cache/snapshot.{gpr,rep}` + `snapshot_meta.json`) preserves `.twdll/analyze/*.gpr` immutability, bounds peak disk to ~2× a single snapshot during staging rebuilds, and eliminates copy-on-hit latency.

## Risks & Mitigations

* **Peak Memory Exhaustion (8 GiB JVM Heap + Python Rows):** Holding all 11 DWARF tables in Python RAM alongside Ghidra's `-Xmx8G` JVM risks OOM on 16 GiB hosts. **Mitigation:** `twdll._ghidra` exports only standalone pass functions; `extract_batch.py` streams `load -> pass -> del` per pass, and Rust pre-computes `dwarf_types.abi_class` so `dwarf_members` and `dwarf_inheritance` are freed immediately after Pass 2.
* **SQLite WAL Data Loss on Staging Rebuild:** `init_extract_staging_db` enables `PRAGMA journal_mode = WAL`; renaming the staging file directly without checkpointing drops `-wal` frames. **Mitigation:** Every DWARF DB rebuild finalizes through `twdll.database.finalize_extract_staging_db`, which executes `PRAGMA wal_checkpoint(TRUNCATE)`, switches to `journal_mode = DELETE`, unlinks `-wal`/`-shm`, and swaps atomically.
* **Ghidra `.lock` Contention & Stale Snapshot Cache:** Opening a shared `.gpr` concurrently can hit Ghidra lock contention; omitting `database.py` or `domain.py` from the cache key serves stale snapshots silently. **Mitigation:** Hash `.meta.json`, `attila_dwarf.sqlite`, `domain.py`, `database.py`, and `_ghidra/*.py` into `snapshot_meta.json`, prune orphaned `_staging/` directories on startup, run local Ghidra commands sequentially, and verify read-only open lock behavior during Phase 3 (falling back to copying into `ghidra_temp_dir` if PyGhidra creates lock files on read-only open).
* **Silent ABI or `.bss` Memory Corruption:** Defaulting missing `ptr_to_member` `DW_AT_byte_size` to 16 corrupts 8-byte data-member pointers; stamping globals without full-range checks can overwrite code or `_ZTV` vtables; skipping `dwarf_inheritance` causes false padding proofs. **Mitigation:** Rust resolves `ptr_to_member` 8/16-byte sizing and `abi_class` using both direct members and non-virtual inheritances; `stamp_dwarf_globals` checks `end_addr` block equality, range-wide instruction emptiness, and `_ZTV`/prior-global disjointness; `prescan.py` uses exact `ClangFieldToken` references + flattened inheritance.
* **Tier 0 False Negatives & LLM Worker Drift:** Filtering Tier 0-clean functions in discovery rounds can hide semantic defects with no syntactic marker; LLM workers can omit addresses or emit invalid enums. **Mitigation:** Discovery rounds enforce a zero-tolerance blind-slice recall circuit breaker (any `HIGH` or fixable `MEDIUM` miss fails Tier 0 recall), the final 500-function exit gate runs 100% LLM review, and `merge_findings.py` rejects invalid chunks before mutating `manifest.json`.
* **Copyright Leakage in Git:** Accidentally committing decompiled C pseudocode or verbatim game code snippets inside `golden_set.json` or `sample_errors` violates repository copyright boundaries. **Mitigation:** All batch runs and snapshots stay under gitignored `tools/ghidra-audit/runs/`; `golden_set.json` stores only hex addresses, symbol names, and declarative regex/boolean assertions; `sample_errors` records only exception types and DWARF/Ghidra addresses.
* **Residual Risk Register Location & Schema:** All accepted irreducible compiler/decompiler limitations certified at the Exit Gate are recorded in a `## Residual Risk Register` section inside `tools/ghidra-audit/README.md` with columns: `Pattern (category/pattern)`, `Example Addresses`, `Root Cause`, `Why Irreducible in Task A`, and `Deferred Action (Task B or Won't Fix)`.

## Open Questions

* **Open Question 1 (`0x28c8a10` `noreturn` caller flow):** Does marking external library and PLT `std::__throw_*` functions `setNoReturn(True)` eliminate decompiler fallthrough in `DecompInterface` for `0x28c8a10` (while keeping `"Non-Returning Functions - Discovered"` disabled in `_ghidra/lifecycle.py` for Task B), or does Ghidra also require clearing/repairing caller fallthrough overrides? (Verified empirically in Phase 2/4; fallback repair API pre-specified in Detailed Implementation Plan Item 11.)
* **Open Question 2 (`dwarf_globals` address resolution coverage & BFS growth):** On `attila_linux_64`, what fraction of static/global `DW_TAG_variable` DIEs carry a direct `DW_OP_addr` expression vs. requiring `.symtab` fallback by linkage name, do any global addresses overlap Pass 2 `_ZTV` vtable addresses, and by how much does seeding `global.type_id` expand `is_referenced` type cardinality? (Measured empirically in Phase 4 when running `--rebuild-dwarf-db`.)

## Non-Functional Requirements

* **Copyright & Git Safety:** Never commit copyrighted decompiled C pseudocode, `.gpr`/`.rep` databases, or SQLite extracts to git. All batch runs, cached snapshots, and rebuilt DWARF DBs stay under gitignored `tools/ghidra-audit/runs/`. Git-tracked `golden_set.json` stores only hex addresses, symbol names, declarative assertions, and numeric error budgets.
* **Workspace Immutability & Disk Budget:** `.twdll/analyze/ghidra_attila_linux_64.{gpr,rep}` remains read-only throughout Task A. The single-slot cache under `runs/cache/` occupies ~0.8–1.2 GiB steady state (~2× during atomic staging rebuilds).
* **Target Performance & Iteration Latency:** Target runtimes (to be measured in Phase 4): ~20–30 s to rebuild `runs/attila_dwarf.sqlite`; ~2–3 minutes to populate `runs/cache/snapshot.{gpr,rep}` when `domain.py`, `database.py`, `_ghidra/*.py`, or `attila_dwarf.sqlite` changes; seconds to ~1–2 minutes for cached 250–300-function extractions, Layer 2–3 pytest runs, and Tier 0 pre-scans.
* **Strict Code Quality:** All production code in `cli/src/twdll/` and `cli/tests/` must pass `ruff format --check`, `ruff check`, `basedpyright` (strict, zero `# type: ignore` / `# pyright: ignore`), `lint-imports`, and `pytest`; all Rust code in `tools/twdll-dwarf/` must pass `cargo test --offline` and `cargo clippy --all-targets --offline -- -D warnings`.

## Additional Resources

* [SPEC.md](../../SPEC.md) — CLI workspace, toolchain, and step-directory architecture.
* [dwarf_extraction_and_decompiler_integration_design.md](dwarf_extraction_and_decompiler_integration_design.md) — Rust `twdll-dwarf` DWARF extraction and Passes 1–4 decompiler integration specification.
* `tools/ghidra-audit/runs/batches/batch_001/SUMMARY.md` *(local gitignored artifact)* — 1,000-function baseline audit data.

## Future Improvements

* **Task B Production CLI & Extraction Pipeline:** Wire the verified `cli/src/twdll/_ghidra/` passes into `twdll extract` / `twdll sql`, reconcile `.gpr` persistence and the `_ghidra/` package layout in `SPEC.md`, add `ParallelDecompiler` CPU+RAM auto-tuning and resumable `.sqlite.staging` checkpoints, and run the 32-bit Windows PE audit (`attila_windows_32`, `rome2_windows_32`, `tob_windows_32`) plus IDA 9.5 adapter (reusing `dwarf_types.abi_class`).
* **Expanded ABI Edge-Case Probes:** Extend `golden_set.json` and Layer 2 SQLite invariants whenever Task B or later 32-bit/64-bit rounds uncover new compiler idioms.

--------------------------------------------------------------------------------

## Detailed Implementation Plan

### Phasing & Checkpoints

Every file created, modified, or deleted in Task A (24 files total) is partitioned into five incremental phases. Each phase leaves the repository buildable and verified before moving on.

| Phase | Target Files (Items) | Checkpoint Gate |
| :--- | :--- | :--- |
| **Phase 1: Rust DWARF Exporter, 11-Table Schema & Root Domain Fixes** | Items 1–5, 13–14 (`vtable.rs`, `db.rs`, `dwarf.rs`, `domain.py`, `database.py`, `test_domain.py`, `test_database.py`) | `cargo test --manifest-path tools/twdll-dwarf/Cargo.toml --offline`, `cargo clippy --manifest-path tools/twdll-dwarf/Cargo.toml --all-targets --offline -- -D warnings`, `cargo build --manifest-path tools/twdll-dwarf/Cargo.toml --release --offline`, `pytest cli/tests/test_domain.py cli/tests/test_database.py` |
| **Phase 2: Production `_ghidra/` Package & Pass 1–4 Fixes** | Items 6–12, 15–16 (delete `_ghidra.py`; create `_ghidra/{__init__,lifecycle,dwarf_types,dwarf_layouts,dwarf_signatures,dwarf_locals}.py`; modify `test_architecture.py`; create `test_ghidra_dwarf.py`) | `ruff format --check cli/`, `ruff check cli/`, `basedpyright --project cli/pyproject.toml`, `lint-imports --config cli/pyproject.toml`, `pytest cli/tests` |
| **Phase 3: Lean Audit Harness & Pytest Local Gates** | Items 17–24 (`conftest.py`, `audit_common.py`, `extract_batch.py`, `prescan.py`, `merge_findings.py`, `golden_set.json`, `test_audit_harness.py`, `test_local_gates.py`, `PROMPT_WORKER.md`, `PROMPT_COORDINATOR.md`, `README.md`) | `ruff format --check cli/ tools/ghidra-audit/`, `ruff check --select E,F,W,I,UP,B tools/ghidra-audit/`, `pytest cli/tests tools/ghidra-audit/test_audit_harness.py` |
| **Phase 4: Local DWARF Rebuild, Layer 2–3 Gates & `batch_001` Re-Verification** | Local execution only (no git-tracked edits except tightening `pass_error_budgets` in `golden_set.json` if needed) | `extract_batch.py --rebuild-dwarf-db`, `pytest tools/ghidra-audit/test_local_gates.py -m local_dwarf`, `pytest tools/ghidra-audit/test_local_gates.py -m local_golden`, and `batch_001` `--mode reverify` confirming all 4 systemic defect classes are eliminated |
| **Phase 5: Active Discovery Rounds & 500-Function Exit Gate** | Local audit rounds + Residual Risk Register update in `tools/ghidra-audit/README.md` | 250–300-function discovery rounds + targeted SQL-filter probes, then final `--mode discovery --count 500 --exit-gate` passing all Section 4 Exit Gate criteria |

---

### Target Files to Create / Modify / Delete

#### A. Rust DWARF Exporter (`tools/twdll-dwarf/`) — Phase 1

##### 1. `tools/twdll-dwarf/src/vtable.rs` (Modify)
* **Type / Scope:** Rust ELF symbol scanner and unit tests.
* **Proposed Changes:**
  * Extend `ElfVtableIndex` with `pub data_sym_by_name: HashMap<String, u64>` and `pub ctor_sym_names: HashSet<String>` populated during the existing `obj.symbols()` pass in `build_elf_vtable_index`:
    * Insert non-zero-address `SymbolKind::Data` and `SymbolKind::Unknown` symbols (excluding `_ZTV*` and `_ZTI*`) into `data_sym_by_name` to supply the address fallback for `DW_TAG_variable` entries lacking `DW_OP_addr`.
    * Insert defined non-zero-address function symbols whose Itanium mangled name contains `C1E` or `C2E` (complete/base constructors) into `ctor_sym_names` for cross-CU copy/move constructor verification.
  * Add a `#[cfg(test)]` unit test verifying `data_sym_by_name` excludes `_ZTV*`/`_ZTI*` and zero-address symbols while retaining data symbols.

##### 2. `tools/twdll-dwarf/src/db.rs` (Modify)
* **Type / Scope:** Rust SQLite batch writer and offline unit tests.
* **Proposed Changes:**
  * Add `pub abi_class: Option<String>` to `TypeRow` and update `INSERT INTO dwarf_types (...)` to write `abi_class`.
  * Add `pub struct GlobalRow` (`global_id: i64`, `unit_id: i64`, `address: i64`, `linkage_name: Option<String>`, `qualified_name: String`, `type_id: i64`, `byte_size: Option<i64>`, `is_static_member: bool`) and `pub globals: Vec<GlobalRow>` on `DwarfBatch`.
  * Add prepared `INSERT INTO dwarf_globals (global_id, unit_id, address, linkage_name, qualified_name, type_id, byte_size, is_static_member) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)` inside `write_batch_to_staging_db`.
  * Increase `DbConn::verify_required_tables` minimum table count from `10` to `11`.
  * Add `#[cfg(test)]` unit tests using a test-only `SQLITE_OPEN_CREATE` constant and a unique temp file in `std::env::temp_dir()` (cleaned up on drop; no external `tempfile` crate required under `--offline`) that execute the 11-table DDL, verify round-trip writes across all 11 `dwarf_*` tables (including `dwarf_types.abi_class` and `dwarf_globals`), verify `UNIQUE(address)` enforcement, and verify `verify_required_tables` returns an explicit error when fewer than 11 tables exist.

##### 3. `tools/twdll-dwarf/src/dwarf.rs` (Modify)
* **Type / Scope:** Rust DWARF v4 parser, ABI classifier, and unit tests.
* **Proposed Changes:**
  * **Global & Static-Member Extraction:**
    * Parse `DW_AT_location` in `walk_cu_node` and `DW_AT_linkage_name` / `DW_AT_MIPS_linkage_name` on `DW_TAG_member` (recording the member's CU section offset).
    * Track per-CU declarations in `HashMap<usize, CuGlobalDecl>` covering both static `DW_TAG_member` declarations (`DW_AT_declaration = true`, `is_static_member = true`) and non-function-scope `DW_TAG_variable` declarations (`is_static_member = false`), storing `(qualified_name, type_off, linkage_name, is_static_member)`.
    * Add a `DW_TAG_variable` arm inside `walk_cu_node` strictly for `in_function_scope == false` (CU, namespace, and class scopes; function-local statics are excluded per Non-Goal #5). Resolve `DW_AT_specification` and `DW_AT_abstract_origin` through `CuGlobalDecl`.
    * Implement a dedicated `parse_op_addr` helper returning `Some(addr)` strictly when the location expression consists of a single `gimli::Operation::Address { address }` with `address != 0` (explicitly rejecting multi-operation expressions and `DW_OP_form_tls_address` / `DW_OP_GNU_push_tls_address`). Fall back to `vtable_index.data_sym_by_name` by `linkage_name` when `parse_op_addr` returns `None`. Skip globals with no resolved non-zero address.
    * Deduplicate globals by `address` across CUs using deterministic precedence: (1) concrete non-void `type_id` (not `is_declaration` and `type_id != 1`), (2) `is_static_member = true`, (3) presence of `linkage_name`, (4) lowest `unit_id`. Populate `GlobalRow.byte_size` post-CU via `type_byte_size(&batch.types, global.type_id)` (`None` when `type_id == 1`), assign `global_id = 1..=N` sorted by `address`, and enqueue every `global.type_id` into the `is_referenced` BFS queue.
  * **Fix `ptr_to_member` Default Sizing (`dwarf.rs:1071`):**
    * Stop defaulting missing `DW_AT_byte_size` on `DW_TAG_ptr_to_member_type` to `16`. Leave `byte_size` as `None` during initial CU scan when `DW_AT_byte_size` is absent, then resolve it post-CU once `target_type_id` is interned: set `Some(16)` when `resolve_global_underlying(&batch.types, target_type_id)` has `kind == "subroutine"`, else `Some(8)`.
  * **Global Empirical Triviality & Cross-CU Copy/Move Ctor Resolution:**
    * Replace the per-CU declaration-only copy/move ctor assumption with a post-CU resolution phase before fixed-point propagation:
      * A non-artificial copy/move constructor sets `has_user_copy_or_move_ctor = true` when it has a concrete definition (`low_pc.is_some()`), a concrete `DW_AT_specification` definition in any CU, or a `linkage_name` matching a defined symbol in `vtable_index.ctor_sym_names`.
      * Across all concrete functions returning a `<= 16`-byte `struct`/`class`/`union` `T` whose first parameter is an integer/pointer type with a concrete DWARF register, tally `rdi_count` (`dwarf_reg == Some(5)`) vs. `rsi_count` (`dwarf_reg == Some(4)`) per `T`. If `rdi_count > 0` and `rsi_count == 0`, force `batch.types[T].is_non_trivial_for_calls = false` before fixed-point propagation (`dwarf.rs:2507`) and `is_sret` finalization (`dwarf.rs:2548`). Also preserve a per-function guard preventing `is_sret = true` on any concrete function with `<= 16`-byte return and `aligned_params[0].dwarf_reg == Some(5)`.
    * Update existing test `test_detect_copy_or_move_ctors_template_base_name` (`dwarf.rs:3541–3678`) so `TEMPLATE_HANDLE`'s user-defined copy constructor carries a concrete definition or `linkage_name` present in `ctor_sym_names`, and add a negative test verifying Clang 3.4.2 declaration-only `std::pair` (`= default`, no linkage name, no concrete body) remains trivial.
  * **Pre-Computed System V AMD64 `dwarf_types.abi_class`:**
    * Add `classify_sysv_abi_class(tid, &batch.types, &raw_direct_members, &raw_direct_inherits) -> Option<String>` executed right after `is_non_trivial_for_calls` fixed-point propagation, returning one of `"int"`, `"sse"`, `"int,int"`, `"sse,sse"`, `"sse,int"`, `"int,sse"`, or `"memory"`:
      * `void` -> `None`; `pointer` / `reference` / `enum` / integer `base` -> `"int"`; `float` (4 B) / `double` (8 B) -> `"sse"`; `long double` (`base` 16 B float / x87) -> `"memory"`.
      * `ptr_to_member`: `byte_size == 16` -> `"int,int"`, `byte_size == 8` -> `"int"`.
      * `struct` / `class` / `union`: return `"memory"` if `byte_size > 16`, `is_non_trivial_for_calls`, `is_declaration`, `is_stl_stub`, contains any virtual base, or contains any unaligned/packed field. For 1–16-byte trivial aggregates, recursively walk `raw_direct_inherits` (at `base_offset`) and `raw_direct_members` (including fixed-size nested arrays and bitfields -> `INTEGER`), merging bytes `[0..8)` and `[8..16)` under System V rules (`INTEGER` dominates `SSE`, padding stays `NO_CLASS`). Empty 1-byte structs with zero fields classify as `"int"`.
  * **Unit Tests:** Add Rust unit tests covering `DW_OP_addr` + `DW_AT_specification` (static member and extern variable) + TLS/`addr == 0` rejection, `is_referenced` BFS seeding from globals, `ptr_to_member` 8-byte vs. 16-byte default resolution, global `RDI`/`RSI` witness + `std::pair` triviality, and `classify_sysv_abi_class` (scalars, `std::pair`, 16-byte `ptr_to_member`, 8/12/16-byte homogeneous float/double structs, derived float structs via `raw_direct_inherits`, mixed SSE/INTEGER structs, `long double` -> `"memory"`, `> 16`-byte -> `"memory"`).

#### B. Production Python Package & Guardrails (`cli/`) — Phases 1 & 2

##### 4. `cli/src/twdll/domain.py` (Modify — Phase 1)
* **Type / Scope:** Stdlib-only backend-neutral domain models and toolchain resolver.
* **Proposed Changes:**
  * Fix `resolve_dwarf_exporter(ctx: WorkspaceContext) -> Path`: check both `ctx.paths.root.joinpath(*DWARF_EXPORTER_BUILD_REL_DIR, exe)` (`repo_bin`) and `ctx.paths.tools_dir / DWARF_EXPORTER_DIR_NAME / exe` (`staged_bin`). When both exist, prefer `repo_bin` if `repo_bin.stat().st_mtime_ns >= staged_bin.stat().st_mtime_ns`, else `staged_bin`; when only one exists, return it.
  * Add `abi_class: str | None = None` to `DwarfTypeRow`.
  * Add `@dataclass(frozen=True, slots=True) class DwarfGlobalRow` with exact fields: `global_id: int`, `unit_id: int`, `address: int`, `linkage_name: str | None`, `qualified_name: str`, `type_id: int`, `byte_size: int | None`, `is_static_member: bool`.
  * Add `class DwarfPassName(StrEnum)` (`PASS1_TYPES = "pass1_types"`, `PASS2_LAYOUTS = "pass2_layouts"`, `PASS3_SIGNATURES = "pass3_signatures"`, `PASS4_LOCALS = "pass4_locals"`).
  * Add `@dataclass(frozen=True) class DwarfPassStats` with immutable fields `pass_name: DwarfPassName`, `counts: tuple[tuple[str, int], ...]`, `error_counts: tuple[tuple[str, int], ...]`, `sample_errors: tuple[str, ...]`, plus `def to_dict(self) -> dict[str, object]` (converting count tuples to JSON-friendly dicts).
  * Add `@dataclass(frozen=True) class DwarfInjectionReport` with fields `pass1: DwarfPassStats`, `pass2: DwarfPassStats`, `pass3: DwarfPassStats`, `pass4: DwarfPassStats`, `total_errors: int`, plus `def to_dict(self) -> dict[str, object]`.
  * Do **not** place `EightbyteClass` or `_ghidra`-internal argument bundles in `domain.py`.

##### 5. `cli/src/twdll/database.py` (Modify — Phase 1)
* **Type / Scope:** Centralized `sqlite3` DDL and pass-scoped loaders.
* **Proposed Changes:**
  * Update `EXTRACT_STAGING_DDL`:
    * Add `abi_class TEXT` column to `dwarf_types`.
    * Add 11th table `dwarf_globals`:
      ```sql
      CREATE TABLE IF NOT EXISTS dwarf_globals (
          global_id INTEGER PRIMARY KEY,
          unit_id INTEGER NOT NULL,
          address INTEGER NOT NULL UNIQUE,
          linkage_name TEXT,
          qualified_name TEXT NOT NULL,
          type_id INTEGER NOT NULL,
          byte_size INTEGER,
          is_static_member INTEGER NOT NULL DEFAULT 0
      );
      ```
  * Update `load_dwarf_types` and `_row_to_dwarf_type` to read `abi_class`.
  * Add `load_dwarf_globals(staging_path: Path) -> tuple[DwarfGlobalRow, ...]` ordered by `address, global_id`.
  * Add `load_dwarf_vtable_function_signatures(staging_path: Path) -> tuple[dict[int, DwarfFunctionRow], dict[int, tuple[DwarfParameterRow, ...]]]` returning concrete and declaration functions referenced by `dwarf_vtables` along with their parameter rows.

##### 6. `cli/src/twdll/_ghidra.py` (Delete — Phase 2)
* **Type / Scope:** Existing single-file PyGhidra helper module.
* **Proposed Changes:** Delete this file and move its contents to `cli/src/twdll/_ghidra/`, preserving re-export parity on `twdll._ghidra`.

##### 7. `cli/src/twdll/_ghidra/__init__.py` (Create — Phase 2)
* **Type / Scope:** Package entrypoint and re-export surface.
* **Proposed Changes:** Define explicit `__all__` (satisfying Ruff `F401`) re-exporting all existing lifecycle helpers (`make_task_monitor`, `require_ghidra_toolchain`, `start_ghidra`, `remove_ghidra_project`, `ghidra_temp_dir`, `analyze_and_save_project`) plus the four standalone pass entrypoints (`pass1_type_shells`, `pass2_vtables_and_structs`, `pass3_signatures`, `pass4_locals_and_inlines`), the package-local input dataclasses (`DwarfLayoutInputs`, `DwarfSignatureInputs`, `DwarfLocalInputs`), and `build_injection_report(pass1, pass2, pass3, pass4) -> DwarfInjectionReport`. Do **not** export an eager `apply_dwarf_passes` wrapper.

##### 8. `cli/src/twdll/_ghidra/lifecycle.py` (Create — Phase 2)
* **Type / Scope:** PyGhidra/JVM lifecycle, Java class bridge, and pass telemetry collector.
* **Proposed Changes:**
  * Move existing `_ghidra.py` contents verbatim (preserving `_disable_dwarf_analyzer` with `"Non-Returning Functions - Discovered"` disabled).
  * Add `jclass(name: str) -> Any` and `jarray(cls_name: str, items: Sequence[Any]) -> Any` wrapping `jpype.JClass` / `jpype.JArray`.
  * Add internal `_StatsCollector` (recording named counts, named `error_counts`, and up to 10 `sample_errors` formatted as `"<context>: <ExcType>: <msg>"`, exporting an immutable `DwarfPassStats` via `.freeze()`).
  * Add `build_injection_report(pass1: DwarfPassStats, pass2: DwarfPassStats, pass3: DwarfPassStats, pass4: DwarfPassStats) -> DwarfInjectionReport`.

##### 9. `cli/src/twdll/_ghidra/dwarf_types.py` (Create — Phase 2)
* **Type / Scope:** Pass 1 DWARF type-shell materialization.
* **Proposed Changes:**
  * Implement `pass1_type_shells(program: Any, types: tuple[DwarfTypeRow, ...], enumerators: tuple[DwarfEnumeratorRow, ...]) -> tuple[dict[int, Any], dict[int, DwarfTypeRow], DwarfPassStats]`.
  * Decompose Pass 1 into helpers (`split_cpp_namespace`, `make_category_path`, `map_primitive`, `unwrap_type_row`, `unwrap_type_size`) meeting Ruff `C901 <= 8`, `PLR0912 <= 8`, `PLR0913 <= 5`, and `PLR0915 <= 30`.
  * For `kind == "ptr_to_member"`, classify strictly by unwrapping `t.target_type_id` (falling back to `t.byte_size == 16` only if `target_type_id` is `None`):
    * Subroutine target -> shared 16-byte struct `/dwarf/_abi/__ptr_to_member_func` with fields `{ void *ptr; long long adj; }`.
    * Non-subroutine target -> `LongLongDataType` (8-byte signed offset).
    * Remove `ptr_to_member` from the `void *` unresolved fallback loop; update `unwrap_type_size` to return `16` for subroutine targets and `8` for data-member pointers (or `t.byte_size`).
  * Record all enum and subroutine exceptions via `_StatsCollector.record_error(...)`.

##### 10. `cli/src/twdll/_ghidra/dwarf_layouts.py` (Create — Phase 2)
* **Type / Scope:** Pass 2 vtable synthesis, union/struct layout population, and `.bss`-safe global stamping.
* **Proposed Changes:**
  * Define `@dataclass(frozen=True) class DwarfLayoutInputs` with fields: `type_map: dict[int, Any]`, `type_rows: dict[int, DwarfTypeRow]`, `members: tuple[DwarfMemberRow, ...]`, `inheritances: tuple[DwarfInheritanceRow, ...]`, `vtables: tuple[DwarfVtableRow, ...]`, `vtable_funcs: dict[int, DwarfFunctionRow]`, `vtable_params: dict[int, tuple[DwarfParameterRow, ...]]`, `globals_: tuple[DwarfGlobalRow, ...]`.
  * Implement `pass2_vtables_and_structs(program: Any, inputs: DwarfLayoutInputs) -> DwarfPassStats` decomposed into `_build_vtable_structs` (returning stamped half-open `[start, end)` `_ZTV` ranges), `_populate_unions` (replacing the silent `_raw` catch with `_StatsCollector.record_error`), `_populate_structs`, and `stamp_dwarf_globals`.
  * In `stamp_dwarf_globals`, iterate `inputs.globals_` and resolve `dt = inputs.type_map.get(g.type_id)` and `trow = inputs.type_rows.get(g.type_id)`:
    * Skip when `dt is None`, `trow is None`, `trow.is_declaration`, `trow.is_stl_stub`, `VoidDataType`, or `length <= 0`.
    * Require `block = mem.getBlock(addr)` is not `None` (do not require `block.isInitialized()`, so `.bss` blocks qualify), `not block.isExecute()`, and `mem.getBlock(end_addr) == block` where `end_addr = addr.add(length - 1)`.
    * Skip if `[addr_int, addr_int + length)` overlaps any stamped `_ZTV` range or any previously stamped global range, or if `listing.getInstructions(AddressSet(addr, end_addr), True).hasNext()`.
    * Clear conflicting data or undefined units over `[addr, end_addr]`, call `listing.createData(addr, dt)`, and when `sym_table.getPrimarySymbol(addr)` is `None` or has `SourceType.DEFAULT`, create/resolve nested Ghidra `Namespace`s from `split_cpp_namespace(g.qualified_name)` and call `sym_table.createLabel(addr, leaf, ns, SourceType.IMPORTED)`. Record all counts and caught exceptions in `DwarfPassStats`.

##### 11. `cli/src/twdll/_ghidra/dwarf_signatures.py` (Create — Phase 2)
* **Type / Scope:** Pass 3 `noreturn` stamping and O(1) `abi_class`-driven `CUSTOM_STORAGE` function signatures.
* **Proposed Changes:**
  * Define `@dataclass(frozen=True) class DwarfSignatureInputs` with fields: `type_map: dict[int, Any]`, `type_rows: dict[int, DwarfTypeRow]`, `functions: tuple[DwarfFunctionRow, ...]`, `parameters: tuple[DwarfParameterRow, ...]`. (Zero `members` or `inheritances` retained in Pass 3.)
  * Implement `_apply_static_noreturn(program, stats)`:
    * Pass A: iterate `fm.getExternalFunctions()` **first**. Walk `getParentNamespace()` past any Ghidra `Library` namespace (`libstdc++.so.6`, `libc.so.6`, `libgcc_s.so.1`, `<EXTERNAL>`) and inspect both the stripped qualified name and `ExternalLocation.getOriginalImportedName()` against `STATIC_NORETURN_NAMES`, `std::__throw_*`, `_ZSt*__throw_*`, `__clang_call_terminate`, `_Unwind_Resume`, `__cxa_throw`, `__cxa_rethrow`, `__cxa_pure_virtual`, `_ZSt9terminatev`, `abort`, and `__assert_fail`. Call `setNoReturn(True)` on matches.
    * Pass B: iterate `fm.getFunctions(True)`. For thunks (`gf.isThunk()`), inspect `target = gf.getThunkedFunction(True)`; if `target` matches or `target.hasNoReturn()`, set `setNoReturn(True)` on both `target` and `gf`. Preserve the existing non-external, non-thunk `panic` `setNoReturn(False)` guard for functions whose last instruction is a normal return.
    * Caller-flow fallback (Open Question 1): if empirical verification on `0x28c8a10` in Phase 4 shows `DecompInterface` still falls through after `setNoReturn(True)`, add a targeted Pass B step iterating incoming `CALL` references to newly marked `noreturn` thunks and applying `ClearFlowAndRepairCmd(caller_addr, True, False)` (or `Instruction.setFlowOverride(FlowOverride.CALL_RETURN)`).
  * Implement `_build_two_reg_storage(program, lo_reg_name, hi_reg_name, total_len)` using MSB-first `jarray("ghidra.program.model.pcode.Varnode", [Varnode(hi_addr, total_len - 8), Varnode(lo_addr, 8)])`, handling all 9–16-byte widths including non-power-of-two aggregate sizes (11, 13, 14, and 15 bytes with tail lengths 3, 5, 6, and 7).
  * Route return and parameter storage through O(1) lookup on `unwrap_type_row(tid, type_rows).abi_class`:
    * Returns: `"sse"` -> `Varnode(XMM0, total_len)`; `"int"` -> `Varnode(RAX, total_len)`; `"sse,sse"` -> `[XMM1, XMM0]`; `"int,int"` -> `[RDX, RAX]`; `"sse,int"` -> `[RAX, XMM0]`; `"int,sse"` -> `[XMM0, RAX]`.
    * Parameters: handle 1–8-byte `"sse"` structs in `XMM0..XMM7`, 1–8-byte `"int"` in `RDI..R9`, and 9–16-byte `"int,int"`, `"sse,sse"`, `"sse,int"`, `"int,sse"` with atomic all-or-nothing register reservation (if either required register pool lacks capacity, spill the entire aggregate to stack without consuming a register from the other pool).
  * Implement `pass3_signatures(program: Any, inputs: DwarfSignatureInputs) -> DwarfPassStats` recording all caught exceptions in `DwarfPassStats`.

##### 12. `cli/src/twdll/_ghidra/dwarf_locals.py` (Create — Phase 2)
* **Type / Scope:** Pass 4 stack/register local variables and inline pre-comments.
* **Proposed Changes:**
  * Define `@dataclass(frozen=True) class DwarfLocalInputs` with fields: `type_map: dict[int, Any]`, `functions: tuple[DwarfFunctionRow, ...]`, `variables: tuple[DwarfVariableRow, ...]`, `inlines: tuple[DwarfInlineRow, ...]`.
  * Implement `pass4_locals_and_inlines(program: Any, inputs: DwarfLocalInputs) -> DwarfPassStats` recording all caught exceptions in `DwarfPassStats`.

##### 13. `cli/tests/test_domain.py` (Modify — Phase 1)
* **Type / Scope:** Unit tests for `cli/src/twdll/domain.py`.
* **Proposed Changes:** Add parametrized `"linux"` / `"windows"` tests for `resolve_dwarf_exporter(ctx)` covering: (a) repo `target/release` newer than `.twdll/init/tools/` returns repo build, (b) `.twdll/init/tools/` newer than repo returns staged build, (c) fallback when only one exists, and (d) `DwarfPassStats.to_dict()` / `DwarfInjectionReport.to_dict()` immutability and serialization.

##### 14. `cli/tests/test_database.py` (Modify — Phase 1)
* **Type / Scope:** Database unit tests.
* **Proposed Changes:** Update `expected_tables` to include `dwarf_globals` (11 DWARF tables + `meta`); verify `dwarf_globals.address` `UNIQUE` constraint and `dwarf_types.abi_class` column; add round-trip tests for `load_dwarf_types` (`abi_class`), `load_dwarf_globals`, and `load_dwarf_vtable_function_signatures`. Keep tests self-contained inside `cli/` (no Rust source regex scraping).

##### 15. `cli/tests/test_architecture.py` (Modify — Phase 2)
* **Type / Scope:** Architecture guardrail tests.
* **Proposed Changes:** Switch `_SRC_DIR.glob("*.py")` and `_TESTS_DIR.glob("*.py")` to `rglob("*.py")`; remove `"_ghidra.py"` from `_EXECUTION_STEP_MODULES`, expand `.config.toolchain` enforcement to include every `*.py` file under `_SRC_DIR / "_ghidra"`, and assert `list((_SRC_DIR / "_ghidra").rglob("*.py"))` is non-empty.

##### 16. `cli/tests/test_ghidra_dwarf.py` (Create — Phase 2)
* **Type / Scope:** Offline mocked unit tests for `cli/src/twdll/_ghidra/`.
* **Proposed Changes:** Test Pass 1 (`ptr_to_member` 16-byte subroutine struct vs. 8-byte data-member offset, enums, typedef/cycle unwrapping), Pass 2 (vtable synthesis, union topo-sort + `_raw` error recording, `.bss` global acceptance, `end_addr` block check, `AddressSet` instruction skip, `_ZTV`/prior-global overlap skip, `Namespace` + `SourceType.IMPORTED` label creation), Pass 3 (`abi_class`-driven returns and parameters across all 6 register classes, MSB-first `Varnode` ordering for non-power-of-two aggregate sizes 11, 13, 14, and 15 bytes, atomic mixed-pool spill rollback, `is_sret` shifting, external `Library`-namespace `noreturn` + thunk propagation + `panic` `setNoReturn(False)` preservation), Pass 4 (locals/inlines + telemetry), and `build_injection_report`.

#### C. Lean Audit Harness & Pytest Local Gates (`tools/ghidra-audit/`) — Phase 3

##### 17. `tools/ghidra-audit/conftest.py` (Create — Phase 3)
* **Type / Scope:** Pytest configuration for `tools/ghidra-audit/`.
* **Proposed Changes:** Add `Path(__file__).resolve().parent` to `sys.path` so `test_audit_harness.py` and `test_local_gates.py` import `audit_common`, `extract_batch`, `prescan`, and `merge_findings` cleanly; register `local_dwarf` and `local_golden` markers in `pytest_configure` (leaving `cli/pyproject.toml` untouched).

##### 18. `tools/ghidra-audit/audit_common.py` (Create — Phase 3)
* **Type / Scope:** Shared audit utilities (template normalization, tiering, single-slot cache, DWARF DB rebuild).
* **Proposed Changes:**
  * Implement balanced-angle-bracket `normalize_template_skeleton(name: str) -> str` (replacing nested `<...>` argument lists with `<T>` while preserving `operator<`, `operator<=`, `operator<<`, `operator->`).
  * Implement `classify_namespace_tier(qualified_name: str) -> Literal["core", "engine", "stl_middleware"]`:
    * Explicit `stl_middleware` prefix allowlist: `std::`, `__gnu_cxx::`, `boost::`, `tbb::`, `Json::`, `lua`, `tinyxml`, `zlib`, `png_`, `google::`, `rapidxml::`.
    * `engine` prefixes: `WARSCAPE::`, `UTILITYLIB::`, `CA::`, `CA_STD::`, `UTILITYDLL::`.
    * `core`: `EMPIRE*`, `UIDLL::`, `TWUI::`, `LUA_SCRIPT::`, and all remaining un-namespaced Warscape classes (such as `CAMPAIGN_UI::`, `FRONT_END_UI::`, `DATABASE_TABLE<*>::`, `UNIT::`).
  * Implement `compute_cache_key(repo_root: Path, dwarf_db_path: Path) -> str` reusing `twdll.domain.compute_file_sha256` across `.twdll/analyze/ghidra_attila_linux_64.meta.json`, `dwarf_db_path`, `cli/src/twdll/domain.py`, `cli/src/twdll/database.py`, and sorted `cli/src/twdll/_ghidra/*.py`.
  * Implement `ensure_cached_snapshot(ctx: WorkspaceContext, dwarf_db_path: Path, cache_dir: Path) -> tuple[Path, dict[str, object]]`:
    * Single slot: `cache_dir / "snapshot.gpr"` + `snapshot.rep/` + `snapshot_meta.json`.
    * On cache hit (`snapshot_meta.json["cache_key"] == key`), return `(cache_dir, meta)` without copying.
    * On cache miss, remove any leftover `cache_dir / "_staging"`, copy immutable `.twdll/analyze/ghidra_attila_linux_64.{gpr,rep}` into `_staging`, execute Passes 1–4 via sequential `database.load_*` -> `_ghidra.pass*` -> `del`, save project, write `snapshot_meta.json` (`cache_key`, `pass_stats`), and atomically replace the single slot.
  * Implement `rebuild_local_dwarf_db(ctx: WorkspaceContext, target_path: Path) -> Path`: initialize staging DB via `twdll.database.init_extract_staging_db`, resolve binary via `twdll.domain.resolve_dwarf_exporter(ctx)` and `ctx.paths.staged_binary_path(BinaryName.ATTILA_LINUX_64)`, invoke `[str(exporter), str(elf_path), str(staging_path)]`, and publish atomically via `twdll.database.finalize_extract_staging_db(staging_path, target_path, binary_sha256=compute_file_sha256(elf_path), image_base=0)`.

##### 19. `tools/ghidra-audit/extract_batch.py` (Modify — Phase 3)
* **Type / Scope:** Batch extraction driver and sampler.
* **Proposed Changes:**
  * Delete all duplicate Pass 1–4 implementations (~1,285 lines) and remove legacy `# noqa: E402` / `# type: ignore` lines and the hardcoded `/var/tmp` SQLite fallback. Resolve the database from `--dwarf-db`, defaulting to `tools/ghidra-audit/runs/attila_dwarf.sqlite`.
  * Add `--rebuild-dwarf-db` flag delegating to `audit_common.rebuild_local_dwarf_db` (exiting after rebuild when no mode is given, or rebuilding before extraction).
  * Support two modes via `--mode discovery|reverify`:
    * `discovery` (default `--count 250`; `--exit-gate` sets `--count 500` and `exit_gate: true`): consumer-weighted quotas (`70%` `core`, `20%` `engine`, `10%` `stl_middleware`), size-strata quotas (`10%` 20–79 B, `85%` 80–15,000 B, `5%` >15,000 B), and template-skeleton cap (`--max-per-skeleton 2`). Writes under `runs/batches/<batch_id>/`.
    * `reverify`: accepts `--from-batch <id>` (reading flagged addresses from `merged_findings.json`), `--addrs-file <path>` (one hex address per line), or `--sql-filter <where_clause>` + `--count <n>` (for targeted ABI probes such as `abi_class IN ('sse', 'sse,sse', 'int,int')`). Writes under `runs/reverify/<run_id>/`.
  * Never write `chunks/` and never mutate `manifest.json`.
  * Open the cached snapshot from `audit_common.ensure_cached_snapshot` read-only. For each decompiled function, walk `res.getCCodeMarkup()` (~20 lines) to collect all `ClangFieldToken` tuples `(struct_name, offset, field_name)` into `field_refs`, and write one JSON object per line to `<run_dir>/extracted.jsonl` (`addr`, `name`, `tier`, `template_skeleton`, `dwarf_bytes`, `ghidra_bytes`, `decompile_ok`, `c_code`, `field_refs`).
  * Write initial `<run_dir>/batch_meta.json` containing `git_sha`, `dwarf_db_sha256`, `cache_key`, `total_dwarf_functions` (queried from SQLite), `mode`, `exit_gate`, `seed`, `sampling_params`, `sampled_addrs`, and `pass_stats` (from `snapshot_meta.json`).

##### 20. `tools/ghidra-audit/prescan.py` (Create — Phase 3)
* **Type / Scope:** Stdlib-only Python Tier 0 pre-scanner, SQLite fact annotator, and sole chunk writer.
* **Proposed Changes:**
  * CLI flags: `--batch-dir <path>` (required), `--dwarf-db <path>` (default `runs/attila_dwarf.sqlite`), `--chunk-size 12`, `--blind-ratio 0.15`, `--seed 42`. When `batch_meta.json["exit_gate"]` is `true`, route 100% of extracted functions to Tier 1 review chunks.
  * Read `<batch_dir>/extracted.jsonl`. Scan every function for syntactic anomalies (`in_R*`, `in_XMM*`, `extraout_*`, `unaff_*`, `**(code **)(*`, `Could not recover jumptable`, `halt_baddata`, raw global offset arithmetic, `ok=False`, plus unresolved `field_0x*`).
  * For every `field_refs` entry whose `field_name` matches `^field_0x([0-9a-fA-F]+)$`: look up `struct_name` in `dwarf_types` and recursively flatten `dwarf_members` + non-virtual `dwarf_inheritance` (aborting proof if `is_declaration`, `is_stl_stub`, virtual base, or `is_layout_conflict`). Label the offset `PROVEN_PADDING_GAP` only when it falls strictly inside an inter-field gap or trailing alignment padding; otherwise keep it flagged as an anomaly.
  * Query SQLite for empty-struct parameters (`byte_size == 1`, zero members) and DWARF-declared `void *` members/params, and build a single-line `/* DWARF-FACTS: ... */` annotation placed on the line **immediately below** the unchanged `FUNC_HEADER_RE` header (`/* === [i/N] 0x... | ... === */`).
  * Route **all** Tier 0 anomaly functions (no second deduplication pass) plus a deterministic `--blind-ratio` slice of Tier 0-clean functions (unmarked to the worker) into `<batch_dir>/chunks/chunk_NNN.c` (`--chunk-size` functions per chunk).
  * Write `<batch_dir>/prescan_report.json` (`routed_anomaly_addrs`, `routed_blind_clean_addrs`, `tier0_clean_skipped_addrs`, per-address `signals` and `proven_padding_refs`) and update `<batch_dir>/batch_meta.json["tier0_summary"]`.

##### 21. `tools/ghidra-audit/merge_findings.py` (Modify — Phase 3)
* **Type / Scope:** Audit reconciliation, clustering, recall circuit breaker, and report generator.
* **Proposed Changes:**
  * Add `--batch-dir <path>` (supporting `runs/batches/`, `runs/reverify/`, or legacy positional batch ID fallback when `batch_meta.json` is absent).
  * Read `total_dwarf_functions`, `mode`, `exit_gate`, and `pass_stats` from `batch_meta.json` (remove hardcoded `TOTAL_DWARF_FUNCTIONS = 166_593`), and record `prompt_sha256` (SHA-256 of `PROMPT_WORKER.md`) into `batch_meta.json`.
  * Enforce exact 1-to-1 `reviewed_addrs` match against every `FUNC_HEADER_RE` entry in `<batch_dir>/chunks/chunk_NNN.c`. Mark a chunk invalid if any `category` (not in `VALID_CATEGORIES`), `severity` (not in `VALID_SEVERITIES`), or `suspect_layer` (not in `VALID_LAYERS`) is invalid—no silent coercion.
  * Evaluate the **blind-slice recall circuit breaker** using `prescan_report.json["routed_blind_clean_addrs"]`: flag `RECALL_GATE: FAIL` in `SUMMARY.md` if any `HIGH` or non-allowlisted `MEDIUM` finding lands on a blind-slice address.
  * Compute distinct `template_skeleton` counts per `(category, pattern)` cluster (excluding `stl_middleware` diagnostic-only hits from stop-rule failures and reading an optional `accepted_irreducible_patterns` list from `golden_set.json` for `MEDIUM` stop rules), and print explicit exit-gate pass/fail status in `SUMMARY.md`.
  * Act as the **sole** mutator of `manifest.json`: atomically update `manifest.json` only when `missing_chunks` and `invalid_chunks` are both empty and `mode == "discovery"`, recording `"flagged"`, `"llm_clean"`, and `"tier0_clean_skipped"` statuses explicitly.

##### 22. `tools/ghidra-audit/golden_set.json` (Create — Phase 3)
* **Type / Scope:** Layer 3 copyright-safe declarative regression fixture (git-tracked, zero decompiled C).
* **Proposed Changes:** Store a structured JSON object with:
  * `accepted_irreducible_patterns`: list of `(category, pattern)` pairs accepted as irreducible limitations (initially empty, populated as documented in the Residual Risk Register).
  * `pass_error_budgets`: initial numeric upper bounds checked against `snapshot_meta.json["pass_stats"]` (`pass1_types: {"total": 0}`, `pass2_layouts: {"vtbl_stamp_err": 0, "globals_err": 0, "union_raw_err": 50, "member_oob": 500, "bitfield_insert_err": 500}`, `pass3_signatures: {"unthunk_err": 0, "sig_err": 200}`, `pass4_locals: {"local_err": 500, "inline_err": 200}` — tightened to observed clean post-fix counts in Phase 4).
  * `functions`: array of `{ "address", "name", "defect_class", "require_decompile_ok", "must_contain_regex", "must_not_contain_regex" }` seeded with concrete `batch_001` targets mapped explicitly to all four defect classes:
    1. *Untyped global singletons:* `0x57d0a0` (`EMPIRE_APP_MODULE::s_instance`), `0x2a1f850` (`CAMPAIGN_UI::s_ui`), `0x2cdedb0` (`FRONT_END_UI::s_ui`), `0x3095a40` (`WS_CONTEXT::m_e_engine`), `0x30a6180` (`WS_PLATFORM::m_pf_device`).
    2. *16-byte integer/member-function returns & `is_sret`:* `0x1de0b20` (`DATABASE_TABLE<...>::clearing_packs` calling `_M_insert_unique`), `0x2b71350` (`UIDLL::CampaignWealthTooltip::Function` calling `TWUI::cc_find_function`).
    3. *8–16-byte SSE float-struct returns:* `0x349af30` (`WARSCAPE::EMITTER_INSTANCE::update_orbital_effect` calling `get_orientation_world`).
    4. *External/PLT `noreturn` fallthrough:* `0x28c8a10` (`std::deque<Json::Reader::ErrorInfo, ...>::_M_reallocate_map` calling `std::__throw_bad_alloc`).

##### 23. `tools/ghidra-audit/test_audit_harness.py` & `tools/ghidra-audit/test_local_gates.py` (Create — Phase 3)
* **Type / Scope:** Offline harness tests (`test_audit_harness.py`) and Layer 2/3 local pytest gates (`test_local_gates.py`).
* **Proposed Changes:**
  * `test_audit_harness.py` (offline CI, `tmp_path` fixtures): test `audit_common` (balanced-bracket `normalize_template_skeleton` with nested templates & `operator<<`, `classify_namespace_tier` including un-namespaced Warscape classes, single-slot `compute_cache_key` sensitivity to `domain.py` + `database.py` + `_ghidra/*.py`), `extract_batch` (70/20/10 tier weighting, 10/85/5 size strata, skeleton cap, `reverify` isolation, zero `manifest.json` writes), `prescan` (`ClangFieldToken` `field_refs` vs. flattened `dwarf_members` + `dwarf_inheritance` padding proof, `/* DWARF-FACTS */` placement preserving `FUNC_HEADER_RE`, blind-slice routing & 100% exit-gate mode), and `merge_findings` (strict enum rejection, exact 1-to-1 `reviewed_addrs`, blind-slice recall circuit breaker, atomic `manifest.json` write with `"tier0_clean_skipped"` vs. `"llm_clean"`, `--batch-dir` support).
  * `test_local_gates.py` (local-only markers):
    * `@pytest.mark.local_dwarf` (Layer 2): assert 11 populated `dwarf_*` tables in `runs/attila_dwarf.sqlite`; `0` concrete `<= 16`-byte functions (unwrapping typedef/cv chains) with `param[0].dwarf_reg == 5` and `is_sret == 1`; all five required Warscape singletons present in `dwarf_globals` with non-zero unique addresses and `is_referenced == 1` types; subroutine `ptr_to_member` rows have `byte_size == 16` and non-subroutine `ptr_to_member` rows have `byte_size == 8`; valid `abi_class` values populated.
    * `@pytest.mark.local_golden` (Layer 3): open the cached snapshot via `audit_common.ensure_cached_snapshot`, decompile every function in `golden_set.json`, evaluate `require_decompile_ok`, `must_contain_regex`, and `must_not_contain_regex`, and assert `snapshot_meta.json["pass_stats"]` is within `golden_set.json["pass_error_budgets"]`.

##### 24. `tools/ghidra-audit/PROMPT_WORKER.md`, `PROMPT_COORDINATOR.md`, and `README.md` (Modify — Phase 3)
* **Type / Scope:** Tier 1 prompts and audit harness documentation + Residual Risk Register.
* **Proposed Changes:**
  * `PROMPT_WORKER.md`: Update for Gemini Flash (`HIGH` thinking) on chunks of 10–15 functions; document how to interpret `/* DWARF-FACTS: ... */` annotations; list the allowed `VALID_CATEGORIES`, `VALID_SEVERITIES`, and `VALID_LAYERS` values and exact `reviewed_addrs` rules enforced by `merge_findings.py`.
  * `PROMPT_COORDINATOR.md`: Update orchestration for `extract_batch.py` -> `prescan.py` -> parallel `HIGH`-thinking Gemini Flash worker subagents -> `merge_findings.py`, including checking the blind-slice recall circuit breaker and exit-gate pass/fail status.
  * `README.md`: Document `--rebuild-dwarf-db`, `extract_batch.py` (`discovery` and `reverify`), `prescan.py`, `merge_findings.py`, Layer 2/3 pytest commands, Tier 1 worker execution, and add the `## Residual Risk Register` section template populated in Phase 5.

---

### Verification Plan

* **Phase 1 — Rust & Core Schema (Layer 1, Offline):**
  * Run with local NixOS `sqlite3` library paths in `RUSTFLAGS` / `LD_LIBRARY_PATH`:
    * `cargo test --manifest-path tools/twdll-dwarf/Cargo.toml --offline` (all existing 15 crate tests—12 in `dwarf.rs`, 2 in `layout.rs`, 1 in `vtable.rs`—with updated `test_detect_copy_or_move_ctors_template_base_name`, plus new `vtable.rs`, `db.rs`, and `dwarf.rs` tests pass)
    * `cargo clippy --manifest-path tools/twdll-dwarf/Cargo.toml --all-targets --offline -- -D warnings`
    * `cargo build --manifest-path tools/twdll-dwarf/Cargo.toml --release --offline`
  * `.twdll/uv/venv/bin/pytest cli/tests/test_domain.py cli/tests/test_database.py`
* **Phase 2 — Production Python Package & Guardrails (Layer 1, Offline):**
  * `.twdll/uv/venv/bin/ruff format --check cli/`
  * `.twdll/uv/venv/bin/ruff check cli/`
  * `.twdll/uv/venv/bin/basedpyright --project cli/pyproject.toml`
  * `.twdll/uv/venv/bin/lint-imports --config cli/pyproject.toml`
  * `.twdll/uv/venv/bin/pytest cli/tests`
* **Phase 3 — Audit Harness Offline Verification (Layer 1, Offline):**
  * `.twdll/uv/venv/bin/ruff format --check cli/ tools/ghidra-audit/`
  * `.twdll/uv/venv/bin/ruff check --select E,F,W,I,UP,B tools/ghidra-audit/`
  * `.twdll/uv/venv/bin/pytest cli/tests tools/ghidra-audit/test_audit_harness.py`
* **Phase 4 — Local DWARF DB, Layer 2–3 Gates & `batch_001` Re-Verification:**
  * `.twdll/uv/venv/bin/python tools/ghidra-audit/extract_batch.py --rebuild-dwarf-db` (also logs Open Question 2 metrics: `DW_OP_addr` vs. `.symtab` counts and `is_referenced` expansion)
  * `.twdll/uv/venv/bin/pytest tools/ghidra-audit/test_local_gates.py -m local_dwarf` (pass across all rows in `dwarf_functions` and `dwarf_globals`)
  * `.twdll/uv/venv/bin/pytest tools/ghidra-audit/test_local_gates.py -m local_golden` (all pinned `golden_set.json` assertions and Pass 1–4 `error_counts` budgets pass; verifies Open Question 1 on `0x28c8a10`; zero mutation of `.twdll/analyze/*.gpr`)
  * `.twdll/uv/venv/bin/python tools/ghidra-audit/extract_batch.py --mode reverify --from-batch batch_001` followed by `prescan.py` and `merge_findings.py --batch-dir <reverify_dir>` confirming elimination of all four systemic `batch_001` defect classes without mutating the discovery `manifest.json`.
* **Phase 5 — Active Discovery Rounds & 500-Function Exit Gate:**
  * Run 250–300-function unseen discovery rounds (`.twdll/uv/venv/bin/python tools/ghidra-audit/extract_batch.py --mode discovery --count 250`) and targeted ABI probes (`--mode reverify --sql-filter ...`), fixing any newly identified systemic gaps and extending `golden_set.json` and `prescan.py` after each round.
  * Run the final held-out exit-gate batch: `.twdll/uv/venv/bin/python tools/ghidra-audit/extract_batch.py --mode discovery --count 500 --exit-gate` -> `prescan.py` (100% Tier 1 routing) -> `HIGH`-thinking Gemini Flash workers -> `merge_findings.py`. Verify all Section 4 Exit Gate criteria pass and update `## Residual Risk Register` in `tools/ghidra-audit/README.md`.

--------------------------------------------------------------------------------

*Drafted and reviewed with EGM*
