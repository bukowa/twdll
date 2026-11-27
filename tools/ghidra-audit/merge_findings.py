#!/usr/bin/env python3
"""Merge Tier 1 chunk JSONs, enforce recall/stop rules, and update `manifest.json`."""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from collections import Counter, defaultdict
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from audit_common import (
    DEFAULT_RUNS_DIR,
    classify_namespace_tier,
    normalize_template_skeleton,
)
from twdll.domain import compute_file_sha256

AUDIT_DIR = Path(__file__).resolve().parent
REPO_ROOT = AUDIT_DIR.parents[1]

LEGACY_TOTAL_DWARF_FUNCTIONS = 166_593
VALID_CATEGORIES = {
    "PARAMS",
    "ABI_REG",
    "STRUCT_FIELD",
    "VTABLE",
    "TRUNCATION",
    "OTHER",
}
VALID_SEVERITIES = {"HIGH", "MEDIUM", "LOW"}
VALID_LAYERS = {
    "dwarf_rust",
    "pass1_types",
    "pass2_struct_vtable",
    "pass3_sig_abi",
    "pass4_locals",
    "ghidra_decomp",
    "unknown",
}
SEVERITY_RANK = {"HIGH": 0, "MEDIUM": 1, "LOW": 2}
FUNC_HEADER_RE = re.compile(
    r"^/\* === \[\d+/\d+\] (0x[0-9a-fA-F]+) \| (.+?) \| "
    r"dwarf=(\d+)B \| gh=(\d+)B \| ok=(True|False) \| lines=(\d+) === \*/$"
)


def resolve_batch_dir(
    runs_dir: Path,
    batch_id: str | None,
    explicit_dir: Path | None,
) -> tuple[str, Path]:
    if explicit_dir is not None:
        resolved = explicit_dir.resolve()
        if not resolved.is_dir():
            raise FileNotFoundError(f"Batch directory not found: {resolved}")
        return resolved.name, resolved
    batches_dir = runs_dir / "batches"
    if not batches_dir.is_dir():
        raise FileNotFoundError(f"No batches directory found: {batches_dir}")
    if batch_id is None:
        candidates = sorted(p.name for p in batches_dir.iterdir() if p.is_dir())
        if not candidates:
            raise FileNotFoundError(f"No batch folders found in {batches_dir}")
        batch_id = candidates[-1]
    batch_dir = batches_dir / batch_id
    if not batch_dir.is_dir():
        raise FileNotFoundError(f"Batch directory not found: {batch_dir}")
    return batch_id, batch_dir


def norm_hex(addr: str) -> str:
    return hex(int(str(addr).strip(), 16)).lower()


def norm_pattern(raw: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "_", str(raw).strip().lower()).strip("_")
    return slug or "unspecified"


def _extract_signature(block_lines: list[str]) -> str:
    sig_lines: list[str] = []
    in_block_comment = False
    for line in block_lines[1:]:
        s = line.strip()
        if not s:
            if sig_lines:
                break
            continue
        if in_block_comment:
            if "*/" in s:
                in_block_comment = False
            continue
        if s.startswith("/*"):
            if "*/" not in s:
                in_block_comment = True
            continue
        if s.startswith("//"):
            continue
        sig_lines.append(line.rstrip())
        if "{" in s or len(sig_lines) >= 5:
            break
    return "\n".join(sig_lines)


