#!/usr/bin/env python3
"""Tier 0 deterministic pre-scanner, SQLite DWARF-FACTS annotator, and chunk writer."""

from __future__ import annotations

import argparse
import json
import random
import re
import sqlite3
from contextlib import closing
from pathlib import Path
from typing import Any

from audit_common import DEFAULT_DWARF_DB

AUDIT_DIR = Path(__file__).resolve().parent

_ABI_REG_RE = re.compile(
    r"\b(?:in_(?:R|XMM)[A-Z0-9_]+|extraout_[A-Za-z0-9_]+|unaff_[A-Za-z0-9_]+)\b"
)
_UNRESOLVED_CALL_RE = re.compile(r"\(\*\*\s*\(code\s*\*\*\)\s*\(\*")
_JUMPTABLE_RE = re.compile(r"(?:Could not recover jumptable|halt_baddata)")
_RAW_DAT_RE = re.compile(r"\b_?DAT_[0-9a-fA-F]{4,}\b")
_FIELD_0X_RE = re.compile(r"^field_0x([0-9a-fA-F]+)$")
_CODE_FIELD_0X_RE = re.compile(r"\bfield_0x([0-9a-fA-F]+)\b")
_NORETURN_CALL_RE = re.compile(
    r"\b(?:std::__throw_[A-Za-z0-9_]+|__cxa_throw|__cxa_rethrow|"
    r"__clang_call_terminate|_Unwind_Resume|__assert_fail)\s*\([^;]*\);"
)


def _resolve_struct_tid(
    conn: sqlite3.Connection, struct_name: str
) -> tuple[int, int] | None:
    rows = conn.execute(
        """
        SELECT type_id, byte_size, is_declaration, is_stl_stub
        FROM dwarf_types
        WHERE kind IN ('struct', 'class') AND qualified_name = ?
        """,
        (struct_name,),
    ).fetchall()
    if not rows:
        rows = conn.execute(
            """
            SELECT type_id, byte_size, is_declaration, is_stl_stub
            FROM dwarf_types
            WHERE kind IN ('struct', 'class') AND qualified_name LIKE ?
            """,
            (f"%::{struct_name}",),
        ).fetchall()
    if len(rows) != 1:
        return None
    tid, bsz, is_decl, is_stl = rows[0]
    if is_decl or is_stl or bsz is None or int(bsz) <= 0:
        return None
    return int(tid), int(bsz)


def _flatten_occupied_intervals(
    conn: sqlite3.Connection,
    tid: int,
    base_off: int,
    seen: set[int],
) -> list[tuple[int, int]] | None:
    if tid in seen:
        return None
    seen.add(tid)
    intervals: list[tuple[int, int]] = []
    inh_rows = conn.execute(
        """
        SELECT base_type_id, byte_offset, is_virtual
        FROM dwarf_inheritance
        WHERE derived_type_id = ?
        """,
        (tid,),
    ).fetchall()
    for base_tid, boff, is_virt in inh_rows:
        if is_virt or boff is None:
            return None
        sub = _flatten_occupied_intervals(
            conn, int(base_tid), base_off + int(boff), seen
        )
        if sub is None:
            return None
        intervals.extend(sub)

    mem_rows = conn.execute(
        """
        SELECT byte_offset, byte_size, is_layout_conflict
        FROM dwarf_members
        WHERE parent_type_id = ?
        """,
        (tid,),
    ).fetchall()
    for m_off, m_sz, is_conflict in mem_rows:
        if is_conflict:
            return None
        start = base_off + int(m_off)
        intervals.append((start, start + max(int(m_sz), 1)))
    seen.remove(tid)
    return intervals


def prove_padding_gap(
    conn: sqlite3.Connection,
    struct_name: str,
    offset: int,
    cache: dict[str, tuple[int, list[tuple[int, int]]] | None],
) -> bool:
    """Return True strictly when `(struct_name, offset)` is in proven padding."""
    if struct_name not in cache:
        resolved = _resolve_struct_tid(conn, struct_name)
        if resolved is None:
            cache[struct_name] = None
        else:
            tid, bsz = resolved
            intervals = _flatten_occupied_intervals(conn, tid, 0, set())
            cache[struct_name] = (bsz, intervals) if intervals is not None else None
    entry = cache[struct_name]
    if entry is None:
        return False
    byte_size, intervals = entry
    if offset < 0 or offset >= byte_size:
        return False
    return not any(start <= offset < end for start, end in intervals)


def _has_noreturn_fallthrough(c_code: str) -> bool:
    lines = c_code.splitlines()
    for idx, line in enumerate(lines):
        if _NORETURN_CALL_RE.search(line):
            for nxt in lines[idx + 1 :]:
                s = nxt.strip()
                if not s or s.startswith(("/*", "//", "}")):
                    continue
                return True
    return False


