#!/usr/bin/env python3
"""Extract decompiled C pseudocode and ClangFieldToken refs into `extracted.jsonl`."""

from __future__ import annotations

import argparse
import json
import random
import sqlite3
import subprocess
import time
from collections import Counter, defaultdict
from contextlib import closing
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from audit_common import (
    DEFAULT_CACHE_DIR,
    DEFAULT_DWARF_DB,
    DEFAULT_RUNS_DIR,
    classify_namespace_tier,
    ensure_cached_snapshot,
    normalize_template_skeleton,
    open_cached_program_readonly,
    rebuild_local_dwarf_db,
)
from twdll import _ghidra
from twdll.domain import WorkspaceContext, compute_file_sha256

AUDIT_DIR = Path(__file__).resolve().parent
REPO_ROOT = AUDIT_DIR.parents[1]

_TIER_QUOTAS: tuple[tuple[str, float], ...] = (
    ("core", 0.70),
    ("engine", 0.20),
    ("stl_middleware", 0.10),
)
_SIZE_STRATA: tuple[tuple[str, int, int, float], ...] = (
    ("small", 20, 79, 0.10),
    ("medium", 80, 15000, 0.85),
    ("large", 15001, 10_000_000, 0.05),
)


def _git_head_sha(repo_root: Path) -> str:
    try:
        res = subprocess.run(
            ["git", "-C", str(repo_root), "rev-parse", "HEAD"],
            check=True,
            capture_output=True,
            text=True,
        )
        return res.stdout.strip()
    except (OSError, subprocess.CalledProcessError):
        return "unknown"


def _next_run_id(parent_dir: Path, prefix: str, explicit: str | None) -> str:
    if explicit:
        return explicit
    if not parent_dir.is_dir():
        return f"{prefix}_001"
    existing = []
    for p in parent_dir.iterdir():
        if p.is_dir() and p.name.startswith(f"{prefix}_"):
            suffix = p.name.removeprefix(f"{prefix}_")
            if suffix.isdigit():
                existing.append(int(suffix))
    return f"{prefix}_{(max(existing) + 1) if existing else 1:03d}"


def _size_stratum(byte_size: int) -> str:
    for name, lo, hi, _ in _SIZE_STRATA:
        if lo <= byte_size <= hi:
            return name
    return "medium"


def _allocate_counts(
    total: int, weights: tuple[tuple[str, float], ...]
) -> dict[str, int]:
    raw = [(k, total * w) for k, w in weights]
    out = {k: int(v) for k, v in raw}
    rem = total - sum(out.values())
    for k, _ in sorted(raw, key=lambda kv: kv[1] - int(kv[1]), reverse=True):
        if rem <= 0:
            break
        out[k] += 1
        rem -= 1
    return out


def sample_discovery_functions(
    db_path: Path,
    exclude_addrs: set[int],
    *,
    count: int,
    seed: int,
    max_per_skeleton: int = 2,
) -> list[dict[str, Any]]:
    """Sample unseen functions with 70/20/10 tier and 10/85/5 size-strata quotas."""
    uri = f"file:{db_path.resolve().as_posix()}?mode=ro"
    with closing(sqlite3.connect(uri, uri=True)) as conn:
        rows = conn.execute(
            """
            SELECT f.address, MIN(f.qualified_name), MAX(COALESCE(f.byte_size, 0))
            FROM dwarf_functions AS f
            WHERE f.address IS NOT NULL
              AND f.is_declaration = 0
              AND COALESCE(f.byte_size, 0) >= 20
            GROUP BY f.address
            """
        ).fetchall()

    rng = random.Random(seed)
    pools: dict[tuple[str, str], list[dict[str, Any]]] = defaultdict(list)
    for addr_i, qn_obj, bsz_i in rows:
        addr = int(addr_i)
        if addr in exclude_addrs:
            continue
        qn = str(qn_obj)
        bsz = int(bsz_i)
        tier = classify_namespace_tier(qn)
        stratum = _size_stratum(bsz)
        pools[(tier, stratum)].append(
            {
                "addr": addr,
                "name": qn,
                "tier": tier,
                "stratum": stratum,
                "template_skeleton": normalize_template_skeleton(qn),
                "dwarf_bytes": bsz,
            }
        )

    for bucket in pools.values():
        rng.shuffle(bucket)

    tier_targets = _allocate_counts(count, _TIER_QUOTAS)
    stratum_weights = tuple((name, w) for name, _, _, w in _SIZE_STRATA)
    selected: list[dict[str, Any]] = []
    seen_addrs: set[int] = set()
    skel_counts: Counter[str] = Counter()

    def _try_pick(cand: dict[str, Any]) -> bool:
        addr = int(cand["addr"])
        skel = str(cand["template_skeleton"])
        if addr in seen_addrs or skel_counts[skel] >= max_per_skeleton:
            return False
        seen_addrs.add(addr)
        skel_counts[skel] += 1
        selected.append(cand)
        return True

    for tier, t_target in tier_targets.items():
        stratum_targets = _allocate_counts(t_target, stratum_weights)
        for stratum, s_target in stratum_targets.items():
            picked = 0
            for cand in pools.get((tier, stratum), ()):
                if picked >= s_target:
                    break
                if _try_pick(cand):
                    picked += 1

    # Backfill across any remaining candidates in matching tier first, then globally.
    if len(selected) < count:
        leftover: list[dict[str, Any]] = []
        for (_tier, _), bucket in sorted(pools.items()):
            for cand in bucket:
                if cand["addr"] not in seen_addrs:
                    leftover.append(cand)
        rng.shuffle(leftover)
        for cand in leftover:
            if len(selected) >= count:
                break
            _try_pick(cand)

    return selected