def index_chunks(
    chunks_dir: Path,
) -> tuple[dict[str, dict[str, Any]], dict[str, set[str]]]:
    index: dict[str, dict[str, Any]] = {}
    addrs_by_chunk: dict[str, set[str]] = {}
    for cf in sorted(chunks_dir.glob("chunk_*.c")):
        lines = cf.read_text(encoding="utf-8").splitlines()
        headers: list[tuple[int, re.Match[str]]] = []
        for idx, line in enumerate(lines):
            m = FUNC_HEADER_RE.match(line)
            if m is not None:
                headers.append((idx, m))
        chunk_addrs: set[str] = set()
        for pos, (start_idx, m) in enumerate(headers):
            end_idx = (
                headers[pos + 1][0] - 1 if pos + 1 < len(headers) else len(lines) - 1
            )
            while end_idx > start_idx and not lines[end_idx].strip():
                end_idx -= 1
            block_lines = lines[start_idx : end_idx + 1]
            addr = norm_hex(m.group(1))
            chunk_addrs.add(addr)
            index[addr] = {
                "chunk_c_file": cf.name,
                "header_func": m.group(2),
                "dwarf_bytes": int(m.group(3)),
                "ghidra_bytes": int(m.group(4)),
                "decompile_ok": m.group(5) == "True",
                "line_start": start_idx + 1,
                "line_end": end_idx + 1,
                "lines": block_lines,
                "signature": _extract_signature(block_lines),
                "full_c": "\n".join(block_lines),
            }
        addrs_by_chunk[cf.name] = chunk_addrs
    return index, addrs_by_chunk


def locate_evidence(
    finfo: dict[str, Any] | None, evidence: str
) -> tuple[int | None, str]:
    if finfo is None:
        return None, evidence
    lines: list[str] = finfo["lines"]
    ev_lines = [ln.strip() for ln in evidence.splitlines() if ln.strip()]
    needle = ev_lines[0] if ev_lines else evidence.strip()
    match_idx: int | None = None
    if needle:
        for i in range(1, len(lines)):
            if needle in lines[i]:
                match_idx = i
                break
        if match_idx is None:
            collapsed_needle = " ".join(needle.split())
            for i in range(1, len(lines)):
                if collapsed_needle in " ".join(lines[i].split()):
                    match_idx = i
                    break
    if match_idx is None:
        fallback = finfo["signature"] or evidence
        return None, fallback
    lo = max(1, match_idx - 3)
    hi = min(len(lines) - 1, match_idx + 3)
    line_start = int(finfo["line_start"])
    snippet_lines = [f"L{line_start + j}: {lines[j]}" for j in range(lo, hi + 1)]
    return line_start + match_idx, "\n".join(snippet_lines)


def validate_chunk_json(
    path: Path,
    expected_addrs: set[str] | None = None,
) -> tuple[dict[str, Any] | None, str | None]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        return None, f"JSON parse error: {exc}"
    if not isinstance(data, dict):
        return None, "Top-level JSON must be an object"
    for req in ("chunk", "reviewed_addrs", "findings"):
        if req not in data:
            return None, f"Missing required key '{req}'"
    if not isinstance(data["reviewed_addrs"], list) or not isinstance(
        data["findings"], list
    ):
        return None, "'reviewed_addrs' and 'findings' must be lists"
    if expected_addrs is not None:
        got_addrs = {norm_hex(a) for a in data["reviewed_addrs"]}
        if got_addrs != expected_addrs:
            missing = sorted(expected_addrs - got_addrs)
            extra = sorted(got_addrs - expected_addrs)
            return (
                None,
                f"reviewed_addrs mismatch (missing={missing[:5]}, extra={extra[:5]})",
            )
    req_fkeys = (
        "addr",
        "func",
        "category",
        "pattern",
        "suspect_layer",
        "severity",
        "evidence",
        "why",
    )
    for item in data["findings"]:
        if not isinstance(item, dict):
            return None, "Each finding must be an object"
        for fkey in req_fkeys:
            if fkey not in item:
                return None, f"Finding missing key '{fkey}'"
        cat = str(item["category"]).strip().upper()
        sev = str(item["severity"]).strip().upper()
        layer = str(item["suspect_layer"]).strip().lower()
        if cat not in VALID_CATEGORIES:
            return None, f"Invalid category {cat!r}"
        if sev not in VALID_SEVERITIES:
            return None, f"Invalid severity {sev!r}"
        if layer not in VALID_LAYERS:
            return None, f"Invalid suspect_layer {layer!r}"
    return data, None


