# Next Session Kickoff Prompt

> [!IMPORTANT]
> **Prompt Maintenance Rule:** After fixing any bug or completing any step, **immediately update this file** to mark that bug/step as **`[DONE]`** with its verified before/after metrics, and advance **`(NEXT)`** to the next item.

Copy-paste the prompt below to start a fresh conversation focused on **Step 5** (`twdll extract` spec formalization + Step 3B / 3C implementation). **Steps 1–4 are already completed.**

```text
Read AGENTS.md, SPEC.md, and docs/designs/dwarf_extraction_and_decompiler_integration_design.md first.

Current repo state on branch `re-cli` (HEAD at `f8409d1`):
- `.twdll/analyze/ghidra_attila_linux_64.gpr` + `.rep/` exist and are verified (NEVER edit or delete `.twdll/` or `twdll.toml`; always copy `.gpr`/`.rep/` to `/var/tmp` via `twdll._ghidra.ghidra_temp_dir(HostOS.LINUX)` for experiments).
- `tools/twdll-dwarf/` (Rust, 14/14 unit tests passing, 59.89s, 875 MiB SQLite DB) already includes:
  - Recursive multi-level EBO base check (`CA::EMPTY_BASE_OPTIMISATION` / `_Alloc_hider` conflicts: 5,898 -> 0).
  - `.eh_frame` CFI CFA stack-offset normalization + `DW_OP_reg6` (`RBP`) fix (valid negative entry-`RSP` stack locals: 7,091 -> 210,821 / 210,908, 99.96%).
  - Prologue-spilled parameter guard (`stack_offset < 8` -> `storage_kind = "none"`; 0 invalid stack params).
  - Exact Itanium `is_non_trivial_for_calls` separated from `byte_size > 16` (`9,195` non-trivial `<= 16B` structs; `f.is_sret` checks `is_non_trivial_for_calls || byte_size > 16`), plus `strip_template_args` in `detect_copy_or_move_ctors` so template classes (`WARSCAPE::TEMPLATE_HANDLE<53>`) with user copy/move constructors are marked `is_non_trivial_for_calls = 1`.
  - Recursive multi-level tail-padding base member promotion (`159` classes).
  - Unstubbed STL container bases (`std::_Rb_tree`, `std::_Vector_base`, `std::_Deque_base`, `std::_List_base`, `std::_Bvector_base`; eliminated all 24 `DATABASE_TABLE::_M_t.field_0x...` clusters).
  - Lazy declaration-only virtual vtable slot signatures (`is_declaration = 1`, `address = NULL` in `dwarf_functions` + `dwarf_parameters`, deduplicated by `(declaring_class_tid, slot_index)`).
  - Bitfield DWARF v4 normalization (`DW_AT_bit_offset` MSB-relative vs `DW_AT_data_bit_offset` LSB-relative), Clang 3.4.2 negative `DW_AT_bit_offset = -bit_size` non-bitfield guard (`resolve_member_spec`), bit-interval layout tracking, and multi-arm union support in `dwarf.rs` / `layout.rs`.
  - Canonical declaration parameter ordering (`align_concrete_params`) fixing Clang 3.4.2 `-O2` reordered `DW_TAG_formal_parameter` DIEs while preserving unmatched concrete `is_artificial` parameters (`vtt` / `__vtt_parm` in `RSI` on Itanium C2/D2 ctors/dtors; `0` functions with dropped parameters vs. Step 3).
- **Step 1 (Already Completed in `80c70a1` — False `noreturn` Truncation Fixed)**:
  - Disabled `"Non-Returning Functions - Discovered"` in `cli/src/twdll/_ghidra.py:_disable_dwarf_analyzer` (keeping `"Non-Returning Functions - Known"` enabled) + test in `cli/tests/test_analyze.py` (`80c70a1`, 54/54 pytest tests and all 5 Python checks passing).
- **Step 2 (Already Completed in `2e65957` — Pure-Virtual Vtable Slot Signatures Linked)**:
  - `twdll-dwarf` emits `8,925` declaration `dwarf_functions` (`is_declaration = 1`), reducing `dwarf_vtable_entries` `function_id IS NULL` from `8,253+ -> 175` (`5` pure-virtual, `170` non-pure). `WARSCAPE::WS_ENGINE` slots 50/51 link to `WARSCAPE::WS_SCENE *` / `const WARSCAPE::WS_SCENE *` and `WARSCAPE::WS_SCENE` is marked `is_referenced = 1`.
- **Step 3 (Already Completed in `2e65957` — Bitfields & Unions Populated)**:
  - SQLite (`59.63s`, `2.15 GiB` peak RSS, `874.3 MiB`): referenced non-union conflicts `794 -> 0`, referenced bitfield conflicts `470 -> 0` (`685` non-conflict referenced bitfields, `0` negative byte offsets), `113` referenced non-stub unions with `542` non-conflict arms (`0` conflicts), hollow referenced structs `> 1B` `21 -> 14`.
- **Step 4 (Already Completed in `f8409d1` + `/var/tmp/step4_work/audit_ghidra.py` — Signatures, `sret`, Thunks, & Final `flash` Audit)**:
  - **Three Ghidra invariants verified in `audit_ghidra.py` (MUST be carried into `cli/src/twdll/_ghidra.py` in Step 3B):**
    1. **Pass 1 (`subroutine` return types):** Call `fd.setReturnType(ret_dt)` on every `kind == "subroutine"` `FunctionDefinitionDataType` from `target_type_id` (fixes `parse_fn record_from_stream` 1-byte `CONCAT71` pointer truncation).
    2. **Pass 3 (un-thunk before `updateFunction`):** Call `gh_func.setThunkedFunction(None)` when `gh_func.isThunk()` is true before `updateFunction(...)` (`31` unthunked, `0` errors; fixes `CA::String::~String` `CHECKED_ARRAY<...PARAMETER_DEFINITION...>` collision: `785/785` clean).
    3. **Pass 4 (`first_use_offset > 0` for register locals):** Skip register locals with `first_use_offset <= 0` (`1,949` skipped; prevents entry-PC `RSI` locals like `rhs_itr` from evicting `CA::operator==(String, char *)` 2nd arg: `33/33` `CA::operator==` calls have 2 args).
  - **Two `twdll-dwarf` fixes in `tools/twdll-dwarf/src/dwarf.rs`:**
    1. `strip_template_args` in `detect_copy_or_move_ctors`: `WARSCAPE::TEMPLATE_HANDLE<53>` `is_non_trivial_for_calls = 1`, `WS_SCENE_NODE_RIGID_V2::register_needed` `is_sret = 1` (all 7 calls in `SIEGE_VEHICLE_DISPLAY::register_siege_vehicles @ 0xd7f120` pass `RIGID_HANDLE *` sret cleanly).
    2. `align_concrete_params`: `FACTION::is_character_visible_to_faction @ 0xee28a0` and `CAMPAIGN_PATHFINDER::search @ 0x18b2dc0` canonically ordered; `FACTION::FACTION @ 0xf59c40` preserves `void **vtt` in `RSI` (`0` `in_RSI` reads; `in` artifacts `68 -> 35`).
  - **Final `audit_ghidra.py` + `flash` verification (`132/132` functions decompiled in `542.93s`, `HWM = 5.37 GiB`):** `has_this_member: 100`, `has_typed_vptr_call: 16`, `0` `halt_baddata` or `unaff_*`, `0` `CONCAT71(extraout_*, ...)`, `0` single-arg `CA::operator==`, `0` `CHECKED_ARRAY` collisions on `CA::String::~String`, and `0` regressions.

Environment & reusable scratch artifacts:
- Offline NixOS Cargo cache for `tools/twdll-dwarf/`:
  `nix-shell -p cargo rustc sqlite --run "CARGO_HOME=/home/buk/.gemini/antigravity-cli/brain/c3fafa7f-69db-4744-a315-d8256b8eb829/scratch/dwarf_bench/ddbug/cargo_home cargo test --offline --manifest-path tools/twdll-dwarf/Cargo.toml"` (and `cargo build --release --offline ...`).
- Project Python: `.twdll/uv/venv/bin/python`.
- Reusable Step 4 scratch scripts, DB & reports (copy into your new `<appDataDir>/brain/<conversation-id>/scratch/` before editing):
  - `/home/buk/.gemini/antigravity-cli/brain/8a3afe86-17fc-4611-96c8-b29e3a6641bd/scratch/audit_sqlite.py`
  - `/var/tmp/step4_work/audit_ghidra.py`
  - `/var/tmp/step4_work/attila_dwarf_step4.sqlite` (875 MiB, extracted with Step 4 `twdll-dwarf`)
  - `/var/tmp/step4_work/step4_audit_report.txt`
  - `/var/tmp/step4_work/large_funcs_sample.txt` (80,928 lines, 22 large functions)

Known non-bugs empirically verified in DWARF (do not chase):
- `CA::read_pixels @ field_0x14` (108 hits): 4 bytes of trailing struct alignment padding at `0x14..0x18` inside `CA::PixelAccessorBase` (`base@0..8`, `wide@8..12`, `high@12..16`, `stride@16..20`) copied during 32-byte struct copy.
- `EMPIRE::EMPIRE_MP` (`m_player_slots.m_slots.m_data` `+ 0xd8`/`+ 0x70`) and `UNIT_TRACKER::update_awareness` (`m_order_queue.m_queue.m_data`): raw `card8 m_data[...]` placement-new byte buffers in CA's C++ source.
- `MP_STEAM::ISteamRemoteStorage` / `ISteamFriends` / `ISteamUser`: external `libsteam_api.so` forward declarations (`is_declaration = 1`, no DWARF definition in `attila_linux_64`).
- `-O2` x87/SIMD/pixel-packing idioms (`psraw`, `packssdw`, `CONCAT12`, `in_FPUControlWord`, unrolled memcpy), and GCC `-O2` multi-bit word loads across packed bitfields (`imposter_data->field_0x11c`, `this->field_0x2a8`/`field_0x2aa`, `header->field_0x13`).

Goal & ordered agenda (Steps 1–4 are already completed; start at Step 5 and follow AGENTS.md step-by-step approval gate before editing production files):
1. **[DONE in `80c70a1`] Fix False `noreturn` Truncation**.
2. **[DONE in `2e65957`] Link Pure-Virtual Interface Vtable Slot Signatures** (`8,925` decl functions emitted; `function_id IS NULL` `8,253+ -> 175`; `WS_ENGINE::_vptr->scene()` -> `WS_SCENE *`; `ENTITY_TRACKER @ 0xf0` eliminated).
3. **[DONE in `2e65957`] Populate Bitfields & Unions** (`0` referenced non-union/bitfield/union conflicts in SQLite; `3,921` bitfields + `113` unions / `542` arms placed in Pass 2 with `0` errors; verified by `flash` subagent with zero regressions).
4. **[DONE in `f8409d1` + `/var/tmp/step4_work/audit_ghidra.py`] Fix `CA::operator==`, `CA::String::~String`, Template `sret`, Subroutine Return Types, & Clang `-O2` Param Ordering + Final `flash` Audit** (`14/14` Rust tests, `132/132` DecompInterface funcs in `542.93s`, `0` single-arg `CA::operator==`, `0` `CHECKED_ARRAY` collisions, `0` regressions).
5. **Step 5 (NEXT) — Formalize `twdll extract` Spec & Implement Step 3B / 3C**:
   - Recommend/run `/egm:egm-brainstorm twdll-extract` (or update `docs/designs/dwarf_extraction_and_decompiler_integration_design.md` + `/egm:egm-review`) to lock the decompiler-extracted SQLite tables (`ELF64` + `PE32`), bulk/parallel/selective `DecompInterface` pseudocode strategy, and immutable `.gpr` vs. caching trade-offs, then implement Step 3B (`twdll extract`, carrying over the 3 Pass 1/3/4 Ghidra invariants from Step 4) and Step 3C (`twdll sql`) with all 5 Python checks (`ruff format --check`, `ruff check`, `basedpyright`, `lint-imports`, `pytest`).
```