def select_reverify_functions(
    db_path: Path,
    *,
    runs_dir: Path,
    from_batch: str | None,
    addrs_file: Path | None,
    sql_filter: str | None,
    count: int,
    seed: int,
) -> list[dict[str, Any]]:
    """Select target functions for non-polluting re-verification runs."""
    explicit_addrs: set[int] = set()
    if from_batch:
        merged_path = runs_dir / "batches" / from_batch / "merged_findings.json"
        if not merged_path.is_file():
            raise FileNotFoundError(
                f"merged_findings.json not found for {from_batch}: {merged_path}"
            )
        merged = json.loads(merged_path.read_text(encoding="utf-8"))
        for f in merged.get("findings", []):
            explicit_addrs.add(int(str(f["addr"]), 16))
    if addrs_file:
        for line in addrs_file.read_text(encoding="utf-8").splitlines():
            stripped = line.split("#", 1)[0].strip()
            if stripped:
                explicit_addrs.add(int(stripped, 16))

    uri = f"file:{db_path.resolve().as_posix()}?mode=ro"
    with closing(sqlite3.connect(uri, uri=True)) as conn:
        if sql_filter:
            query = f"""
                SELECT DISTINCT f.address, f.qualified_name, COALESCE(f.byte_size, 0)
                FROM dwarf_functions AS f
                LEFT JOIN dwarf_types AS rt ON rt.type_id = f.return_type_id
                LEFT JOIN dwarf_types AS ct ON ct.type_id = f.containing_type_id
                WHERE f.address IS NOT NULL AND f.is_declaration = 0 AND ({sql_filter})
            """
            rows = conn.execute(query).fetchall()
            rng = random.Random(seed)
            cands = [
                {
                    "addr": int(a),
                    "name": str(qn),
                    "tier": classify_namespace_tier(str(qn)),
                    "stratum": _size_stratum(int(bsz)),
                    "template_skeleton": normalize_template_skeleton(str(qn)),
                    "dwarf_bytes": int(bsz),
                }
                for a, qn, bsz in rows
            ]
            rng.shuffle(cands)
            return cands[:count]

        if not explicit_addrs:
            raise ValueError(
                "reverify mode requires --from-batch, --addrs-file, or --sql-filter"
            )

        placeholders = ",".join("?" for _ in explicit_addrs)
        ordered_addrs = sorted(explicit_addrs)
        rows = conn.execute(
            f"""
            SELECT f.address, MIN(f.qualified_name), MAX(COALESCE(f.byte_size, 0))
            FROM dwarf_functions AS f
            WHERE f.address IN ({placeholders})
            GROUP BY f.address
            """,
            ordered_addrs,
        ).fetchall()
    by_addr = {int(a): (str(qn), int(bsz)) for a, qn, bsz in rows}
    out: list[dict[str, Any]] = []
    for addr in ordered_addrs:
        qn, bsz = by_addr.get(addr, (f"sub_{addr:x}", 0))
        out.append(
            {
                "addr": addr,
                "name": qn,
                "tier": classify_namespace_tier(qn),
                "stratum": _size_stratum(bsz),
                "template_skeleton": normalize_template_skeleton(qn),
                "dwarf_bytes": bsz,
            }
        )
    return out


