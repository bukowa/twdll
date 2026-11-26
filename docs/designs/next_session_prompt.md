# Next Session Kickoff Prompt

> [!IMPORTANT]
> **Prompt Maintenance Rule:** After fixing any bug or completing any step, **immediately update this file** to mark that bug/step as **`[DONE]`** with its verified before/after metrics, and advance **`(NEXT)`** to the next item.

Copy-paste the prompt below to start a fresh conversation focused on **Step 4** (`CA::operator==` & `CA::String::~String` + final `flash` audit) through **Step 5** (`twdll extract` & `twdll sql`). **Steps 1, 2, and 3 are already completed.**

```text
Read AGENTS.md, SPEC.md, and docs/designs/dwarf_extraction_and_decompiler_integration_design.md first.

Current repo state on branch `re-cli` (HEAD at `80c70a1` + uncommitted Step 2 & Step 3 changes in `tools/twdll-dwarf/src/dwarf.rs` and `tools/twdll-dwarf/src/layout.rs`):
- `.twdll/analyze/ghidra_attila_linux_64.gpr` + `.rep/` exist and are verified (NEVER edit or delete `.twdll/` or `twdll.toml`; always copy `.gpr`/`.rep/` to `/var/tmp` via `twdll._ghidra.ghidra_temp_dir(HostOS.LINUX)` for experiments).
- `tools/twdll-dwarf/` (Rust, 11/11 unit tests passing, 59.63s, 2.15 GiB peak RSS, 874.3 MiB SQLite DB) already includes:
  - Recursive multi-level EBO base check (`CA::EMPTY_BASE_OPTIMISATION` / `_Alloc_hider` conflicts: 5,898 -> 0).
  - `.eh_frame` CFI CFA stack-offset normalization + `DW_OP_reg6` (`RBP`) fix (valid negative entry-`RSP` stack locals: 7,091 -> 210,821 / 210,908, 99.96%).
  - Prologue-spilled parameter guard (`stack_offset < 8` -> `storage_kind = "none"`; 0 invalid stack params).
  - Exact Itanium `is_non_trivial_for_calls` separated from `byte_size > 16` (`9,195` non-trivial `<= 16B` structs; `f.is_sret` checks `is_non_trivial_for_calls || byte_size > 16`).
  - Recursive multi-level tail-padding base member promotion (`159` classes).
  - Unstubbed STL container bases (`std::_Rb_tree`, `std::_Vector_base`, `std::_Deque_base`, `std::_List_base`, `std::_Bvector_base`; eliminated all 24 `DATABASE_TABLE::_M_t.field_0x...` clusters).
  - Lazy declaration-only virtual vtable slot signatures (`is_declaration = 1`, `address = NULL` in `dwarf_functions` + `dwarf_parameters`, deduplicated by `(declaring_class_tid, slot_index)`).
  - Bitfield DWARF v4 normalization (`DW_AT_bit_offset` MSB-relative vs `DW_AT_data_bit_offset` LSB-relative), Clang 3.4.2 negative `DW_AT_bit_offset = -bit_size` non-bitfield guard (`resolve_member_spec`), bit-interval layout tracking, and multi-arm union support in `dwarf.rs` / `layout.rs`.
- **Step 1 (Already Completed in `80c70a1` — False `noreturn` Truncation Fixed)**:
  - Disabled `"Non-Returning Functions - Discovered"` in `cli/src/twdll/_ghidra.py:_disable_dwarf_analyzer` (keeping `"Non-Returning Functions - Known"` enabled) + test in `cli/tests/test_analyze.py` (`80c70a1`, 54/54 pytest tests and all 5 Python checks passing).
- **Step 2 (Already Completed in `tools/twdll-dwarf/src/dwarf.rs` — Pure-Virtual Vtable Slot Signatures Linked)**:
  - `twdll-dwarf` emits `8,925` declaration `dwarf_functions` (`is_declaration = 1`), reducing `dwarf_vtable_entries` `function_id IS NULL` from `8,253+ -> 175` (`5` pure-virtual, `170` non-pure). `WARSCAPE::WS_ENGINE` slots 50/51 link to `WARSCAPE::WS_SCENE *` / `const WARSCAPE::WS_SCENE *` and `WARSCAPE::WS_SCENE` is marked `is_referenced = 1`.
- **Step 3 (Already Completed in `tools/twdll-dwarf/src/dwarf.rs` & `layout.rs` + scratch `audit_ghidra.py` — Bitfields & Unions Populated)**:
  - SQLite (`59.63s`, `2.15 GiB` peak RSS, `874.3 MiB`): referenced non-union conflicts `794 -> 0`, referenced bitfield conflicts `470 -> 0` (`685` non-conflict referenced bitfields, `0` negative byte offsets), `113` referenced non-stub unions with `542` non-conflict arms (`0` conflicts), hollow referenced structs `> 1B` `21 -> 14`.
  - Scratch `audit_ghidra.py` (`130/132` functions decompiled; note `SIEGE_VEHICLE_DISPLAY::register_siege_vehicles @ 0xd7f120` still timed out at 120s — bump or isolate if needed):
    - Pass 1 (`93.58s`, `1.84 GiB`): `722,704` types mapped.
    - Pass 2 (`208.24s`, `5.02 GiB` HWM): `12,504` `<Class>_vtbl` structs (`166,669` slots, `7,562` stamped), `113` unions (`542` arms, `0` errors), `233,534` struct members + `3,921` bitfields placed (`0` bitfield/union/replace errors, `member_oob = 2`).
    - Pass 3 (`589.12s`, `5.18 GiB` HWM): `166,593 / 166,593` (100.0%, `0` errors) `CUSTOM_STORAGE` signatures applied.
    - Pass 4 (`941.91s`, `5.25 GiB` HWM): `82,010` stack + `247,248` register locals + `1,456,976` `CommentType.PRE` inline comments.
    - `DecompInterface` (`537.44s`, `5.25 GiB` HWM): `has_this_member: 104`, `has_typed_vptr_call: 13`.
  - Verified by `flash` subagent on `step3a_audit_report.txt` and `large_funcs_sample.txt` (71,512 lines): `CA::MemFileIn` union (`pos`/`c16`/`i16`), `CUTSCENE_CAMERA_KEYFRAME_SCRIPT::parse` unions (`0x30`/`0x34`), `EMPIRE_MP_PLAYER_INFO` union (`0xd8` `m_members_union`), and `IMPOSTER_DATA` / `ENTITY_TRACKER` single-bit bitfields all resolve cleanly with zero regressions in `PENDING_BATTLE`, `FACTION`, or `BATTLE_ENV`.

Environment & reusable scratch artifacts:
- Offline NixOS Cargo cache for `tools/twdll-dwarf/`:
  `nix-shell -p cargo rustc sqlite --run "CARGO_HOME=/home/buk/.gemini/antigravity-cli/brain/c3fafa7f-69db-4744-a315-d8256b8eb829/scratch/dwarf_bench/ddbug/cargo_home cargo test --offline --manifest-path tools/twdll-dwarf/Cargo.toml"` (and `cargo build --release --offline ...`).
- Project Python: `.twdll/uv/venv/bin/python`.
- Reusable scratch scripts & DB (copy into your new `<appDataDir>/brain/<conversation-id>/scratch/` before editing):
  - `/home/buk/.gemini/antigravity-cli/brain/8a3afe86-17fc-4611-96c8-b29e3a6641bd/scratch/audit_sqlite.py`
  - `/home/buk/.gemini/antigravity-cli/brain/8a3afe86-17fc-4611-96c8-b29e3a6641bd/scratch/audit_ghidra.py`
  - `/home/buk/.gemini/antigravity-cli/brain/8a3afe86-17fc-4611-96c8-b29e3a6641bd/scratch/attila_dwarf_audit.sqlite` (874.3 MiB, extracted with Step 3 `twdll-dwarf`)
  - `/home/buk/.gemini/antigravity-cli/brain/8a3afe86-17fc-4611-96c8-b29e3a6641bd/scratch/step3a_audit_report.txt`
  - `/home/buk/.gemini/antigravity-cli/brain/8a3afe86-17fc-4611-96c8-b29e3a6641bd/scratch/large_funcs_sample.txt`

Known non-bugs empirically verified in DWARF (do not chase):
- `CA::read_pixels @ field_0x14` (108 hits): 4 bytes of trailing struct alignment padding at `0x14..0x18` inside `CA::PixelAccessorBase` (`base@0..8`, `wide@8..12`, `high@12..16`, `stride@16..20`) copied during 32-byte struct copy.
- `EMPIRE::EMPIRE_MP` (`m_player_slots.m_slots.m_data` `+ 0xd8`/`+ 0x70`) and `UNIT_TRACKER::update_awareness` (`m_order_queue.m_queue.m_data`): raw `card8 m_data[...]` placement-new byte buffers in CA's C++ source.
- `MP_STEAM::ISteamRemoteStorage` / `ISteamFriends` / `ISteamUser`: external `libsteam_api.so` forward declarations (`is_declaration = 1`, no DWARF definition in `attila_linux_64`).
- `-O2` x87/SIMD/pixel-packing idioms (`psraw`, `packssdw`, `CONCAT12`, `in_FPUControlWord`, unrolled memcpy), and GCC `-O2` multi-bit word loads across packed bitfields (`imposter_data->field_0x11c`, `this->field_0x2a8`/`field_0x2aa`, `header->field_0x13`).

Goal & ordered agenda (Steps 1–3 are already completed; start at Step 4 and follow AGENTS.md step-by-step approval gate before editing production files; use a `flash` subagent `invoke_subagent(..., Model="flash", TypeName="research")` on `large_funcs_sample.txt` after each fix round to verify large 2 KB–35 KB functions cheaply):
1. **[DONE in `80c70a1`] Fix False `noreturn` Truncation**.
2. **[DONE in `tools/twdll-dwarf/src/dwarf.rs`] Link Pure-Virtual Interface Vtable Slot Signatures** (`8,925` decl functions emitted; `function_id IS NULL` `8,253+ -> 175`; `WS_ENGINE::_vptr->scene()` -> `WS_SCENE *`; `ENTITY_TRACKER @ 0xf0` eliminated).
3. **[DONE in `tools/twdll-dwarf/src/dwarf.rs` & `layout.rs`] Populate Bitfields & Unions** (`0` referenced non-union/bitfield/union conflicts in SQLite; `3,921` bitfields + `113` unions / `542` arms placed in Pass 2 with `0` errors; verified by `flash` subagent with zero regressions).
4. **Step 4 (NEXT) — Investigate `CA::operator==` Dropped 2nd Arg & `CA::String::~String` Type Collision + Run Final `flash` Large-Function Audit**:
   - `CA::operator==` has both `lhs` (`RDI`) and `rhs` (`RSI`) in `dwarf_parameters`, yet `DATABASE_TABLE::DATABASE_TABLE @ 0x1da6b60` emitted 1-arg `CA::operator==((String *)this_01)`. Also fix `CA::String::~String` param type colliding with `CHECKED_ARRAY<EMPIRECAMPAIGN::EVENT_FEED::PARAMETERS_BASE<...>::PARAMETER_DEFINITION,_1> *` (likely `func_id_by_address` ICF/alias first-CU-wins in `dwarf.rs`).
   - Re-run `audit_ghidra.py` and dispatch a `flash` subagent on `large_funcs_sample.txt`; iterate until zero critical bugs remain.
5. **Step 5 — Formalize `twdll extract` Spec & Implement Step 3B / 3C**:
   - Once Step 3A pseudocode quality is signed off, recommend/run `/egm:egm-brainstorm twdll-extract` (or update `docs/designs/dwarf_extraction_and_decompiler_integration_design.md` + `/egm:egm-review`) to lock the decompiler-extracted SQLite tables (`ELF64` + `PE32`), bulk/parallel/selective `DecompInterface` pseudocode strategy, and immutable `.gpr` vs. caching trade-offs, then implement Step 3B (`twdll extract`) and Step 3C (`twdll sql`) with all 5 Python checks (`ruff format --check`, `ruff check`, `basedpyright`, `lint-imports`, `pytest`).
```
