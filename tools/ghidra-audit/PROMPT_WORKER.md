# Worker Prompt — Gemini Flash (`HIGH` Thinking)

You are auditing Ghidra C++ decompiler output produced from DWARF debug info for a 64-bit x86-64 ELF binary (System V AMD64 ABI).

**Your job:**
1. Read ONLY your assigned chunk file(s): `<RUN_DIR>/chunks/chunk_NNN.c`.
2. Inspect EVERY function in the chunk (each starts with `/* === [i/N] 0xADDR | qualified_name | dwarf=...B | gh=...B | ok=... | lines=... === */`, optionally followed by `/* DWARF-FACTS: ... */`).
3. Write valid JSON directly to your assigned output path: `<RUN_DIR>/findings/chunk_NNN.json`.
4. Reply ONLY with a 1-line summary: `chunk_NNN.json: X reviewed, Y findings`.

## How to Use `/* DWARF-FACTS: ... */`
- `is_sret=0|1`: Whether DWARF expects a hidden `RDI` return-storage pointer (`__return_storage_ptr__`).
- `PROVEN_PADDING_GAP=Type+0xNN`: Deterministic SQLite layout proof that `+0xNN` falls strictly in inter-field or trailing alignment padding (do **NOT** flag these as `STRUCT_FIELD` bugs).
- `empty_struct_params=...` / `dwarf_void_ptr_params=...`: Known DWARF-level parameter facts (do not misclassify intentional empty tags or DWARF `void *` as Ghidra injection bugs).

## Allowed Enum Values (Strictly Enforced by `merge_findings.py`)
- `category`: `PARAMS`, `ABI_REG`, `STRUCT_FIELD`, `VTABLE`, `TRUNCATION`, `OTHER`
- `severity`: `HIGH`, `MEDIUM`, `LOW`
- `suspect_layer`: `dwarf_rust`, `pass1_types`, `pass2_struct_vtable`, `pass3_sig_abi`, `pass4_locals`, `ghidra_decomp`, `unknown`
- `reviewed_addrs`: MUST match the exact set of hex addresses in `chunk_NNN.c` 1-to-1.

## What Counts as a REAL Bug
- `PARAMS`: Wrong parameter count, swapped/duplicate/unnamed bogus parameters, missing `this`, or missing/spurious `__return_storage_ptr__` (`sret`).
- `ABI_REG`: Unresolved ABI parameter/return registers (`in_RDI`, `in_RSI`, `in_RDX`, `in_RCX`, `in_R8`, `in_R9`, `in_XMM*`, `extraout_*`, `unaff_*`) used as inputs/returns instead of typed args/returns.
- `STRUCT_FIELD`: Missing struct/class fields (`field_0x...` not listed in `PROVEN_PADDING_GAP`, or raw `*(type *)(ptr + 0x...)` on a defined game class).
- `VTABLE`: Raw `(**(code **)(*ptr + 0x...))` virtual call on a typed game class that should be `ptr->_vptr->method(...)`.
- `TRUNCATION`: `ok=False`, `halt_baddata`, fallthrough past a `noreturn` helper, or jump-table failure (`Could not recover jumptable`).
- `OTHER`: Any other clear DWARF/Ghidra type or signature corruption.

## DO NOT REPORT (Known Verified Non-Bugs)
1. Any offset explicitly listed in `PROVEN_PADDING_GAP=...` on that function's `/* DWARF-FACTS */` line.
2. Placement-new `card8 m_data[...]` byte buffers (`m_player_slots`, `m_order_queue`, `CA::String` globals).
3. External `libsteam_api.so` forward declarations (`MP_STEAM::ISteam*`).
4. `-O2` SIMD/x87/bitfield word-load idioms (`CONCAT*` on vector/float packing, `psraw`, `packssdw`, multi-bit packed bitfield reads).
5. Harmless Ghidra banner comments (`/* WARNING: Enum ... */`, `/* WARNING: Globals starting with '_' ... */`).
6. `-O2` stride loop-induction pointer arithmetic on typed struct arrays (e.g., byte-offset induction variable `lVar += 0x50` added to a typed struct pointer whose fields are already defined and resolved nearby).
7. Intentional `void *this` on Itanium adjusting thunks (`_ZThn*` / `_ZTvn*`, where incoming `RDI` points to a secondary base subobject at `+N` bytes and Ghidra cannot represent negative-shifted struct pointer parameters).

## Exact Output JSON Schema
```json
{
  "chunk": "chunk_001.c",
  "reviewed_addrs": ["0x1592190", "0x3553c00"],
  "findings": [
    {
      "addr": "0x123456",
      "func": "EMPIRECAMPAIGN::FOO::bar",
      "category": "STRUCT_FIELD",
      "pattern": "unmapped_struct_field",
      "suspect_layer": "pass2_struct_vtable",
      "severity": "MEDIUM",
      "evidence": "  uVar1 = *(uint *)(this + 0x48);",
      "why": "Raw offset +0x48 on typed FOO* this pointer instead of struct member."
    }
  ]
}
```
If every function in the chunk is clean, output `"findings": []`.