def save_json_atomic(path: Path, data: dict[str, Any]) -> None:
    tmp = path.with_suffix(".json.tmp")
    tmp.write_text(json.dumps(data, indent=2, sort_keys=False) + "\n", encoding="utf-8")
    os.replace(tmp, path)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--batch", type=str, default=None, help="Batch ID (default: latest batch)."
    )
    parser.add_argument(
        "--batch-dir", type=Path, default=None, help="Explicit run directory."
    )
    parser.add_argument("--runs-dir", type=Path, default=DEFAULT_RUNS_DIR)
    parser.add_argument(
        "--golden-set", type=Path, default=AUDIT_DIR / "golden_set.json"
    )
    return parser.parse_args()


def _write_flagged_bundles(
    batch_dir: Path,
    conclusions: list[dict[str, Any]],
    func_index: dict[str, dict[str, Any]],
) -> None:
    flagged_dir = batch_dir / "flagged"
    if flagged_dir.exists():
        for old in flagged_dir.glob("*.c"):
            old.unlink()
    else:
        flagged_dir.mkdir(parents=True, exist_ok=True)

    for conc in conclusions:
        out_path = batch_dir / conc["flagged_file"]
        layers_str = ", ".join(f"{k}:{v}" for k, v in conc["suspect_layers"].items())
        by_addr: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for item in conc["findings"]:
            by_addr[item["addr"]].append(item)
        ordered_addrs = sorted(
            by_addr.keys(),
            key=lambda a: (
                min(SEVERITY_RANK[x["severity"]] for x in by_addr[a]),
                int(a, 16),
            ),
        )
        parts = [
            "/*",
            f" * CONCLUSION: [{conc['max_severity']}] "
            f"{conc['category']} / {conc['pattern']} ({conc['count']} findings)",
            f" * SUSPECT LAYERS: {layers_str}",
            f" * FUNCTIONS ({len(ordered_addrs)}): {', '.join(ordered_addrs)}",
            " */",
            "",
        ]
        for addr in ordered_addrs:
            items = by_addr[addr]
            finfo = func_index.get(addr)
            parts.append("/* --- FINDING(S) ---")
            for x in items:
                loc = (
                    f"{x['chunk_c_file']}:L{x['func_line_start']}-L{x['func_line_end']}"
                    if x["func_line_start"] is not None
                    else x["chunk_c_file"]
                )
                ev_l = (
                    f"L{x['evidence_line']}"
                    if x["evidence_line"] is not None
                    else "n/a"
                )
                parts.append(
                    f" * [{x['severity']}] layer={x['suspect_layer']} | {loc} ({ev_l})"
                )
                parts.append(f" *   why: {x['why']}")
                parts.append(f" *   evidence: {x['evidence']}")
            parts.append(" */")
            parts.append(
                finfo["full_c"]
                if finfo is not None
                else f"/* Full C body unavailable for {addr} ({items[0]['func']}) */"
            )
            parts.append("")
        out_path.write_text("\n".join(parts), encoding="utf-8")


def _md_escape(text: str) -> str:
    return text.replace("|", "\\|").replace("\n", " ")