def _query_dwarf_facts(conn: sqlite3.Connection, addr: int) -> str:
    row = conn.execute(
        """
        SELECT function_id, is_sret
        FROM dwarf_functions
        WHERE address = ? AND is_declaration = 0
        ORDER BY function_id
        LIMIT 1
        """,
        (addr,),
    ).fetchone()
    if row is None:
        return ""
    fid, is_sret = int(row[0]), bool(row[1])
    params = conn.execute(
        """
        SELECT p.name, p.type_id, t.kind, t.qualified_name,
               t.byte_size, t.target_type_id
        FROM dwarf_parameters AS p
        LEFT JOIN dwarf_types AS t ON t.type_id = p.type_id
        WHERE p.function_id = ?
        ORDER BY p.ordinal
        """,
        (fid,),
    ).fetchall()
    empty_params: list[str] = []
    void_ptr_params: list[str] = []
    for pname, ptid, kind, qname, bsz, target_tid in params:
        label = str(pname or "param")
        if kind in ("struct", "class") and bsz == 1:
            mcnt = conn.execute(
                "SELECT COUNT(*) FROM dwarf_members WHERE parent_type_id = ?",
                (int(ptid),),
            ).fetchone()[0]
            icnt = conn.execute(
                "SELECT COUNT(*) FROM dwarf_inheritance WHERE derived_type_id = ?",
                (int(ptid),),
            ).fetchone()[0]
            if mcnt == 0 and icnt == 0:
                empty_params.append(f"{label}:{qname}")
        if kind == "pointer" and target_tid == 1:
            void_ptr_params.append(label)
    parts = [f"is_sret={int(is_sret)}"]
    if empty_params:
        parts.append("empty_struct_params=" + ",".join(empty_params[:4]))
    if void_ptr_params:
        parts.append("dwarf_void_ptr_params=" + ",".join(void_ptr_params[:4]))
    return " ".join(parts)


def scan_function_record(
    record: dict[str, Any],
    conn: sqlite3.Connection,
    cache: dict[str, tuple[int, list[tuple[int, int]]] | None],
) -> tuple[list[str], list[str], str]:
    """Scan an `extracted.jsonl` record for Tier 0 signals and `DWARF-FACTS`."""
    c_code = str(record.get("c_code", ""))
    signals: list[str] = []
    if not record.get("decompile_ok", False):
        signals.append("DECOMPILE_FAILED")
    if _ABI_REG_RE.search(c_code):
        signals.append("SUSPECT_SRET_OR_REGISTER_SHIFT")
    if _UNRESOLVED_CALL_RE.search(c_code):
        signals.append("UNRESOLVED_CALL_TARGET")
    if _JUMPTABLE_RE.search(c_code):
        signals.append("TRUNCATION_OR_JUMPTABLE")
    if _RAW_DAT_RE.search(c_code):
        signals.append("MISSING_GLOBAL_OR_EXTERN_TYPE")
    if _has_noreturn_fallthrough(c_code):
        signals.append("SUSPECT_CALLEE_NORETURN")

    proven_padding: list[str] = []
    unproven_offsets: list[str] = []
    checked_tokens: set[str] = set()
    for ref in record.get("field_refs", []):
        fname = str(ref.get("field_name", ""))
        m = _FIELD_0X_RE.match(fname)
        if m is None:
            continue
        checked_tokens.add(fname)
        struct_name = str(ref.get("struct_name", ""))
        offset = int(ref.get("offset", -1))
        if prove_padding_gap(conn, struct_name, offset, cache):
            proven_padding.append(f"{struct_name}+0x{offset:x}")
        else:
            unproven_offsets.append(f"{struct_name}+0x{offset:x}")

    for m in _CODE_FIELD_0X_RE.finditer(c_code):
        tok = f"field_0x{m.group(1)}"
        if tok not in checked_tokens:
            unproven_offsets.append(tok)

    if unproven_offsets:
        signals.append("UNPROVEN_FIELD_OFFSET")

    addr = int(str(record["addr"]), 16)
    facts_str = _query_dwarf_facts(conn, addr)
    if proven_padding:
        pad_csv = ",".join(sorted(set(proven_padding))[:6])
        facts_str = f"{facts_str} PROVEN_PADDING_GAP={pad_csv}".strip()
    return sorted(set(signals)), sorted(set(proven_padding)), facts_str


