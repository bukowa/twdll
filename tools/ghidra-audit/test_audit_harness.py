"""Offline CI tests for `tools/ghidra-audit` harness modules."""

from __future__ import annotations

import json
import sqlite3
import sys
from contextlib import closing
from pathlib import Path
from typing import Any

import audit_common
import extract_batch
import merge_findings
import prescan
import pytest
from twdll.database import init_extract_staging_db


def _seed_test_db(db_path: Path) -> None:
    init_extract_staging_db(db_path)
    with closing(sqlite3.connect(db_path)) as conn:
        conn.executemany(
            "INSERT INTO dwarf_types "
            "(type_id, kind, qualified_name, byte_size, target_type_id, array_count, "
            "containing_type_id, vtable_symbol, vtable_address, is_declaration, "
            "is_non_trivial_for_calls, is_stl_stub, is_referenced, abi_class) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                (
                    1,
                    "primitive",
                    "void",
                    0,
                    None,
                    None,
                    None,
                    None,
                    None,
                    0,
                    0,
                    0,
                    1,
                    None,
                ),
                (
                    2,
                    "primitive",
                    "int",
                    4,
                    None,
                    None,
                    None,
                    None,
                    None,
                    0,
                    0,
                    0,
                    1,
                    "int",
                ),
                (
                    3,
                    "struct",
                    "Base",
                    8,
                    None,
                    None,
                    None,
                    None,
                    None,
                    0,
                    0,
                    0,
                    1,
                    "int",
                ),
                (
                    4,
                    "struct",
                    "Derived",
                    16,
                    None,
                    None,
                    None,
                    None,
                    None,
                    0,
                    0,
                    0,
                    1,
                    "int,int",
                ),
                (
                    5,
                    "struct",
                    "VirtDerived",
                    16,
                    None,
                    None,
                    None,
                    None,
                    None,
                    0,
                    0,
                    0,
                    1,
                    "memory",
                ),
                (
                    6,
                    "struct",
                    "EmptyTag",
                    1,
                    None,
                    None,
                    None,
                    None,
                    None,
                    0,
                    0,
                    0,
                    1,
                    "int",
                ),
                (
                    7,
                    "pointer",
                    "void *",
                    8,
                    1,
                    None,
                    None,
                    None,
                    None,
                    0,
                    0,
                    0,
                    1,
                    "int",
                ),
            ),
        )
        conn.executemany(
            "INSERT INTO dwarf_inheritance "
            "(inheritance_id, derived_type_id, base_type_id, byte_offset, "
            "vbase_vtable_offset, is_virtual, is_empty_base, has_tail_padding_overlap) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            (
                (1, 4, 3, 0, None, 0, 0, 0),
                (2, 5, 3, None, -24, 1, 0, 0),
            ),
        )
        conn.executemany(
            "INSERT INTO dwarf_members "
            "(member_id, parent_type_id, ordinal, name, member_type_id, byte_offset, "
            "byte_size, bit_offset, bit_size, is_artificial, "
            "is_flattened_base_member, is_layout_conflict) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                (1, 3, 0, "b_i", 2, 0, 4, None, None, 0, 0, 0),
                (2, 4, 0, "d_i", 2, 8, 4, None, None, 0, 0, 0),
            ),
        )

        # Populate functions across all 3 tiers & 3 size strata, plus template clones.
        fn_rows: list[tuple[Any, ...]] = []
        fid = 1
        for i in range(40):
            bsz = 40 if i < 6 else (500 if i < 36 else 20000)
            fn_rows.append(
                (
                    fid,
                    1,
                    0x100000 + fid * 0x100,
                    bsz,
                    None,
                    f"EMPIRECAMPAIGN::CLS_{i}::update",
                    4,
                    2,
                    None,
                    0,
                    0,
                    0,
                    0,
                    1,
                    0,
                    None,
                    None,
                )
            )
            fid += 1
        for i in range(15):
            fn_rows.append(
                (
                    fid,
                    1,
                    0x200000 + fid * 0x100,
                    200,
                    None,
                    f"WARSCAPE::ENG_{i}::tick",
                    None,
                    2,
                    None,
                    0,
                    0,
                    0,
                    0,
                    0,
                    0,
                    None,
                    None,
                )
            )
            fid += 1
        for i in range(10):
            fn_rows.append(
                (
                    fid,
                    1,
                    0x300000 + fid * 0x100,
                    200,
                    None,
                    f"std::vector<Item{i}, std::allocator<Item{i}>>::push_back",
                    None,
                    1,
                    None,
                    0,
                    0,
                    0,
                    0,
                    0,
                    0,
                    None,
                    None,
                )
            )
            fid += 1
        conn.executemany(
            "INSERT INTO dwarf_functions "
            "(function_id, unit_id, address, byte_size, linkage_name, qualified_name, "
            "containing_type_id, return_type_id, vtable_slot_index, virtuality, "
            "is_inline, is_declaration, is_variadic, has_object_pointer, is_sret, "
            "decl_file, decl_line) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            fn_rows,
        )
        conn.executemany(
            "INSERT INTO dwarf_parameters "
            "(function_id, ordinal, name, type_id, is_artificial, is_object_pointer, "
            "storage_kind, dwarf_reg, reg_name, stack_offset) "
            "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                (1, 0, "tag", 6, 0, 0, "register", 5, "RDI", None),
                (1, 1, "raw", 7, 0, 0, "register", 4, "RSI", None),
            ),
        )
        conn.commit()