def collect_clang_field_refs(markup_root: Any) -> list[dict[str, Any]]:
    """Walk Ghidra `ClangNode` AST to collect deduplicated `ClangFieldToken` refs."""
    if markup_root is None:
        return []
    refs: list[dict[str, Any]] = []
    seen: set[tuple[str, int, str]] = set()
    stack: list[Any] = [markup_root]
    while stack:
        node = stack.pop()
        if node is None:
            continue
        cls_name = type(node).__name__
        if (
            cls_name == "ClangFieldToken" or "ClangFieldToken" in str(type(node))
        ) and hasattr(node, "getDataType"):
            dt = node.getDataType()
            off = int(node.getOffset()) if hasattr(node, "getOffset") else -1
            text = str(node.getText()).strip() if hasattr(node, "getText") else ""
            struct_name = (
                str(dt.getName()) if dt is not None and hasattr(dt, "getName") else ""
            )
            if struct_name and off >= 0 and text:
                key = (struct_name, off, text)
                if key not in seen:
                    seen.add(key)
                    refs.append(
                        {"struct_name": struct_name, "offset": off, "field_name": text}
                    )
        if hasattr(node, "numChildren"):
            n_ch = int(node.numChildren())
            for idx in range(n_ch - 1, -1, -1):
                stack.append(node.Child(idx))
    return refs


def _query_db_metrics(db_path: Path) -> tuple[int, int, int]:
    uri = f"file:{db_path.resolve().as_posix()}?mode=ro"
    with closing(sqlite3.connect(uri, uri=True)) as conn:
        total_funcs = int(
            conn.execute(
                "SELECT COUNT(DISTINCT address) FROM dwarf_functions "
                "WHERE address IS NOT NULL AND is_declaration = 0"
            ).fetchone()[0]
        )
        total_globals = int(
            conn.execute("SELECT COUNT(*) FROM dwarf_globals").fetchone()[0]
        )
        ref_types = int(
            conn.execute(
                "SELECT COUNT(*) FROM dwarf_types WHERE is_referenced = 1"
            ).fetchone()[0]
        )
    return total_funcs, total_globals, ref_types