def _evaluate_gates(
    deduped: list[dict[str, Any]],
    conclusions: list[dict[str, Any]],
    blind_clean_addrs: set[str],
    accepted_patterns: set[tuple[str, str]],
) -> tuple[bool, list[dict[str, Any]], bool, list[str]]:
    recall_hits = [
        f
        for f in deduped
        if f["addr"] in blind_clean_addrs
        and (
            f["severity"] == "HIGH"
            or (
                f["severity"] == "MEDIUM"
                and (f["category"], f["pattern"]) not in accepted_patterns
            )
        )
    ]
    recall_ok = not recall_hits

    exit_failures: list[str] = []
    for f in deduped:
        if f["severity"] == "HIGH" and f.get("tier") != "stl_middleware":
            exit_failures.append(
                f"HIGH finding at {f['addr']} ({f['category']}/{f['pattern']})"
            )
    for conc in conclusions:
        if (
            conc["non_stl_distinct_skeletons"] >= 2
            and (conc["category"], conc["pattern"]) not in accepted_patterns
            and conc["max_severity"] in ("HIGH", "MEDIUM")
        ):
            exit_failures.append(
                f"Systemic {conc['max_severity']} cluster "
                f"{conc['category']}/{conc['pattern']} "
                f"({conc['non_stl_distinct_skeletons']} skeletons)"
            )
    return recall_ok, recall_hits, (recall_ok and not exit_failures), exit_failures


def _build_summary_md(
    *,
    batch_id: str,
    mode: str,
    exit_gate: bool,
    total_dwarf_funcs: int,
    chunk_files: list[Path],
    missing_chunks: list[str],
    invalid_chunks: list[tuple[str, str]],
    reviewed_addrs: set[str],
    deduped: list[dict[str, Any]],
    conclusions: list[dict[str, Any]],
    cat_counts: Counter[str],
    sev_counts: Counter[str],
    layer_counts: Counter[str],
    manifest: dict[str, Any],
    recall_ok: bool,
    recall_hits: list[dict[str, Any]],
    exit_ok: bool,
    exit_failures: list[str],
) -> str:
    all_funcs = manifest.get("functions", {}).values()
    done_statuses = ("clean", "llm_clean", "tier0_clean_skipped", "flagged")
    global_reviewed = sum(1 for v in all_funcs if v.get("status") in done_statuses)
    global_flagged = sum(1 for v in all_funcs if v.get("status") == "flagged")
    done_chunks = len(chunk_files) - len(missing_chunks) - len(invalid_chunks)
    pct = (100 * global_reviewed / total_dwarf_funcs) if total_dwarf_funcs else 0.0

    recall_txt = "PASS" if recall_ok else f"FAIL ({len(recall_hits)} blind-slice hits)"
    exit_txt = "PASS" if exit_ok else f"FAIL ({len(exit_failures)} blockers)"
    sev_summary = (
        f"`HIGH`: {sev_counts['HIGH']}, "
        f"`MEDIUM`: {sev_counts['MEDIUM']}, `LOW`: {sev_counts['LOW']}"
    )
    md = [
        f"# Audit Summary — `{batch_id}` (`{mode}`, exit_gate={exit_gate})",
        "",
        f"- **RECALL_GATE:** {recall_txt}",
        f"- **EXIT_GATE:** {exit_txt}",
        f"- **Chunks completed:** {done_chunks} / {len(chunk_files)}",
        f"- **Functions reviewed in batch:** {len(reviewed_addrs):,}",
        f"- **Deduped findings:** {len(deduped)} ({sev_summary})",
        f"- **Distinct conclusions:** {len(conclusions)}",
        f"- **Global cumulative progress:** {global_reviewed:,} / "
        f"{total_dwarf_funcs:,} ({pct:.2f}%) — `flagged`: {global_flagged:,}",
        "",
    ]
    if exit_failures:
        md.append("## Gate Blockers")
        for msg in exit_failures:
            md.append(f"- {msg}")
        md.append("")
    if missing_chunks or invalid_chunks:
        md.append("## Incomplete / Invalid Chunks")
        for mc in missing_chunks:
            md.append(f"- **MISSING:** `{mc}`")
        for ic, err in invalid_chunks:
            md.append(f"- **INVALID:** `{ic}` — {err}")
        md.append("")
    md.append("## Breakdown")
    if cat_counts:
        md.append(
            "- **Categories:** "
            + ", ".join(f"`{k}`: {v}" for k, v in cat_counts.most_common())
        )
        md.append(
            "- **Suspect layers:** "
            + ", ".join(f"`{k}`: {v}" for k, v in layer_counts.most_common())
        )
    else:
        md.append("- None (0 findings)")
    md.append("")
    md.append("## Conclusions -> Mapped Functions & Pseudocode")
    if not conclusions:
        md.append("- Batch is completely clean.\n")
        return "\n".join(md)
    for conc in conclusions:
        ff = conc["flagged_file"]
        md.append(
            f"### [{conc['max_severity']}] `{conc['category']} / {conc['pattern']}` — "
            f"{conc['count']} hit(s) "
            f"({conc['non_stl_distinct_skeletons']} non-STL skeletons)"
        )
        md.append(f"- Full C bundle: [{ff}]({ff})\n")
        md.append("| Addr | Function | Tier | Skeleton | Sev | Why |")
        md.append("| --- | --- | --- | --- | --- | --- |")
        for x in conc["findings"]:
            skel_esc = _md_escape(x["template_skeleton"])
            md.append(
                f"| `{x['addr']}` | `{_md_escape(x['func'])}` | `{x['tier']}` | "
                f"`{skel_esc}` | `{x['severity']}` | {_md_escape(x['why'])} |"
            )
        md.append("")
    return "\n".join(md)