def test_normalize_template_skeleton_and_tier_classification() -> None:
    assert (
        audit_common.normalize_template_skeleton(
            "DATABASE_TABLE<UNIT_RECORD, std::map<int, CA::String>>::clearing_packs"
        )
        == "DATABASE_TABLE<T>::clearing_packs"
    )
    assert (
        audit_common.normalize_template_skeleton("CA::Stream::operator<<=")
        == "CA::Stream::operator<<="
    )
    assert (
        audit_common.normalize_template_skeleton("CA::Iter<Foo>::operator<=<Bar>")
        == "CA::Iter<T>::operator<=<T>"
    )

    assert (
        audit_common.classify_namespace_tier("std::vector<int>::push_back")
        == "stl_middleware"
    )
    assert (
        audit_common.classify_namespace_tier("Json::Reader::parse") == "stl_middleware"
    )
    assert audit_common.classify_namespace_tier("WARSCAPE::EMITTER::update") == "engine"
    assert audit_common.classify_namespace_tier("CA::String::assign") == "engine"
    assert audit_common.classify_namespace_tier("CAMPAIGN_UI::s_ui") == "core"
    assert audit_common.classify_namespace_tier("DATABASE_TABLE<X>::load") == "core"
    assert audit_common.classify_namespace_tier("UNIT::tick") == "core"


def test_compute_cache_key_sensitivity(tmp_path: Path) -> None:
    (tmp_path / ".twdll" / "analyze").mkdir(parents=True)
    (tmp_path / ".twdll" / "analyze" / "ghidra_attila_linux_64.meta.json").write_text(
        "{}", encoding="utf-8"
    )
    (tmp_path / "cli" / "src" / "twdll" / "_ghidra").mkdir(parents=True)
    (tmp_path / "cli" / "src" / "twdll" / "domain.py").write_text(
        "x = 1", encoding="utf-8"
    )
    (tmp_path / "cli" / "src" / "twdll" / "database.py").write_text(
        "y = 1", encoding="utf-8"
    )
    (tmp_path / "cli" / "src" / "twdll" / "_ghidra" / "dwarf_types.py").write_text(
        "z = 1", encoding="utf-8"
    )
    db_path = tmp_path / "dwarf.sqlite"
    db_path.write_bytes(b"db1")

    k1 = audit_common.compute_cache_key(tmp_path, db_path)
    (tmp_path / "cli" / "src" / "twdll" / "_ghidra" / "dwarf_types.py").write_text(
        "z = 2", encoding="utf-8"
    )
    k2 = audit_common.compute_cache_key(tmp_path, db_path)
    assert k1 != k2