def _load_manifest_exclude_addrs(manifest_path: Path) -> set[int]:
    if not manifest_path.is_file():
        return set()
    data = json.loads(manifest_path.read_text(encoding="utf-8"))
    return {int(k, 16) for k in data.get("functions", {})}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--rebuild-dwarf-db", action="store_true")
    parser.add_argument("--mode", choices=("discovery", "reverify"), default=None)
    parser.add_argument("--count", type=int, default=250)
    parser.add_argument("--exit-gate", action="store_true")
    parser.add_argument("--seed", type=int, default=1)
    parser.add_argument("--max-per-skeleton", type=int, default=2)
    parser.add_argument("--from-batch", type=str, default=None)
    parser.add_argument("--addrs-file", type=Path, default=None)
    parser.add_argument("--sql-filter", type=str, default=None)
    parser.add_argument("--batch", "--run-id", dest="run_id", type=str, default=None)
    parser.add_argument("--dwarf-db", type=Path, default=DEFAULT_DWARF_DB)
    parser.add_argument("--runs-dir", type=Path, default=DEFAULT_RUNS_DIR)
    parser.add_argument("--timeout", type=int, default=60)
    parser.add_argument("--skip-cache", action="store_true")
    parser.add_argument("--dry-run", action="store_true")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    runs_dir = args.runs_dir.resolve()
    db_path = args.dwarf_db.resolve()
    ctx = WorkspaceContext.from_root(REPO_ROOT)

    if args.rebuild_dwarf_db:
        rebuild_local_dwarf_db(ctx, db_path)
        t_funcs, t_globals, ref_types = _query_db_metrics(db_path)
        print(
            f"Rebuilt DWARF DB: {db_path} | concrete_funcs={t_funcs:,} | "
            f"globals={t_globals:,} | referenced_types={ref_types:,}",
            flush=True,
        )
        if args.mode is None:
            return

    mode = args.mode or "discovery"
    count = 500 if args.exit_gate else args.count
    total_dwarf_funcs, _, _ = _query_db_metrics(db_path)

    if mode == "discovery":
        exclude_addrs = _load_manifest_exclude_addrs(runs_dir / "manifest.json")
        selected = sample_discovery_functions(
            db_path,
            exclude_addrs,
            count=count,
            seed=args.seed,
            max_per_skeleton=args.max_per_skeleton,
        )
        run_id = _next_run_id(runs_dir / "batches", "batch", args.run_id)
        run_dir = runs_dir / "batches" / run_id
    else:
        selected = select_reverify_functions(
            db_path,
            runs_dir=runs_dir,
            from_batch=args.from_batch,
            addrs_file=args.addrs_file,
            sql_filter=args.sql_filter,
            count=count,
            seed=args.seed,
        )
        run_id = _next_run_id(runs_dir / "reverify", "reverify", args.run_id)
        run_dir = runs_dir / "reverify" / run_id

    tier_counts = Counter(x["tier"] for x in selected)
    stratum_counts = Counter(x["stratum"] for x in selected)
    print(
        f"Run {run_id} ({mode}, exit_gate={args.exit_gate}): "
        f"selected {len(selected)} functions | "
        f"tiers={dict(tier_counts)} | strata={dict(stratum_counts)}",
        flush=True,
    )
    if args.dry_run or not selected:
        return

    cache_dir, snap_meta = ensure_cached_snapshot(
        ctx, db_path, DEFAULT_CACHE_DIR, skip_cache=args.skip_cache
    )
    run_dir.mkdir(parents=True, exist_ok=True)
    jsonl_path = run_dir / "extracted.jsonl"

    t0 = time.monotonic()
    with open_cached_program_readonly(cache_dir) as program:
        decomp = _ghidra.jclass("ghidra.app.decompiler.DecompInterface")()
        monitor = _ghidra.jclass("ghidra.util.task.ConsoleTaskMonitor")()
        fm = program.getFunctionManager()
        addr_space = program.getAddressFactory().getDefaultAddressSpace()
        decomp.openProgram(program)
        try:
            with jsonl_path.open("w", encoding="utf-8") as handle:
                for idx, item in enumerate(selected, start=1):
                    addr = int(item["addr"])
                    gh_func = fm.getFunctionAt(addr_space.getAddress(addr))
                    field_refs: list[dict[str, Any]] = []
                    if gh_func is None:
                        gh_sz, ok, err = 0, False, "missing Ghidra function at entry"
                        c_code = f"/* DECOMPILE FAILED: {err} */\n"
                    else:
                        gh_sz = int(gh_func.getBody().getNumAddresses())
                        res = decomp.decompileFunction(gh_func, args.timeout, monitor)
                        ok = bool(res is not None and res.decompileCompleted())
                        if ok:
                            err = ""
                            c_code = str(res.getDecompiledFunction().getC())
                            field_refs = collect_clang_field_refs(res.getCCodeMarkup())
                        else:
                            err = (
                                str(res.getErrorMessage()).strip()
                                if res is not None
                                else "None result"
                            )
                            c_code = f"/* DECOMPILE FAILED: {err} */\n"
                    rec = {
                        "addr": f"0x{addr:x}",
                        "name": item["name"],
                        "tier": item["tier"],
                        "template_skeleton": item["template_skeleton"],
                        "dwarf_bytes": item["dwarf_bytes"],
                        "ghidra_bytes": gh_sz,
                        "decompile_ok": ok,
                        "decompile_err": err,
                        "c_code": c_code,
                        "field_refs": field_refs,
                    }
                    handle.write(json.dumps(rec) + "\n")
                    if idx % 50 == 0 or idx == len(selected):
                        dt = time.monotonic() - t0
                        print(
                            f"[{dt:6.2f}s] Decompiled {idx}/{len(selected)}",
                            flush=True,
                        )
        finally:
            decomp.dispose()

    batch_meta = {
        "run_id": run_id,
        "created_at": datetime.now(UTC).isoformat(),
        "git_sha": _git_head_sha(REPO_ROOT),
        "dwarf_db_sha256": compute_file_sha256(db_path),
        "cache_key": snap_meta.get("cache_key"),
        "total_dwarf_functions": total_dwarf_funcs,
        "mode": mode,
        "exit_gate": bool(args.exit_gate),
        "seed": args.seed,
        "sampling_params": {
            "count": count,
            "max_per_skeleton": args.max_per_skeleton,
            "from_batch": args.from_batch,
            "sql_filter": args.sql_filter,
        },
        "sampled_addrs": [f"0x{int(item['addr']):x}" for item in selected],
        "pass_stats": snap_meta.get("pass_stats", {}),
    }
    (run_dir / "batch_meta.json").write_text(
        json.dumps(batch_meta, indent=2) + "\n", encoding="utf-8"
    )
    print(f"Extracted {len(selected)} functions -> {run_dir}", flush=True)


if __name__ == "__main__":
    main()