def _update_discovery_manifest(
    manifest_path: Path,
    manifest: dict[str, Any],
    *,
    batch_id: str,
    reviewed_addrs: set[str],
    finding_counts_by_addr: Counter[str],
    prescan_report: dict[str, Any],
    func_index: dict[str, dict[str, Any]],
) -> None:
    now_iso = datetime.now(UTC).isoformat()
    skipped_addrs = {
        norm_hex(a) for a in prescan_report.get("tier0_clean_skipped_addrs", [])
    }
    for addr in reviewed_addrs:
        fcnt = finding_counts_by_addr.get(addr, 0)
        finfo = func_index.get(addr, {})
        entry = manifest["functions"].setdefault(
            addr,
            {"qualified_name": finfo.get("header_func", ""), "batch_id": batch_id},
        )
        entry["status"] = "flagged" if fcnt > 0 else "llm_clean"
        entry["finding_count"] = fcnt
        entry["reviewed_at"] = now_iso
    for addr in skipped_addrs:
        entry = manifest["functions"].setdefault(addr, {"batch_id": batch_id})
        entry["status"] = "tier0_clean_skipped"
        entry["finding_count"] = 0
        entry["reviewed_at"] = now_iso
    manifest_path.parent.mkdir(parents=True, exist_ok=True)
    save_json_atomic(manifest_path, manifest)