def test_discovery_sampling_quotas_skeleton_cap_and_reverify(tmp_path: Path) -> None:
    db_path = tmp_path / "attila.sqlite"
    _seed_test_db(db_path)

    selected = extract_batch.sample_discovery_functions(
        db_path, set(), count=20, seed=7, max_per_skeleton=2
    )
    assert len(selected) == 20
    skel_counts = {}
    for item in selected:
        sk = item["template_skeleton"]
        skel_counts[sk] = skel_counts.get(sk, 0) + 1
    assert max(skel_counts.values()) <= 2

    runs_dir = tmp_path / "runs"
    (runs_dir / "batches" / "batch_001").mkdir(parents=True)
    (runs_dir / "batches" / "batch_001" / "merged_findings.json").write_text(
        json.dumps({"findings": [{"addr": "0x100100"}]}), encoding="utf-8"
    )
    rev = extract_batch.select_reverify_functions(
        db_path,
        runs_dir=runs_dir,
        from_batch="batch_001",
        addrs_file=None,
        sql_filter=None,
        count=10,
        seed=1,
    )
    assert [r["addr"] for r in rev] == [0x100100]

    sql_rev = extract_batch.select_reverify_functions(
        db_path,
        runs_dir=runs_dir,
        from_batch=None,
        addrs_file=None,
        sql_filter="rt.abi_class = 'int'",
        count=5,
        seed=1,
    )
    assert len(sql_rev) == 5


