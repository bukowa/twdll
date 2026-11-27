# `tools/ghidra-audit` — 64-Bit Ghidra Quality Gateway & Audit Pipeline

Lean two-stage pipeline for extracting DWARF-injected Ghidra C pseudocode (`extract_batch.py`), deterministic Tier 0 anomaly/fact pre-scanning (`prescan.py`), parallel `HIGH`-thinking Gemini Flash review, and strict reconciliation (`merge_findings.py`), backed by a single-slot read-only Ghidra snapshot cache (`runs/cache/snapshot.{gpr,rep}`) and 3-layer `pytest` gates.

## Commands

1. **Rebuild local DWARF SQLite DB (`runs/attila_dwarf.sqlite`):**
   ```bash
   .twdll/uv/venv/bin/python tools/ghidra-audit/extract_batch.py --rebuild-dwarf-db
   ```

2. **Run Layer 2 & Layer 3 local `pytest` gates:**
   ```bash
   .twdll/uv/venv/bin/pytest tools/ghidra-audit/test_local_gates.py -m local_dwarf
   .twdll/uv/venv/bin/pytest tools/ghidra-audit/test_local_gates.py -m local_golden
   ```

3. **Extract a discovery batch (250 functions) or 500-function exit gate:**
   ```bash
   .twdll/uv/venv/bin/python tools/ghidra-audit/extract_batch.py --mode discovery --count 250 --seed 2
   .twdll/uv/venv/bin/python tools/ghidra-audit/extract_batch.py --mode discovery --count 500 --exit-gate --seed 99
   ```

4. **Extract a non-polluting re-verification run:**
   ```bash
   .twdll/uv/venv/bin/python tools/ghidra-audit/extract_batch.py --mode reverify --from-batch batch_001
   .twdll/uv/venv/bin/python tools/ghidra-audit/extract_batch.py --mode reverify --sql-filter "rt.abi_class IN ('sse', 'sse,sse', 'sse,int', 'int,sse')" --count 50
   ```

5. **Tier 0 pre-scan, Tier 1 worker review, and merge:**
   ```bash
   .twdll/uv/venv/bin/python tools/ghidra-audit/prescan.py --batch-dir <RUN_DIR>
   # Dispatch Gemini Flash HIGH-thinking workers per PROMPT_COORDINATOR.md
   .twdll/uv/venv/bin/python tools/ghidra-audit/merge_findings.py --batch-dir <RUN_DIR>
   ```

## Residual Risk Register

Campaign totals across `Attila` (`166,593` concrete DWARF functions): **1,750** audited functions (`batch_001`: 1,000, `batch_002`: 250, `batch_003` exit gate: 500, plus `reverify_001`: 131), **18** pinned regression assertions in `golden_set.json`, and **9 / 166,593** (`0.0054%`) Pass 3 signature-application errors.

| Pattern / Limitation | Category | Root Cause | Disposition | Golden Set Allowlist |
| :--- | :--- | :--- | :--- | :--- |
| `>70 KB` function 60s decompile timeouts (`0x13e3700`, `0x2e9f880`) | `TRUNCATION` | Ghidra Decompiler 60s per-function timeout on massive generated functions (`73–83 KB`). | Accepted (callers can raise per-call timeout if needed). | `("TRUNCATION", "decompile_timeout")` |
| Type propagation non-convergence (`0x1047e30` `CAMPAIGNAI_HLP_DEBUG_VIEW::draw`) | `ABI_REG` / `OTHER` | Ghidra Decompiler internal iteration limit (`Type propagation algorithm not settling`). | Accepted (Ghidra Decompiler limitation). | `("ABI_REG", "extraout_return_register_from_bad_vcall_type")`, `("OTHER", "type_propagation_failure")` |
| Adjusting thunks (`_ZThn*` / `_ZTvn*`) use `void *this` | `OTHER` / `PARAMS` | Incoming `RDI` points to a secondary base at `+N` bytes; Ghidra cannot represent negative-offset `Derived *` parameters without corrupting fields/vptrs. | Accepted intentional trade-off (`dwarf_signatures.py`). | `("OTHER", "untyped_thunk_this_pointer")`, `("PARAMS", "untyped_thunk_this_param")` |
| GCC IPA-CP dead-`this` elimination (`0x1597af0`, `0x2ee4a60`) | `PARAMS` | GCC optimizes away unused `this` in staticized callers while DWARF keeps `object_pointer`. | Accepted (GCC DWARF / `-O2` ABI mismatch). | — |
| Polymorphic base-pointer narrowing (`this_00[1]` / `m_impl.m_object`) | `OTHER` / `STRUCT_FIELD` | DWARF local/member types use base pointers (`ComponentCallbacks *`, `CAMPAIGN_MISSION_OBJECTIVE_IMPL *`) without runtime downcast info. | Accepted (Ghidra casts / DWARF static type limitation). | `("OTHER", "base_pointer_overlay")`, `("STRUCT_FIELD", "base_ptr_out_of_bounds_indexing")` |
| Indirect functor/switch jumptable recovery failures | `TRUNCATION` | Ghidra cannot statically recover indirect tail-jumps through functor tables or 60–90-case switches. | Accepted (Ghidra Decompiler limitation). | `("TRUNCATION", "unrecovered_jumptable")`, `("TRUNCATION", "could_not_recover_jumptable")`, `("TRUNCATION", "jumptable_recovery_failed")` |
| Empty 1-byte C++ tag structs (`random_access_iterator_tag`, empty lambdas) | `ABI_REG` / `PARAMS` | SysV AMD64 ABI passes `MEMORY`-like empty tags in 0 registers when no `DW_AT_location` exists. | Partially mitigated (`void` `storage_kind=none` skipped); remaining 1B structs accepted. | — |
| Forward-declared DWARF types & `_Hashtable` aligned-node storage | `STRUCT_FIELD` | DWARF omits definitions for `is_declaration=1` or `__aligned_membuf` internals. | Accepted (DWARF source limitation). | — |
| Variadic C functions lacking `DW_TAG_unspecified_parameters` (`luaL_error`) | `OTHER` | Compiler omitted `DW_TAG_unspecified_parameters` in DWARF (`is_variadic=0` across all 166,593 functions). | Accepted (DWARF source limitation). | `("OTHER", "missing_callee_varargs")` |
| 9 Pass 3 `sig_apply_err` storage-size mismatches (`0.0054%`) | `PARAMS` | Ghidra `InvalidInputException` when `std::_Hashtable::_M_emplace` or 12B return/param size mismatches `VariableStorage`. | Accepted (`<= 200` budget in `golden_set.json`). | — |