def main() -> None:
    args = parse_args()
    runs_dir = args.runs_dir.resolve()
    batch_id, batch_dir = resolve_batch_dir(runs_dir, args.batch, args.batch_dir)
    chunks_dir = batch_dir / "chunks"
    findings_dir = batch_dir / "findings"
    meta_path = batch_dir / "batch_meta.json"
    prescan_path = batch_dir / "prescan_report.json"

    batch_meta = (
        json.loads(meta_path.read_text(encoding="utf-8")) if meta_path.is_file() else {}
    )
    prescan_report = (
        json.loads(prescan_path.read_text(encoding="utf-8"))
        if prescan_path.is_file()
        else {}
    )
    mode = str(batch_meta.get("mode", "discovery"))
    exit_gate = bool(batch_meta.get("exit_gate", False))
    total_dwarf_funcs = int(
        batch_meta.get("total_dwarf_functions", LEGACY_TOTAL_DWARF_FUNCTIONS)
    )

    worker_prompt = AUDIT_DIR / "PROMPT_WORKER.md"
    if meta_path.is_file() and worker_prompt.is_file():
        batch_meta["prompt_sha256"] = compute_file_sha256(worker_prompt)
        save_json_atomic(meta_path, batch_meta)

    golden = (
        json.loads(args.golden_set.read_text(encoding="utf-8"))
        if args.golden_set.is_file()
        else {}
    )
    accepted_patterns = {
        (str(p[0]).upper(), norm_pattern(str(p[1])))
        for p in golden.get("accepted_irreducible_patterns", [])
        if isinstance(p, list | tuple) and len(p) == 2
    }

    func_index, addrs_by_chunk = index_chunks(chunks_dir)
    chunk_files = sorted(chunks_dir.glob("chunk_*.c"))
    missing_chunks: list[str] = []
    invalid_chunks: list[tuple[str, str]] = []
    reviewed_addrs: set[str] = set()
    raw_findings: list[dict[str, Any]] = []

    for cf in chunk_files:
        jf = findings_dir / f"{cf.stem}.json"
        if not jf.is_file():
            missing_chunks.append(cf.name)
            continue
        data, err = validate_chunk_json(jf, addrs_by_chunk.get(cf.name))
        if err is not None or data is None:
            invalid_chunks.append((jf.name, err or "unknown error"))
            continue
        for raw_a in data["reviewed_addrs"]:
            reviewed_addrs.add(norm_hex(raw_a))
        for item in data["findings"]:
            addr = norm_hex(item["addr"])
            func_name = str(item["func"])
            finfo = func_index.get(addr)
            ev_line, ctx_snippet = locate_evidence(finfo, str(item["evidence"]).strip())
            raw_findings.append(
                {
                    "addr": addr,
                    "func": func_name,
                    "tier": classify_namespace_tier(func_name),
                    "template_skeleton": normalize_template_skeleton(func_name),
                    "category": str(item["category"]).strip().upper(),
                    "pattern": norm_pattern(item["pattern"]),
                    "suspect_layer": str(item["suspect_layer"]).strip().lower(),
                    "severity": str(item["severity"]).strip().upper(),
                    "evidence": str(item["evidence"]).strip(),
                    "why": str(item["why"]).strip(),
                    "chunk_c_file": finfo["chunk_c_file"] if finfo else cf.name,
                    "findings_file": jf.name,
                    "func_line_start": finfo["line_start"] if finfo else None,
                    "func_line_end": finfo["line_end"] if finfo else None,
                    "evidence_line": ev_line,
                    "dwarf_bytes": finfo["dwarf_bytes"] if finfo else None,
                    "ghidra_bytes": finfo["ghidra_bytes"] if finfo else None,
                    "decompile_ok": finfo["decompile_ok"] if finfo else None,
                    "signature": finfo["signature"] if finfo else "",
                    "context_snippet": ctx_snippet,
                }
            )

    deduped: list[dict[str, Any]] = []
    seen_keys: set[tuple[str, str, str, str]] = set()
    finding_counts_by_addr: Counter[str] = Counter()
    for f in raw_findings:
        key = (f["addr"], f["category"], f["pattern"], f["evidence"])
        if key not in seen_keys:
            seen_keys.add(key)
            deduped.append(f)
            finding_counts_by_addr[f["addr"]] += 1

    manifest_path = runs_dir / "manifest.json"
    manifest = (
        json.loads(manifest_path.read_text(encoding="utf-8"))
        if manifest_path.is_file()
        else {"functions": {}, "batches": {}}
    )
    if mode == "discovery" and not missing_chunks and not invalid_chunks:
        _update_discovery_manifest(
            manifest_path,
            manifest,
            batch_id=batch_id,
            reviewed_addrs=reviewed_addrs,
            finding_counts_by_addr=finding_counts_by_addr,
            prescan_report=prescan_report,
            func_index=func_index,
        )

    grouped: dict[tuple[str, str], list[dict[str, Any]]] = defaultdict(list)
    cat_counts: Counter[str] = Counter()
    sev_counts: Counter[str] = Counter()
    layer_counts: Counter[str] = Counter()
    for f in deduped:
        cat_counts[f["category"]] += 1
        sev_counts[f["severity"]] += 1
        layer_counts[f["suspect_layer"]] += 1
        grouped[(f["category"], f["pattern"])].append(f)

    conclusions: list[dict[str, Any]] = []
    for (cat, pat), items in sorted(
        grouped.items(),
        key=lambda kv: (
            min(SEVERITY_RANK[x["severity"]] for x in kv[1]),
            -len(kv[1]),
            kv[0],
        ),
    ):
        items_sorted = sorted(
            items, key=lambda x: (SEVERITY_RANK[x["severity"]], int(x["addr"], 16))
        )
        non_stl_skels = {
            x["template_skeleton"]
            for x in items_sorted
            if x["tier"] != "stl_middleware"
        }
        conclusions.append(
            {
                "category": cat,
                "pattern": pat,
                "count": len(items_sorted),
                "non_stl_distinct_skeletons": len(non_stl_skels),
                "max_severity": min(
                    items_sorted, key=lambda x: SEVERITY_RANK[x["severity"]]
                )["severity"],
                "severity_counts": dict(Counter(x["severity"] for x in items_sorted)),
                "suspect_layers": dict(
                    Counter(x["suspect_layer"] for x in items_sorted).most_common()
                ),
                "flagged_file": f"flagged/{cat.lower()}__{pat}.c",
                "functions": sorted({x["addr"] for x in items_sorted}),
                "findings": items_sorted,
            }
        )

    _write_flagged_bundles(batch_dir, conclusions, func_index)
    blind_clean_addrs = {
        norm_hex(a) for a in prescan_report.get("routed_blind_clean_addrs", [])
    }
    recall_ok, recall_hits, exit_ok, exit_failures = _evaluate_gates(
        deduped, conclusions, blind_clean_addrs, accepted_patterns
    )

    merged_out = {
        "batch_id": batch_id,
        "mode": mode,
        "exit_gate": exit_gate,
        "recall_gate": "PASS" if recall_ok else "FAIL",
        "exit_gate_status": "PASS" if exit_ok else "FAIL",
        "exit_gate_blockers": exit_failures,
        "total_chunks": len(chunk_files),
        "completed_chunks": len(chunk_files)
        - len(missing_chunks)
        - len(invalid_chunks),
        "missing_chunks": missing_chunks,
        "invalid_chunks": [{"file": n, "error": e} for n, e in invalid_chunks],
        "reviewed_functions": len(reviewed_addrs),
        "total_findings": len(deduped),
        "category_counts": dict(cat_counts),
        "severity_counts": dict(sev_counts),
        "suspect_layer_counts": dict(layer_counts),
        "conclusions": conclusions,
        "findings": deduped,
    }
    save_json_atomic(batch_dir / "merged_findings.json", merged_out)
    summary_md = _build_summary_md(
        batch_id=batch_id,
        mode=mode,
        exit_gate=exit_gate,
        total_dwarf_funcs=total_dwarf_funcs,
        chunk_files=chunk_files,
        missing_chunks=missing_chunks,
        invalid_chunks=invalid_chunks,
        reviewed_addrs=reviewed_addrs,
        deduped=deduped,
        conclusions=conclusions,
        cat_counts=cat_counts,
        sev_counts=sev_counts,
        layer_counts=layer_counts,
        manifest=manifest,
        recall_ok=recall_ok,
        recall_hits=recall_hits,
        exit_ok=exit_ok,
        exit_failures=exit_failures,
    )
    (batch_dir / "SUMMARY.md").write_text(summary_md, encoding="utf-8")
    done_cnt = merged_out["completed_chunks"]
    print(
        f"Merged {batch_id}: {done_cnt}/{len(chunk_files)} chunks, "
        f"{len(deduped)} findings, RECALL_GATE={merged_out['recall_gate']}, "
        f"EXIT_GATE={merged_out['exit_gate_status']}",
        flush=True,
    )
    if missing_chunks or invalid_chunks:
        sys.exit(2)


if __name__ == "__main__":
    main()