def test_prescan_padding_proof_and_routing(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    db_path = tmp_path / "attila.sqlite"
    _seed_test_db(db_path)

    with closing(sqlite3.connect(db_path)) as conn:
        cache: dict[str, Any] = {}
        # Base: [0,4), gap: [4,8); Derived: [8,12), trailing gap: [12,16).
        assert prescan.prove_padding_gap(conn, "Derived", 5, cache) is True
        assert prescan.prove_padding_gap(conn, "Derived", 13, cache) is True
        assert prescan.prove_padding_gap(conn, "Derived", 2, cache) is False
        assert prescan.prove_padding_gap(conn, "Derived", 9, cache) is False
        # Virtual base aborts proof.
        assert prescan.prove_padding_gap(conn, "VirtDerived", 5, cache) is False

    batch_dir = tmp_path / "runs" / "batches" / "batch_002"
    batch_dir.mkdir(parents=True)
    records = [
        {
            "addr": "0x100100",
            "name": "EMPIRECAMPAIGN::CLS_0::update",
            "tier": "core",
            "template_skeleton": "EMPIRECAMPAIGN::CLS_0::update",
            "dwarf_bytes": 40,
            "ghidra_bytes": 40,
            "decompile_ok": True,
            "decompile_err": "",
            "c_code": "void update(void) { int x = d->field_0x5; }",
            "field_refs": [
                {"struct_name": "Derived", "offset": 5, "field_name": "field_0x5"}
            ],
        },
        {
            "addr": "0x100200",
            "name": "EMPIRECAMPAIGN::CLS_1::update",
            "tier": "core",
            "template_skeleton": "EMPIRECAMPAIGN::CLS_1::update",
            "dwarf_bytes": 40,
            "ghidra_bytes": 40,
            "decompile_ok": True,
            "decompile_err": "",
            "c_code": "void update(void) { return in_RSI; }",
            "field_refs": [],
        },
    ]
    (batch_dir / "extracted.jsonl").write_text(
        "\n".join(json.dumps(r) for r in records) + "\n", encoding="utf-8"
    )
    (batch_dir / "batch_meta.json").write_text(
        json.dumps(
            {"exit_gate": False, "mode": "discovery", "total_dwarf_functions": 65}
        ),
        encoding="utf-8",
    )

    monkeypatch.setattr(
        sys,
        "argv",
        [
            "prescan.py",
            "--batch-dir",
            str(batch_dir),
            "--dwarf-db",
            str(db_path),
            "--blind-ratio",
            "1.0",
        ],
    )
    prescan.main()

    report = json.loads((batch_dir / "prescan_report.json").read_text(encoding="utf-8"))
    assert report["routed_anomaly_addrs"] == ["0x100200"]
    assert report["routed_blind_clean_addrs"] == ["0x100100"]
    chunk_text = (batch_dir / "chunks" / "chunk_001.c").read_text(encoding="utf-8")
    assert "/* DWARF-FACTS: is_sret=0 empty_struct_params=tag:EmptyTag" in chunk_text
    assert "PROVEN_PADDING_GAP=Derived+0x5" in chunk_text


def test_merge_findings_strict_validation_recall_breaker_and_manifest(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    runs_dir = tmp_path / "runs"
    batch_dir = runs_dir / "batches" / "batch_002"
    (batch_dir / "chunks").mkdir(parents=True)
    (batch_dir / "findings").mkdir(parents=True)
    (batch_dir / "batch_meta.json").write_text(
        json.dumps(
            {"mode": "discovery", "exit_gate": False, "total_dwarf_functions": 100}
        ),
        encoding="utf-8",
    )
    (batch_dir / "prescan_report.json").write_text(
        json.dumps(
            {
                "routed_anomaly_addrs": ["0x100200"],
                "routed_blind_clean_addrs": ["0x100100"],
                "tier0_clean_skipped_addrs": ["0x100300"],
            }
        ),
        encoding="utf-8",
    )
    (batch_dir / "chunks" / "chunk_001.c").write_text(
        "/* === [1/2] 0x100100 | EMPIRE::A::f | "
        "dwarf=40B | gh=40B | ok=True | lines=1 === */\n"
        "void f(void) {}\n\n"
        "/* === [2/2] 0x100200 | EMPIRE::B::g | "
        "dwarf=40B | gh=40B | ok=True | lines=1 === */\n"
        "void g(void) { return in_RSI; }\n",
        encoding="utf-8",
    )

    bad_json = batch_dir / "findings" / "chunk_001.json"
    bad_json.write_text(
        json.dumps(
            {
                "chunk": "chunk_001.c",
                "reviewed_addrs": ["0x100100", "0x100200"],
                "findings": [
                    {
                        "addr": "0x100100",
                        "func": "EMPIRE::A::f",
                        "category": "BOGUS_CATEGORY",
                        "pattern": "p",
                        "suspect_layer": "pass3_sig_abi",
                        "severity": "HIGH",
                        "evidence": "void f(void) {}",
                        "why": "test",
                    }
                ],
            }
        ),
        encoding="utf-8",
    )
    _, err = merge_findings.validate_chunk_json(bad_json, {"0x100100", "0x100200"})
    assert err is not None and "Invalid category" in err

    # Valid finding hitting blind-clean 0x100100 triggers RECALL_GATE: FAIL.
    bad_json.write_text(
        json.dumps(
            {
                "chunk": "chunk_001.c",
                "reviewed_addrs": ["0x100100", "0x100200"],
                "findings": [
                    {
                        "addr": "0x100100",
                        "func": "EMPIRE::A::f",
                        "category": "ABI_REG",
                        "pattern": "unresolved_abi_reg",
                        "suspect_layer": "pass3_sig_abi",
                        "severity": "HIGH",
                        "evidence": "void f(void) {}",
                        "why": "missed by tier 0",
                    }
                ],
            }
        ),
        encoding="utf-8",
    )
    monkeypatch.setattr(
        sys,
        "argv",
        [
            "merge_findings.py",
            "--batch-dir",
            str(batch_dir),
            "--runs-dir",
            str(runs_dir),
        ],
    )
    merge_findings.main()

    merged = json.loads(
        (batch_dir / "merged_findings.json").read_text(encoding="utf-8")
    )
    assert merged["recall_gate"] == "FAIL"
    manifest = json.loads((runs_dir / "manifest.json").read_text(encoding="utf-8"))
    assert manifest["functions"]["0x100100"]["status"] == "flagged"
    assert manifest["functions"]["0x100200"]["status"] == "llm_clean"
    assert manifest["functions"]["0x100300"]["status"] == "tier0_clean_skipped"