def _write_chunks(
    batch_dir: Path,
    routed: list[dict[str, Any]],
    *,
    chunk_size: int,
) -> list[str]:
    chunks_dir = batch_dir / "chunks"
    if chunks_dir.exists():
        for old in chunks_dir.glob("chunk_*.c"):
            old.unlink()
    else:
        chunks_dir.mkdir(parents=True, exist_ok=True)
    (batch_dir / "findings").mkdir(parents=True, exist_ok=True)

    chunk_files: list[str] = []
    for chunk_idx, start in enumerate(range(0, len(routed), chunk_size), start=1):
        items = routed[start : start + chunk_size]
        chunk_name = f"chunk_{chunk_idx:03d}.c"
        total = len(items)
        blocks: list[str] = []
        for pos, rec in enumerate(items, start=1):
            c_code = str(rec["c_code"]).rstrip("\n")
            nlines = len(c_code.splitlines())
            header = (
                f"/* === [{pos}/{total}] {rec['addr']} | {rec['name']} | "
                f"dwarf={rec['dwarf_bytes']}B | gh={rec['ghidra_bytes']}B | "
                f"ok={bool(rec['decompile_ok'])} | lines={nlines} === */"
            )
            facts = str(rec.get("_dwarf_facts", "")).strip()
            if facts:
                blocks.append(f"{header}\n/* DWARF-FACTS: {facts} */\n{c_code}\n")
            else:
                blocks.append(f"{header}\n{c_code}\n")
        (chunks_dir / chunk_name).write_text("\n".join(blocks) + "\n", encoding="utf-8")
        chunk_files.append(chunk_name)
    return chunk_files


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--batch-dir", type=Path, required=True)
    parser.add_argument("--dwarf-db", type=Path, default=DEFAULT_DWARF_DB)
    parser.add_argument("--chunk-size", type=int, default=12)
    parser.add_argument("--blind-ratio", type=float, default=0.15)
    parser.add_argument("--seed", type=int, default=42)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    batch_dir = args.batch_dir.resolve()
    db_path = args.dwarf_db.resolve()
    jsonl_path = batch_dir / "extracted.jsonl"
    meta_path = batch_dir / "batch_meta.json"
    if not jsonl_path.is_file():
        raise FileNotFoundError(f"extracted.jsonl not found: {jsonl_path}")

    meta = (
        json.loads(meta_path.read_text(encoding="utf-8")) if meta_path.is_file() else {}
    )
    exit_gate = bool(meta.get("exit_gate", False))
    records = [
        json.loads(line)
        for line in jsonl_path.read_text(encoding="utf-8").splitlines()
        if line.strip()
    ]

    uri = f"file:{db_path.resolve().as_posix()}?mode=ro"
    cache: dict[str, tuple[int, list[tuple[int, int]]] | None] = {}
    anomalies: list[dict[str, Any]] = []
    clean: list[dict[str, Any]] = []
    per_addr: dict[str, Any] = {}

    with closing(sqlite3.connect(uri, uri=True)) as conn:
        for rec in records:
            signals, proven_pad, facts_str = scan_function_record(rec, conn, cache)
            rec["_dwarf_facts"] = facts_str
            addr = str(rec["addr"]).lower()
            per_addr[addr] = {
                "signals": signals,
                "proven_padding_refs": proven_pad,
                "dwarf_facts": facts_str,
            }
            if signals:
                anomalies.append(rec)
            else:
                clean.append(rec)

    if exit_gate:
        blind_clean = list(clean)
        skipped_clean: list[dict[str, Any]] = []
    else:
        rng = random.Random(args.seed)
        shuffled = list(clean)
        rng.shuffle(shuffled)
        blind_n = int(round(len(clean) * args.blind_ratio)) if clean else 0
        if clean and args.blind_ratio > 0 and blind_n == 0:
            blind_n = 1
        blind_clean = shuffled[:blind_n]
        skipped_clean = shuffled[blind_n:]

    routed = sorted([*anomalies, *blind_clean], key=lambda r: int(str(r["addr"]), 16))
    chunk_files = _write_chunks(batch_dir, routed, chunk_size=args.chunk_size)

    report = {
        "total_extracted": len(records),
        "exit_gate": exit_gate,
        "routed_anomaly_addrs": [str(r["addr"]).lower() for r in anomalies],
        "routed_blind_clean_addrs": [str(r["addr"]).lower() for r in blind_clean],
        "tier0_clean_skipped_addrs": [str(r["addr"]).lower() for r in skipped_clean],
        "chunk_files": chunk_files,
        "per_addr": per_addr,
    }
    (batch_dir / "prescan_report.json").write_text(
        json.dumps(report, indent=2) + "\n", encoding="utf-8"
    )
    if meta_path.is_file():
        meta["tier0_summary"] = {
            "total_extracted": len(records),
            "routed_anomalies": len(anomalies),
            "routed_blind_clean": len(blind_clean),
            "tier0_clean_skipped": len(skipped_clean),
            "chunk_count": len(chunk_files),
        }
        meta_path.write_text(json.dumps(meta, indent=2) + "\n", encoding="utf-8")

    print(
        f"Prescan complete: extracted={len(records)}, anomalies={len(anomalies)}, "
        f"blind_clean={len(blind_clean)}, skipped={len(skipped_clean)}, "
        f"chunks={len(chunk_files)} -> {batch_dir}",
        flush=True,
    )


if __name__ == "__main__":
    main()
