# Next Session Kickoff Prompt

Copy-paste the prompt below to start a fresh conversation focused on the Step 3A Automated DWARF-to-Ghidra Quality Audit and Steps 3B/3C:

```text
Read AGENTS.md, SPEC.md, and docs/designs/dwarf_extraction_and_decompiler_integration_design.md first.

Current repo state on branch `re-cli`:
- `.twdll/analyze/ghidra_attila_linux_64.gpr` is already analyzed and verified (NEVER edit or delete `.twdll/` or `twdll.toml`; always copy `.gpr`/`.rep/` to `/var/tmp` via `ghidra_temp_dir` for experiments).
- `tools/twdll-dwarf/` (Rust extractor), `domain.py`, and `database.py` (`dwarf_*` schema + 4 pass loaders) are implemented.
- Offline NixOS Cargo cache for `tools/twdll-dwarf/`:
  `nix-shell -p cargo rustc sqlite --run "CARGO_HOME=/home/buk/.gemini/antigravity-cli/brain/c3fafa7f-69db-4744-a315-d8256b8eb829/scratch/dwarf_bench/ddbug/cargo_home cargo test --offline --manifest-path tools/twdll-dwarf/Cargo.toml"` (and `cargo build --release --offline ...`).

Agenda (follow the AGENTS.md step-by-step approval gate before editing production files):
1. Fix the recursive multi-level EBO base check in `tools/twdll-dwarf/src/dwarf.rs:2173` (`5,898` false `is_layout_conflict = 1` members on `CA::EMPTY_BASE_OPTIMISATION`, `std::basic_string::_Alloc_hider`, etc.), add a unit test, and rebuild `tools/twdll-dwarf` release binary.
2. Build and run a **Step 3A Automated Decompilation Quality Auditor** script in `<appDataDir>/brain/<conversation-id>/scratch/` against a `/var/tmp` copy of `ghidra_attila_linux_64.gpr` and `twdll-dwarf` SQLite output:
   - Audit SQLite struct completeness (false `is_layout_conflict`, hollow structs, missing `_vptr`).
   - Run Pass 1–4 injection (with namespace `CategoryPath` splitting, recursive non-virtual base promotion, typed `<Class>_vtbl` slots, `AL`/`AX`/`EAX`/`RAX` return sizing, 64-bit register-root collision tracking, and stack frame bounds guard) and scan `DecompInterface` C++ output across stratified game functions (`EMPIREBATTLE::`, `EMPIRECAMP::`, `EMPIRE::`, `CA::`, `UTILITYLIB::`) for unresolved offsets (`+ 0x...`, `.field_0x...`), raw virtual calls (`(**(code **)(*...))`), and `CONCAT`/`extraout`/`unkfloat` artifacts, fixing issues case by case.
3. Once Step 3A pseudocode quality is signed off, implement Step 3B (`twdll extract`) and Step 3C (`twdll sql`) incrementally with all 5 Python checks (`ruff format --check`, `ruff check`, `basedpyright`, `lint-imports`, `pytest`).
```
